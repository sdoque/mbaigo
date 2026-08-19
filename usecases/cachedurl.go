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

import "sync"

// CachedURL holds a core-system URL discovered once and reused until the system
// it names stops answering.
//
// Not a component. It appears in no block definition diagram and in no ontology,
// because it is not a thing a local cloud contains — it is a way of not asking
// the same question twice, owned by the behavior that asks. It lived in the
// components package for a while for the reason such things always do: that is
// the package everything imports.
//
// It exists because the obvious way to write this is wrong. A plain string field
// looks harmless:
//
//	if t.leadingRegistrar == "" {
//	    t.leadingRegistrar, err = components.GetRunningCoreSystemURL(sys, "serviceregistrar")
//	}
//	url := t.leadingRegistrar + "/query"
//
// but net/http serves every request in its own goroutine, so on a core system's
// request path that is a data race — and not a benign one. The second read can
// see the empty string a concurrent failure handler just stored, and the request
// then goes to "/query" with no host at all.
type CachedURL struct {
	mu  sync.RWMutex
	url string
}

// Resolve returns the cached URL, calling find to discover one if none is held.
//
// find runs outside the lock. Two callers arriving together may both discover,
// which costs one redundant lookup; holding the lock across it would put every
// concurrent request behind a network round trip instead. For the registrar that
// round trip is real — GetRunningCoreSystemURL status-checks it — and a core
// system that serializes its whole request path behind one lookup is worse than
// one that occasionally looks twice.
//
// The returned string is a copy, so the caller keeps a stable value for the rest
// of its request even if another goroutine forgets it meanwhile.
func (c *CachedURL) Resolve(find func() (string, error)) (string, error) {
	c.mu.RLock()
	url := c.url
	c.mu.RUnlock()
	if url != "" {
		return url, nil
	}

	url, err := find()
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.url = url
	c.mu.Unlock()
	return url, nil
}

// Forget clears the cached URL so the next Resolve discovers again. Call it when
// the system behind the URL stops answering.
func (c *CachedURL) Forget() {
	c.mu.Lock()
	c.url = ""
	c.mu.Unlock()
}

// URL returns what is cached without discovering, empty if nothing is.
func (c *CachedURL) URL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.url
}
