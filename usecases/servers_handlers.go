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

// servers and requests handles the IP requests

package usecases

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"html"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// SetoutServers setups the http and https servers and starts them
func SetoutServers(sys *components.System) error {
	// get the servers port number (from configuration file)
	httpPort := sys.Husk.ProtoPort["http"]
	httpsPort := sys.Husk.ProtoPort["https"]

	if httpPort == 0 && httpsPort == 0 {
		return fmt.Errorf("missing http(s) port in configuration")
	}

	// how to handle requests to the servers
	http.HandleFunc("/"+sys.Name+"/", createResourceHandler(sys))

	// Obtain the key incoming access tokens are verified against. Started here
	// rather than in each system's main so that every provider enforces alike.
	AcquireAuthorizerKey(sys)

	// HTTPS bind is deferred until the certificate is ready. We start a
	// goroutine that waits on CertReady (closed by RequestCertificate when
	// the cert is in place) and then binds the TLS server. The HTTP server
	// below is unaffected — it starts immediately, so the system is
	// reachable on its plain-HTTP services even while cert acquisition is
	// still in progress.
	if httpsPort != 0 {
		certReady := EnsureCertReady(sys)
		go func() {
			select {
			case <-certReady:
				// proceed to bind HTTPS
			case <-sys.Ctx.Done():
				return
			}
			if err := startHTTPSServer(sys, httpsPort); err != nil {
				log.Printf("HTTPS server failed to start: %v", err)
			}
		}()
	}

	// if an HTTP server is required (configuration file) set it up and start it
	if httpPort != 0 {
		// Create a HTTP server
		httpServer := &http.Server{
			Addr:         ":" + strconv.Itoa(httpPort),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			Handler:      nil,
		}

		// Initiate graceful shutdown on signal reception
		go func() {
			<-sys.Ctx.Done()
			if err := httpServer.Shutdown(context.Background()); err != nil {
				log.Printf("Error during shutdown: %v", err)
			}
		}()

		// Inform the user how to access the system's web server (black box documentation)
		httpURL := "http://" + sys.Husk.Host.IPAddresses[0] + ":" + strconv.Itoa(httpPort) + "/" + sys.Name
		log.Printf("The system %s is up with its web server available at %s\n", sys.Name, httpURL)

		// Start and monitor the server
		go func() {
			err := httpServer.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				log.Fatalf("Error from web server: %v\n", err)
			}
		}()
	}

	return nil
}

// startHTTPSServer builds the TLS configuration from the system's now-ready
// certificate and binds the HTTPS server on the given port. Called by
// SetoutServers from a goroutine that waited on CertReady, so the cert is
// guaranteed to be in place by the time we reach here.
func startHTTPSServer(sys *components.System, httpsPort int) error {
	privateKeyPEM, err := encodeECDSAPrivateKeyToPEM(sys.Husk.Pkey)
	if err != nil {
		return fmt.Errorf("encoding private key: %w", err)
	}

	cert, err := tls.X509KeyPair([]byte(sys.Husk.Certificate), privateKeyPEM)
	if err != nil {
		return fmt.Errorf("parsing certificate/private key: %w", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM([]byte(sys.Husk.CA_cert))

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS12,
	}

	httpsServer := &http.Server{
		Addr:         ":" + strconv.Itoa(httpsPort),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		TLSConfig:    tlsConfig,
		Handler:      nil,
	}

	// Graceful shutdown on context cancellation.
	go func() {
		<-sys.Ctx.Done()
		if err := httpsServer.Shutdown(context.Background()); err != nil {
			log.Printf("Error during HTTPS shutdown: %v", err)
		}
	}()

	httpsURL := "https://" + sys.Husk.Host.IPAddresses[0] + ":" + strconv.Itoa(httpsPort) + "/" + sys.Name
	log.Printf("The system %s is up with its web server available at %s\n", sys.Name, httpsURL)

	if err := httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTPS server: %w", err)
	}
	return nil
}

// encodeECDSAPrivateKeyToPEM translates the system's husk's private key to a PEM to configure the TLS setup
func encodeECDSAPrivateKeyToPEM(privateKey *ecdsa.PrivateKey) ([]byte, error) {
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	return privateKeyPEM, nil
}

// createResourceHandler builds up the resource handler function
func createResourceHandler(sys *components.System) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ResourceHandler(sys, w, r)
	}
}

// ResourceHandler break up the request in parts and finds out what is requested
// as in http://192.168.1.4:8700/photographer/picam/files/image_20240325-211555.jpg
// where photographer is part[1], picam is part[2](with len==3), files is part[3] (with len==4)
func ResourceHandler(sys *components.System, w http.ResponseWriter, r *http.Request) {
	logPeer(sys, r)

	parts := strings.Split(r.URL.Path, "/")

	if len(parts) < 3 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	assetName := parts[2]
	servicePath := ""
	if len(parts) > 3 {
		servicePath = parts[3]
	}
	record := ""
	if len(parts) > 4 {
		record = parts[4]
	}

	switch len(parts) {
	case 3:
		handleThreeParts(w, r, parts[2], sys)
	case 4:
		handleFourParts(w, r, assetName, servicePath, sys)
	case 5:
		handleFiveParts(w, r, assetName, servicePath, record, sys)
	default:
		http.Error(w, "Invalid request", http.StatusBadRequest)
	}
}

// handleThreeParts handles a request with three parts
func handleThreeParts(w http.ResponseWriter, r *http.Request, part string, sys *components.System) {
	switch part {
	case "":
		http.Redirect(w, r, "/"+sys.Name+"/doc", http.StatusFound)
	case "doc":
		SysHateoas(w, r, *sys)
	case "kgraph":
		KGraphing(w, r, sys)
	case "smodel":
		SModeling(w, r, sys)
	case "cert":
		forms.Certificate(w, r, *sys)
	case "msg":
		RegisterMessenger(w, r, sys)
	default:
		http.Error(w, "Invalid request", http.StatusBadRequest)
	}
}

// handleFourParts handles a request with four parts
func handleFourParts(w http.ResponseWriter, r *http.Request, resourceName, servicePath string, sys *components.System) {
	Resource, ok := sys.UAssets[resourceName]
	if !ok {
		http.Error(w, "Resource not found", http.StatusNotFound)
		return
	}

	switch servicePath {
	case "doc":
		ResHateoas(w, r, *Resource, *sys)
		return

	default:
		uAsset := *Resource
		if !permitted(sys, w, r, resourceName, uAsset.GetServices(), servicePath) {
			return
		}
		uAsset.Serving(w, r, servicePath)
	}
}

// handleFiveParts handles a request with five parts
func handleFiveParts(w http.ResponseWriter, r *http.Request, resourceName, servicePath, record string, sys *components.System) {
	Resource, ok := sys.UAssets[resourceName]
	if !ok {
		http.Error(w, "Resource not found", http.StatusNotFound)
		return
	}

	uAsset := *Resource
	if servicePath == "files" {
		forms.TransferFile(w, r)
		// return
	}

	switch record {
	case "doc":
		service := findServiceByPath(uAsset.GetServices(), servicePath)
		if service != nil {
			ServiceHateoas(w, r, *service, *sys)
		} else {
			http.Error(w, "Service not found", http.StatusNotFound)
		}
	case "subs", "cansel":
		fmt.Fprintf(w, "Service %s has no subscription available", html.EscapeString(servicePath))
	case "cost":
		service := findServiceByDefinition(uAsset.GetServices(), servicePath)
		if service != nil {
			ACServices(w, r, &uAsset, servicePath)
		} else {
			http.Error(w, "Service not found", http.StatusNotFound)
		}
	case "cfootprint":
		service := findServiceByDefinition(uAsset.GetServices(), servicePath)
		if service != nil {
			FCServices(w, r, &uAsset, servicePath)
		} else {
			http.Error(w, "Service not found", http.StatusNotFound)
		}
	default:
		if !permitted(sys, w, r, resourceName, uAsset.GetServices(), servicePath) {
			return
		}
		uAsset.Serving(w, r, servicePath)
	}
}

// permitted refuses a request the authorizer has not sanctioned, writing the
// refusal itself and reporting whether serving may continue.
//
// It guards service dispatch only. The system-level endpoints — /doc, /kgraph,
// /smodel and above all /cert — stay open: a provider fetches the authorizer's
// own certificate through /cert, so requiring a token to read one would leave the
// cloud unable to bootstrap verification at all.
func permitted(sys *components.System, w http.ResponseWriter, r *http.Request, assetName string, services map[string]*components.Service, servicePath string) bool {
	serv := findServiceByPath(services, servicePath)
	if serv == nil {
		// Nothing to classify the request as. Harmless where no authorizer is
		// configured, and refused where one is.
		serv = &components.Service{}
	}

	status, err := AuthorizeRequest(sys, r, assetName, serv)
	if status == 0 {
		return true
	}
	log.Printf("%s: refusing %s %s: %v\n", sys.Name, r.Method, r.URL.Path, err)
	http.Error(w, err.Error(), status)
	return false
}

// findServiceByPath returns a service's pointer based on it sub-path
func findServiceByPath(services map[string]*components.Service, path string) *components.Service {
	for sPath, service := range services {
		if sPath == path {
			return service
		}
	}
	return nil
}

// findServiceByDefinition returns a service's pointer based on its definition
func findServiceByDefinition(services map[string]*components.Service, definition string) *components.Service {
	for _, service := range services {
		if service.Definition == definition {
			return service
		}
	}
	return nil
}
