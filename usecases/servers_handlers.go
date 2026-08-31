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
	"net"
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

	// Said once, plainly, at the point the system starts serving. An adopter
	// running a cloud for the first time should learn what protection is in
	// force from the terminal rather than by reading the configuration back.
	// The same facts are in the knowledge graph, where a whole cloud's posture
	// can be read at once.
	log.Printf("%s: %s\n", sys.Name, Posture(sys))

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

		// Bind before announcing. ListenAndServe does both at once, so the
		// system reported it was up and then, on a port already in use, failed
		// immediately afterwards — two lines that contradict each other, in
		// that order.
		listener, err := net.Listen("tcp", httpServer.Addr)
		if err != nil {
			return fmt.Errorf("binding the HTTP port: %w", err)
		}
		sys.Husk.Bound.Bind("http", httpPort)

		// Inform the user how to access the system's web server (black box documentation)
		httpURL := "http://" + sys.Husk.Host.IPAddresses[0] + ":" + strconv.Itoa(httpPort) + "/" + sys.Name
		log.Printf("The system %s is up with its web server available at %s\n", sys.Name, httpURL)

		// Start and monitor the server
		go func() {
			err := httpServer.Serve(listener)
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

	// Peer certificates are verified against the cloud's CA alone — deliberately
	// not the host's trusted authorities, which the client pool includes: a
	// WebPKI certificate must never become a peer identity inside the cloud.
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM([]byte(sys.Husk.CA_cert)) {
		// Binding anyway gives an HTTPS port that refuses every peer, with a TLS
		// error at each caller and nothing here to say why. Refusing to bind
		// leaves the system on HTTP, which the rest of the cloud already knows
		// how to treat, and puts the reason in one place.
		return fmt.Errorf("the CA certificate could not be read, so no peer could be verified")
	}

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

	listener, err := net.Listen("tcp", httpsServer.Addr)
	if err != nil {
		return fmt.Errorf("binding the HTTPS port: %w", err)
	}
	// Recorded only now. Everything before this point — the certificate
	// request, the enrollment, the wait on CertReady — can take minutes, and for
	// all of it this port refuses connections. Registering it as though it were
	// serving is what sent consumers to a dead endpoint while the HTTP one
	// beside it worked.
	sys.Husk.Bound.Bind("https", httpsPort)
	defer sys.Husk.Bound.Release("https")

	httpsURL := "https://" + sys.Husk.Host.IPAddresses[0] + ":" + strconv.Itoa(httpsPort) + "/" + sys.Name
	log.Printf("The system %s is up with its web server available at %s\n", sys.Name, httpsURL)

	if err := httpsServer.ServeTLS(listener, "", ""); err != nil && err != http.ErrServerClosed {
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
		if !enrolled(w, r) {
			return
		}
		KGraphing(w, r, sys)
	case "smodel":
		if !enrolled(w, r) {
			return
		}
		SModeling(w, r, sys)
	case "cert":
		// Deliberately open, and it must stay that way. A provider fetches the
		// authorizer's certificate through this to learn the key it verifies
		// tokens with, so requiring a credential to read one would leave the
		// cloud unable to bootstrap verification at all. It is safe precisely
		// because a certificate is public and nobody can forge a CA-signed one.
		forms.Certificate(w, r, *sys)
	case "msg":
		if !enrolled(w, r) {
			return
		}
		RegisterMessenger(w, r, sys)
	default:
		http.Error(w, "Invalid request", http.StatusBadRequest)
	}
}

// enrolled reports whether the request came from a system this cloud enrolled,
// refusing it if not.
//
// This is authentication, not authorization: it asks only whether the caller
// holds a certificate this cloud's CA issued, never which system it is or what
// policy says about it. No token is required and no authorizer need exist, so a
// deployment that has not adopted authorization is affected exactly as much as
// one that has — which is correct, because what these three endpoints disclose
// does not depend on that choice.
//
// The HTTPS server is configured with tls.RequireAndVerifyClientCert, so a
// request that reaches a handler there has already presented a certificate
// checked against the CA. On the plain-HTTP server r.TLS is nil and this always
// refuses. That asymmetry is the whole mechanism.
//
// What it protects, and why these three:
//
//   - /kgraph and /smodel describe the system completely — every asset, service,
//     endpoint, unit, quantity kind and functional location. Together with the
//     registrar's syslist they let an unauthenticated host on the network draw
//     an accurate map of the plant without ever presenting a credential.
//   - /msg is not a read at all. It registers an arbitrary host to receive every
//     log message this system emits, pushed outbound to it. Left open, any host
//     on the LAN could subscribe itself on every system in the cloud and be sent
//     a continuous account of what the plant is doing.
//
// /cert stays open above, for the bootstrap reason stated there. /doc is a page
// for a person and is left open for now, which is a decision rather than an
// oversight: it discloses much of what /kgraph does, in HTML.
func enrolled(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := PeerCN(r); ok {
		return true
	}
	// The same words a caller gets for a service it may not have, so the two
	// cannot be told apart by their refusals.
	http.Error(w, "the caller presented no verified certificate", http.StatusUnauthorized)
	return false
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
		// The same resource in two representations: ask for the value and it is
		// answered once, ask for a stream and it is answered now and again
		// whenever it moves. On the service's own path rather than a /subscribe
		// beside it, so there is no second path to declare, discover and
		// authorize — following a value is the read it already is, and this
		// cloud has already been bitten once by a path the framework served
		// without declaring.
		//
		// Stream is nil on every service the framework has not prepared one for,
		// which is most of them, and calling a method on a nil interface panics
		// rather than returning false. net/http recovers that and closes the
		// connection with no status line, so the caller sees an empty reply and
		// the provider looks unreachable — while the pane behind it fills with
		// stack traces nobody is reading. It cost the ESR its subscription
		// service: kgrapher asked to follow syslist, was disconnected on every
		// attempt, never rebuilt, and so never published anything to the triple
		// store. Nothing in the cloud reported a fault, because from the outside
		// there was only a connection that closed.
		if serv := findServiceByPath(uAsset.GetServices(), servicePath); serv != nil &&
			wantsStream(r) && serv.Stream != nil && serv.Stream.Subscribable() {
			serv.Stream.ServeStream(w, r)
			return
		}
		uAsset.Serving(w, r, servicePath)
	}
}

// wantsStream reports whether the caller asked to follow the value rather than
// read it once.
func wantsStream(r *http.Request) bool {
	return r.Method == http.MethodGet &&
		strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

// handleFiveParts handles a request with five parts
func handleFiveParts(w http.ResponseWriter, r *http.Request, resourceName, servicePath, record string, sys *components.System) {
	Resource, ok := sys.UAssets[resourceName]
	if !ok {
		http.Error(w, "Resource not found", http.StatusNotFound)
		return
	}

	uAsset := *Resource

	// Files are a service's payload, so they are guarded like one. The check has
	// to precede the transfer: TransferFile writes headers and body, and a
	// refusal issued afterwards is a superfluous WriteHeader against a response
	// that has already gone out.
	//
	// The guard needs something to call this request, and no unit asset
	// registers a service whose subpath is "files" — the three systems that
	// serve files handle the word inside their own dispatch. Guarding it against
	// whatever findServiceByPath returned meant guarding it against a service
	// with no definition, and a token can never name one of those: mismatch
	// requires claimed == actual and claimed != "". So every file request in an
	// authorized cloud was refused, permanently, by a check that was meant to
	// let the authorized ones through.
	//
	// FileService is what such a request is called now. A policy can name it, a
	// consumer can be granted it, and an asset that registers a service by that
	// subpath — which makes it discoverable — is judged by its own record
	// instead.
	if servicePath == FileService {
		serv := findServiceByPath(uAsset.GetServices(), servicePath)
		if serv == nil {
			serv = &components.Service{Definition: FileService, SubPath: FileService}
		}
		if !permittedAs(sys, w, r, resourceName, serv) {
			return
		}
		forms.TransferFile(w, r)
		return
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
// It guards service dispatch only, which is not the same as saying the rest is
// open. The system-level endpoints are handled in handleThreeParts: /kgraph,
// /smodel and /msg require a caller this cloud enrolled (see enrolled), /cert
// stays open so a provider can fetch the authorizer's certificate and bootstrap
// verification at all, and /doc is still served to anyone.
//
// The distinction is authentication against authorization. Those three ask only
// whether the caller holds a certificate this CA issued; this function asks what
// policy says about a named subject and a classified service. A system-level
// endpoint has no service record — no definition, no mission — so there is
// nothing for a policy to reason about, which is why they are guarded by
// identity rather than by rule.
func permitted(sys *components.System, w http.ResponseWriter, r *http.Request, assetName string, services map[string]*components.Service, servicePath string) bool {
	serv := findServiceByPath(services, servicePath)
	if serv == nil {
		// Nothing to classify the request as. Refusing here rather than passing
		// on a service with no definition: an empty one cannot satisfy mismatch,
		// so it would be refused anyway — but with a message about a token claim
		// rather than about the path being unknown, which sends the reader to
		// the policy file for a problem that is not in it.
		if _, err := components.GetRunningCoreSystemURL(sys, AuthorizerName); err == nil {
			log.Printf("%s: refusing %s %s: no service is registered at that path\n",
				sys.Name, r.Method, ForLog(r.URL.Path)) //#nosec G706 -- sanitized by ForLog
			// Which refusal depends on who is asking, because the two answers
			// differ and the difference is information. A caller with no
			// certificate that got 404 here and 401 on a real path could map
			// this system's whole service surface without ever presenting a
			// credential — one request per guess, and the status tells it which
			// guesses were right.
			//
			// So an unidentified caller gets what it would have got for a
			// service that does exist. A caller the connection can name gets the
			// path problem stated plainly, which is what it is useful for: it
			// sends the reader to the configuration rather than to the policy
			// file.
			if _, identified := PeerCN(r); !identified {
				http.Error(w, "the caller presented no verified certificate", http.StatusUnauthorized)
				return false
			}
			http.Error(w, "no service is registered at that path", http.StatusNotFound)
			return false
		}
		return true // no authorizer in this local cloud
	}
	return permittedAs(sys, w, r, assetName, serv)
}

// permittedAs is permitted for a request whose service is already resolved.
func permittedAs(sys *components.System, w http.ResponseWriter, r *http.Request, assetName string, serv *components.Service) bool {
	status, err := AuthorizeRequest(sys, r, assetName, serv)
	if status == 0 {
		return true
	}
	// ForLog strips the control characters that would let a caller forge log
	// entries; see its doc comment. gosec's taint analysis does not follow a
	// value through a sanitizing function, so it reports the path as tainted
	// here regardless.
	log.Printf("%s: refusing %s %s: %v\n", sys.Name, r.Method, ForLog(r.URL.Path), err) //#nosec G706 -- sanitized by ForLog
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
