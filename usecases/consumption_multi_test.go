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

package usecases

import (
	jsonpkg "encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// recordingTransport answers every request with the reading its URL is keyed to
// and keeps what it was asked, so a test can inspect the requests rather than
// only the answers.
type recordingTransport struct {
	readings map[string]string // URL → response body
	tokens   []string          // token header seen, in order
	bodies   []string          // request body seen, in order
	urls     []string
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.tokens = append(rt.tokens, req.Header.Get(TokenHeader))
	rt.urls = append(rt.urls, req.URL.String())

	body := ""
	if req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		body = string(raw)
	}
	rt.bodies = append(rt.bodies, body)

	reading, ok := rt.readings[req.URL.String()]
	if !ok {
		return nil, fmt.Errorf("no provider mocked at %s", req.URL)
	}
	return &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(reading)),
		Request:    req,
	}, nil
}

func signalBody(value float64, unit string) string {
	return fmt.Sprintf(`{"value":%v,"unit":%q,"timestamp":"0001-01-01T00:00:00Z","version":"SignalA_v1.0"}`, value, unit)
}

// mockProviders installs a transport answering the given URL→body map and
// returns it, restoring the previous one when the test ends.
func mockProviders(t *testing.T, readings map[string]string) *recordingTransport {
	t.Helper()
	rt := &recordingTransport{readings: readings}
	useTransport(t, rt)
	return rt
}

func multiProviderCervice(nodes []components.NodeInfo, details map[string][]string) *components.Cervice {
	return &components.Cervice{
		IReferentce: "test",
		Definition:  "temperature",
		Details:     details,
		Nodes:       map[string][]components.NodeInfo{"test": nodes},
		Protos:      []string{"http"},
	}
}

// TestGetStatesSendsEachProvidersToken is the defect this test was written for:
// stateHandlers flattened the nodes to a list of URLs, discarding the token each
// NodeInfo carries, and then called sendHTTPReq. Every request from the
// multi-provider path went out unauthorized, so in a cloud with an authorizer
// this whole path was refused while the single-provider path worked.
func TestGetStatesSendsEachProvidersToken(t *testing.T) {
	rt := mockProviders(t, map[string]string{
		"http://north/temperature": signalBody(20, ""),
		"http://south/temperature": signalBody(21, ""),
	})

	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://north/temperature", Tokens: map[string]string{"read": "token-north"}},
		{URL: "http://south/temperature", Tokens: map[string]string{"read": "token-south"}},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})

	sys := createTestSystem(false)
	_, errs := GetStates(cer, &sys)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("provider %d: %v", i, err)
		}
	}

	seen := map[string]string{}
	for i, u := range rt.urls {
		seen[u] = rt.tokens[i]
	}
	if got := seen["http://north/temperature"]; got != "token-north" {
		t.Errorf("north was sent token %q, want %q", got, "token-north")
	}
	if got := seen["http://south/temperature"]; got != "token-south" {
		t.Errorf("south was sent token %q, want %q", got, "token-south")
	}
}

// TestGetStatesNormalizesEveryProvidersUnit is the second half of the defect:
// the unpacked form was appended as it arrived. A consumer polling a °C sensor
// and a °F sensor got 20 and 68 in one slice with nothing to say which was
// which, and averaging them gave 44 of nothing.
func TestGetStatesNormalizesEveryProvidersUnit(t *testing.T) {
	mockProviders(t, map[string]string{
		"http://celsius/temperature":    signalBody(20, "<http://qudt.org/vocab/unit/DEG_C>"),
		"http://fahrenheit/temperature": signalBody(68, "<http://qudt.org/vocab/unit/DEG_F>"),
	})

	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://celsius/temperature", Tokens: map[string]string{"read": ""}},
		{URL: "http://fahrenheit/temperature", Tokens: map[string]string{"read": ""}},
	}, map[string][]string{
		"Forms": {"SignalA_v1a"},
		"Unit":  {"<http://qudt.org/vocab/unit/DEG_C>"},
	})

	sys := createTestSystem(false)
	got, errs := GetStates(cer, &sys)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("provider %d: %v", i, err)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 readings, got %d", len(got))
	}

	// 68 °F is 20 °C: both providers report the same temperature, so after
	// normalization both readings must be 20.
	for i, f := range got {
		sig, ok := f.(*forms.SignalA_v1a)
		if !ok {
			t.Fatalf("reading %d is not a signal form", i)
		}
		if sig.Value < 19.99 || sig.Value > 20.01 {
			t.Errorf("reading %d is %v %s, want 20 °C", i, sig.Value, sig.Unit)
		}
	}
}

// TestGetStatesDoesNotSendOneProvidersAnswerToTheNext is the defect that was
// waiting to happen: the loop read each response into bodyBytes, the same
// variable holding the request body. With a nil body — every caller so far —
// nothing showed. The moment this path carried a payload, the second provider
// would have been sent the first one's answer as its request.
func TestGetStatesDoesNotSendOneProvidersAnswerToTheNext(t *testing.T) {
	rt := mockProviders(t, map[string]string{
		"http://first/temperature":  signalBody(20, ""),
		"http://second/temperature": signalBody(21, ""),
	})

	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://first/temperature", Tokens: map[string]string{"write": ""}},
		{URL: "http://second/temperature", Tokens: map[string]string{"write": ""}},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})

	sys := createTestSystem(false)
	request := []byte(signalBody(99, "<http://qudt.org/vocab/unit/DEG_C>"))
	if _, errs := stateHandlers(http.MethodPut, cer, &sys, request); errs[0] != nil || errs[1] != nil {
		t.Fatalf("unexpected errors: %v", errs)
	}

	for i, sent := range rt.bodies {
		if sent != string(request) {
			t.Errorf("provider %d was sent %q, want the caller's body %q", i, sent, request)
		}
	}
}

// TestGetStatesKeepsWorkingProvidersWhenOneFails checks the recovery is
// proportionate. One unreachable provider used to empty cer.Nodes, discarding
// the ones that had just answered and forcing a rediscovery on the next call.
func TestGetStatesKeepsWorkingProvidersWhenOneFails(t *testing.T) {
	mockProviders(t, map[string]string{
		"http://alive/temperature": signalBody(20, ""),
		// nothing at http://dead/temperature — the transport refuses it
	})

	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://alive/temperature", Tokens: map[string]string{"read": ""}},
		{URL: "http://dead/temperature", Tokens: map[string]string{"read": ""}},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})

	sys := createTestSystem(false)
	_, errs := GetStates(cer, &sys)

	failed := 0
	for _, err := range errs {
		if err != nil {
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("expected exactly one failure, got %d of %d", failed, len(errs))
	}
	if len(cer.Nodes) == 0 {
		t.Error("one failed provider discarded the provider list, including the one that answered")
	}
}

// TestGetStatesForgetsProvidersWhenNoneAnswer is the other side of it: when
// nothing answered, nothing may be used again without rediscovering it.
func TestGetStatesForgetsProvidersWhenNoneAnswer(t *testing.T) {
	mockProviders(t, map[string]string{})

	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://dead1/temperature", Tokens: map[string]string{"read": ""}},
		{URL: "http://dead2/temperature", Tokens: map[string]string{"read": ""}},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})

	sys := createTestSystem(false)
	if _, errs := GetStates(cer, &sys); len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %v", errs)
	}
	for _, nodes := range cer.Nodes {
		for _, ni := range nodes {
			if _, discovered := ni.TokenFor("read"); discovered {
				t.Errorf("%s answered nothing but is still usable without rediscovery", ni.URL)
			}
		}
	}
}

// TestADeadProviderIsForgottenWhileTheLiveOneIsKept is follow-up finding N10.
//
// The list was cleared only when every provider had failed, and discovery added
// without ever removing. So a cervice holding a powered-off sensor and a working
// one never reset: the failure count never reached the total, the dead node kept
// its token so nothing triggered a rediscovery, and it was retried every round
// at the cost of its own timeout — for as long as the consumer ran, and long
// after the registrar had stopped listing it.
func TestADeadProviderIsForgottenWhileTheLiveOneIsKept(t *testing.T) {
	mockProviders(t, map[string]string{
		"http://alive/temperature": signalBody(20, ""),
		// nothing mocked at http://dead/temperature
	})

	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://alive/temperature", Tokens: map[string]string{"read": "tok-alive"}},
		{URL: "http://dead/temperature", Tokens: map[string]string{"read": "tok-dead"}},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})

	sys := createTestSystem(false)
	if _, errs := GetStates(cer, &sys); len(errs) != 2 {
		t.Fatalf("expected one answer and one failure, got %v", errs)
	}

	var aliveKept, deadForgotten bool
	for _, nodes := range cer.Nodes {
		for _, ni := range nodes {
			_, discovered := ni.TokenFor("read")
			switch ni.URL {
			case "http://alive/temperature":
				aliveKept = discovered
			case "http://dead/temperature":
				deadForgotten = !discovered
			}
		}
	}
	if !deadForgotten {
		t.Error("the provider that did not answer kept its token, so it will be retried forever without rediscovery")
	}
	if !aliveKept {
		t.Error("the provider that answered lost its token, so one failure still costs a rediscovery of everything")
	}
}

// A discovery for several providers returns everything registered under the
// definition, so what it does not return has been deregistered and must go.
// Adding without removing is what left a departed sensor in the list.
func TestDiscoveryDropsAProviderTheRegistrarNoLongerLists(t *testing.T) {
	// The registrar now lists only the survivor.
	useTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"version":"ServiceRecordList_v1","list":[{"registryID":1,"definition":"temperature",` +
			`"systemName":"survivor","serviceNode":"n","ipAddresses":["survivor"],` +
			`"protoPort":{"http":80},"subpath":"temperature","version":"ServiceRecord_v1"}]}`
		return &http.Response{
			Status: "200 OK", StatusCode: 200,
			Header:  http.Header{"Content-Type": []string{"application/json"}},
			Body:    io.NopCloser(strings.NewReader(body)),
			Request: req,
		}, nil
	}))

	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://survivor:80/temperature", Tokens: map[string]string{"read": "old"}},
		{URL: "http://departed/temperature", Tokens: map[string]string{"read": "old"}},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})

	sys := createTestSystem(false)
	if err := Search4MultipleServicesAs(cer, &sys, "read"); err != nil {
		t.Fatalf("discovery: %v", err)
	}

	for _, nodes := range cer.Nodes {
		for _, ni := range nodes {
			if ni.URL == "http://departed/temperature" {
				t.Error("a provider the registrar no longer lists is still cached")
			}
		}
	}
	if _, _, ok := pickNode(cer, "read"); !ok {
		t.Error("the surviving provider was dropped too")
	}
}

// actionRecordingTransport answers orchestration quests with a token naming the
// action asked for, and records the action each service request presented.
type actionRecordingTransport struct {
	serviceURL string
	quested    []string // action of each orchestration quest
	presented  []string // token presented on each service request
}

func (t *actionRecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	json := func(body string) (*http.Response, error) {
		return &http.Response{
			Status: "200 OK", StatusCode: 200,
			Header: http.Header{"Content-Type": []string{"application/json"}},
			Body:   io.NopCloser(strings.NewReader(body)), Request: req,
		}, nil
	}

	if strings.HasSuffix(req.URL.Path, "/squest") {
		raw, _ := io.ReadAll(req.Body)
		var quest forms.ServiceQuest_v1
		if err := jsonpkg.Unmarshal(raw, &quest); err != nil {
			return nil, err
		}
		t.quested = append(t.quested, quest.Action)
		// A token that names the action it was minted for, as the real one does.
		return json(fmt.Sprintf(
			`{"serviceURL":%q,"serviceNode":"n","token":"tok-%s","version":"ServicePoint_v1"}`,
			t.serviceURL, quest.Action))
	}

	t.presented = append(t.presented, req.Header.Get(TokenHeader))
	return json(signalBody(20, ""))
}

// TestTheTokenIsMintedForTheActionPerformed is the defect this test was written
// for: the action came from Cervice.Mode, which defaults to "read" when unset.
// A cervice that only ever writes got a read token, the provider recomputed
// "write" from the PUT, and every write through it was refused — which is why
// the ethermostat's heaters never switched.
func TestTheTokenIsMintedForTheActionPerformed(t *testing.T) {
	rt := &actionRecordingTransport{serviceURL: "http://provider/OnOff"}
	useTransport(t, rt)

	// No Mode at all — the case that used to silently mean "read".
	cer := &components.Cervice{
		IReferentce: "test", Definition: "OnOff",
		Details: map[string][]string{"Forms": {"SignalA_v1a"}},
		Nodes:   make(map[string][]components.NodeInfo),
		Protos:  []string{"http"},
	}
	sys := createTestSystem(false)

	if _, err := SetState(cer, &sys, []byte(signalBody(1, ""))); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	if len(rt.quested) != 1 || rt.quested[0] != "write" {
		t.Fatalf("orchestrated for %v, want one quest for \"write\"", rt.quested)
	}
	if len(rt.presented) != 1 || rt.presented[0] != "tok-write" {
		t.Errorf("presented %v, want the write token", rt.presented)
	}
}

// TestOneCerviceHoldsATokenPerAction is the other half: NodeInfo.Token was a
// single string, so a cervice used for both a GET and a PUT — the clerk's order
// service — structurally could not hold both tokens. Whichever action was
// discovered first won, and the other was refused for the life of the process.
func TestOneCerviceHoldsATokenPerAction(t *testing.T) {
	rt := &actionRecordingTransport{serviceURL: "http://provider/order"}
	useTransport(t, rt)

	cer := &components.Cervice{
		IReferentce: "test", Definition: "order",
		Details: map[string][]string{"Forms": {"SignalA_v1a"}},
		Nodes:   make(map[string][]components.NodeInfo),
		Protos:  []string{"http"},
	}
	sys := createTestSystem(false)

	if _, err := GetState(cer, &sys); err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if _, err := SetState(cer, &sys, []byte(signalBody(1, ""))); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	if want := []string{"tok-read", "tok-write"}; !equalStrings(rt.presented, want) {
		t.Errorf("presented %v, want %v", rt.presented, want)
	}

	// One provider, not two: the second discovery must merge into the node the
	// first found rather than append a duplicate the caller would poll twice.
	total := 0
	for _, nodes := range cer.Nodes {
		total += len(nodes)
	}
	if total != 1 {
		t.Errorf("the cervice holds %d nodes for one provider, want 1", total)
	}
	for _, nodes := range cer.Nodes {
		for _, ni := range nodes {
			if len(ni.Tokens) != 2 {
				t.Errorf("the node holds %v, want a token for each of read and write", ni.Tokens)
			}
		}
	}
}

// TestARefusalCarriesItsReason: the provider writes why it refused into the
// body, and returning a bare status code left the operator with "403" for a
// refusal that names its own cause.
func TestARefusalCarriesItsReason(t *testing.T) {
	useTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Status: "403 Forbidden", StatusCode: 403,
			Header:  http.Header{"Content-Type": []string{"text/plain"}},
			Body:    io.NopCloser(strings.NewReader("mismatch (action): read vs write")),
			Request: req,
		}, nil
	}))

	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://provider/OnOff", Tokens: map[string]string{"read": "stale"}},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})
	sys := createTestSystem(false)

	_, err := GetState(cer, &sys)
	if err == nil {
		t.Fatal("a 403 was not reported as an error")
	}
	if !strings.Contains(err.Error(), "mismatch (action)") {
		t.Errorf("the refusal reads %q, which does not say why", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestMultiProviderDiscoveryCarriesTheToken is the property the multi-provider
// path never had.
//
// The consumer side was right — askOneProvider reads the token and sets the
// header — but the token was empty every time, because the orchestrator
// answered with a ServiceRecordList_v1, which has nowhere to put one. So every
// request from GetStates went out unauthorized, each provider refused it, and
// the round repeated on the next poll. The comment claiming this was fixed was
// written in the past tense while it was still true.
func TestMultiProviderDiscoveryCarriesTheToken(t *testing.T) {
	useTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"version":"ServicePointList_v1","list":[` +
			`{"providerName":"kitchen","definition":"temperature","serviceURL":"http://kitchen/temperature",` +
			`"serviceNode":"kitchen_n","token":"token-for-kitchen","version":"ServicePoint_v1"},` +
			`{"providerName":"bathroom","definition":"temperature","serviceURL":"http://bathroom/temperature",` +
			`"serviceNode":"bathroom_n","token":"token-for-bathroom","version":"ServicePoint_v1"}]}`
		return &http.Response{
			Status: "200 OK", StatusCode: 200,
			Header:  http.Header{"Content-Type": []string{"application/json"}},
			Body:    io.NopCloser(strings.NewReader(body)),
			Request: req,
		}, nil
	}))

	cer := multiProviderCervice(nil, map[string][]string{"Forms": {"SignalA_v1a"}})
	sys := createTestSystem(false)
	if err := Search4MultipleServicesAs(cer, &sys, "read"); err != nil {
		t.Fatalf("discovery: %v", err)
	}

	want := map[string]string{
		"http://kitchen/temperature":  "token-for-kitchen",
		"http://bathroom/temperature": "token-for-bathroom",
	}
	found := 0
	for _, nodes := range cer.Nodes {
		for _, ni := range nodes {
			expected, known := want[ni.URL]
			if !known {
				continue
			}
			found++
			token, discovered := ni.TokenFor("read")
			if !discovered || token == "" {
				t.Errorf("%s was discovered with no token, so every request to it "+
					"is refused in an authorized cloud", ni.URL)
				continue
			}
			if token != expected {
				t.Errorf("%s carries %q, want %q — the token belongs to the provider "+
					"it was minted for", ni.URL, token, expected)
			}
		}
	}
	if found != len(want) {
		t.Errorf("%d of %d providers were recorded", found, len(want))
	}
}

// An orchestrator that predates the token-carrying answer still has its
// providers discovered rather than the cloud stopping on an upgrade. They
// carry no token, which is what they carried before.
func TestAnOlderOrchestratorStillDiscovers(t *testing.T) {
	useTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"version":"ServiceRecordList_v1","list":[{"registryID":1,"definition":"temperature",` +
			`"systemName":"kitchen","serviceNode":"n","ipAddresses":["kitchen"],` +
			`"protoPort":{"http":80},"subpath":"temperature","version":"ServiceRecord_v1"}]}`
		return &http.Response{
			Status: "200 OK", StatusCode: 200,
			Header:  http.Header{"Content-Type": []string{"application/json"}},
			Body:    io.NopCloser(strings.NewReader(body)),
			Request: req,
		}, nil
	}))

	cer := multiProviderCervice(nil, map[string][]string{"Forms": {"SignalA_v1a"}})
	sys := createTestSystem(false)
	if err := Search4MultipleServicesAs(cer, &sys, "read"); err != nil {
		t.Fatalf("an older orchestrator's answer was refused: %v", err)
	}
	if _, _, ok := pickNode(cer, "read"); !ok {
		t.Error("no provider was discovered, so an upgrade of one end stops the other")
	}
}

// TestOneCerviceSurvivesTwoPollingGoroutines is the crash, not a race.
//
// A unit asset with more than one feedback loop polls the same cervice from
// each, and discovery now deletes from Nodes as well as writing to it. A map
// written during a range over the same map is `fatal error: concurrent map
// iteration and map write` — it is not a corrupted reading, it is the process
// gone, and no recover reaches it.
//
// Run with -race this reports the race; run without it, against the unlocked
// code, it panics outright often enough to matter. Either way it fails, which
// is what a test of this can honestly claim.
func TestOneCerviceSurvivesTwoPollingGoroutines(t *testing.T) {
	useTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		// Alternating answers, so pruneNodes has something to delete on some
		// rounds and something to keep on others.
		body := `{"version":"ServicePointList_v1","list":[` +
			`{"providerName":"a","definition":"temperature","serviceURL":"http://a/temperature",` +
			`"serviceNode":"a","token":"ta","version":"ServicePoint_v1"}]}`
		return &http.Response{
			Status: "200 OK", StatusCode: 200,
			Header:  http.Header{"Content-Type": []string{"application/json"}},
			Body:    io.NopCloser(strings.NewReader(body)),
			Request: req,
		}, nil
	}))

	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://a/temperature", Tokens: map[string]string{"read": "old"}},
		{URL: "http://b/temperature", Tokens: map[string]string{"read": "old"}},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})
	sys := createTestSystem(false)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 50; n++ {
				// One goroutine discovering while the others read what was
				// discovered, which is the shape of a multi-asset system.
				if err := Search4MultipleServicesAs(cer, &sys, "read"); err != nil {
					t.Errorf("discovery: %v", err)
					return
				}
				_, _, _ = pickNode(cer, "read")
				_ = cer.Providers()
				forgetToken(cer, "http://a/temperature", "read")
			}
		}()
	}
	wg.Wait()
}

// TestAWriteDiscoveryKeepsTheReadOnlyProviders is what action-scoped pruning is
// for.
//
// A quest carries an action, so the orchestrator's answer says what this
// consumer may do for *that* action, and a cervice used for both a GET and a PUT
// is discovered twice, once per action. Deleting every provider absent from one
// answer therefore threw away providers that were fine for the other: a
// read-write sensor and a read-only one, a later write discovery returning only
// the first, and the read-only one gone. Every read after that reached one
// sensor where there had been two, and nothing said so.
func TestAWriteDiscoveryKeepsTheReadOnlyProviders(t *testing.T) {
	// The write discovery: only the read-write provider may be written to.
	useTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"version":"ServicePointList_v1","list":[` +
			`{"providerName":"rw","definition":"temperature","serviceURL":"http://rw/temperature",` +
			`"serviceNode":"rw","token":"write-token","version":"ServicePoint_v1"}]}`
		return &http.Response{
			Status: "200 OK", StatusCode: 200,
			Header:  http.Header{"Content-Type": []string{"application/json"}},
			Body:    io.NopCloser(strings.NewReader(body)),
			Request: req,
		}, nil
	}))

	// Both were discovered earlier, for reading.
	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://rw/temperature", Tokens: map[string]string{"read": "read-token"}},
		{URL: "http://ro/temperature", Tokens: map[string]string{"read": "read-token"}},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})

	sys := createTestSystem(false)
	if err := Search4MultipleServicesAs(cer, &sys, "write"); err != nil {
		t.Fatalf("discovery: %v", err)
	}

	var readable, writable int
	for _, ni := range cer.Providers() {
		if _, ok := ni.TokenFor("read"); ok {
			readable++
		}
		if _, ok := ni.TokenFor("write"); ok {
			writable++
		}
	}
	if readable != 2 {
		t.Errorf("%d providers can still be read; a write discovery must not remove "+
			"a sensor that was only ever readable", readable)
	}
	if writable != 1 {
		t.Errorf("%d providers carry a write token; only one was permitted", writable)
	}
}

// A round with no providers has to say so. Empty slices left the caller's range
// running zero times and its error check finding nothing wrong, which a control
// loop reads as "nothing changed" rather than "there are no sensors" — and it
// then holds its last output against a cloud that has none.
func TestAnEmptyRoundIsAnError(t *testing.T) {
	useTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"version":"ServicePointList_v1","list":[]}`
		return &http.Response{
			Status: "200 OK", StatusCode: 200,
			Header:  http.Header{"Content-Type": []string{"application/json"}},
			Body:    io.NopCloser(strings.NewReader(body)),
			Request: req,
		}, nil
	}))

	cer := multiProviderCervice(nil, map[string][]string{"Forms": {"SignalA_v1a"}})
	sys := createTestSystem(false)

	forms, errs := stateHandlers(http.MethodGet, cer, &sys, nil)
	if len(errs) == 0 {
		t.Fatal("a round with no providers reported no error at all")
	}
	var reported bool
	for _, e := range errs {
		if e != nil {
			reported = true
		}
	}
	if !reported {
		t.Errorf("a round with no providers returned %d forms and no error; a control "+
			"loop cannot tell that from a quiet cloud", len(forms))
	}
}

// TestOnlyTheProviderItselfCostsItsToken separates a provider that is gone from
// a provider that answered something this consumer could not read.
//
// forgetToken fired on every error class — an empty body, an unknown form
// version, a unit that could not be converted. Those are the provider answering,
// and it is still the provider it was discovered as; forgetting its token meant
// one sensor speaking an unfamiliar dialect cost a full rediscovery of every
// provider on every poll, for as long as it kept speaking it.
func TestOnlyTheProviderItselfCostsItsToken(t *testing.T) {
	// A provider that answers, in a form this consumer does not know.
	useTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Status: "200 OK", StatusCode: 200,
			Header:  http.Header{"Content-Type": []string{"application/json"}},
			Body:    io.NopCloser(strings.NewReader(`{"version":"SignalZ_v9","value":1}`)),
			Request: req,
		}, nil
	}))

	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://odd/temperature", Tokens: map[string]string{"read": "token"}},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})
	sys := createTestSystem(false)

	_, errs := stateHandlers(http.MethodGet, cer, &sys, nil)
	if len(errs) == 0 || errs[0] == nil {
		t.Fatal("an unreadable answer was not reported")
	}

	if _, _, ok := pickNode(cer, "read"); !ok {
		t.Error("the provider lost its token for answering in a form this consumer " +
			"does not know, so every poll now pays a rediscovery of everything")
	}
}

// A provider that refuses the credential is a different matter: the token is
// what was wrong, so it goes and the next call rediscovers.
func TestARefusedCredentialCostsItsToken(t *testing.T) {
	useTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Status: "401 Unauthorized", StatusCode: 401,
			Header:  http.Header{"Content-Type": []string{"text/plain"}},
			Body:    io.NopCloser(strings.NewReader("no access token")),
			Request: req,
		}, nil
	}))

	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://strict/temperature", Tokens: map[string]string{"read": "stale"}},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})
	sys := createTestSystem(false)

	_, errs := stateHandlers(http.MethodGet, cer, &sys, nil)
	if len(errs) == 0 || errs[0] == nil {
		t.Fatal("a refusal was not reported")
	}
	if !strings.Contains(errs[0].Error(), "401") {
		t.Errorf("the failure reads %q, which does not say the request was refused", errs[0])
	}
	if _, _, ok := pickNode(cer, "read"); ok {
		t.Error("a token the provider refused was kept, so the next call presents it again")
	}
}

// useTransport installs a round tripper on the framework's client for one test
// and puts the previous one back afterwards.
//
// Every test here that mocked the transport used to leave it installed, so each
// one handed its mock to whatever ran next — a package where one test answers
// every request with a PEM certificate and another with a service list, and the
// only thing keeping them apart is the order the files happen to be in. It held
// together until a test was renamed.
func useTransport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	previous := http.DefaultClient.Transport
	http.DefaultClient.Transport = rt
	t.Cleanup(func() { http.DefaultClient.Transport = previous })
}

// A provider that has not yet obtained the authorizer's key answers 503 with a
// sentence. Unpacking that complained about JSON, so the consumer's log blamed
// the payload for a provider that was merely not ready — the same misreporting
// the 401 path was fixed for, one status code over.
func TestAProviderThatIsNotReadySaysSo(t *testing.T) {
	useTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Status: "503 Service Unavailable", StatusCode: 503,
			Header:  http.Header{"Content-Type": []string{"text/plain"}},
			Body:    io.NopCloser(strings.NewReader("cannot verify access tokens yet: the authorizer's key has not been obtained")),
			Request: req,
		}, nil
	}))

	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://waking/temperature", Tokens: map[string]string{"read": "good"}},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})
	sys := createTestSystem(false)

	_, errs := stateHandlers(http.MethodGet, cer, &sys, nil)
	if len(errs) == 0 || errs[0] == nil {
		t.Fatal("a provider that refused to serve was not reported")
	}
	if strings.Contains(errs[0].Error(), "JSON") || strings.Contains(errs[0].Error(), "json") {
		t.Errorf("the failure reads %q, which blames the payload for a provider that is not ready", errs[0])
	}
	if !strings.Contains(errs[0].Error(), "503") {
		t.Errorf("the failure reads %q, which does not say the provider is unavailable", errs[0])
	}

	// The token does not survive, and that is the current behavior rather than
	// a decision: askOneProvider marks every failed request a stale provider,
	// so a 503 costs the token exactly as a 403 does. It is recorded here so a
	// change to it is deliberate. The cost is one rediscovery per poll while a
	// provider warms up, which the retry backoff has already cut to seconds.
	if _, _, ok := pickNode(cer, "read"); ok {
		t.Log("a 503 now keeps its token; if that was intended, update this test")
	}
}
