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

// The authorization forms carry one exchange: the orchestrator asks which of the
// providers the registrar offered a given consumer may use, and the authorizer
// answers with the survivors. The schema is specified in
// systems/authorizer/POLICY.md.

package forms

import (
	"reflect"
	"time"
)

// AuthorizationQuest_v1 asks which of a set of candidate providers a subject may
// use for one action.
//
// The whole candidate list travels in a single question rather than one request
// per candidate: the authorizer resolves the subject's attributes once, and the
// orchestrator waits on one round trip instead of one per provider the registrar
// happened to return.
type AuthorizationQuest_v1 struct {
	// RequesterName is the core system asking — normally the orchestrator. It is
	// for logging and has no bearing on the decision.
	RequesterName string `json:"requesterName"`

	// Subject is the Common Name of the *consumer's* verified client
	// certificate, established by the orchestrator from the TLS connection that
	// carried the service quest. It is never a name any party wrote into a form:
	// a self-asserted subject would make every policy decorative.
	Subject string `json:"subject"`

	// Action is what the subject intends: read, write or invoke.
	Action string `json:"action"`

	// Candidates are the registration records the registrar returned, verbatim.
	// They carry the mission and the details the decision is made on, so the
	// authorizer does not have to query the registrar again for them.
	Candidates []ServiceRecord_v1 `json:"candidates"`

	Version string `json:"version"`
}

// NewForm creates a new form of type AuthorizationQuest
func (f *AuthorizationQuest_v1) NewForm() Form {
	f.Version = "AuthorizationQuest_v1"
	return f
}

// FormVersion returns the version of the form
func (f *AuthorizationQuest_v1) FormVersion() string {
	return f.Version
}

func init() {
	FormTypeMap["AuthorizationQuest_v1"] = reflect.TypeOf(AuthorizationQuest_v1{})
}

// AuthorizationGrant_v1 is one candidate the subject may use, with the token
// that proves it to the provider.
type AuthorizationGrant_v1 struct {
	// Record is the candidate, unchanged, so the orchestrator can build a service
	// point from it without consulting the registrar again.
	Record ServiceRecord_v1 `json:"record"`

	// Token is what the consumer presents to the provider, which verifies it
	// locally against the authorizer's public key. Empty while filtering is in
	// place but enforcement is not: an empty token means "permitted, unproven".
	Token string `json:"token,omitempty"`

	// TTL is how long the token stays valid, as a duration string ("5m"). It
	// bounds revocation latency: withdrawing a permission takes effect only as
	// tokens expire.
	TTL string `json:"ttl"`

	// Reason records which policy permitted this, for the audit trail. An
	// allowance is as worth recording as a refusal.
	Reason string `json:"reason"`
}

// AuthorizationRefusal_v1 records a candidate that was excluded, and why.
//
// Refusals are returned rather than silently dropped so an operator can tell
// "no such service exists" from "you may not use the one that does". Without
// them a consumer whose policy is wrong looks exactly like a consumer whose
// provider is down, which is a miserable thing to debug in a plant.
type AuthorizationRefusal_v1 struct {
	ProviderName string `json:"providerName"`
	ServiceNode  string `json:"serviceNode"`
	Reason       string `json:"reason"`
}

// AuthorizationGrantList_v1 is the authorizer's answer.
//
// An empty Grants list is a valid and complete answer: it means the subject may
// use none of the candidates. It is not an error, and the orchestrator must not
// treat it as one — deny by default is the design, not a failure.
type AuthorizationGrantList_v1 struct {
	Grants   []AuthorizationGrant_v1   `json:"grants"`
	Refusals []AuthorizationRefusal_v1 `json:"refusals,omitempty"`
	Version  string                    `json:"version"`
}

// NewForm creates a new form of type AuthorizationGrantList
func (f *AuthorizationGrantList_v1) NewForm() Form {
	f.Version = "AuthorizationGrantList_v1"
	return f
}

// FormVersion returns the version of the form
func (f *AuthorizationGrantList_v1) FormVersion() string {
	return f.Version
}

func init() {
	FormTypeMap["AuthorizationGrantList_v1"] = reflect.TypeOf(AuthorizationGrantList_v1{})
}

// Lifetime parses a grant's TTL. A grant whose TTL is missing or unparsable is
// treated as already expired rather than as long-lived: a token nobody can date
// is one nobody should trust.
func (g AuthorizationGrant_v1) Lifetime() time.Duration {
	if g.TTL == "" {
		return 0
	}
	d, err := time.ParseDuration(g.TTL)
	if err != nil || d < 0 {
		return 0
	}
	return d
}
