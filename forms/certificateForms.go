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

// The "forms" package is designed to define structured schemas, known as "structs,"
// which represent the format and organization of documents intended for data exchange.
// These structs are utilized to create forms that are populated with data, acting as
// standardized payloads for transmission between different systems. This ensures that
// the data exchanged maintains a consistent structure, facilitating seamless
// integration and processing across system boundaries.
// Basic forms include the service registration and the service query forms.
// The form version is used for backward compatibility.

// HATEOAS forms are HTML templates used to describe the systems, resources and services

package forms

import (
	"log"
	"net/http"

	"github.com/sdoque/mbaigo/components"
)

// function Certificate provide one's own certificate upon request
func Certificate(w http.ResponseWriter, req *http.Request, sys components.System) {
	// Extract the remote IP address from the request
	remoteAddr := req.RemoteAddr

	// if need for more detailed information about the requester, such as the user-agent or specific headers like X-Forwarded-For (which is often used in proxies)
	// userAgent := req.Header.Get("User-Agent")
	// xForwardedFor := req.Header.Get("X-Forwarded-For")

	// Log the request with the remote address
	log.Printf("serving system's certificate upon request from %s", remoteAddr)
	// log.Printf("serving system's certificate upon request from %s (User-Agent: %s, X-Forwarded-For: %s)", remoteAddr, userAgent, xForwardedFor)

	cert := sys.Husk.Certificate

	// Set the content type to text/plain
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(cert))
}
