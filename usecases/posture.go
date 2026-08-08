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
	"fmt"
	"strings"

	"github.com/sdoque/mbaigo/components"
)

// A local cloud runs at whatever level of protection its operator has deployed,
// and every level short of the last one is a legitimate deployment: a cloud with
// no certificate authority is how anyone starts, and making it refuse to run
// would teach nothing. What is not legitimate is a cloud whose operator believes
// it is protected when it is not.
//
// So the framework never withholds function to enforce security. It states what
// it is doing instead — once at startup, and as facts in the knowledge graph, so
// the posture of a whole cloud can be read off the graph rather than inferred
// from each system's configuration file.
const (
	// PostureOpen: no certificate authority is configured. Nothing is
	// identified, nothing is authorized, and everything is in the clear.
	PostureOpen = "open"
	// PostureEnrolling: a CA is configured but this system has no certificate
	// yet. Its HTTPS endpoint is not bound and it is reachable only in the
	// clear, if at all.
	PostureEnrolling = "enrolling"
	// PostureIdentified: this system holds a certificate from the cloud's CA, so
	// callers over TLS are named and verified. What they may do is not
	// restricted.
	PostureIdentified = "identified"
	// PostureAuthorized: as identified, and an authorizer's key is held, so
	// every incoming request must also carry a token minted for that caller,
	// that service and that action.
	PostureAuthorized = "authorized"
)

// SecurityPosture is what a system can truthfully say about how it is protected.
//
// Every field is a fact the system observes about itself, not a setting it was
// asked for: NamesAuthorizer says an authorizer is configured, VerifiesTokens
// says its key was actually obtained. A cloud where those two disagree is one
// that intends to authorize and currently cannot, which is worth seeing.
type SecurityPosture struct {
	Level string

	NamesCA         bool // a certificate authority is configured
	NamesAuthorizer bool // an authorizer is configured
	Identified      bool // this system holds a certificate issued by that CA
	CanVerifyPeers  bool // it holds the CA certificate, so it can verify callers
	VerifiesTokens  bool // it holds the authorizer's key, so it can check tokens

	OffersTLS        bool // an HTTPS port is configured
	AcceptsPlaintext bool // an HTTP port is configured, so requests need no TLS
}

// Posture reports how this system is currently protected.
func Posture(sys *components.System) SecurityPosture {
	var p SecurityPosture

	_, caErr := components.GetRunningCoreSystemURL(sys, "ca")
	p.NamesCA = caErr == nil
	_, authErr := components.GetRunningCoreSystemURL(sys, AuthorizerName)
	p.NamesAuthorizer = authErr == nil

	// The enrollment goroutine writes these while requests are being served, so
	// they are read under the system's lock. A system assembled without
	// NewSystem has no lock to take, and nothing concurrent to guard against
	// either — reporting its posture should not panic.
	if sys.Mutex != nil {
		sys.Mutex.Lock()
		defer sys.Mutex.Unlock()
	}
	p.Identified = sys.Husk.Certificate != ""
	p.CanVerifyPeers = sys.Husk.CA_cert != ""
	p.VerifiesTokens = sys.Husk.AuthorizerKey.Load() != nil

	p.OffersTLS = sys.Husk.ProtoPort["https"] != 0
	p.AcceptsPlaintext = sys.Husk.ProtoPort["http"] != 0

	switch {
	case !p.NamesCA:
		p.Level = PostureOpen
	case !p.Identified:
		p.Level = PostureEnrolling
	case p.NamesAuthorizer && p.VerifiesTokens:
		p.Level = PostureAuthorized
	default:
		p.Level = PostureIdentified
	}

	return p
}

// String renders the posture as one line for a startup log: the level, then
// whatever qualifies it. An adopter reading the terminal should not have to
// cross-reference a configuration file to learn what protection is in force.
func (p SecurityPosture) String() string {
	var notes []string

	switch p.Level {
	case PostureOpen:
		notes = append(notes, "no certificate authority configured: callers are neither identified nor restricted")
	case PostureEnrolling:
		notes = append(notes, "waiting for a certificate: the HTTPS endpoint is not bound yet")
	case PostureIdentified:
		notes = append(notes, "callers over TLS are identified")
		if p.NamesAuthorizer {
			// The 503 state: an authorizer is named and its key is not in hand,
			// so every request is refused rather than served unauthorized.
			notes = append(notes, "an authorizer is configured but its key is not held yet, so requests are refused until it is")
		} else {
			notes = append(notes, "no authorizer configured: any identified system may use any service")
		}
	case PostureAuthorized:
		notes = append(notes, "callers are identified and every request must carry a token for that service and action")
	}

	// Stated separately because it qualifies every level above open: a system
	// listening in the clear can be reached without any of the protection the
	// level describes.
	if p.AcceptsPlaintext && p.Level != PostureOpen {
		notes = append(notes, "an HTTP port is open, so this system is also reachable without TLS")
	}

	return fmt.Sprintf("security: %s — %s", p.Level, strings.Join(notes, "; "))
}
