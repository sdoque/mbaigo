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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/sdoque/mbaigo/forms"
)

// HTTPProcessSetRequest processes a Get request
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
		http.Error(w, fmt.Sprintf("Error packing response: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", bestContentType)
	w.WriteHeader(http.StatusOK)
	w.Write(responseData)
}

// HTTPProcessSetRequest processes a SET request
func HTTPProcessSetRequest(w http.ResponseWriter, req *http.Request) (f forms.SignalA_v1a, err error) {
	defer req.Body.Close()
	bodyBytes, err := io.ReadAll(req.Body) // Use io.ReadAll instead of ioutil.ReadAll
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		return
	}
	var jsonData map[string]interface{}
	err = json.Unmarshal(bodyBytes, &jsonData)
	if err != nil {
		log.Printf("Error unmarshaling JSON data: %v", err)
		return
	}
	formVersion, ok := jsonData["version"].(string)
	if !ok {
		log.Printf("Error: 'version' key not found in JSON data")
		return
	}
	switch formVersion {
	case "SignalA_v1.0":
		var sig forms.SignalA_v1a
		err = json.Unmarshal(bodyBytes, &sig)
		if err != nil {
			log.Println("Unable to extract signal set request ")
			return
		}
		f = sig
	default:
		err = errors.New("unsupported service set request form version")
	}
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
			fmt.Sscanf(parts[1], "q=%f", &qValue)
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
