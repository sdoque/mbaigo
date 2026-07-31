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

package forms

import (
	"reflect"
	"time"
)

// AccessToken_v1 is what the authorizer signs and a provider checks.
//
// It is deliberately narrow. Every claim names one specific thing the bearer may
// do — this subject, to this provider's asset, through this service, with this
// action — so a token stolen from one consumer authorises nothing else. A token
// that said only "the thermostat is allowed" would be a password.
//
// The signature travels outside the payload, in the encoded form produced by
// usecases.MintToken; this struct is only the claims.
type AccessToken_v1 struct {
	// Subject is the Common Name of the consumer the token was issued to. A
	// provider must check it against the certificate on the connection, or the
	// token becomes a bearer credential anyone can replay.
	Subject string `json:"sub"`

	// Provider, Asset and Service name what may be reached. They are checked
	// against the request being served, so a token for a temperature reading
	// cannot be presented to a valve.
	Provider string `json:"provider"`
	Asset    string `json:"asset"`
	Service  string `json:"service"`

	// Action is read, write or invoke.
	Action string `json:"action"`

	// IssuedAt and Expires bound the token's life. Expiry is the only revocation
	// mechanism: a permission withdrawn from policies.json takes effect as
	// outstanding tokens lapse, not at the moment of the edit.
	IssuedAt time.Time `json:"iat"`
	Expires  time.Time `json:"exp"`

	// Issuer is the authorizer that signed it, for the audit trail and for
	// choosing a verification key when a cloud has more than one.
	Issuer string `json:"iss"`

	Version string `json:"version"`
}

// NewForm creates a new form of type AccessToken
func (f *AccessToken_v1) NewForm() Form {
	f.Version = "AccessToken_v1"
	return f
}

// FormVersion returns the version of the form
func (f *AccessToken_v1) FormVersion() string {
	return f.Version
}

func init() {
	FormTypeMap["AccessToken_v1"] = reflect.TypeOf(AccessToken_v1{})
}

// Expired reports whether the token is outside its validity window at the given
// instant.
//
// A token with no expiry is treated as expired rather than as eternal: an
// unbounded token is the one failure this design cannot recover from, so the
// missing-field case has to fail the safe way.
func (f AccessToken_v1) Expired(now time.Time) bool {
	if f.Expires.IsZero() {
		return true
	}
	if now.After(f.Expires) {
		return true
	}
	// A token issued in the future is not yet valid; clocks across a plant do
	// drift, so a small allowance keeps that from refusing honest requests.
	const clockSkew = 30 * time.Second
	return !f.IssuedAt.IsZero() && now.Add(clockSkew).Before(f.IssuedAt)
}
