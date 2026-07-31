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

// Access tokens are minted by the authorizer and checked by every provider, so
// both sides share this code. A provider verifies locally against the
// authorizer's public key: no network call happens on the request path, which is
// what lets a control loop keep its period.

package usecases

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sdoque/mbaigo/forms"
)

// TokenHeader is the HTTP header a consumer presents its token in.
const TokenHeader = "X-Arrowhead-Token"

// encoding is URL-safe and unpadded so a token survives a header, a query string
// and a log line unchanged.
var encoding = base64.RawURLEncoding

// MintToken signs a set of claims with the authorizer's private key and returns
// the wire form: the base64url-encoded claims, a dot, and the base64url-encoded
// ECDSA signature over them.
//
// The claims are signed as the exact bytes that travel, not as a re-serialisation
// of the parsed struct. Verifying a re-encoding would let two different payloads
// share one signature whenever field order or whitespace differed.
func MintToken(key *ecdsa.PrivateKey, claims forms.AccessToken_v1) (string, error) {
	if key == nil {
		return "", fmt.Errorf("minting a token: no signing key")
	}
	claims.NewForm()

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("minting a token: %w", err)
	}

	digest := sha256.Sum256(payload)
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		return "", fmt.Errorf("signing a token: %w", err)
	}

	return encoding.EncodeToString(payload) + "." + encoding.EncodeToString(signature), nil
}

// ParseToken decodes a token's claims without checking its signature.
//
// It exists for logging and diagnostics only. Nothing that decides whether to
// serve a request may use it: unverified claims are an assertion by whoever sent
// them, which is exactly what the signature is there to replace.
func ParseToken(token string) (forms.AccessToken_v1, error) {
	claims, _, _, err := splitToken(token)
	return claims, err
}

// TokenRequest is what a provider is actually being asked to do, for the token
// to be checked against.
type TokenRequest struct {
	// Subject is the CN of the verified client certificate on the connection —
	// never a name from the token or the body.
	Subject  string
	Provider string
	Asset    string
	Service  string
	Action   string
}

// VerifyToken checks a token's signature, its validity window, and that its
// claims are the request being made.
//
// The last part is the one that is easy to leave out and fatal to omit. A valid
// signature only proves the authorizer issued *some* permission; without
// comparing the claims to this request, a token minted for reading a temperature
// would open a valve.
func VerifyToken(token string, pub *ecdsa.PublicKey, req TokenRequest, now time.Time) (forms.AccessToken_v1, error) {
	if pub == nil {
		return forms.AccessToken_v1{}, fmt.Errorf("verifying a token: no authorizer public key")
	}

	claims, payload, signature, err := splitToken(token)
	if err != nil {
		return claims, err
	}

	digest := sha256.Sum256(payload)
	if !ecdsa.VerifyASN1(pub, digest[:], signature) {
		return claims, fmt.Errorf("the token's signature is not the authorizer's")
	}

	if claims.Expired(now) {
		return claims, fmt.Errorf("the token expired at %s", claims.Expires.Format(time.RFC3339))
	}

	// The subject is compared against the connection's certificate by the
	// caller supplying req.Subject, so a token cannot be replayed by another
	// system that got hold of it.
	if err := mismatch("subject", claims.Subject, req.Subject); err != nil {
		return claims, err
	}
	if err := mismatch("provider", claims.Provider, req.Provider); err != nil {
		return claims, err
	}
	if err := mismatch("asset", claims.Asset, req.Asset); err != nil {
		return claims, err
	}
	if err := mismatch("service", claims.Service, req.Service); err != nil {
		return claims, err
	}
	if err := mismatch("action", claims.Action, req.Action); err != nil {
		return claims, err
	}

	return claims, nil
}

// splitToken separates and decodes the two halves of a token.
func splitToken(token string) (claims forms.AccessToken_v1, payload, signature []byte, err error) {
	dot := strings.IndexByte(token, '.')
	if token == "" || dot < 1 || dot == len(token)-1 {
		return claims, nil, nil, fmt.Errorf("malformed token: expected claims.signature")
	}

	payload, err = encoding.DecodeString(token[:dot])
	if err != nil {
		return claims, nil, nil, fmt.Errorf("malformed token claims: %w", err)
	}
	signature, err = encoding.DecodeString(token[dot+1:])
	if err != nil {
		return claims, nil, nil, fmt.Errorf("malformed token signature: %w", err)
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, nil, nil, fmt.Errorf("unreadable token claims: %w", err)
	}
	return claims, payload, signature, nil
}

// mismatch reports a claim that does not describe the request being served. An
// empty claim never satisfies a requirement: a token that omits what it is for
// is not a token that is for everything.
func mismatch(what, claimed, actual string) error {
	if claimed == actual && claimed != "" {
		return nil
	}
	return fmt.Errorf("the token's %s is %q but the request's is %q", what, claimed, actual)
}
