/*******************************************************************************
 * Copyright (c) 2025 Synecdoque
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

// Package "components" addresses the structures of the components that
// are aggregated to form Arrowhead compliant systems in a local cloud.
// An Arrowhead local cloud is a system of systems, which are made up of a husk
// (a.k.a. a shell) and a unit-asset (a.k.a. an asset or a thing). The husk runs on a device,
// and exposes the unit assets' functionalities as services.
package components

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// System struct aggregates an Arrowhead compliant system
type System struct {
	Name    string                `json:"systemName"`
	Husk    *Husk                 // the system aggregates a "husk" (a wrapper or a shell)
	UAssets map[string]*UnitAsset // the system aggregates "asset", which is made up of one or more unit-asset
	Ctx     context.Context       // create a context that can be canceled
	Sigs    chan os.Signal        // channel to initiate a graceful shutdown when Ctrl+C is pressed
	Mutex   *sync.Mutex           // used in service provision and consumption to avoid race conditions
	learned *learnedCores         // core systems the registry told this system about; see CoreSystems
}

// CoreSystem struct holds details about the core system included in the configuration file
type CoreSystem struct {
	Name string `json:"coreSystem"`
	Url  string `json:"url"`
}

// learnedCores holds the core systems a system found out about from the
// registry, as opposed to those an operator wrote into its configuration.
//
// Under its own lock rather than the system's. GetRunningCoreSystemURL is
// called from every request path and from goroutines that may already hold
// sys.Mutex, so reading the learned list must never need it; and the list is
// appended to by one goroutine and read by many, which without a lock is the
// race the -race detector reports as a torn slice header.
type learnedCores struct {
	mu   sync.RWMutex
	list []*CoreSystem
}

// CoreSystems is every core system this system may try, in the order it will
// try them: what the configuration file says first, then what was learned.
//
// The order is the point. A file entry is the operator's statement and wins
// over anything the cloud said about itself; a learned entry is what lets the
// system find the standby registrar the file never mentioned.
func (sys *System) CoreSystems() []*CoreSystem {
	all := append([]*CoreSystem(nil), sys.Husk.CoreS...)
	if sys.learned == nil {
		return all
	}
	sys.learned.mu.RLock()
	defer sys.learned.mu.RUnlock()
	return append(all, sys.learned.list...)
}

// LearnedCoreSystems is only what was learned, for a cache to write.
func (sys *System) LearnedCoreSystems() []*CoreSystem {
	if sys.learned == nil {
		return nil
	}
	sys.learned.mu.RLock()
	defer sys.learned.mu.RUnlock()
	return append([]*CoreSystem(nil), sys.learned.list...)
}

// AddLearnedCoreSystem records a core system the registry reported, and says
// whether it was new. A URL already known — from the file or from an earlier
// lesson — is not added twice, so the list cannot grow with every refresh.
func (sys *System) AddLearnedCoreSystem(cs CoreSystem) bool {
	if sys.learned == nil || cs.Url == "" {
		return false
	}
	for _, known := range sys.Husk.CoreS {
		if known.Url == cs.Url {
			return false
		}
	}
	sys.learned.mu.Lock()
	defer sys.learned.mu.Unlock()
	for _, known := range sys.learned.list {
		if known.Url == cs.Url {
			return false
		}
	}
	c := cs
	sys.learned.list = append(sys.learned.list, &c)
	return true
}

// NewSystem instantiates the new system and gathers the host information
func NewSystem(name string, ctx context.Context) System {
	getBuildInfo()
	newSystem := System{Name: name}
	newSystem.Ctx = ctx
	newSystem.Sigs = make(chan os.Signal, 1)
	signal.Notify(newSystem.Sigs, syscall.SIGINT)
	newSystem.UAssets = make(map[string]*UnitAsset) // initialize UAsset as an empty map
	// Since the return System isn't a pointer (incorrectly), this map needs to
	// be a pointer instead (usually not normal) and initialized (usually not needed)
	// in order to avoid linter errors.
	// The errors is due to this func returning a copy of newSystem and attempts
	// to copy the mutex too, but it's not allowed for sync objects.
	// Reference: https://stackoverflow.com/questions/37242009/function-returns-lock-by-value
	newSystem.Mutex = &sync.Mutex{}
	newSystem.learned = &learnedCores{}
	return newSystem
}

// candidates reports whether more than one core system of this type is known,
// which is when it is worth asking each whether it answers.
func candidates(sys *System, systemType string) bool {
	n := 0
	for _, core := range sys.CoreSystems() {
		if core.Name == systemType && strings.TrimSpace(core.Url) != "" {
			n++
		}
	}
	return n > 1
}

// reachable reports whether something accepts connections at the URL's host
// and port. A TCP dial and nothing more: whether what answers is the right
// system is the caller's business, and a full request here would put a round
// trip in front of every lookup.
func reachable(u *url.URL) bool {
	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	conn, err := net.DialTimeout("tcp", host, 700*time.Millisecond)
	if err != nil {
		return false
	}
	// The dial was the whole question; closing it is housekeeping whose failure
	// changes nothing about whether something answered.
	_ = conn.Close()
	return true
}

// verifyStatus fetches a registrar's /status and returns what it said,
// whatever the status code: a standby answers 503 and the body is the part
// that matters, because it names the lead.
func verifyStatus(u *url.URL) (int, []byte, string, error) {
	resp, err := http.Get(u.String())
	if err != nil {
		return 0, nil, "", err
	}
	defer resp.Body.Close()
	// Body must be fully drained AND closed upon returning, otherwise it might leak memory
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, body, resp.Header.Get(LocalCloudHeader), err
}

const ServiceRegistrarName string = "serviceregistrar"
const ServiceRegistrarLeader string = "lead Service Registrar since"

// ServiceRegistrarStandby prefixes a standby's /status answer, followed by the
// lead's core URL. Exported so the registrar says it in exactly the form the
// client follows; two spellings of the same sentence would be a referral
// nobody takes.
const ServiceRegistrarStandby string = "On standby, leading registrar is "

// LocalCloudHeader carries a registrar's cloud name on its /status answer, so
// a peer can refuse to elect across a cloud boundary.
const LocalCloudHeader = "X-Local-Cloud"

// leadsByStatus reports whether the registrar at coreURL answers /status as
// the lead of the named cloud.
//
// A referral is followed only within the cloud that gave it. A system does not
// know its own cloud's name — only registrars declare one — but it can hold a
// standby to its word: the lead it names must say the same cloud the standby
// said. Without that, a host whose file named a standby of another cloud (a
// copied configuration, two clouds on one LAN) would follow its referral and
// register every service with a lead it was never meant to reach, while the
// registrars themselves refused to elect across the same boundary.
func leadsByStatus(coreURL, cloud string) bool {
	u, err := url.Parse(coreURL)
	if err != nil {
		return false
	}
	_, body, theirs, err := verifyStatus(u.JoinPath("status"))
	if err != nil || !bytes.HasPrefix(body, []byte(ServiceRegistrarLeader)) {
		return false
	}
	return cloud == "" || theirs == "" || theirs == cloud
}

// GetRunningCoreSystemURL returns the URL of a running core system based on the provided type.
// When systemType is "serviceregistrar", it verifies the service is the lead registrar by checking
// its /status endpoint response. For other core system types, it simply tests that the URL is accessible.
func GetRunningCoreSystemURL(sys *System, systemType string) (string, error) {
	// Store the latest error encountered when iterating thru the system list
	// and then return this error if no matching system was found.
	var lastErr error

	for _, core := range sys.CoreSystems() {
		// Ignore unrelated systems
		if core.Name != systemType {
			continue
		}

		// An entry with no URL is a slot, not a core system. The generated
		// configuration carries an empty authorizer entry so an operator can see
		// where the URL goes; until one is written, the cloud has no authorizer
		// and every caller of this function must reach that conclusion — a
		// provider that treated the slot as present would demand tokens nothing
		// could issue.
		if strings.TrimSpace(core.Url) == "" {
			continue
		}

		coreURL, err := url.Parse(core.Url)
		if err != nil {
			lastErr = fmt.Errorf("parsing core URL: %w", err)
			continue
		}

		coreSystemURL := coreURL.String() // Preserves the original URL
		if core.Name != ServiceRegistrarName {
			// The first entry used to be returned unexamined, which was fine
			// while a file named exactly one of each. It is not fine now that
			// the list has alternates: a second host's generated file names
			// an orchestrator on that host, which does not exist, and the one
			// learned from the lead sat behind it, never reached. So when
			// there is a next address to try, this one has to answer first.
			// A single entry is still returned without a probe — nothing to
			// fall through to, and the caller's own error says more.
			if !candidates(sys, systemType) || reachable(coreURL) {
				return coreSystemURL, nil
			}
			lastErr = fmt.Errorf("%s does not answer", coreSystemURL)
			continue
		}

		// Perform extra checks on the response from a service registrar
		status, body, cloud, err := verifyStatus(coreURL.JoinPath("status"))
		if err != nil {
			lastErr = fmt.Errorf("verifying registrar: %w", err)
			continue
		}
		if bytes.HasPrefix(body, []byte(ServiceRegistrarLeader)) {
			return coreSystemURL, nil
		}

		// A standby is a referral, not a dead end. With a registrar on every
		// host, the one this system was configured with — its own — is usually
		// not the lead, and skipping it would mean the file had to name the
		// lead after all. So the standby's answer is followed, once, and the
		// destination is asked to confirm it leads: a referral is taken on the
		// standby's word only as far as the next question.
		if status == http.StatusServiceUnavailable && bytes.HasPrefix(body, []byte(ServiceRegistrarStandby)) {
			lead := strings.TrimSpace(string(bytes.TrimPrefix(body, []byte(ServiceRegistrarStandby))))
			if leadsByStatus(lead, cloud) {
				return lead, nil
			}
			lastErr = fmt.Errorf("%s refers to %s as the lead, which does not answer as the lead of the same cloud", coreSystemURL, lead)
			continue
		}
		lastErr = fmt.Errorf("%s is not the lead registrar: %d %s", coreSystemURL, status, strings.TrimSpace(string(body)))
	}

	err := fmt.Errorf("core system '%s' not found", systemType)
	if lastErr != nil {
		err = fmt.Errorf("core system '%s' not found: %w", systemType, lastErr)
	}
	return "", err
}

// The following code is used only for issues support on GitHub @sdoque
var (
	AppName   string
	Version   string
	BuildDate string
	BuildHash string
)

func getBuildInfo() {
	// TODO: This info should be updated when setting up version release tools
	// Leaving the fmt.Prints as is for now.
	if AppName != "" {
		fmt.Printf("System: %s - %s\n", AppName, Version)
		fmt.Printf("Build date: %s\n", BuildDate)
		fmt.Printf("Build hash: %s\n", BuildHash)
	}
}
