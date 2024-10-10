/*******************************************************************************
 * Copyright (c) 2024 Synecdoque
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
)

// An Arrowhead husk enwraps the "thing" and has specific properties
type Husk struct {
	Description string              `json:"-"`
	Pkey        *ecdsa.PrivateKey   `json:"-"`
	Certificate string              `json:"-"`
	CA_cert     string              `json:"-"`
	TlsConfig   *tls.Config         `json:"-"` // client side mutual TLS configuration
	DName       pkix.Name           `json:"distinguishedName"`
	Details     map[string][]string `json:"details"`
	ProtoPort   map[string]int      `json:"protoPort"`
	InfoLink    string              `json:"onlineDocumentation"`
	Messenger   string              `json:"-"`
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
