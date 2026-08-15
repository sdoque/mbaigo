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
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/sdoque/mbaigo/forms"
)

// Pack serializes a form to a byte array for payload shipment with serialization format (sf) request
func Pack(f forms.Form, contentType string) (data []byte, err error) {
	switch contentType {
	case "application/xml":
		data, err = xml.MarshalIndent(f, "", "  ")
		if err != nil {
			err = fmt.Errorf("error encoding XML: %w", err)
			return
		}
	default:
		data, err = json.MarshalIndent(f, "", "  ")
		if err != nil {
			err = fmt.Errorf("error encoding JSON: %w", err)
			return
		}
	}
	return
}

// Unpack function to deserialize data into appropriate form structs
func Unpack(data []byte, contentType string) (forms.Form, error) {
	var rawData map[string]interface{}

	// Heuristic handling for text/plain with possible charset
	if strings.Contains(contentType, "text/plain") {
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) > 0 {
			switch trimmed[0] {
			case '{', '[':
				contentType = "application/json"
			case '<':
				contentType = "application/xml"
			default:
				return nil, fmt.Errorf("plain text content is neither valid JSON nor XML")
			}
		} else {
			return nil, fmt.Errorf("empty payload with content type text/plain")
		}
	}

	// Unmarshal to get the form version
	switch {
	case strings.Contains(contentType, "application/json"):
		if err := json.Unmarshal(data, &rawData); err != nil {
			return nil, fmt.Errorf("error unmarshalling JSON: %w", err)
		}
	case strings.Contains(contentType, "application/xml"):
		if err := xml.Unmarshal(data, &rawData); err != nil {
			return nil, fmt.Errorf("error unmarshalling XML: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported content type")
	}

	// Retrieve form version
	formVersion, ok := rawData["version"].(string)
	if !ok {
		return nil, fmt.Errorf("'version' key not found in data")
	}

	// Look up the form type in the map
	formType, exists := forms.FormTypeMap[formVersion]
	if !exists {
		return nil, fmt.Errorf("unsupported form version: %s", formVersion)
	}

	// Create a new instance of the form
	formInstance := reflect.New(formType).Interface().(forms.Form)

	// Unmarshal the full data into the form instance
	switch {
	case strings.Contains(contentType, "application/json"):
		if err := json.Unmarshal(data, formInstance); err != nil {
			return nil, fmt.Errorf("error unmarshalling JSON into form: %w", err)
		}
	case strings.Contains(contentType, "application/xml"):
		if err := xml.Unmarshal(data, formInstance); err != nil {
			return nil, fmt.Errorf("error unmarshalling XML into form: %w", err)
		}
	}

	return formInstance, nil
}

// ------- Naming Conventions Tools -------

// ToCamel converts PascalCase to camelCase.
func ToCamel(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// ToPascal converts camelCase to PascalCase.
func ToPascal(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// IsFirstLetterUpper returns true if the first rune is uppercase.
func IsFirstLetterUpper(s string) bool {
	if s == "" {
		return false
	}
	return unicode.IsUpper([]rune(s)[0])
}

// IsFirstLetterLower returns true if the first rune is lowercase.
func IsFirstLetterLower(s string) bool {
	if s == "" {
		return false
	}
	return unicode.IsLower([]rune(s)[0])
}

// IsPascalCase returns true if the string starts with an uppercase letter.
func IsPascalCase(s string) bool {
	return IsFirstLetterUpper(s)
}

// IsCamelCase returns true if the string starts with a lowercase letter.
func IsCamelCase(s string) bool {
	return IsFirstLetterLower(s)
}

// ------- HTTP Client Tools -------

// The one place the framework's HTTP client is installed.
//
// There were two: this one, and another in authentication.go that added the TLS
// dial. Go runs a package's files in name order, so this one ran second and
// replaced the other — leaving a client with no transport, which means no CA
// pool and no client certificate. Every system-to-system call over HTTPS then
// failed with "certificate signed by unknown authority", because the client had
// nothing to verify against.
//
// Installed here, once, at package load, so it is in place before any goroutine
// in any system can read it. The TLS configuration it dials with arrives later,
// when enrollment completes, and is published through clientTLS rather than by
// replacing the client — http.DefaultClient is a package-level variable that
// three dozen call sites read.
//
// The tests depend on this client too, and sometimes replace its transport with
// a mock.
func init() {
	http.DefaultClient = &http.Client{
		Timeout: time.Second * 30,
		Transport: &http.Transport{
			DialTLSContext: dialTLS,
		},
	}
}

const userAgent string = "mbaigo"

func sendHTTPReq(method string, url string, data []byte) (*http.Response, error) {
	return sendHTTPReqWithToken(method, url, "", data)
}

// sendHTTPReqWithToken is sendHTTPReq with an access token attached. The token
// is what proves to the provider that the authorizer permitted this specific
// call; without it a provider in an authorized cloud refuses.
func sendHTTPReqWithToken(method string, url string, token string, data []byte) (*http.Response, error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if token != "" {
		req.Header.Set(TokenHeader, token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The body carries the reason — "the token expired at ...", "mismatch
		// (action): read vs write" — and returning without it left the operator
		// with a bare status code for a refusal that names its own cause.
		// Reading it also closes the response: returning early with the body
		// open leaked a connection on every refusal, which a control loop
		// polling against a standing 403 does once per tick.
		reason, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		if detail := strings.TrimSpace(string(reason)); detail != "" {
			return nil, fmt.Errorf("%s: %s", resp.Status, detail)
		}
		return nil, fmt.Errorf("bad response: %s", resp.Status)
	}
	return resp, nil
}

// ForLog makes a caller-supplied string safe to write into a log line.
//
// A request path and a certificate common name both reach the log, and both are
// chosen by whoever is calling. A newline in either lets that caller write log
// entries of their own: a forged "first request from peer X" line is
// indistinguishable from a real one once it is in the file, and the log is what
// an operator reads to work out what happened. Control characters go too — a
// terminal displaying a log should not be driven by the traffic it describes.
//
// Bounded as well, because a caller choosing the length of a log line is a
// smaller problem of the same kind.
func ForLog(s string) string {
	const most = 256
	cleaned := strings.Map(func(r rune) rune {
		// Control characters, and the three other categories a log viewer or a
		// terminal gives meaning to: the line and paragraph separators some
		// honour as newlines, and the format characters, which include the
		// bidirectional overrides that can reverse how a line reads.
		if r == utf8.RuneError || unicode.IsControl(r) ||
			unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return -1
		}
		return r
	}, s)

	// Cut on a rune boundary. Slicing bytes puts back exactly what the map above
	// exists to remove: half of a multi-byte character is invalid UTF-8.
	runes := []rune(cleaned)
	if len(runes) > most {
		return string(runes[:most]) + "…(truncated)"
	}
	return cleaned
}
