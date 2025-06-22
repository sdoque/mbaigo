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
	"strings"

	"github.com/sdoque/mbaigo/components"
)

// RequestCertificate generates the system's public key and a certificate signing request to be sent to the CA
func RequestCertificate(sys *components.System) {
	// Generate ECDSA Private Key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate private key: %v\n", err)
	}
	sys.Husk.Pkey = privateKey

	dnsNames := []string{"localhost"}
	var ipAddrs []net.IP
	for _, ipStr := range sys.Host.IPAddresses {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			ipAddrs = append(ipAddrs, ip)
		}
	}
	csrTemplate := x509.CertificateRequest{
		Subject:            sys.Husk.DName,
		DNSNames:           dnsNames, // this is the SAN DNS
		IPAddresses:        ipAddrs,  // this is the SAN IPs
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, privateKey)
	if err != nil {
		log.Fatalf("Failed to create CSR: %v\n", err)
		return
	}

	// Encode the CSR to PEM format
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})

	// Send the CSR to the CA and receive the certificate in response
	response, err := sendCSR(sys, csrPEM)
	if err != nil {
		log.Printf("certification failure: %v\n", err)
		return
	}

	// Save the received certificate
	sys.Husk.Certificate = response

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
		Certificates:       []tls.Certificate{clientCert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: false,
		MinVersion:         tls.VersionTLS12,
	}
	sys.Husk.TlsConfig = tlsConfig

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

	resp, err := http.Post(url, "application/x-pem-file", bytes.NewReader(csrPEM))
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
		log.Printf("Error creating NewRequest: %v", err)
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
