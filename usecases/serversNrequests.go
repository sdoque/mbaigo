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
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// SetoutServers setup the http and https servers and starts them
func SetoutServers(sys *components.System) (err error) {
	// get the servers port number (from configuration file)
	httpPort := sys.Husk.ProtoPort["http"]
	httpsPort := sys.Husk.ProtoPort["https"]

	if httpPort == 0 && httpsPort == 0 {
		fmt.Printf("The system %s has no web server configured\n", sys.Name)
		return
	}

	// how to handle requests to the servers
	http.HandleFunc("/"+sys.Name+"/", createResourceHandler(sys))

	// if an HTTPS server is required (configuration file) and the system as a signed certificate, set up and start an HTTPS server
	if httpsPort != 0 && sys.Husk.Certificate != "" {
		// Encode the ECDSA private key to PEM format
		privateKeyPEM, err := encodeECDSAPrivateKeyToPEM(sys.Husk.Pkey)
		if err != nil {
			log.Fatalf("Failed to encode private key: %v", err)
		}

		// Load the certificate and key
		cert, err := tls.X509KeyPair([]byte(sys.Husk.Certificate), privateKeyPEM)
		if err != nil {
			log.Fatalf("Failed to parse certificate or private key: %v", err)
		}

		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM([]byte(sys.Husk.CA_cert))

		// create a TLS config with the certificate and CA pool
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    caCertPool,
		}

		// Create a HTTPS server with the TLS config
		httpsServer := &http.Server{
			Addr:      ":" + strconv.Itoa(httpsPort),
			TLSConfig: tlsConfig,
			Handler:   nil,
		}

		// Initiate graceful shutdown on signal reception
		go func() {
			<-sys.Ctx.Done()
			time.Sleep(1 * time.Second) // this line is for the leading service registrar to deregister its own services
			fmt.Printf("Initiating graceful shutdown of the HTTPS server.\n")
			httpsServer.Shutdown(sys.Ctx)
		}()

		// Inform the user how to access the system's web server (black box documentation)
		httpsURL := "https://" + sys.Host.IPAddresses[0] + ":" + strconv.Itoa(httpsPort) + "/" + sys.Name
		fmt.Printf("The system %s is up with its web server available at %s\n", sys.Name, httpsURL)

		// Start and monitor the server
		go func() {
			err = httpsServer.ListenAndServeTLS("", "")
			if err != nil && err != http.ErrServerClosed {
				log.Fatalf("Listen: %s\n", err)
			}
		}()
	}

	// if an HTTP server is required (configuration file) set it up and start it
	if httpPort != 0 {
		// Create a HTTP server
		httpServer := &http.Server{
			Addr:    ":" + strconv.Itoa(httpPort),
			Handler: nil,
		}

		// Initiate graceful shutdown on signal reception
		go func() {
			<-sys.Ctx.Done()
			time.Sleep(1 * time.Second) // this line is for the leading service registrar to deregister its own services
			fmt.Printf("Initiating graceful shutdown of the HTTP server.\n")
			httpServer.Shutdown(sys.Ctx)
		}()

		// Inform the user how to access the system's web server (black box documentation)
		httpURL := "http://" + sys.Host.IPAddresses[0] + ":" + strconv.Itoa(httpPort) + "/" + sys.Name
		fmt.Printf("The system %s is up with its web server available at %s\n", sys.Name, httpURL)

		// Start and monitor the server
		err = httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Listen: %s\n", err)
		}
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
	parts := strings.Split(r.URL.Path, "/")

	if len(parts) < 3 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	resourceName := parts[2]
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
		handleFourParts(w, r, resourceName, servicePath, sys)
	case 5:
		handleFiveParts(w, r, resourceName, servicePath, record, sys)
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
		forms.SysHateoas(w, r, *sys)
	case "onto":
		forms.Ontology(w, r, *sys)
	case "cert":
		forms.Certificate(w, r, *sys)
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
		forms.ResHateoas(w, r, *Resource, *sys)
		return

	default:
		uAsset := *Resource
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
	}

	switch record {
	case "doc":
		service := findServiceByPath(uAsset.GetServices(), servicePath)
		if service != nil {
			forms.ServiceHateoas(w, r, *service, *sys)
		} else {
			http.Error(w, "Service not found", http.StatusNotFound)
		}
	case "subs", "cansel":
		fmt.Fprintf(w, "Service %s has no subscription available", servicePath)
	case "cost":
		service := findServiceByDefinition(uAsset.GetServices(), servicePath)
		if service != nil {
			ACServices(w, r, &uAsset, servicePath)
		} else {
			http.Error(w, "Service not found", http.StatusNotFound)
		}
	default:
		uAsset.Serving(w, r, servicePath)
	}
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
	service := services[definition]
	return service
}
