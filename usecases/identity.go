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
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/sdoque/mbaigo/components"
)

// PeerCN returns the Common Name of the verified client certificate presented on
// an incoming request. The HTTPS server is configured with
// tls.RequireAndVerifyClientCert against the CA pool, so a certificate reaching
// this point has already been validated against the local cloud's chain.
//
// ok is false when the request arrived over plain HTTP (r.TLS is nil) or when no
// client certificate was presented. Callers must treat that as "unidentified",
// never as a permissive default: the requester's self-declared name, such as
// ServiceQuest_v1.RequesterName, is not an identity.
func PeerCN(r *http.Request) (cn string, ok bool) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return "", false
	}
	return r.TLS.PeerCertificates[0].Subject.CommonName, true
}

// seenPeers records the (system, peer) pairs already reported. Identified peers
// are announced once; unidentified traffic is tracked separately below.
var seenPeers sync.Map

// unidentifiedReportInterval bounds how often a system reports that it is still
// being called without a verifiable identity. A variable rather than a constant
// so tests can collapse it.
var unidentifiedReportInterval = 5 * time.Minute

var (
	unidentifiedMu   sync.Mutex
	unidentifiedLast = make(map[string]time.Time)
)

// logPeer reports who is calling.
//
// An identified peer is announced the first time a given system sees it.
// Consumers poll on control-loop periods of a few seconds, so logging every
// request would drown the journal, and first sighting is enough to establish who
// talks to whom.
//
// An unidentified caller is reported repeatedly, at most once per
// unidentifiedReportInterval. Reporting it once would be indistinguishable from
// a deployment that had since moved to mTLS, and knowing whether unidentified
// traffic is *still* arriving is the whole point of this logging. It stays noisy
// on purpose until the transport is fixed.
//
// This observes only. No request is refused on the basis of what it finds.
func logPeer(sys *components.System, r *http.Request) {
	if cn, ok := PeerCN(r); ok {
		if _, seen := seenPeers.LoadOrStore(sys.Name+"|"+cn, struct{}{}); !seen {
			log.Printf("first request to %s from peer %q\n", sys.Name, ForLog(cn))
		}
		return
	}

	transport := "plain HTTP"
	if r.TLS != nil {
		transport = "TLS without a client certificate"
	}

	unidentifiedMu.Lock()
	defer unidentifiedMu.Unlock()

	if last, seen := unidentifiedLast[sys.Name]; seen && time.Since(last) < unidentifiedReportInterval {
		return
	}
	unidentifiedLast[sys.Name] = time.Now()
	log.Printf("request to %s from an unidentified peer (%s)\n", sys.Name, transport)
}
