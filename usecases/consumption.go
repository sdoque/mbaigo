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
	"fmt"
	"io"
	"log"
	"testing"

	"net/http"
	"net/url"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// GetState request the current state of a unit asset (via the asset's service)
func GetState(cer *components.Cervice, sys *components.System) (f forms.Form, err error) {
	return stateHandler(http.MethodGet, cer, sys, nil)
}

// GetStates requests the current state of certain services of a unit asset depending on requested definition and/or details
func GetStates(cer *components.Cervice, sys *components.System) (f []forms.Form, err []error) {
	return stateHandlers(http.MethodGet, cer, sys, nil)
}

// SetState puts a request to change the state of a unit asset (via the asset's service)
func SetState(cer *components.Cervice, sys *components.System, bodyBytes []byte) (f forms.Form, err error) {
	return stateHandler(http.MethodPut, cer, sys, bodyBytes)
}

func stateHandler(httpMethod string, cer *components.Cervice, sys *components.System, bodyBytes []byte) (f forms.Form, err error) {
	// The action is what this call will actually do, not what Cervice.Mode says
	// it might. The provider recomputes it from the method, so a token minted for
	// anything else is refused.
	action := ActionForMethod(httpMethod)

	// Nothing discovered yet, or what is discovered was discovered for a
	// different action — a cervice used for both a GET and a PUT, or one whose
	// Mode did not describe this call. Either way, ask for this action rather
	// than present a token minted for another one.
	serviceUrl, token, found := pickNode(cer, action)
	if !found {
		if err = Search4ServicesAs(cer, sys, action); err != nil {
			return f, err
		}
		serviceUrl, token, _ = pickNode(cer, action)
	}

	resp, err := sendHTTPReqWithToken(httpMethod, serviceUrl, token, bodyBytes)
	if err != nil {
		cer.Nodes = make(map[string][]components.NodeInfo) // Failed to get the resource at that location: reset the providers list, which will trigger a new service search
		return f, err
	}
	defer resp.Body.Close()

	// If the response includes a payload, unpack it into a forms.Form
	bodyBytes, err = io.ReadAll(resp.Body)
	if err != nil {
		return f, fmt.Errorf("reading state response body: %w", err)
	}

	if len(bodyBytes) < 1 {
		return f, fmt.Errorf("got empty response body")

	}

	headerContentType := resp.Header.Get("Content-Type")
	f, err = Unpack(bodyBytes, headerContentType)
	if err != nil {
		return f, err
	}
	// The provider answers in its own unit; the consumer reads in the one it
	// asked for. Neither has to know about the other.
	return NormaliseUnits(cer, f)
}

// pickNode returns the first node discovered for one action, and whether any
// was. It looks past the first entry: a cervice discovered for two actions holds
// one node per provider, but a provider that answered only one of the two
// discoveries is present without a token for the other.
func pickNode(cer *components.Cervice, action string) (url, token string, ok bool) {
	for _, nodes := range cer.Nodes {
		for _, ni := range nodes {
			if tok, discovered := ni.TokenFor(action); discovered {
				return ni.URL, tok, true
			}
		}
	}
	return "", "", false
}

const messengerMaxErrors int = 3

func LogDebug(sys *components.System, msg string, args ...any) {
	Log(sys, forms.LevelDebug, msg, args...)
}

func LogInfo(sys *components.System, msg string, args ...any) {
	Log(sys, forms.LevelInfo, msg, args...)
}

func LogWarn(sys *components.System, msg string, args ...any) {
	Log(sys, forms.LevelWarn, msg, args...)
}

func LogError(sys *components.System, msg string, args ...any) {
	Log(sys, forms.LevelError, msg, args...)
}

func Log(sys *components.System, lvl forms.MessageLevel, msg string, args ...any) {
	sm := forms.NewSystemMessage_v1(lvl, fmt.Sprintf(msg, args...), sys.Name)
	if !testing.Testing() {
		// Only print the msg locally if not running during `go test`
		log.Println(sm.String())
	}
	// Snapshot under the lock, send outside it. The lock used to be held for the
	// whole loop, across a POST to each messenger on a client with a 30-second
	// timeout — so one unreachable messenger held System.Mutex for half a minute
	// and every other holder of it waited. Sending is network work and does not
	// belong under a lock the rest of the system contends for.
	sys.Mutex.Lock()
	messengers := make(map[string]int, len(sys.Husk.Messengers))
	for host, errors := range sys.Husk.Messengers {
		messengers[host] = errors
	}
	sys.Mutex.Unlock()

	if len(messengers) == 0 {
		return
	}

	body, err := Pack(forms.Form(&sm), "application/json")
	if err != nil {
		log.Printf("failed to pack SystemMessage: %v\n", err)
		return
	}

	// Outcomes are collected and applied afterwards rather than written as they
	// happen: a messenger may have been registered or dropped while this was
	// sending, and reinstating one from a stale snapshot would resurrect it.
	failed := make(map[string]bool, len(messengers))
	for host := range messengers {
		failed[host] = sendLogMessage(host, body) != nil
	}

	sys.Mutex.Lock()
	defer sys.Mutex.Unlock()
	for host, wasFailure := range failed {
		errors, stillRegistered := sys.Husk.Messengers[host]
		if !stillRegistered {
			continue
		}
		errCount := 0 // If there's no error while sending msg, the count is reset
		if wasFailure {
			// Don't care what kinds of errors might be returned
			errCount = errors + 1
		}
		if errCount >= messengerMaxErrors {
			// Too many errors indicates a problematic messenger
			delete(sys.Husk.Messengers, host)
			continue
		}
		sys.Husk.Messengers[host] = errCount
	}
}

// Hard-coding the path is ugly but it skips an extra service discovery cycle for now
const logMessagePath string = "/log/message"

func sendLogMessage(host string, body []byte) error {
	u, err := url.Parse(host)
	if err != nil {
		return err
	}
	u = u.JoinPath(logMessagePath)
	resp, err := sendHTTPReq(http.MethodPost, u.String(), body)
	if err != nil {
		return err
	}
	_ = resp.Body.Close() // Don't care about the response body or any errors it might cause
	return nil
}

func stateHandlers(httpMethod string, cer *components.Cervice, sys *components.System, bodyBytes []byte) (f []forms.Form, err []error) {
	// As in stateHandler: the action is what this call performs, not what
	// Cervice.Mode says it might.
	action := ActionForMethod(httpMethod)

	if len(cer.Nodes) == 0 {
		if currentErr := Search4MultipleServicesAs(cer, sys, action); currentErr != nil {
			f = append(f, nil)
			err = append(err, currentErr)
			return f, err
		}
	}

	// The whole NodeInfo, not just its URL. The token is what proves to the
	// provider that the authorizer permitted this call, and flattening the nodes
	// to a list of strings threw it away — every request from this path went out
	// unauthorised while the single-provider path above sent one.
	var providers []components.NodeInfo
	for _, nodes := range cer.Nodes {
		providers = append(providers, nodes...)
	}

	// Discovered for a different action than this call performs. One round for
	// the whole cervice rather than one per provider: they were all discovered
	// together and they all need the same action.
	if needsDiscovery(providers, action) {
		if currentErr := Search4MultipleServicesAs(cer, sys, action); currentErr != nil {
			f = append(f, nil)
			err = append(err, currentErr)
			return f, err
		}
		providers = providers[:0]
		for _, nodes := range cer.Nodes {
			providers = append(providers, nodes...)
		}
	}

	failures := 0
	for _, ni := range providers {
		if len(ni.URL) == 0 {
			continue
		}
		formValue, currentErr := askOneProvider(httpMethod, ni, cer, action, bodyBytes)
		if currentErr != nil {
			failures++
			f = append(f, nil)
			err = append(err, currentErr)
			continue
		}
		f = append(f, formValue)
		err = append(err, nil)
	}

	// Only when nothing answered. Discarding every provider because one of them
	// failed threw away the ones that had just worked, and forced a rediscovery
	// on the next call for no reason.
	if failures > 0 && failures == len(f) {
		cer.Nodes = make(map[string][]components.NodeInfo)
	}

	return f, err
}

// needsDiscovery reports whether any provider lacks a token for this action, and
// so has not been discovered for it.
func needsDiscovery(providers []components.NodeInfo, action string) bool {
	for _, ni := range providers {
		if len(ni.URL) == 0 {
			continue
		}
		if _, discovered := ni.TokenFor(action); !discovered {
			return true
		}
	}
	return false
}

// askOneProvider performs one request of a multi-provider round and returns the
// reading in the unit the consumer asked for.
//
// It is a function rather than the body of the loop so that the response is
// closed when this provider is done with. The loop used to defer every Close to
// the end of the round, holding one connection open per provider for the
// duration of the slowest of them.
func askOneProvider(httpMethod string, ni components.NodeInfo, cer *components.Cervice, action string, bodyBytes []byte) (forms.Form, error) {
	token, _ := ni.TokenFor(action)
	resp, err := sendHTTPReqWithToken(httpMethod, ni.URL, token, bodyBytes)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// A separate variable: assigning into bodyBytes made the previous provider's
	// answer the request body sent to the next one.
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading state response body: %w", err)
	}
	if len(respBytes) < 1 {
		return nil, fmt.Errorf("got empty response body")
	}

	formValue, err := Unpack(respBytes, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("unpacking response body: %w", err)
	}

	// Each provider answers in its own unit. Without this the caller received a
	// mixture — °C from one sensor and °F from the next — with nothing in the
	// slice to say which was which.
	return NormaliseUnits(cer, formValue)
}
