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
 ***************************************************************************SDG*/

package usecases

import (
	"sync"
	"testing"

	"github.com/sdoque/mbaigo/components"
	"time"
)

// EnsureCertReady is the gate between RequestCertificate (the producer that
// closes the channel once a cert is in place) and SetoutServers (the consumer
// that waits before binding HTTPS). The same channel must be returned to all
// callers regardless of who arrives first or how many goroutines call
// concurrently.

func TestEnsureCertReadyIdempotent(t *testing.T) {
	sys := &components.System{
		Husk:  &components.Husk{},
		Mutex: &sync.Mutex{},
	}

	first := EnsureCertReady(sys)
	if first == nil {
		t.Fatal("EnsureCertReady returned nil on first call")
	}

	second := EnsureCertReady(sys)
	if second != first {
		t.Errorf("EnsureCertReady returned a different channel on second call: %p vs %p", second, first)
	}
}

func TestEnsureCertReadyConcurrent(t *testing.T) {
	sys := &components.System{
		Husk:  &components.Husk{},
		Mutex: &sync.Mutex{},
	}

	const goroutines = 50
	results := make(chan chan struct{}, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- EnsureCertReady(sys)
		}()
	}
	wg.Wait()
	close(results)

	var first chan struct{}
	for ch := range results {
		if first == nil {
			first = ch
			continue
		}
		if ch != first {
			t.Fatal("concurrent calls returned different channels")
		}
	}
}

// CertReady's value is significant: a closed channel is the signal that the
// cert is ready. Reading from a closed channel returns immediately.
func TestEnsureCertReadyClosesOnSignal(t *testing.T) {
	sys := &components.System{
		Husk:  &components.Husk{},
		Mutex: &sync.Mutex{},
	}

	ch := EnsureCertReady(sys)

	// Channel must be open initially.
	select {
	case <-ch:
		t.Fatal("CertReady channel was closed without anyone closing it")
	default:
	}

	// Closing the channel must be observable through subsequent EnsureCertReady calls.
	close(ch)
	again := EnsureCertReady(sys)
	select {
	case <-again:
		// success
	default:
		t.Fatal("EnsureCertReady returned a channel that did not reflect the close")
	}
}

// The backoff exists because a cold start's first failure is nearly always
// brief — the CA is a second from listening, or this host's maitreD is a second
// from having the certificate it needs before it can sign.
func TestCertRetryBacksOffFromASecond(t *testing.T) {
	if firstCertRetry != time.Second {
		t.Errorf("first retry is %s; a cold start pays this before its first success", firstCertRetry)
	}
	d := firstCertRetry
	steps := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	for _, want := range steps {
		d = nextCertRetry(d)
		if d != want {
			t.Fatalf("backoff reached %s, want %s", d, want)
		}
	}
	// It must settle rather than grow without bound: a CA that is genuinely
	// absent should be asked once a minute, not once an hour.
	for i := 0; i < 20; i++ {
		d = nextCertRetry(d)
	}
	if d != maxCertRetry {
		t.Errorf("backoff settled at %s, want %s", d, maxCertRetry)
	}
}

// Sixteen systems each waiting a flat minute per unmet dependency is what made
// a simultaneous cold start take two minutes. Four tries now cover the first
// fifteen seconds, which is longer than any dependency in a cold start took.
func TestBackoffCoversAColdStartQuickly(t *testing.T) {
	total, d := time.Duration(0), firstCertRetry
	tries := 0
	for total < 15*time.Second {
		total += d
		d = nextCertRetry(d)
		tries++
	}
	if tries > 5 {
		t.Errorf("%d attempts to cover 15 s; the old flat minute needed 1 and took 60 s", tries)
	}
}
