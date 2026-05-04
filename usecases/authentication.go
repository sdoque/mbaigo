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

// Package "usecases" addresses system behaviors and actions in given use cases
// such as configuration, registration, authentication, orchestration, ...

package usecases

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sdoque/mbaigo/components"
)

// EnsureCertReady returns the system's CertReady channel, initialising it on
// first call. Safe for concurrent use: either RequestCertificate or
// SetoutServers may be the first to need it.
func EnsureCertReady(sys *components.System) chan struct{} {
	sys.Mutex.Lock()
	defer sys.Mutex.Unlock()
	if sys.Husk.CertReady == nil {
		sys.Husk.CertReady = make(chan struct{})
	}
	return sys.Husk.CertReady
}

// RequestCertificate kicks off TLS-certificate acquisition for the system.
// It is non-blocking: a goroutine generates a fresh ECDSA key pair in
// memory, builds a CSR, and retries enrolment with the CA until success or
// context cancellation. CertReady is closed when the cert lands on
// sys.Husk.Certificate and TLS is installed on http.DefaultClient.
//
// **Every system enrols, regardless of whether it serves HTTPS.**
// Cert acquisition is decoupled from the HTTPS server binding: the cert
// configures `http.DefaultClient` for outbound mTLS calls (so any system
// can call HTTPS-only services elsewhere in the cloud), while the HTTPS
// server is bound by SetoutServers if and only if `https != 0`. A
// pure-consumer system (e.g. an aggregator or monitor) gains a CA-signed
// identity for client-side mTLS without exposing an HTTPS endpoint of its
// own. This also means every system passes through the maitreD's
// attestation gate at startup — the security model applies uniformly,
// not just to systems that serve HTTPS.
//
// **Memory-only keys.** Application systems do not persist their private
// keys to disk. A fresh key is generated on every startup and the resulting
// cert lives only for the lifetime of the process. Consequences:
//
//   - The system's identity in the cloud is the *running binary* (whitelist
//     hash + attestation), not a long-lived on-disk credential. The cert is
//     the cryptographic instantiation of that identity, and ends with the
//     process.
//   - There is no filesystem attack surface for the key. An attacker who
//     compromises the host as the system's user cannot extract the key
//     without entering the running process's address space.
//   - Revocation by whitelist edit takes effect at next restart with no
//     stale cached cert outliving the operator's authorisation.
//   - The CA must be reachable for the system to reach a TLS-enabled state.
//     The HTTP server (per SetoutServers) starts immediately regardless,
//     so plain-HTTP services remain available during a CA outage.
//
// The CA itself is the necessary exception: it persists its root key on
// disk because the entire trust chain depends on its identity surviving
// restarts, and it bypasses RequestCertificate entirely (which would
// nonsensically attempt to enrol with itself). See systems/ca/thing.go.
func RequestCertificate(sys *components.System) {
	certReady := EnsureCertReady(sys)
	go acquireCertificate(sys, certReady)
}

// acquireCertificate generates a key pair in memory, builds a CSR, and
// retries enrolment with the CA until success or context cancellation. On
// success it installs TLS on http.DefaultClient and closes certReady so
// consumers can proceed. On context cancellation, certReady is left open:
// any consumer waiting on it should also select on sys.Ctx.Done() to avoid
// a permanent block.
func acquireCertificate(sys *components.System, certReady chan struct{}) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Printf("failed to generate private key for %s: %v", sys.Name, err)
		return
	}
	sys.Husk.Pkey = privateKey

	dnsNames := []string{"localhost"}
	var ipAddrs []net.IP
	for _, ipStr := range sys.Husk.Host.IPAddresses {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			ipAddrs = append(ipAddrs, ip)
		}
	}
	csrTemplate := x509.CertificateRequest{
		Subject:            sys.Husk.DName,
		DNSNames:           dnsNames,
		IPAddresses:        ipAddrs,
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, privateKey)
	if err != nil {
		log.Printf("failed to create CSR for %s: %v", sys.Name, err)
		return
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})

	var response string
	for {
		response, err = sendCSR(sys, csrPEM)
		if err == nil {
			break
		}
		log.Printf("certification attempt failed (%v); retrying in 1 minute\n", err)
		select {
		case <-time.After(time.Minute):
		case <-sys.Ctx.Done():
			log.Println("context cancelled, aborting certificate request")
			return
		}
	}

	sys.Husk.Certificate = response
	installTLSConfig(sys)
	close(certReady)
}

// installTLSConfig fetches the CA certificate, builds the TLS configuration from the system's
// certificate and private key, and installs it on http.DefaultClient.
func installTLSConfig(sys *components.System) {
	// Get CA's certificate
	caCert, err := getCACertificate(sys)
	if err != nil {
		log.Printf("failed to obtain CA's certificate: %v\n", err)
		return
	}
	sys.Husk.CA_cert = caCert

	// Load CA certificate
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM([]byte(caCert)); !ok {
		log.Fatalf("Failed to append CA certificate to pool\n")
	}

	// Prepare the client's certificate and key for TLS configuration
	clientCert, err := prepareClientCertificate(sys.Husk.Certificate, sys.Husk.Pkey)
	if err != nil {
		log.Fatalf("Failed to prepare client certificate: %v\n", err)
	}

	// Configure Transport Layer Security (TLS)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
	}
	sys.Husk.TlsConfig = tlsConfig

	// Install the TLS config on the default HTTP client so that all subsequent
	// outbound calls (registration, orchestration, service invocation) present
	// the client certificate when connecting over HTTPS.
	http.DefaultClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	// Output the certificate details
	fmt.Printf("System %s's parsed Certificate:\n", sys.Name)
	cert, err := x509.ParseCertificate(clientCert.Certificate[0])
	if err != nil {
		log.Printf("failed to parse certificate: %v\n", err)
		return
	}
	fmt.Printf("  Subject: %s\n", cert.Subject)
	fmt.Printf("  Issuer: %s\n", cert.Issuer)
	fmt.Printf("  Serial Number: %d\n", cert.SerialNumber)
	fmt.Printf("  Not Before: %s\n", cert.NotBefore)
	fmt.Printf("  Not After: %s\n", cert.NotAfter)
	fmt.Printf("  DNS Names: %v\n", cert.DNSNames)
	fmt.Printf("  IP Addresses: %v\n", cert.IPAddresses)
}

func sendCSR(sys *components.System, csrPEM []byte) (string, error) {
	url, err := components.GetRunningCoreSystemURL(sys, "ca") // Assuming the first core system is the CA
	if err != nil {
		return "", fmt.Errorf("failed to get CA URL: %w", err)
	}
	url += "/certify"

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(csrPEM))
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-pem-file")
	req.Header.Set("X-Process-PID", strconv.Itoa(os.Getpid()))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send CSR: %w", err)
	}
	defer resp.Body.Close()

	// Check if the request was successful
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("CA returned non-OK status: %s", resp.Status)
	}

	// Read the response body
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		log.Printf("Error while reading body: %v", err)
		return "", err
	}

	return buf.String(), nil
}

// getCACertificate gets the CA's certificate necessary for the dual server-client authentication in the TLS setup
func getCACertificate(sys *components.System) (string, error) {
	coreUAurl, err := components.GetRunningCoreSystemURL(sys, "ca") // Assuming the first core system is the CA
	if err != nil {
		return "", fmt.Errorf("failed to get CA URL: %w", err)
	}
	// Remove the "ification" suffix from the URL to get the CA's address
	url := strings.TrimSuffix(coreUAurl, "ification")

	// Make a GET request to the CA's endpoint
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request to CA: %w", err)
	}
	defer resp.Body.Close()

	// Check if the request was successful
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("CA returned non-OK status: %s", resp.Status)
	}

	// Read the response body
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		log.Printf("Error while reading body: %v", err)
		return "", err
	}

	return buf.String(), nil
}

// prepareClientCertificate is a helper function to prepare client's certificate
func prepareClientCertificate(certPEM string, privateKey *ecdsa.PrivateKey) (tls.Certificate, error) {
	// Load the certificate from PEM string
	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		return tls.Certificate{}, fmt.Errorf("failed to decode PEM block containing the certificate")
	}

	// Convert the private key to PEM format
	keyPEM, err := encodeECDSAPrivateKeyToPEM(privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to encode private key to PEM: %v", err)
	}

	// Create a tls.Certificate structure
	clientCert, err := tls.X509KeyPair([]byte(certPEM), keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create X509 key pair: %v", err)
	}

	return clientCert, nil
}
