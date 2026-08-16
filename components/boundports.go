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

package components

import (
	"maps"
	"sync"
)

// BoundPorts records the protocols a system is actually serving, as opposed to
// the ones its configuration names.
//
// The two are not the same and the difference is not brief. An HTTPS endpoint
// binds only once the system has a certificate, which means waiting for the CA,
// which means waiting for maitreD to attest it — minutes in a cloud that is
// still starting, indefinitely in one where a core system is down. Registration
// meanwhile advertised every configured port, and a consumer is handed the
// HTTPS one in preference to the HTTP one, so it was sent to a port nothing was
// listening on while a working HTTP port sat unused beside it.
//
// Written by the goroutines that bind the servers and read by the registration
// loops, so it carries its own lock.
type BoundPorts struct {
	mu    sync.RWMutex
	ports map[string]int
	// ever records the port a protocol was last bound on and is never cleared.
	//
	// ports answers "what is being served now", which is the right question for
	// registration and the wrong one for a security decision. A system that
	// refuses plaintext while TLS is bound would start accepting it again the
	// moment the TLS server returned — a cert rotation, a stolen port, a
	// cancelled context — because Release drops the entry and the check reads
	// zero. A permission that can come back on its own is not a permission.
	ever map[string]int
}

// Bind records that a protocol is being served on a port. Call it after the
// listener is established, never before: the point of this type is to say what
// is true rather than what was asked for.
func (b *BoundPorts) Bind(protocol string, port int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ports == nil {
		b.ports = make(map[string]int, 2)
	}
	if b.ever == nil {
		b.ever = make(map[string]int, 2)
	}
	b.ports[protocol] = port
	b.ever[protocol] = port
}

// EverBound reports the port a protocol was last served on, and whether it has
// ever been served at all. Unlike Port it does not go back to zero when the
// server stops, so a decision taken on the strength of TLS having been bound
// stays taken.
func (b *BoundPorts) EverBound(protocol string) (int, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	port, ok := b.ever[protocol]
	return port, ok
}

// Release records that a protocol is no longer being served.
func (b *BoundPorts) Release(protocol string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.ports, protocol)
}

// Serving returns a copy of what is bound. Empty means nothing is listening
// yet, which is a system that has nothing to advertise rather than one to
// advertise with no port.
func (b *BoundPorts) Serving() map[string]int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return maps.Clone(b.ports)
}

// Port returns the port a protocol is being served on, or zero if it is not.
//
// Zero means "not listening", not "not configured": a system whose HTTPS port
// is set but whose certificate has not arrived reports zero here, which is the
// question worth asking before deciding that a caller could have used TLS.
func (b *BoundPorts) Port(protocol string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.ports[protocol]
}

// Any reports whether anything is being served.
func (b *BoundPorts) Any() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.ports) > 0
}
