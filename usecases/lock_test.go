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
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/sdoque/mbaigo/components"
)

// hangingTransport is a messenger that accepted the connection and never
// answers — the failure mode that matters, because it holds for the client's
// full timeout rather than failing fast.
type hangingTransport struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (t *hangingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.once.Do(func() { close(t.entered) })
	<-t.release
	return nil, context.DeadlineExceeded
}

// TestLoggingDoesNotStallTheRequestPath is the defect this test was written for:
// Log took System.Mutex and held it across a POST to every registered messenger,
// while AuthorizerKey took the same mutex on every inbound request. One
// unreachable messenger stalled every request for the client's timeout — 30
// seconds once TLS is installed — handlers piled up, and consuming control loops
// missed their period. Before token verification existed the request path never
// touched System.Mutex at all.
func TestLoggingDoesNotStallTheRequestPath(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}

	sys := components.NewSystem("provider", t.Context())
	sys.Husk = &components.Husk{
		Messengers: map[string]int{"http://messenger.invalid": 0},
		Host:       &components.HostingDevice{Name: "testhost"},
	}
	sys.Husk.AuthorizerKey.Store(&key.PublicKey)

	hung := &hangingTransport{entered: make(chan struct{}), release: make(chan struct{})}
	useTransport(t, hung)
	defer close(hung.release)

	logging := make(chan struct{})
	go func() {
		defer close(logging)
		LogInfo(&sys, "a message that has to go out to a messenger that will not answer")
	}()

	// Wait until the send is genuinely in flight, so the request below competes
	// with a lock actually held rather than racing the goroutine's start.
	select {
	case <-hung.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the log message never reached the transport")
	}

	served := make(chan struct{})
	go func() {
		defer close(served)
		if _, ok := AuthorizerKey(&sys); !ok {
			t.Error("the key is not in place")
		}
	}()

	select {
	case <-served:
	case <-time.After(2 * time.Second):
		t.Fatal("the request path is blocked behind a messenger that will not answer")
	}
}

// TestAMessengerDroppedWhileLoggingIsNotResurrected: the outcomes are applied
// after the sends, so a messenger deregistered meanwhile must stay gone rather
// than be written back from the snapshot.
func TestAMessengerDroppedWhileLoggingIsNotResurrected(t *testing.T) {
	sys := components.NewSystem("provider", t.Context())
	sys.Husk = &components.Husk{
		Messengers: map[string]int{"http://messenger.invalid": 0},
		Host:       &components.HostingDevice{Name: "testhost"},
	}

	hung := &hangingTransport{entered: make(chan struct{}), release: make(chan struct{})}
	useTransport(t, hung)

	logging := make(chan struct{})
	go func() {
		defer close(logging)
		LogInfo(&sys, "a message")
	}()

	select {
	case <-hung.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the log message never reached the transport")
	}

	// Deregistered while the send is in flight.
	sys.Mutex.Lock()
	delete(sys.Husk.Messengers, "http://messenger.invalid")
	sys.Mutex.Unlock()

	close(hung.release)
	<-logging

	sys.Mutex.Lock()
	defer sys.Mutex.Unlock()
	if _, back := sys.Husk.Messengers["http://messenger.invalid"]; back {
		t.Error("a messenger deregistered mid-send was written back from the snapshot")
	}
}
