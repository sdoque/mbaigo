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

// FileService is what a request for a stored file is called when the asset
// serving it registers no service of its own by that name.
//
// A policy names it like any other service, and an asset that does register a
// "files" service — which makes it discoverable, so a consumer can be granted a
// token for it — is judged by that record instead.
const FileService = "files"

// AuthorizerName is the core-system name a provider pins its trust to.
const AuthorizerName = "authorizer"

// OrchestratorName is the core-system name of the orchestrator, and so the
// common name its certificate carries. Named here beside AuthorizerName because
// the authorizer has to recognize it: the orchestrator is the only system that
// may ask for an authorization decision.
const OrchestratorName = "orchestrator"

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
// It is non-blocking, like certificate enrollment, because the authorizer may
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
		log.Printf("%s: no authorizer in this local cloud — incoming requests are not authorized\n", sys.Name)
		return
	}

	go func() {
		// Wait for this system's own certificate before the first attempt.
		// fetchAuthorizerKey validates the authorizer's certificate against
		// CA_cert, which installTLSConfig writes at the end of enrollment — so at
		// SetoutServers time it is essentially always empty. The first fetch
		// failed on that, the loop slept a full minute, and AuthorizeRequest
		// answered 503 to everything for the whole of it: every provider
		// unservable for up to a minute after it was otherwise up.
		select {
		case <-EnsureCertReady(sys):
		case <-sys.Ctx.Done():
			return
		}

		wait := firstRetry
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
			case <-time.After(wait):
			case <-sys.Ctx.Done():
				return
			}
			wait = nextRetry(wait)
		}
	}()
}

// How long to wait before asking the authorizer for its key again.
//
// A flat minute made the start order look like a dependency it is not. Nothing
// here requires the authorizer to be running first — a provider that starts
// ahead of it answers 503 and serves as soon as it has the key — but for up to
// a minute *after* the authorizer became available, every provider was still
// refusing. An operator watching that concludes the cloud must be started in
// order, and begins sequencing what does not need sequencing.
//
// Doubling from a second reaches the same ceiling in seven attempts and costs
// nothing when the authorizer is already up, which is the ordinary case: the
// first attempt succeeds and none of this runs.
const (
	firstRetry = time.Second
	maxRetry   = time.Minute
)

// nextRetry doubles the wait up to the ceiling.
func nextRetry(wait time.Duration) time.Duration {
	if wait <= 0 {
		return firstRetry
	}
	if doubled := wait * 2; doubled < maxRetry {
		return doubled
	}
	return maxRetry
}

// AuthorizerKey returns the key to verify with, and whether one is in place.
//
// It takes no lock. This runs on every inbound request, and System.Mutex is held
// by Log across a POST to each registered messenger: taking it here put the
// whole request path behind an unreachable messenger's 30-second timeout.
func AuthorizerKey(sys *components.System) (*ecdsa.PublicKey, bool) {
	key := sys.Husk.AuthorizerKey.Load()
	return key, key != nil
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

	// Read under the lock: the enrollment goroutine writes CA_cert from
	// installTLSConfig, and there is no ordering between that and this.
	sys.Mutex.Lock()
	caCert := sys.Husk.CA_cert
	sys.Mutex.Unlock()

	key, err := authorizerPublicKey(certPEM, caCert)
	if err != nil {
		return err
	}

	sys.Husk.AuthorizerKey.Store(key)
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
// An unrecognized method yields the empty string, which no token claim can match:
// a method nobody classified is one nobody authorized.
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

// isBootstrapService reports whether this service belongs to the plane that
// makes authorization possible, and so cannot be authorized itself.
//
// Decided on the mission rather than a list of names, because the mission is
// already the vocabulary policy is written in and a core system that gains a
// service should not have to be remembered here. A service's own mission wins
// over its asset's, which is what lets a core system offer something that is
// not core and have it gated normally.
//
// This is not a hole a rogue provider can climb through. Enforcement is already
// the provider's own choice — a system that did not want to check tokens would
// simply leave the authorizer out of its configuration — so a provider calling
// itself core weakens nothing that was not already its to weaken.
func isBootstrapService(sys *components.System, assetName string, serv *components.Service) bool {
	ua, known := sys.UAssets[assetName]
	if !known {
		return false
	}
	return components.EffectiveMission(ua, serv) == components.MissionCore
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

	// A core service cannot require a token, because a token comes from a core
	// service.
	//
	// One configuration list says both "this is the authorizer I verify against"
	// and "I demand tokens on my own services", so naming the authorizer at the
	// orchestrator made /squest demand one — and a service quest is how a
	// consumer obtains a token in the first place. The registrar deadlocks the
	// same way: registration would need a token that registration is a
	// precondition for. Between them, authorization could not be switched on in
	// any configuration at all.
	//
	// So the bootstrap plane is exempt: discovery, registration, certification
	// and attestation are what make tokens possible and therefore cannot be
	// gated by one. What protects them instead is the layer beneath — mutual TLS
	// with a certificate the CA signed only for a binary whose hash is on the
	// whitelist. That is a real boundary and POLICY.md states it: any system
	// this cloud has enrolled may call a core service without a token.
	//
	// Before the key check on purpose. A provider still fetching the
	// authorizer's key answers 503, and a core service that did that would stop
	// the cloud discovering anything for as long as the fetch took — which is
	// the start-order dependency this framework does not have and should not
	// acquire.
	if isBootstrapService(sys, assetName, serv) {
		return 0, nil
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
