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
// returns it. The default client is already hijacked by the other tests in this
// package, so replacing it here is in keeping.
func mockProviders(readings map[string]string) *recordingTransport {
	rt := &recordingTransport{readings: readings}
	http.DefaultClient.Transport = rt
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
// multi-provider path went out unauthorised, so in a cloud with an authorizer
// this whole path was refused while the single-provider path worked.
func TestGetStatesSendsEachProvidersToken(t *testing.T) {
	rt := mockProviders(map[string]string{
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

// TestGetStatesNormalisesEveryProvidersUnit is the second half of the defect:
// the unpacked form was appended as it arrived. A consumer polling a °C sensor
// and a °F sensor got 20 and 68 in one slice with nothing to say which was
// which, and averaging them gave 44 of nothing.
func TestGetStatesNormalisesEveryProvidersUnit(t *testing.T) {
	mockProviders(map[string]string{
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
	// normalisation both readings must be 20.
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
	rt := mockProviders(map[string]string{
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
	mockProviders(map[string]string{
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
// nothing answered, the list is worth discarding so the next call rediscovers.
func TestGetStatesForgetsProvidersWhenNoneAnswer(t *testing.T) {
	mockProviders(map[string]string{})

	cer := multiProviderCervice([]components.NodeInfo{
		{URL: "http://dead1/temperature", Tokens: map[string]string{"read": ""}},
		{URL: "http://dead2/temperature", Tokens: map[string]string{"read": ""}},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})

	sys := createTestSystem(false)
	if _, errs := GetStates(cer, &sys); len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %v", errs)
	}
	if len(cer.Nodes) != 0 {
		t.Error("no provider answered, but the stale list was kept")
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
	http.DefaultClient.Transport = rt

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
	http.DefaultClient.Transport = rt

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
	http.DefaultClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Status: "403 Forbidden", StatusCode: 403,
			Header:  http.Header{"Content-Type": []string{"text/plain"}},
			Body:    io.NopCloser(strings.NewReader("mismatch (action): read vs write")),
			Request: req,
		}, nil
	})

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
