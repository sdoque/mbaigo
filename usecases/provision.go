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
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// HTTPProcessSetRequest processes a Get request
// TODO: this function should really return an error too and behave like everyone
// else. And causing http.Errors is an ugly side effect.
func HTTPProcessGetRequest(w http.ResponseWriter, r *http.Request, f forms.Form) {
	if f == nil {
		http.Error(w, "No payload found.", http.StatusNotFound)
		return
	}
	if f.FormVersion() == "" {
		http.Error(w, "No payload information found.", http.StatusNotFound)
		return
	}

	acceptHeader := r.Header.Get("Accept")
	bestContentType := getBestContentType(acceptHeader)

	responseData, err := Pack(f, bestContentType)
	if err != nil {
		log.Printf("Error packing response: %v", err)
		http.Error(w, "Error packing response.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", bestContentType)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(responseData)
	if err != nil {
		log.Printf("Error while writing response: %v", err)
		http.Error(w, "Error writing response.", http.StatusInternalServerError)
	}
}

// HTTPProcessSetRequest processes a SET request
func HTTPProcessSetRequest(w http.ResponseWriter, req *http.Request) (sig forms.SignalA_v1a, err error) {
	bodyBytes, err := io.ReadAll(req.Body) // Use io.ReadAll instead of ioutil.ReadAll
	if err != nil {
		err = fmt.Errorf("reading request body: %w", err)
		return
	}
	defer req.Body.Close()
	headerContentType := req.Header.Get("Content-Type")
	f, err := Unpack(bodyBytes, headerContentType)
	if err != nil {
		return
	}
	temp, ok := f.(*forms.SignalA_v1a)
	if !ok {
		err = fmt.Errorf("form is not of type SignalA_v1a")
		return
	}
	sig = *temp // Stupid type conversion because return type was picked incorrectly
	return
}

// getBestContentType parses the Accept header and returns the best content type based on q-values
func getBestContentType(acceptHeader string) string {
	if acceptHeader == "" {
		return "application/json" // Default to JSON if no Accept header is provided
	}

	// Split the header by commas to get individual MIME types
	mimeTypes := strings.Split(acceptHeader, ",")
	bestType := ""
	bestQValue := 0.0

	for _, mimeType := range mimeTypes {
		parts := strings.Split(strings.TrimSpace(mimeType), ";")
		contentType := parts[0]
		qValue := 1.0 // Default q-value is 1.0

		// Check for q-value in the MIME type
		if len(parts) > 1 && strings.HasPrefix(parts[1], "q=") {
			_, err := fmt.Sscanf(parts[1], "q=%f", &qValue)
			if err != nil {
				continue
			}
		}

		// Update the best content type if this one has a higher q-value
		if qValue > bestQValue {
			bestQValue = qValue
			bestType = contentType
		}
	}

	// Default to JSON if no valid MIME type is found
	if bestType == "" {
		return "application/json"
	}

	return bestType
}

func RegisterMessenger(w http.ResponseWriter, r *http.Request, s *components.System) {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("read request body: %v\n", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// Won't bother logging the following errors as they are caused by bad/poor
	// client requests, which we don't really care about on the server side.
	f, err := Unpack(b, r.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	reg, ok := f.(*forms.MessengerRegistration_v1)
	if !ok {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if len(reg.Host) < 1 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	if _, found := s.Messengers[reg.Host]; found {
		// The system already knows the messenger, avoid re-storing it so that
		// the error count don't get reset
		return
	}
	s.Messengers[reg.Host] = 0 // Registers the new messenger with zero errors
}
