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

// Reading the registry's list of systems, in one place rather than in each of
// the systems that wants it.

package usecases

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// SystemListPath is the sub-path the registrar lists its systems on, and the
// definition of the service that does it.
const SystemListPath = "syslist"

// SystemListCervice is the cervice a system needs in order to read the registry.
//
// Held by the caller rather than made here on each call, because it is where the
// access token lives: discovery costs an orchestration, and a consumer that
// rediscovered on every read would pay it every few seconds.
func SystemListCervice(sys *components.System) *components.Cervice {
	return &components.Cervice{
		Definition: SystemListPath,
		Protos:     components.SProtocols(sys.Husk.ProtoPort),
		Nodes:      make(map[string][]components.NodeInfo),
		Mode:       "get",
	}
}

// SystemList asks the lead registrar which systems are registered.
//
// The registrar is a core system, so where to ask comes from the configured core
// systems rather than from discovery. What discovery is for here is the token:
// syslist is a declared service like any other, and in a cloud with an
// authorizer a request without one is refused.
//
// Three systems each built this request by hand — kgrapher to assemble the
// graph, modeler for its SysML model, messenger for its dashboard — and none of
// them carried a token, so declaring syslist as a service broke all three in
// exactly the clouds the declaration was for. One implementation means one place
// for the next thing the registry asks of its readers.
func SystemList(cer *components.Cervice, sys *components.System) ([]string, error) {
	registrar, err := components.GetRunningCoreSystemURL(sys, components.ServiceRegistrarName)
	if err != nil {
		return nil, fmt.Errorf("locating the lead service registrar: %w", err)
	}

	// The system's context so a read stops when the system does, but not blindly:
	// context.WithTimeout panics on a nil parent, and a System can reach here
	// without one — a unit asset built outside the usual startup, or a test.
	// Panicking inside a read of the registry is a poor way to find that out.
	parent := sys.Ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registrar+"/"+SystemListPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if token, ok := RegistryToken(cer, sys); ok {
		req.Header.Set(TokenHeader, token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		// The body of a refusal is a sentence, not a form, so it is reported
		// rather than unpacked — Unpack on it fails with a message about JSON
		// that says nothing about the request having been refused.
		return nil, fmt.Errorf("the registrar refused to list its systems: %s: %s",
			resp.Status, strings.TrimSpace(ForLog(string(body))))
	}

	form, err := Unpack(body, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	list, ok := form.(*forms.SystemRecordList_v1)
	if !ok {
		return nil, fmt.Errorf("the registrar's system list is not a SystemRecordList_v1 (got %T)", form)
	}
	return list.List, nil
}

// RegistryToken discovers the registry service and returns the token to present
// when reading it, if the cloud issues one.
//
// Discovered on each call rather than once, because a token outlives neither the
// authorizer that minted it nor its own expiry. A cloud with no authorizer mints
// none and the read goes without one — the registrar says whether it wants one.
func RegistryToken(cer *components.Cervice, sys *components.System) (string, bool) {
	if cer == nil {
		return "", false
	}
	// Derived from the method rather than named here, so this asks for exactly
	// the action the provider will recompute from the request it receives.
	action := ActionForMethod(http.MethodGet)
	if err := Search4ServicesAs(cer, sys, action); err != nil {
		return "", false
	}
	for _, ni := range cer.Providers() {
		if token, discovered := ni.TokenFor(action); discovered && token != "" {
			return token, true
		}
	}
	return "", false
}
