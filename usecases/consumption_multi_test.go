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
		{URL: "http://north/temperature", Token: "token-north"},
		{URL: "http://south/temperature", Token: "token-south"},
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
		{URL: "http://celsius/temperature"},
		{URL: "http://fahrenheit/temperature"},
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
		{URL: "http://first/temperature"},
		{URL: "http://second/temperature"},
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
		{URL: "http://alive/temperature"},
		{URL: "http://dead/temperature"},
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
		{URL: "http://dead1/temperature"},
		{URL: "http://dead2/temperature"},
	}, map[string][]string{"Forms": {"SignalA_v1a"}})

	sys := createTestSystem(false)
	if _, errs := GetStates(cer, &sys); len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %v", errs)
	}
	if len(cer.Nodes) != 0 {
		t.Error("no provider answered, but the stale list was kept")
	}
}
