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
	"errors"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

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

// knownURLs is the set of providers this cervice is already bound to.
func knownURLs(cer *components.Cervice) map[string]bool {
	cer.Mutex.RLock()
	defer cer.Mutex.RUnlock()
	urls := make(map[string]bool)
	for _, nodes := range cer.Nodes {
		for _, ni := range nodes {
			urls[ni.URL] = true
		}
	}
	return urls
}

// keepOnly discards every node the cervice was not already bound to, so a
// discovery made to refresh a credential cannot quietly widen or move the
// binding.
func keepOnly(cer *components.Cervice, allowed map[string]bool) {
	cer.Mutex.Lock()
	defer cer.Mutex.Unlock()
	for node, nodes := range cer.Nodes {
		kept := make([]components.NodeInfo, 0, len(nodes))
		for _, ni := range nodes {
			if allowed[ni.URL] {
				kept = append(kept, ni)
			}
		}
		if len(kept) == 0 {
			delete(cer.Nodes, node)
			continue
		}
		cer.Nodes[node] = kept
	}
}

// renewalMargin is how much of a token's life must remain for a consumer to go
// on using it. Below that it fetches a new one rather than waiting to be
// refused.
const renewalMargin = 0.2

// dueForRenewal reports whether a token is close enough to expiry that it should
// be replaced before being presented again.
//
// Nothing checked this before, and by design nothing needed to: renewal happened
// by being refused, and the comment on NodeInfo.Tokens said so. It works, and it
// costs one guaranteed 403 per binding per token lifetime — plus the reading
// that request was carrying, because the call fails and the loop moves on.
//
// At the cottage that is roughly forty bindings on a five-minute lifetime: a
// refusal every few seconds across the cloud, each losing a control cycle or a
// recorded point. The gaps drew flat lines across an InfluxDB chart for five
// hours and looked exactly like a frozen sensor.
//
// A consumer can see its own token's expiry without the authorizer's key:
// splitToken parses the claims before it checks the signature, because it has to
// know what it is verifying. So there is no reason for a consumer ever to be
// surprised by an expiry.
//
// Only a token whose expiry can actually be read is ever due. Renewal is an
// optimisation over being refused, so anything this cannot reason about is
// presented as-is and judged by the provider, which is the party that decides
// anyway. An empty token means an unauthorized cloud issued none; an unreadable
// or unbounded one means something is wrong with it that a provider will say so
// about. Calling either of them "due" would re-orchestrate before every single
// call — the same storm this exists to stop, arrived at from the other side.
func dueForRenewal(token string, now time.Time) bool {
	claims, _, _, err := splitToken(token)
	if err != nil || claims.Expires.IsZero() {
		return false
	}
	life := claims.Expires.Sub(claims.IssuedAt)
	if life <= 0 {
		return false
	}
	return now.After(claims.Expires.Add(-time.Duration(float64(life) * renewalMargin)))
}

// resolveProvider returns the provider and token this call should use,
// discovering or renewing as required.
//
// Renewal is best-effort on purpose. When a token is merely near expiry the one in
// hand is still valid, so a failed renewal is not a failed call: it proceeds on
// what it has and tries again next time. Renewing early must never leave a
// consumer worse off than not renewing at all.
func resolveProvider(cer *components.Cervice, sys *components.System, action string) (string, string, error) {
	url, token, found := pickNode(cer, action)
	if found && !dueForRenewal(token, time.Now()) {
		return url, token, nil
	}

	// A first discovery and a renewal arrive here by the same door, and they are
	// not the same question. A cervice with no nodes is asking "who provides
	// this?", and any answer will do. One that already knows a provider — a
	// thermostat paired to the thermometer in its own room — is asking only for
	// a new token for *that* one.
	//
	// So they ask differently. A first discovery takes the single answer the
	// orchestrator prefers. A renewal asks for *every* permitted provider and
	// then keeps only the one it already held, which is the sole way to be sure
	// the token it gets back belongs to the provider it meant.
	//
	// Asking the singular question for a renewal deadlocks, and did: the
	// orchestrator answers with whichever candidate it likes, keepOnly discards
	// it as a stranger, and the cervice is left holding a provider with no
	// token. Nothing recovers it — a provider that is up never produces the
	// transport failure that would allow re-binding — so the consumer repeats
	// "no read token for the provider this cervice is bound to" for ever. The
	// cottage's dining room lost its thermometer that way, ten seconds at a
	// time, while the kitchen recovered on the same tick by luck of which
	// candidate came back.
	bound := knownURLs(cer)
	var searchErr error
	if len(bound) > 0 {
		searchErr = Search4MultipleServicesAs(cer, sys, action)
		if searchErr == nil {
			keepOnly(cer, bound)
		}
	} else {
		searchErr = Search4ServicesAs(cer, sys, action)
	}

	if fresh, freshToken, ok := pickNode(cer, action); ok && !dueForRenewal(freshToken, time.Now()) {
		return fresh, freshToken, nil
	}
	if found {
		return url, token, nil
	}
	if searchErr != nil {
		return "", "", searchErr
	}
	return "", "", fmt.Errorf("no %s token for the %s provider this cervice is bound to", action, cer.Definition)
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
	serviceUrl, token, err := resolveProvider(cer, sys, action)
	if err != nil {
		return f, err
	}

	// A value somebody is already keeping current, answered without asking for
	// it. The caller's loop is unchanged and does not know: it asks on its own
	// clock and gets a reading that is at most one publisher heartbeat old,
	// where before every one of those calls was a request over the network.
	//
	// Only for a read. A PUT is an instruction to a provider and there is
	// nothing cached about it.
	if httpMethod == http.MethodGet {
		Follow(cer, sys)
		if payload, mediaType, fresh := cer.Recall(); fresh {
			f, err = Unpack(payload, mediaType)
			if err == nil {
				// The same conversion a polled reading gets, by the same code:
				// the provider publishes in its own unit and the consumer reads
				// in the one it asked for, cached or not.
				return NormalizeUnits(cer, f)
			}
			// Unreadable, so fall through and ask. Something changed at the
			// other end that this consumer does not understand, and a request
			// will fail loudly rather than quietly serving a stale reading.
			cer.Forget()
		}
	}

	var resp *http.Response
	for attempt := 0; ; attempt++ {
		resp, err = sendHTTPReqWithToken(httpMethod, serviceUrl, token, bodyBytes)
		if err == nil {
			break
		}

		var refused *ProviderRefusal
		if errors.As(err, &refused) {
			// A provider that answers is where this cervice thinks it is, and
			// the binding must survive — unless what it answered is that the
			// thing being asked for is no longer there. A 404 says the address
			// is right and the resource is not, which is a change of topology
			// wearing the clothes of an answer, and the only cure is to look
			// again.
			if refused.Vanished() {
				cer.Mutex.Lock()
				cer.Nodes = make(map[string][]components.NodeInfo)
				cer.Mutex.Unlock()
				return f, err
			}
			// Otherwise nothing about the topology is in doubt; at most the
			// credential is stale.
			if !refused.StaleCredential() || attempt > 0 {
				return f, err
			}
			// One retry, and only for a credential. Renewal ahead of expiry
			// should have prevented this, so reaching here means a clock
			// disagreed or a permission was withdrawn mid-flight — and in both
			// cases the reading should arrive late rather than not at all. A
			// control loop that loses a cycle to a housekeeping failure is how
			// five hours of flat line got drawn across a chart.
			forgetToken(cer, serviceUrl, action)
			serviceUrl, token, err = resolveProvider(cer, sys, action)
			if err != nil {
				return f, err
			}
			continue
		}

		// Could not reach it at all. The cloud's shape may genuinely have
		// changed, so forget what was discovered and let the next call search
		// without constraint. This is the only path that may re-bind.
		cer.Mutex.Lock()
		cer.Nodes = make(map[string][]components.NodeInfo)
		cer.Mutex.Unlock()
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
	return NormalizeUnits(cer, f)
}

// pickNode returns the first node discovered for one action, and whether any
// was. It looks past the first entry: a cervice discovered for two actions holds
// one node per provider, but a provider that answered only one of the two
// discoveries is present without a token for the other.
func pickNode(cer *components.Cervice, action string) (url, token string, ok bool) {
	cer.Mutex.RLock()
	defer cer.Mutex.RUnlock()

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

	if cer.ProviderCount() == 0 {
		if currentErr := Search4MultipleServicesAs(cer, sys, action); currentErr != nil {
			f = append(f, nil)
			err = append(err, currentErr)
			return f, err
		}
	}

	// The whole NodeInfo, not just its URL. The token is what proves to the
	// provider that the authorizer permitted this call, and flattening the nodes
	// to a list of strings threw it away — every request from this path went out
	// unauthorized while the single-provider path above sent one.
	// A snapshot, and the requests below are made from it rather than from the
	// map. Ranging over cer.Nodes while a discovery on another goroutine deletes
	// from it is `fatal error: concurrent map iteration and map write`; holding
	// the lock across the requests instead would block every other user of this
	// cervice for as long as the slowest provider takes to time out.
	providers := cer.Providers()

	// Discovered for a different action than this call performs. One round for
	// the whole cervice rather than one per provider: they were all discovered
	// together and they all need the same action.
	if needsDiscovery(providers, action) {
		if currentErr := Search4MultipleServicesAs(cer, sys, action); currentErr != nil {
			f = append(f, nil)
			err = append(err, currentErr)
			return f, err
		}
		providers = cer.Providers()
	}

	// No providers is an answer, and it has to be given as one. Returning empty
	// slices left the caller's range running zero times and its error check
	// finding nothing wrong — a control loop reads that as "no readings changed"
	// rather than "there are no sensors", and holds its last output. The
	// registrar restarting, or a detail that stopped matching, is enough to
	// produce it.
	if len(providers) == 0 {
		return []forms.Form{nil}, []error{
			fmt.Errorf("no provider of %q is available for %s", cer.Definition, action)}
	}

	failures := 0
	for _, ni := range providers {
		if len(ni.URL) == 0 {
			continue
		}
		formValue, currentErr := askOneProvider(httpMethod, ni, cer, action, bodyBytes)
		if currentErr != nil {
			failures++
			// Forget this one provider's token, not the whole set. Clearing
			// everything on one failure threw away the providers that had just
			// answered; clearing nothing until they had all failed left a
			// powered-off sensor in the list forever, retried every round at the
			// cost of its own timeout, and still there long after the registrar
			// had stopped listing it. Without a token for this action the node
			// is rediscovered on the next call, which either finds it again or
			// does not.
			if errors.As(currentErr, &staleProvider{}) {
				forgetToken(cer, ni.URL, action)
			}
			f = append(f, nil)
			err = append(err, currentErr)
			continue
		}
		f = append(f, formValue)
		err = append(err, nil)
	}

	_ = failures // every failure has already forgotten its own provider

	return f, err
}

// staleProvider marks a failure that says something about the *provider* rather
// than about the answer it gave.
//
// Only these are worth forgetting a token over: it could not be reached, or it
// refused the credential. An empty body, a form version this consumer does not
// know, a unit it cannot convert — those are the provider answering, and it is
// still the provider it was discovered as. Forgetting the token on those meant
// one sensor answering in an unknown form cost a full rediscovery of every
// provider on every poll thereafter, for as long as it kept answering.
type staleProvider struct{ err error }

func (s staleProvider) Error() string { return s.err.Error() }
func (s staleProvider) Unwrap() error { return s.err }

// forgetToken drops one provider's token for one action, so the next call
// rediscovers that provider rather than presenting a token to something that
// did not answer.
//
// The node itself is left in place. It carries the tokens for the other actions
// this cervice may also use, and discovery reconciles the set: a provider the
// registrar no longer lists is removed there, where the whole list is known.
func forgetToken(cer *components.Cervice, url, action string) {
	cer.Mutex.Lock()
	defer cer.Mutex.Unlock()

	for node, nodes := range cer.Nodes {
		for i, ni := range nodes {
			if ni.URL != url || ni.Tokens == nil {
				continue
			}
			if _, held := ni.Tokens[action]; !held {
				continue
			}
			ni.Tokens = tokensWithout(ni.Tokens, action)
			cer.Nodes[node][i] = ni
		}
	}
}

// tokensWithout returns a copy of a node's tokens with one action removed,
// because the map may not be written in place.
//
// A NodeInfo is copied by value when a consumer pins a provider into a cervice
// of its own — which is what ethermostat does, one cervice per heater — and
// copying the struct copies the map *header*, not the map. Every copy therefore
// shares one set of tokens, guarded by whichever cervice's mutex the writer
// happens to hold.
//
// Two of the cottage's heaters read the same thermometer, so two feedback loops
// held two different locks over one map and deleted from it at the same moment.
// Go stops that with "fatal error: concurrent map writes", which takes the whole
// system down — the control loop, the servers, all of it. Replacing the map
// instead of editing it means a reader holding an older copy sees a stale token
// rather than a corrupted map, and a stale token is a thing this code already
// knows how to handle.
func tokensWithout(tokens map[string]string, action string) map[string]string {
	fresh := make(map[string]string, len(tokens))
	for act, tok := range tokens {
		if act != action {
			fresh[act] = tok
		}
	}
	return fresh
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
		// Unreachable: whatever was discovered is not there now.
		return nil, staleProvider{err}
	}
	defer resp.Body.Close()

	// Every non-2xx answer, refusal included, has already been turned into an
	// error by sendHTTPReqWithToken — which reads the body, sanitizes it and
	// reports the status with the reason beside it. A status check here would
	// never fire: what reaches this line is always a 2xx.

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
	return NormalizeUnits(cer, formValue)
}
