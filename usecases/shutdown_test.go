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
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/sdoque/mbaigo/components"
)

func TestWatchShutdownCancelsOnSignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sys := &components.System{
		Name: "test-system",
		Ctx:  ctx,
		Sigs: make(chan os.Signal, 1),
	}

	WatchShutdown(sys, cancel)

	// Simulate a SIGINT arriving on the channel.
	sys.Sigs <- syscall.SIGINT

	// Context should be cancelled within a small window.
	select {
	case <-ctx.Done():
		// success
	case <-time.After(time.Second):
		t.Fatal("context was not cancelled within 1s of signal")
	}
}

func TestWatchShutdownDoesNotCancelWithoutSignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sys := &components.System{
		Name: "test-system",
		Ctx:  ctx,
		Sigs: make(chan os.Signal, 1),
	}

	WatchShutdown(sys, cancel)

	// No signal sent. Context must remain live.
	select {
	case <-ctx.Done():
		t.Fatal("context was cancelled without a signal")
	case <-time.After(50 * time.Millisecond):
		// success — still live
	}
}
