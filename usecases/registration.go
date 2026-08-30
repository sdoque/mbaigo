/*******************************************************************************
 * Copyright (c) 2024 Synecdoque
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

// Package "usecases" addresses system behaviors and actions in given use cases
// such as configuration, registration, authentication, orchestration, ...

package usecases

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

type registrarTracker struct {
	url   string
	mutex sync.RWMutex
	// last is the most recent lead that existed, and generation counts the
	// times it changed to a different one. A failover is seen by the poller as
	// old lead, then nothing while the election runs, then new lead — so a
	// move is judged against the last lead that was, not against the empty
	// reading in between, or the ordinary sequence would never count as one.
	//
	// moved is closed, and replaced, on each such move — a broadcast every
	// registration loop can select on. The generation is what a loop compares
	// after any wake, so a move that lands while it is inside an HTTP call
	// (which is when a dying lead is slowest to answer) is not lost: the
	// channel it was holding is the old one, and the count says what happened.
	// Without this a system noticed a failover only on its next registration
	// tick — for a controller, two minutes invisible to a cloud whose new lead
	// started with an empty registry.
	last       string
	generation uint64
	moved      chan struct{}
}

// newRegistrarTracker makes the channel once, so no method has to wonder
// whether it exists: a nil channel blocks a select arm forever, which would be
// this fix's own bug reintroduced by a method that forgot the check.
func newRegistrarTracker() *registrarTracker {
	return &registrarTracker{moved: make(chan struct{})}
}

func (rt *registrarTracker) set(url string) {
	rt.mutex.RLock()
	same := url == rt.url
	rt.mutex.RUnlock()
	if same {
		return // the common tick: nothing changed, no writer takes the lock
	}
	rt.mutex.Lock()
	defer rt.mutex.Unlock()
	rt.url = url
	if url == "" || url == rt.last {
		return // lost, or back where it was: nowhere new to go
	}
	if rt.last != "" {
		rt.generation++
		close(rt.moved)
		rt.moved = make(chan struct{})
	}
	rt.last = url
}

// changed returns a channel that closes on the next move to a different lead.
func (rt *registrarTracker) changed() <-chan struct{} {
	rt.mutex.RLock()
	defer rt.mutex.RUnlock()
	return rt.moved
}

// moves returns how many times the lead has moved, for a loop to compare.
func (rt *registrarTracker) moves() uint64 {
	rt.mutex.RLock()
	defer rt.mutex.RUnlock()
	return rt.generation
}

func (rt *registrarTracker) get() string {
	rt.mutex.RLock()
	defer rt.mutex.RUnlock()
	return rt.url
}

// RegisterServices keeps track of the leading Service Registrar and keeps all services registered
func RegisterServices(sys *components.System) {
	// Refuse to start rather than register a service the authorizer cannot
	// classify. A mission that may be left blank is one that gets left blank,
	// and an absent mission has no safe reading: a permissive default is a hole
	// and a restrictive one gets worked around. Failing here — after the unit
	// assets are built, before anything is advertised — is what keeps the field
	// trustworthy enough to authorize against.
	if err := ValidateMissions(sys); err != nil {
		log.Fatalf("mission configuration error: %v\n", err)
	}

	// Mobility moved from Details to a field of its own; a configuration file
	// written before that still says it the old way, and is honoured once.
	AdoptMobilityDetails(sys)
	if err := ValidateMobilities(sys); err != nil {
		log.Fatalf("mobility configuration error: %v\n", err)
	}

	// Before anything is advertised, so a service registered as subscribable can
	// be followed from the moment a consumer discovers it. Every system calls
	// this, so turning subscription on stays a matter of configuration.
	PreparePublishers(sys)

	// Keep track of the registrar URL. The URL is shared between goroutines,
	// so it must be protected from data races using a mutex.
	registrar := newRegistrarTracker()

	// Goroutine looking for leading service registrar every 5 seconds
	go func() {
		ticker := time.Tick(5 * time.Second)
		// What the cloud has been asked about itself, and when. Learning costs
		// four registry queries, so it happens when the lead changes and once
		// a minute otherwise — not on every tick.
		var learnedFrom string
		var learnedAt time.Time
		for {
			newURL, err := components.GetRunningCoreSystemURL(sys, components.ServiceRegistrarName)
			registrar.set(newURL) // should be empty on error anyway
			if err != nil {
				log.Println("failed to find lead registrar:", err)
			}
			if newURL != "" && holdsCertificate(sys) && (newURL != learnedFrom || time.Since(learnedAt) > time.Minute) {
				if _, err := LearnCoreSystems(sys, newURL); err != nil {
					log.Printf("%s: learning the cloud's core systems: %v\n", sys.Name, err)
				}
				learnedFrom, learnedAt = newURL, time.Now()
			}

			select {
			case <-ticker:
			case <-sys.Ctx.Done():
				return
			}
		}
	}()

	// Run registration loops for each services
	assetList := &sys.UAssets
	for _, aResource := range *assetList {
		servs := (*aResource).GetServices()
		for _, service := range servs {
			go func(theUnitAsset *components.UnitAsset, theService *components.Service) {
				delay := 1 * time.Second
				var err error
				// One timer, reset after each registration, rather than a
				// time.After per iteration that a move would orphan for a period.
				wait := time.NewTimer(delay)
				defer wait.Stop()
				seen := registrar.moves()
				for {
					// The channel is taken before waiting and the count compared
					// after, whichever arm woke: a move during the registration
					// below closes the channel this holds and bumps the count,
					// and neither is lost to the next iteration.
					moved := registrar.changed()
					select {
					case <-wait.C:
					case <-moved:
					case <-sys.Ctx.Done():
						err = unregisterService(registrar.get(), theService)
						if err != nil {
							log.Println("unregistering service:", err)
						}
						return
					}
					if now := registrar.moves(); now != seen {
						// A new lead holds nothing of this system, and the id it
						// has is the old lead's: the next registration is a
						// fresh one, whichever arm woke — so the timer path cannot
						// renew a foreign id and the move path then register a
						// second record. After a short random pause, so a cloud of
						// many systems does not arrive at a registrar that just
						// took the lead all in the same instant.
						seen = now
						theService.ID = 0
						time.Sleep(time.Duration(rand.IntN(3000)) * time.Millisecond)
					}
					delay, err = registerService(sys, registrar.get(), theUnitAsset, theService)
					if err != nil {
						log.Println("registering service:", err)
					}
					if !wait.Stop() {
						select {
						case <-wait.C:
						default:
						}
					}
					wait.Reset(delay)
				}
			}(aResource, service)
		}
	}
}

// registerService makes a POST or PUT request to register or register individual services
func registerService(sys *components.System, registrar string, ua *components.UnitAsset, serv *components.Service) (delay time.Duration, err error) {
	delay = 15 * time.Second

	// Nothing bound yet, so there is no endpoint to advertise. Registration
	// starts before the servers do, and a record with no port is worse than no
	// record: a consumer would be sent to port 0.
	if !sys.Husk.Bound.Any() {
		return 2 * time.Second, nil
	}

	if registrar == "" {
		if serv.ID != 0 {
			serv.ID = 0 // reset the service ID, so that a new registration (POST) will be made when the registrar is back
		}
		return
	}

	// Prepare request
	reqPayload, err := serviceRegistrationForm(sys, ua, serv, "ServiceRecord_v1")
	if err != nil {
		err = fmt.Errorf("registration marshall: %w", err)
		return
	}
	registrationURL := registrar + "/registry"

	var req *http.Request // Declare req outside the blocks
	if serv.ID == 0 {
		req, err = http.NewRequest("POST", registrationURL, bytes.NewBuffer(reqPayload))
		if err != nil {
			err = fmt.Errorf("unable to register service %s with lead registrar", serv.Definition)
			return
		}
	} else {
		req, err = http.NewRequest("PUT", registrationURL, bytes.NewBuffer(reqPayload))
		if err != nil {
			err = fmt.Errorf("unable to confirm the %s service with lead registrar", serv.Definition)
			return
		}
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := http.DefaultClient.Do(req) // execute the request and get the reply
	if err != nil {
		err = fmt.Errorf("registration request: %w", err)
		serv.ID = 0 // if re-registration failed, a complete new one should be made (POST)
		return
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		err = fmt.Errorf("bad registration response: %s", resp.Status)
		serv.ID = 0
		return
	}

	// Handle response ------------------------------------------------

	var b []byte
	b, err = io.ReadAll(resp.Body) // Use io.ReadAll instead of ioutil.ReadAll
	if err != nil {
		err = fmt.Errorf("reading registration response body: %w", err)
		return
	}
	defer resp.Body.Close()

	headerContentType := resp.Header.Get("Content-Type")
	rRecord, err := Unpack(b, headerContentType)
	if err != nil {
		err = fmt.Errorf("extracting the registration record reply: %w", err)
		return
	}

	// Perform a type assertion to convert the returned Form to ServiceRecord_v1
	rr, ok := rRecord.(*forms.ServiceRecord_v1)
	if !ok {
		err = fmt.Errorf("invalid form from the service registration reply")
		return
	}

	serv.ID = rr.Id
	serv.RegTimestamp = rr.Created
	serv.RegExpiration = rr.EndOfValidity
	parsedTime, err := time.Parse(time.RFC3339, rr.EndOfValidity)
	if err != nil {
		err = fmt.Errorf("parsing time: %w", err)
		return
	}
	// should not wait until the deadline to start to confirm live status
	delay = time.Until(parsedTime.Add(-5 * time.Second))
	if delay < 1*time.Second {
		// Avoid using zero/negative delays
		delay = 1 * time.Second
	}
	return
}

// unregisterService deletes a service from the database based on its service id
func unregisterService(registrar string, serv *components.Service) error {
	if registrar == "" {
		return nil // there is no need to deregister if there is no leading registrar
	}
	u := registrar + "/registry/" + strconv.Itoa(serv.ID)
	req, err := http.NewRequest("DELETE", u, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Can't do anything about network errors. Don't care much either,
		// since this system is shutting down. Ignorering this error for now.
		return nil
	}
	defer resp.Body.Close()
	return nil
}

// serviceRegistrationForm returns a json data byte array with the data of the service to be registered
// in the form of choice [Sending @ Application system]
func serviceRegistrationForm(sys *components.System, ua *components.UnitAsset, serv *components.Service, version string) (payload []byte, err error) {
	var f forms.Form
	switch version {
	case "ServiceRecord_v1":
		resName := (*ua).GetName()
		var sr forms.ServiceRecord_v1 // declare a new service form
		sr.NewForm()
		sr.Id = serv.ID
		sr.ServiceDefinition = serv.Definition
		sr.SystemName = sys.Name
		sr.ServiceNode = sys.Husk.Host.Name + "_" + sys.Name + "_" + resName + "_" + serv.Definition
		sr.IPAddresses = sys.Husk.Host.IPAddresses
		// What this system is serving, not what its configuration names. An
		// HTTPS port binds only after enrollment, and a consumer is handed the
		// HTTPS endpoint in preference to the HTTP one — so advertising it early
		// sent every consumer to a port nothing was listening on, for as long as
		// enrollment took, while the HTTP port beside it worked the whole time.
		sr.ProtoPort = sys.Husk.Bound.Serving()
		// The mission travels on the record because the authorizer evaluates
		// policy along it and reads from the registrar, not from each system's
		// local configuration file. Registration copies only Details otherwise,
		// so without this line the mission never leaves the providing system.
		// It is the service's effective mission, not the asset's: an asset that
		// fronts a device — a PLC, a broker, a gateway — is too coarse to
		// authorize against.
		sr.Mission = components.EffectiveMission(ua, serv).String()
		// Whether this service can be followed rather than asked repeatedly.
		// It travels for the same reason the mission does: a consumer decides
		// whether to subscribe from what the registrar told it, never from the
		// provider's own configuration file, which it cannot read.
		//
		// The field existed on both the service and the record and nothing
		// joined them, so every service in every cloud has registered as not
		// subscribable whatever it declared — and a consumer, believing the
		// registry, polled a service that was willing to publish.
		sr.SubscribeAble = serv.SubscribeAble
		sr.Details = deepCopyMap((*ua).GetDetails())
		for key, valueSlice := range serv.Details {
			sr.Details[key] = append(sr.Details[key], valueSlice...)
		}
		sr.SubPath = resName + "/" + serv.SubPath

		if serv.RegPeriod != 0 {
			sr.RegLife = serv.RegPeriod
		} else {
			sr.RegLife = 30
		}
		sr.Created = serv.RegTimestamp
		f = &sr
	default:
		err = errors.New("unsupported service registration form version")
		return
	}
	payload, err = json.MarshalIndent(f, "", "  ")
	return
}

// deepCopyMap is necessary to prevent adding values to the original map at every re-registration
func deepCopyMap(m map[string][]string) map[string][]string {
	newMap := make(map[string][]string)
	for k, v := range m {
		newValue := make([]string, len(v))
		copy(newValue, v)
		newMap[k] = newValue
	}
	return newMap
}

// ServiceRegistrationFormsList returns the list of forms that the service registration handles
func ServiceRegistrationFormsList() []string {
	return []string{"ServiceRecord_v1"}
}

// holdsCertificate reports whether this system is enrolled yet. Learning waits
// for it: what is learned is meant to be asked for over mTLS, and before
// enrollment there is nothing to present.
func holdsCertificate(sys *components.System) bool {
	if sys.Mutex != nil {
		sys.Mutex.Lock()
		defer sys.Mutex.Unlock()
	}
	return sys.Husk.Certificate != ""
}
