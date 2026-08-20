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

// Certificates: the plain-text PEM a system receives from the certificate authority.

import (
	"log"
	"net/http"
	"strings"

	"github.com/sdoque/mbaigo/components"
)

// function Certificate provide one's own certificate upon request
func Certificate(w http.ResponseWriter, req *http.Request, sys components.System) {
	// Extract the remote IP address from the request
	remoteAddr := req.RemoteAddr

	// if need for more detailed information about the requester, such as the user-agent or specific headers like X-Forwarded-For (which is often used in proxies)
	// userAgent := req.Header.Get("User-Agent")
	// xForwardedFor := req.Header.Get("X-Forwarded-For")

	// Log the request with the remote address (newlines stripped to prevent log injection)
	safeAddr := strings.NewReplacer("\n", "", "\r", "").Replace(remoteAddr)
	log.Printf("serving system's certificate upon request from %s", safeAddr) // #nosec G706 -- remoteAddr is sanitised above
	// log.Printf("serving system's certificate upon request from %s (User-Agent: %s, X-Forwarded-For: %s)", remoteAddr, userAgent, xForwardedFor)

	// Under the lock the writer takes. This endpoint is served by the HTTP
	// server, which binds before enrollment finishes, and it is what every other
	// system polls to bootstrap verification — so it is read precisely while
	// acquireCertificate is writing the field. System.Mutex is a pointer, so the
	// copy this function receives guards the same lock as the original.
	sys.Mutex.Lock()
	cert := sys.Husk.Certificate
	sys.Mutex.Unlock()

	// Set the content type to text/plain
	w.Header().Set("Content-Type", "text/plain")
	_, err := w.Write([]byte(cert))
	if err != nil {
		log.Println("Error writing the certificate: ", err)
	}
}
