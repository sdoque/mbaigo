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

// Telling consumers when a value moves, instead of being asked over and over.

package usecases

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

const (
	// defaultHeartbeat is how long a publisher may say nothing before saying so
	// anyway. A subscriber measures liveness by it, so silence past a few of
	// these means the publisher is gone rather than the value being steady.
	defaultHeartbeat = 30 * time.Second

	// slowestHeartbeat bounds what a subscriber may ask for at the other end. A
	// subscription that reports once an hour is a poll with extra machinery, and
	// it makes the consumer's liveness detection uselessly slow.
	slowestHeartbeat = 5 * time.Minute

	// maxSubscribers is how many consumers one service will carry. Each holds a
	// connection and a buffer; a sensor on a Raspberry Pi has a handful of real
	// consumers, and anything past this is a client in a reconnect loop rather
	// than a cloud that grew.
	maxSubscribers = 32
)

// Publisher holds one service's current value and tells subscribers when it
// moves.
//
// The system that owns the service samples as it always did and hands each
// sample here; what to do with it — whether it is a change worth reporting, and
// whether anyone is owed a heartbeat — is bookkeeping, and belongs in one place
// rather than in every system that has a sensor.
//
// The baseline is shared across subscribers rather than kept per subscription.
// A subscriber that joins late is sent the current value immediately, so it
// never waits to learn the state; what it gives up is that later change events
// are measured from a cloud-wide baseline rather than from what that particular
// subscriber last saw. That trade buys a publisher which holds one value and one
// timer instead of a set of them per consumer.
type Publisher struct {
	service *components.Service

	mu sync.Mutex
	// latest is the most recent sample, whatever it was. A heartbeat carries it,
	// so a subscriber always sees the present state on every event whatever the
	// cause.
	latest    forms.Form
	hasLatest bool
	// baseline is the value last broadcast, which is what a change is measured
	// from. Kept apart from latest because they are different questions: a value
	// drifting by less than the threshold moves latest on every sample and must
	// leave baseline alone, or the drift is never reported however far it goes.
	baseline    float64
	hasBaseline bool
	subscribers map[int]*subscription
	nextID      int
}

// subscription is one consumer listening.
type subscription struct {
	events chan forms.Form
	terms  terms
}

// terms are what a publisher and one subscriber agreed to.
//
// Agreed rather than dictated. The consumer knows what it needs — a controller
// on a ten-second loop needs to hear more often than that — and the publisher
// knows what it can honour, since a threshold below the sensor's resolution is
// meaningless and a heartbeat faster than the sampling period is a promise it
// cannot keep. So a subscriber proposes, the publisher clamps, and the agreed
// terms are sent back in the first event: a control loop that believes it will
// hear about a change of 0.1 and will not is worse off than one that knows.
type terms struct {
	Heartbeat time.Duration
	Threshold float64
	Unit      string
}

// PreparePublishers gives every service that declares itself subscribable
// somewhere to publish from.
//
// Called by the framework at startup, so a system turns subscription on in its
// configuration rather than in its code: what it then has to do is hand each
// sample to Publish, which is a line in a loop it already has.
func PreparePublishers(sys *components.System) {
	for _, ua := range sys.UAssets {
		asset := *ua
		for _, serv := range asset.GetServices() {
			if serv.SubscribeAble && serv.Stream == nil {
				serv.Stream = NewPublisher(serv)
			}
		}
	}
}

// Publish hands a fresh sample to whoever is following a service.
//
// Safe to call whether or not anybody is: a service that is not subscribable has
// no publisher, and this does nothing. That is what lets a system call it
// unconditionally on its sampling clock without asking whether it matters.
func Publish(ua *components.UnitAsset, subPath string, value forms.Form) {
	if ua == nil {
		return
	}
	serv, known := (*ua).GetServices()[subPath]
	if !known || serv.Stream == nil {
		return
	}
	if publisher, ok := serv.Stream.(*Publisher); ok {
		publisher.Sample(value)
	}
}

// NewPublisher prepares a service to be followed.
func NewPublisher(service *components.Service) *Publisher {
	return &Publisher{
		service:     service,
		subscribers: make(map[int]*subscription),
	}
}

// Sample takes a fresh reading and tells whoever is owed something.
//
// Called by the system on its own sampling clock, which is unchanged: a service
// becoming subscribable does not alter how often it is read from its sensor,
// only who is told about it.
func (p *Publisher) Sample(value forms.Form) {
	if p == nil {
		return
	}
	reading, ok := value.(forms.UnitBearer)
	if !ok {
		// A form with no value to compare cannot be thresholded, so every sample
		// is a change. That is the honest reading of "I cannot tell whether this
		// moved".
		p.broadcast(value, true)
		return
	}

	p.mu.Lock()
	current := reading.GetValue()
	worth := !p.hasBaseline || moved(p.baseline, current, p.service.Threshold)
	p.latest, p.hasLatest = value, true
	if worth {
		// The baseline moves only when the value is broadcast. Moving it on every
		// sample would measure each change against the one before it, so a value
		// creeping by a tenth of the threshold every second would never report a
		// change however far it drifted from where the subscribers think it is.
		p.baseline, p.hasBaseline = current, true
	}
	p.mu.Unlock()

	p.broadcast(value, worth)
}

// moved reports whether a reading has changed enough to be worth sending.
func moved(from, to, threshold float64) bool {
	if threshold <= 0 {
		return from != to
	}
	return math.Abs(to-from) >= threshold
}

// broadcast offers the value to every subscriber, and drops none of them for
// being slow.
//
// A send that would block is skipped rather than waited on: one consumer that
// has stopped reading must not hold up a sampling loop that is also driving a
// control loop. The subscriber is not disconnected for it — the next event, or
// the next heartbeat, carries the current value, and a value is not a sequence
// of changes that can be missed. That is the difference between publishing a
// reading and publishing a registry's events, where a dropped event does need
// resynchronising.
func (p *Publisher) broadcast(value forms.Form, changed bool) {
	if !changed {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, sub := range p.subscribers {
		select {
		case sub.events <- value:
		default:
		}
	}
}

// Current returns the value last sampled, and whether there has been one.
func (p *Publisher) Current() (forms.Form, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.latest, p.hasLatest
}

// Subscribable reports whether this publisher is meant to be followed.
func (p *Publisher) Subscribable() bool {
	return p != nil && p.service != nil && p.service.SubscribeAble
}

// agree turns what a subscriber asked for into what it will get.
func (p *Publisher) agree(asked terms) terms {
	agreed := terms{
		Heartbeat: time.Duration(p.service.Heartbeat) * time.Second,
		Threshold: p.service.Threshold,
		Unit:      firstDetail(p.service.Details, "Unit"),
	}
	if agreed.Heartbeat <= 0 {
		agreed.Heartbeat = defaultHeartbeat
	}

	if asked.Heartbeat > 0 {
		agreed.Heartbeat = asked.Heartbeat
	}
	fastest := time.Duration(p.service.FastestHeartbeat) * time.Second
	if fastest <= 0 {
		fastest = time.Second
	}
	if agreed.Heartbeat < fastest {
		agreed.Heartbeat = fastest
	}
	if agreed.Heartbeat > slowestHeartbeat {
		agreed.Heartbeat = slowestHeartbeat
	}

	if asked.Threshold > 0 {
		agreed.Threshold = asked.Threshold
	}
	if p.service.FinestThreshold > 0 && agreed.Threshold < p.service.FinestThreshold {
		agreed.Threshold = p.service.FinestThreshold
	}
	return agreed
}

// ServeStream answers a subscription request and holds the connection open.
//
// On the service's own path, chosen by the Accept header, rather than at a
// /subscribe of its own — the same resource in two representations, as the
// registry's system list already does. That is not only tidiness: a path the
// framework serves without declaring is invisible to the authorizer and to the
// Orchestrator, which is a fault this cloud has already had once. Here there is
// nothing new to declare, and following a value is authorized as the read it is.
func (p *Publisher) ServeStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	agreed := p.agree(proposed(r))

	sub, remove := p.addSubscriber(agreed)
	if sub == nil {
		log.Printf("%s: refusing a subscription; %d are already open\n",
			p.service.Definition, maxSubscribers)
		http.Error(w, "too many subscriptions are open on this service", http.StatusServiceUnavailable)
		return
	}
	defer remove()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	send := func(name string, payload any) bool {
		body, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, body); err != nil {
			return false // the subscriber has gone
		}
		flusher.Flush()
		return true
	}

	// What was agreed, before any value: the subscriber has to know the terms it
	// actually got, not the ones it asked for.
	if !send("terms", map[string]any{
		"heartbeat": agreed.Heartbeat.Seconds(),
		"threshold": agreed.Threshold,
		"unit":      agreed.Unit,
	}) {
		return
	}

	// The current value, so nobody waits for a heartbeat to learn the state.
	if value, known := p.Current(); known {
		if !send("value", value) {
			return
		}
	}

	beat := time.NewTicker(agreed.Heartbeat)
	defer beat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case value := <-sub.events:
			if !send("value", value) {
				return
			}
			// A change resets the heartbeat: the rule is "this long since
			// anything was said", not "every this long", so a busy value does
			// not also carry a stream of heartbeats nobody needs.
			beat.Reset(agreed.Heartbeat)
		case <-beat.C:
			value, known := p.Current()
			if !known {
				continue
			}
			if !send("value", value) {
				return
			}
		}
	}
}

// proposed reads what a subscriber asked for from its request.
func proposed(r *http.Request) terms {
	var asked terms
	query := r.URL.Query()
	if seconds, err := strconv.ParseFloat(query.Get("heartbeat"), 64); err == nil && seconds > 0 {
		asked.Heartbeat = time.Duration(seconds * float64(time.Second))
	}
	if threshold, err := strconv.ParseFloat(query.Get("threshold"), 64); err == nil && threshold > 0 {
		asked.Threshold = threshold
	}
	return asked
}

// addSubscriber registers one listener, or refuses when there are too many.
func (p *Publisher) addSubscriber(agreed terms) (*subscription, func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.subscribers) >= maxSubscribers {
		return nil, nil
	}
	p.nextID++
	id := p.nextID
	sub := &subscription{events: make(chan forms.Form, 8), terms: agreed}
	p.subscribers[id] = sub
	return sub, func() {
		p.mu.Lock()
		delete(p.subscribers, id)
		p.mu.Unlock()
	}
}

// Subscribers reports how many consumers are following, which a system may want
// to log and a test needs.
func (p *Publisher) Subscribers() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.subscribers)
}
