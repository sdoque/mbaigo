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
	b.ports[protocol] = port
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

// Any reports whether anything is being served.
func (b *BoundPorts) Any() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.ports) > 0
}
