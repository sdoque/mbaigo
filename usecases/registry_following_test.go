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
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func registryChange(system, definition, change string) string {
	return fmt.Sprintf("event: change\ndata: {\"change\":%q,\"record\":{\"systemName\":%q,"+
		"\"definition\":%q,\"version\":\"ServiceRecord_v1\"},\"version\":\"RegistryEvent_v1\"}\n\n",
		change, system, definition)
}

// counting is a follower whose rebuilds are counted, and which never schedules
// a second look within a test's lifetime unless the test asks it to.
func counting(complete bool) (*RegistryFollower, *atomic.Int32) {
	var n atomic.Int32
	f := &RegistryFollower{
		Name:        "test",
		Rebuild:     func() bool { n.Add(1); return complete },
		SettleDelay: time.Hour, IncompleteRetry: time.Hour,
	}
	return f, &n
}

// A system starting up registers its services one at a time, so an arrival is
// several events a few milliseconds apart; rebuilding on each would read the
// whole cloud several times over for one arrival. And a rebuild owed when the
// connection drops before the registry goes quiet is still owed.
func TestABurstOfRegistrationsRebuildsOnce(t *testing.T) {
	stream := "event: snapshot\ndata: {\"systemurl\":[],\"version\":\"SystemRecordList_v1\"}\n\n"
	for _, s := range []string{"setpoint", "deviation", "jitter"} {
		stream += registryChange("thermostat", s, "registered")
	}
	f, rebuilds := counting(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	read := make(chan error, 1)
	go func() {
		read <- f.read(ctx, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(stream))})
	}()
	select {
	case <-read:
	case <-time.After(5 * time.Second):
		t.Fatal("read did not return after the stream ended")
	}
	if got := rebuilds.Load(); got != 1 {
		t.Errorf("four events produced %d rebuilds; a burst owes exactly one", got)
	}
}

// The registry going quiet is what triggers the rebuild, on a connection that
// stays open as it does in a running cloud.
func TestTheCloudIsReadAgainAfterTheRegistrySettles(t *testing.T) {
	f, rebuilds := counting(true)
	f.SettleFor = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pr, pw := io.Pipe()
	defer pw.Close()
	go func() { _ = f.read(ctx, &http.Response{StatusCode: 200, Body: pr}) }()
	for _, s := range []string{"setpoint", "deviation", "jitter"} {
		if _, err := io.WriteString(pw, registryChange("thermostat", s, "registered")); err != nil {
			t.Fatal(err)
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && rebuilds.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond) // long enough for a duplicate to show
	if got := rebuilds.Load(); got != 1 {
		t.Errorf("three registrations produced %d rebuilds, want 1", got)
	}
}

// A stream that says nothing, not even a keep-alive, is a dead connection, and
// read must end so Follow reconnects. Driven by the watchdog rather than the
// context, which would pass whether or not the watchdog existed.
func TestASilentStreamIsTreatedAsDead(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	f, _ := counting(true)
	f.SilenceLimit = 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	read := make(chan error, 1)
	go func() { read <- f.read(ctx, &http.Response{StatusCode: 200, Body: pr}) }()
	select {
	case err := <-read:
		if err == nil || !strings.Contains(err.Error(), "keep-alive") {
			t.Errorf("the stream ended with %v; the watchdog should have ended it, and said so", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("read never returned on a stream that said nothing")
	}
}

// The registrar writes a keep-alive every 20 seconds. The watchdog must allow
// more than one interval, or a healthy idle stream is torn down and the whole
// cloud read again every time.
func TestTheWatchdogOutlastsAMissedKeepAlive(t *testing.T) {
	const keepAlive = 20 * time.Second
	if DefaultSilenceLimit < 2*keepAlive {
		t.Errorf("DefaultSilenceLimit is %s, less than two keep-alive intervals", DefaultSilenceLimit)
	}
}

// A pass that could not read every system is tried again, and again while it
// still cannot: the omission must not stand in a cloud that has gone quiet.
func TestAnIncompletePassIsRetriedUntilComplete(t *testing.T) {
	var n atomic.Int32
	f := &RegistryFollower{
		Name:            "test",
		Rebuild:         func() bool { return n.Add(1) >= 3 }, // incomplete twice
		IncompleteRetry: 5 * time.Millisecond,
		SettleDelay:     time.Hour,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f.rebuild(ctx, true)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && n.Load() < 3 {
		time.Sleep(time.Millisecond)
	}
	if got := n.Load(); got != 3 {
		t.Fatalf("%d passes; an incomplete pass must be retried until one is complete", got)
	}
	time.Sleep(50 * time.Millisecond)
	if got := n.Load(); got != 3 {
		t.Errorf("%d passes after the complete one; retrying should have stopped", got)
	}
	if !f.settlePending.Load() {
		t.Error("the complete pass did not arrange a later look for consumers to bind")
	}
}

// After a complete pass, one more look once consumers have bound, and only one:
// neither a second change while it is pending nor the look itself adds another.
func TestACompletePassLooksOnceMore(t *testing.T) {
	f, rebuilds := counting(true)
	f.SettleDelay = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f.rebuild(ctx, true)
	f.rebuild(ctx, true)
	time.Sleep(200 * time.Millisecond)
	if got := rebuilds.Load(); got != 3 {
		t.Errorf("%d passes; want the two asked for and exactly one later look", got)
	}
}
