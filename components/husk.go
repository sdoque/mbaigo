/*******************************************************************************
 * Copyright (c) 2025 Synecdoque
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

// Package "components" addresses the structures of the components that
// are aggregated to form Arrowhead compliant systems in a local cloud.
// An Arrowhead local cloud is a system of systems, which are made up of a husk
// (a.k.a. a shell) and a unit-asset (a.k.a. an asset or a thing). The husk runs on a device,
// and exposes the unit assets' functionalities as services.

package components

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509/pkix"
	"sync/atomic"
)

// An Arrowhead husk enwraps the "thing" and has specific properties
type Husk struct {
	Description   string              `json:"-"`
	Host          *HostingDevice      // the system runs on a device
	CoreS         []*CoreSystem       // the system is part of a local cloud with mandatory core systems
	RegistrarChan chan *CoreSystem    // channel for the lead service registrar
	Pkey          *ecdsa.PrivateKey   `json:"-"`
	Certificate   string              `json:"-"`
	CA_cert       string              `json:"-"`
	TlsConfig     *tls.Config         `json:"-"` // client side mutual TLS configuration
	DName         pkix.Name           `json:"-"`
	Details       map[string][]string `json:"details"`
	ProtoPort     map[string]int      `json:"protoPort"`
	InfoLink      string              `json:"onlineDocumentation"`
	Messengers    map[string]int      `json:"-"` // list of messenger systems

	// CertReady is closed once a valid certificate is in Certificate and TLS
	// has been configured on http.DefaultClient. Lazily initialized by
	// usecases.RequestCertificate / SetoutServers so existing system mains
	// do not need to construct it; consumers waiting for the cert (e.g. the
	// HTTPS server bind) read from it via select with sys.Ctx.Done().
	CertReady chan struct{} `json:"-"`

	// AuthorizerKey is the public key a provider verifies access tokens with,
	// taken from the authorizer's certificate and validated against CA_cert.
	//
	// It is re-acquired rather than held for the process's life because
	// application systems keep their private keys in memory only: the authorizer
	// generates a fresh key at every startup, so a provider that cached one
	// forever would reject every token minted after the authorizer restarted.
	//
	// Atomic rather than guarded by System.Mutex, because it is read on every
	// inbound request and Log takes that same mutex while POSTing to each
	// registered messenger. One unreachable messenger would otherwise stall
	// every request behind a 30-second HTTP timeout — before token verification
	// existed, the request path never touched System.Mutex at all.
	AuthorizerKey atomic.Pointer[ecdsa.PublicKey] `json:"-"`

	// Bound is what this system is actually serving, which is not what
	// ProtoPort names: an HTTPS endpoint binds only after enrollment. Services
	// are registered with what is bound, so a consumer is never handed a port
	// nothing is listening on.
	Bound BoundPorts `json:"-"`

	// AuthorizerReady is closed once AuthorizerKey holds a chain-validated key.
	// Until then a provider refuses token-bearing requests rather than guessing.
	AuthorizerReady chan struct{} `json:"-"`
}

// SProtocols returns a slice of supported protocols (i.e., those not configured with 0)
func SProtocols(protoPort map[string]int) []string {
	var protocols []string
	for protocol, port := range protoPort {
		if port != 0 {
			protocols = append(protocols, protocol)
		}
	}
	return protocols
}
