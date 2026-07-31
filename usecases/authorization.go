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

// A provider verifies access tokens against the authorizer's public key, which
// it obtains from the authorizer's own certificate and validates against the CA
// certificate it already trusts. Nothing here runs on the request path: the key
// is acquired at startup and refreshed only when the authorizer's key changes.

package usecases

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sdoque/mbaigo/components"
)

// AuthorizerName is the core-system name a provider pins its trust to.
const AuthorizerName = "authorizer"

// reacquireInterval bounds how often a signature failure may trigger a fresh
// fetch of the authorizer's certificate, so a flood of bad tokens cannot be
// turned into a flood of requests to the authorizer.
const reacquireInterval = 30 * time.Second

var lastReacquire struct {
	sync.Mutex
	at time.Time
}

// EnsureAuthorizerReady returns the channel closed once the authorizer's public
// key is in place, creating it if this is the first caller.
func EnsureAuthorizerReady(sys *components.System) chan struct{} {
	sys.Mutex.Lock()
	defer sys.Mutex.Unlock()
	if sys.Husk.AuthorizerReady == nil {
		sys.Husk.AuthorizerReady = make(chan struct{})
	}
	return sys.Husk.AuthorizerReady
}

// AcquireAuthorizerKey obtains the authorizer's public key at startup, retrying
// until it succeeds or the system shuts down.
//
// It is non-blocking, like certificate enrolment, because the authorizer may
// still be starting: a provider comes up, says plainly that it cannot verify
// tokens yet, and refuses token-bearing requests until it can. That is a
// different thing from serving them unverified.
//
// A cloud that declares no authorizer acquires nothing and verifies nothing:
// authorization is adopted per deployment, and a system that predates it keeps
// working. The absence is logged so it cannot pass for a working control.
func AcquireAuthorizerKey(sys *components.System) {
	ready := EnsureAuthorizerReady(sys)

	if _, err := components.GetRunningCoreSystemURL(sys, AuthorizerName); err != nil {
		log.Printf("%s: no authorizer in this local cloud — incoming requests are not authorised\n", sys.Name)
		return
	}

	go func() {
		for {
			if err := fetchAuthorizerKey(sys); err != nil {
				log.Printf("%s: cannot verify tokens yet: %v\n", sys.Name, err)
			} else {
				sys.Mutex.Lock()
				select {
				case <-ready: // already closed by an earlier acquisition
				default:
					close(ready)
				}
				sys.Mutex.Unlock()
				log.Printf("%s: ready to verify access tokens\n", sys.Name)
				return
			}

			select {
			case <-time.After(time.Minute):
			case <-sys.Ctx.Done():
				return
			}
		}
	}()
}

// AuthorizerKey returns the key to verify with, and whether one is in place.
func AuthorizerKey(sys *components.System) (*ecdsa.PublicKey, bool) {
	sys.Mutex.Lock()
	defer sys.Mutex.Unlock()
	return sys.Husk.AuthorizerKey, sys.Husk.AuthorizerKey != nil
}

// ReacquireAuthorizerKey refreshes the key after a signature failure.
//
// The authorizer generates a fresh key whenever it restarts, so a signature that
// does not verify is as likely to mean "the authorizer restarted" as "this token
// is forged". Refusing the request either way is correct; refusing every
// subsequent one because the key went stale is not. Rate-limited so bad tokens
// cannot be amplified into traffic.
func ReacquireAuthorizerKey(sys *components.System) bool {
	lastReacquire.Lock()
	if time.Since(lastReacquire.at) < reacquireInterval {
		lastReacquire.Unlock()
		return false
	}
	lastReacquire.at = time.Now()
	lastReacquire.Unlock()

	if err := fetchAuthorizerKey(sys); err != nil {
		log.Printf("%s: could not refresh the authorizer's key: %v\n", sys.Name, err)
		return false
	}
	log.Printf("%s: refreshed the authorizer's key\n", sys.Name)
	return true
}

// fetchAuthorizerKey retrieves the authorizer's certificate, validates it, and
// stores its public key.
func fetchAuthorizerKey(sys *components.System) error {
	coreURL, err := components.GetRunningCoreSystemURL(sys, AuthorizerName)
	if err != nil {
		return err
	}

	certURL, err := systemRootURL(coreURL, AuthorizerName)
	if err != nil {
		return err
	}

	resp, err := http.Get(certURL) // #nosec G107 -- the URL comes from the system's own configuration
	if err != nil {
		return fmt.Errorf("fetching %s: %w", certURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fetching %s: %s", certURL, resp.Status)
	}
	certPEM, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading the authorizer's certificate: %w", err)
	}

	key, err := authorizerPublicKey(certPEM, sys.Husk.CA_cert)
	if err != nil {
		return err
	}

	sys.Mutex.Lock()
	sys.Husk.AuthorizerKey = key
	sys.Mutex.Unlock()
	return nil
}

// authorizerPublicKey validates an authorizer certificate and extracts its key.
//
// The certificate is checked against the CA the system already trusts, which is
// what makes fetching it over plain HTTP safe: nobody can forge a CA-signed
// certificate. The Common Name is pinned to the authorizer, so a certificate
// belonging to some other enrolled system — every one of which is CA-signed —
// cannot stand in for it.
func authorizerPublicKey(certPEM []byte, caPEM string) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("the authorizer's certificate is not PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing the authorizer's certificate: %w", err)
	}

	if caPEM == "" {
		return nil, fmt.Errorf("no CA certificate to validate the authorizer against")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(caPEM)) {
		return nil, fmt.Errorf("the CA certificate could not be parsed")
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err != nil {
		return nil, fmt.Errorf("the authorizer's certificate does not chain to this cloud's CA: %w", err)
	}

	if cert.Subject.CommonName != AuthorizerName {
		return nil, fmt.Errorf("the certificate served by the authorizer belongs to %q", cert.Subject.CommonName)
	}

	key, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("the authorizer's key is %T, not ECDSA", cert.PublicKey)
	}
	return key, nil
}

// systemRootURL turns a configured core-system URL, which addresses a unit asset,
// into the system-level certificate endpoint.
//
// "http://host:20104/authorizer/authorization" becomes
// "http://host:20104/authorizer/cert": the configuration points at the asset that
// serves orchestration, while the certificate is served by the system itself.
func systemRootURL(coreURL, systemName string) (string, error) {
	parsed, err := url.Parse(coreURL)
	if err != nil {
		return "", fmt.Errorf("parsing the authorizer's URL %q: %w", coreURL, err)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("the authorizer's URL %q names no host", coreURL)
	}
	if !strings.Contains(parsed.Path, "/"+systemName) {
		return "", fmt.Errorf("the authorizer's URL %q does not address the %s system", coreURL, systemName)
	}
	return parsed.Scheme + "://" + parsed.Host + "/" + systemName + "/cert", nil
}

// ActionForMethod maps an HTTP method to the action a policy reasons about,
// mirroring the table in POLICY.md.
//
// An unrecognised method yields the empty string, which no token claim can match:
// a method nobody classified is one nobody authorised.
func ActionForMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead:
		return "read"
	case http.MethodPut, http.MethodPatch:
		return "write"
	case http.MethodPost:
		return "invoke"
	default:
		return ""
	}
}

// AuthorizeRequest decides whether a provider may serve one incoming request.
//
// It runs at dispatch, after the unit asset and service are known, and returns
// the HTTP status to refuse with — 0 when the request may proceed.
//
// A cloud that declares no authorizer is not enforced: authorization is adopted
// per deployment, and a system that predates it keeps serving. Once an authorizer
// is declared, a provider that cannot yet verify refuses with 503 rather than
// serving unverified requests — "not ready" is not "allowed".
func AuthorizeRequest(sys *components.System, r *http.Request, assetName string, serv *components.Service) (int, error) {
	if _, err := components.GetRunningCoreSystemURL(sys, AuthorizerName); err != nil {
		return 0, nil // no authorizer in this local cloud
	}

	key, ready := AuthorizerKey(sys)
	if !ready {
		return http.StatusServiceUnavailable,
			fmt.Errorf("cannot verify access tokens yet: the authorizer's key has not been obtained")
	}

	subject, ok := PeerCN(r)
	if !ok {
		return http.StatusUnauthorized,
			fmt.Errorf("the caller presented no verified certificate")
	}

	token := r.Header.Get(TokenHeader)
	if token == "" {
		return http.StatusUnauthorized, fmt.Errorf("no access token")
	}

	want := TokenRequest{
		Subject:  subject,
		Provider: sys.Name,
		Asset:    assetName,
		Service:  serv.Definition,
		Action:   ActionForMethod(r.Method),
	}

	_, err := VerifyToken(token, key, want, time.Now())
	if err == nil {
		return 0, nil
	}

	// The authorizer generates a fresh key whenever it restarts, so a signature
	// that does not verify may mean the key is stale rather than the token
	// forged. Refuse this request either way, but refresh so the next one is
	// judged against the current key.
	if strings.Contains(err.Error(), "signature") && ReacquireAuthorizerKey(sys) {
		if refreshed, ok := AuthorizerKey(sys); ok {
			if _, retryErr := VerifyToken(token, refreshed, want, time.Now()); retryErr == nil {
				return 0, nil
			}
		}
	}

	return http.StatusForbidden, err
}
