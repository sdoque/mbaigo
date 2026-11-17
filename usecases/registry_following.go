/*******************************************************************************
 * Copyright (c) 2026 Synecdoque
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, subject to the following conditions:
 *
 * The software is licensed under the MIT License. See the LICENSE file in this repository for details.
 *
 * Contributors:
 *   Jan A. van Deventer, Luleå - initial implementation
 *   Thomas Hedeler, Hamburg - initial implementation
 ***************************************************************************SDG*/

// Following the service registry, so that what a system builds from the cloud
// (the kgrapher's graph, the modeler's SysML model) describes the cloud as it
// is rather than as it was when somebody last asked.
//
// Written for the kgrapher and moved here when the modeler needed the same: the
// failure modes below were each found once, and a second copy would have had to
// find them again.

package usecases

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

const (
	// DefaultSettleFor is how long to wait for the registry to go quiet before
	// rebuilding.
	//
	// A system starting up registers its services one at a time, so an arrival
	// is several events a few milliseconds apart, and each one would otherwise
	// cost a full rebuild: one request to the registrar and one to every system
	// in the cloud. Waiting for quiet turns a burst into a single rebuild, at
	// the price of lagging a change by this much.
	DefaultSettleFor = 2 * time.Second

	// DefaultSilenceLimit is how long the stream may say nothing at all before
	// it is treated as dead.
	//
	// The registry writes a keep-alive comment every 20 seconds, so silence past
	// this is not a quiet cloud but a connection that no longer exists (a NAT
	// idle timeout, a partition, a registrar killed without a FIN). Left to
	// itself the read blocks forever, the follower never reconnects, and what it
	// keeps goes on describing a cloud it stopped watching, without a line in
	// the log. Three keep-alives, so a missed one does not cost a reconnection.
	DefaultSilenceLimit = 65 * time.Second

	// DefaultSettleDelay is how long after a change to look once more.
	//
	// A registration is an event and a binding is not. A consumer registers,
	// the cloud is read, and only then does the consumer discover its
	// providers, so the picture taken at the event shows a controller that
	// wants a temperature and is bound to nothing, and no later event corrects
	// it. On the first two-host deployment a thermostat reading a sensor on the
	// other host showed two unmet wants for as long as anyone looked.
	DefaultSettleDelay = 60 * time.Second

	// DefaultIncompleteRetry is how long to wait before rebuilding again after
	// a pass that could not read every system.
	//
	// A system is registered under its plain-HTTP URL for the seconds between
	// registering and its certificate arriving, and refuses to describe itself
	// to an unverified caller then. Rebuilds happen on registry changes, and a
	// settled cloud makes none, so the omission would stand until something
	// else moved: the authorizer went missing from the cottage's graph for ten
	// minutes this way.
	DefaultIncompleteRetry = 30 * time.Second

	// retryAfter is the first wait before opening the stream again; see backoff.
	retryAfter = 5 * time.Second
)

// RegistryFollower keeps something built from the whole cloud current, by
// subscribing to the registry's syslist and rebuilding when it changes.
//
// It never gives up. A registry that cannot be reached is a reason to wait and
// try again, not to stop following: the cloud outlives any one registrar, and a
// follower that quietly stopped would keep serving something that looked
// current.
type RegistryFollower struct {
	// Name is who is following, for the log.
	Name string
	// Registry is the cervice the subscription is read through, as made by
	// SystemListCervice, so that in an authorized cloud it carries a token.
	Registry *components.Cervice
	System   *components.System

	// Rebuild reads the cloud and publishes what it makes of it, and reports
	// whether every system was read. A pass that was not complete, or failed,
	// is tried again after IncompleteRetry; one that was is followed by one
	// more after SettleDelay.
	Rebuild func() (complete bool)

	// Zero means the default of the same name.
	SettleFor, SilenceLimit, SettleDelay, IncompleteRetry time.Duration

	// One pending pass of each kind at a time: a burst of registrations is one
	// settling, and a retry that is still incomplete schedules its own rather
	// than a crowd of duplicates.
	settlePending, retryPending atomic.Bool

	// building holds one Rebuild at a time. A scheduled pass can come due
	// while the stream's own is running, and what Rebuild publishes (a graph
	// to the store, a model to SysView) must not race itself.
	building sync.Mutex
}

func orDefault(d, def time.Duration) time.Duration {
	if d == 0 {
		return def
	}
	return d
}

// Follow subscribes to the registry until ctx ends, reconnecting whenever the
// subscription does.
func (f *RegistryFollower) Follow(ctx context.Context) {
	attempt := 0
	for {
		if err := f.followOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("%s: the registry subscription ended (%v); reconnecting shortly\n", f.Name, err)
		} else {
			attempt = 0 // a connection that worked starts the backoff again
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff(attempt)):
		}
		attempt++
	}
}

// followOnce opens one subscription and reads it until it ends.
func (f *RegistryFollower) followOnce(ctx context.Context) error {
	registrar, err := components.GetRunningCoreSystemURL(f.System, components.ServiceRegistrarName)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registrar+"/"+SystemListPath, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	// The token is asked for on each attempt rather than once at startup: it
	// outlives neither the authorizer that minted it nor this connection, and
	// a reconnection is when the old one is most likely to have expired. A
	// cloud with no authorizer mints none, which is no reason not to subscribe;
	// the registry will say whether it wants one.
	if token, ok := RegistryToken(f.Registry, f.System); ok {
		req.Header.Set(TokenHeader, token)
	}

	client := &http.Client{Transport: http.DefaultClient.Transport} // no timeout: the point is to stay open
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		reason, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("the registry refused the subscription: %s: %s",
			resp.Status, strings.TrimSpace(string(reason)))
	}
	log.Printf("%s: following the service registry at %s\n", f.Name, registrar)
	return f.read(ctx, resp)
}

// read consumes the event stream, rebuilding once the registry goes quiet.
//
// The registry sends a snapshot when the connection opens and an event for each
// change after it. Both mean the same thing here, that the cloud may not be
// what was built, so both start the settle timer rather than a rebuild.
func (f *RegistryFollower) read(ctx context.Context, resp *http.Response) error {
	events := make(chan string)
	done := make(chan error, 1)
	// alive carries "the connection said something", keep-alive comments
	// included. Separate from events because a comment is not a change and
	// must not start a rebuild, but it is what tells the watchdog the stream is
	// still there.
	alive := make(chan struct{}, 1)

	go func() {
		scanner := bufio.NewScanner(resp.Body)
		// The snapshot is the whole system list on one line, so this bounds how
		// large a cloud can be followed. Past it the scanner fails with
		// bufio.ErrTooLong and Follow retries for ever. 8 MB is thousands of
		// systems.
		scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		var kind string
		for scanner.Scan() {
			line := scanner.Text()
			select {
			case alive <- struct{}{}:
			default: // already flagged
			}
			switch {
			case strings.HasPrefix(line, "event: "):
				kind = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				f.describe(kind, strings.TrimPrefix(line, "data: "))
				select {
				case events <- kind:
				case <-ctx.Done():
					done <- ctx.Err()
					return
				}
			}
		}
		done <- scanner.Err()
	}()

	limit := orDefault(f.SilenceLimit, DefaultSilenceLimit)
	silence := time.NewTimer(limit)
	defer silence.Stop()

	var settle <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-alive:
			if !silence.Stop() {
				select {
				case <-silence.C:
				default:
				}
			}
			silence.Reset(limit)
		case <-silence.C:
			// Returning hands this to Follow, which reconnects, and a
			// reconnection reads the whole registry again, so nothing missed
			// during the silence stays missed.
			if settle != nil {
				f.rebuild(ctx, true)
			}
			return fmt.Errorf("the registry said nothing for %s, not even a keep-alive", limit)
		case err := <-done:
			// A rebuild owed when the connection dropped is done on the way
			// out, rather than left to the reconnection and its delay.
			if settle != nil {
				f.rebuild(ctx, true)
			}
			return err
		case <-events:
			// Restarted on every event, so a burst rebuilds once, after the
			// last of it.
			settle = time.After(orDefault(f.SettleFor, DefaultSettleFor))
		case <-settle:
			settle = nil
			f.rebuild(ctx, true)
		}
	}
}

// describe logs what a change was, so an operator can see why the build moved.
func (f *RegistryFollower) describe(kind, data string) {
	if kind != "change" {
		return
	}
	var event forms.RegistryEvent_v1
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return
	}
	log.Printf("%s: %s %s from %s\n", f.Name, event.Record.ServiceDefinition, event.Change, event.Record.SystemName)
}

// rebuild runs one pass. An incomplete one schedules a retry; a complete one,
// when settle is set, schedules one more look after the consumers have had
// time to bind. The settling pass itself does not, so a quiet cloud is read
// twice per change and not for ever.
func (f *RegistryFollower) rebuild(ctx context.Context, settle bool) {
	f.building.Lock()
	complete := f.Rebuild()
	f.building.Unlock()
	if !complete {
		f.later(ctx, &f.retryPending, orDefault(f.IncompleteRetry, DefaultIncompleteRetry), true,
			"%s: not every system could be read; reading the cloud again in %v\n")
		return
	}
	if settle {
		f.later(ctx, &f.settlePending, orDefault(f.SettleDelay, DefaultSettleDelay), false,
			"%s: looking once more in %v, when consumers have had time to bind\n")
	}
}

// later schedules one pass after wait, unless one of its kind is pending.
func (f *RegistryFollower) later(ctx context.Context, pending *atomic.Bool, wait time.Duration, settle bool, why string) {
	if !pending.CompareAndSwap(false, true) {
		return
	}
	log.Printf(why, f.Name, wait)
	go func() {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			pending.Store(false)
			return
		}
		// Cleared before the pass rather than after it, so a retry that is
		// still incomplete can schedule the next one. Cleared after, as the
		// kgrapher did, the second retry was refused by the first's own guard,
		// and a system that stayed unreadable for one round stayed out.
		pending.Store(false)
		f.rebuild(ctx, settle)
	}()
}

// backoff is how long to wait before the next attempt, growing with consecutive
// failures and jittered.
//
// A fixed delay had every subscriber in the cloud reconnect in lockstep after a
// registrar restart, arriving together when it is least able to answer.
func backoff(attempt int) time.Duration {
	wait := retryAfter << min(attempt, 4) // 5s, 10s, 20s, 40s, 80s
	// Jitter is what breaks the lockstep; the growth only bounds the cost.
	// Derived from the clock rather than a random source, so it needs no seed.
	jitter := time.Duration(time.Now().UnixNano()%int64(wait/2)) - wait/4
	return wait + jitter
}
