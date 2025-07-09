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
	"net/url"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// GetState request the current state of a unit asset (via the asset's service)
func GetState(cer *components.Cervice, sys *components.System) (f forms.Form, err error) {
	return stateHandler(http.MethodGet, cer, sys, nil)
}

// SetState puts a request to change the state of a unit asset (via the asset's service)
func SetState(cer *components.Cervice, sys *components.System, bodyBytes []byte) (f forms.Form, err error) {
	return stateHandler(http.MethodPut, cer, sys, bodyBytes)
}

func stateHandler(httpMethod string, cer *components.Cervice, sys *components.System, bodyBytes []byte) (f forms.Form, err error) {
	if len(cer.Nodes) == 0 {
		err := Search4Services(cer, sys)
		if err != nil {
			return f, err
		}
	}

	var serviceUrl string
	for _, values := range cer.Nodes {
		if len(values) > 0 {
			serviceUrl = values[0]
			break
		}
	}

	resp, err := sendHTTPReq(httpMethod, serviceUrl, bodyBytes)
	if err != nil {
		cer.Nodes = make(map[string][]string) // Failed to get the resource at that location: reset the providers list, which will trigger a new service search
		return f, err
	}
	defer resp.Body.Close()

	// If the response includes a payload, unpack it into a forms.Form
	bodyBytes, err = io.ReadAll(resp.Body)
	if err != nil {
		return f, fmt.Errorf("reading state response body: %w", err)
	}

	if len(bodyBytes) < 1 {
		return f, fmt.Errorf("got empty response body")

	}

	headerContentType := resp.Header.Get("Content-Type")
	return Unpack(bodyBytes, headerContentType)
}

const messengerMaxErrors int = 3

func Log(sys *components.System, lvl forms.MessageLevel, msg string, args ...any) {
	sm := forms.NewSystemMessage_v1(lvl, fmt.Sprintf(msg+"\n", args...), sys.Name)
	body, err := Pack(forms.Form(&sm), "application/json")
	if err != nil {
		log.Print(sm.Body)
		log.Printf("failed to pack SystemMessage: %v\n", err)
		return
	}

	// Iterate over all messengers and try sending a copy of the log msg
	// (can't use a regular for-loop for this type)
	sys.Messengers.Range(func(k, v any) bool {
		host, ok1 := k.(string) // Should always be a host string!
		errors, ok2 := v.(int)
		if !ok1 || !ok2 {
			sys.Messengers.Delete(k) // if not, removes the unusable cruft
			return true
		}

		newErrors := 0 // If there's no error while sending msg, the count is reset
		if err := sendLogMessage(host, body); err != nil {
			if errors >= messengerMaxErrors {
				// Too many errors indicates a problematic messenger
				sys.Messengers.Delete(k)
				return true
			}
			newErrors = errors + 1
		}
		sys.Messengers.Store(k, newErrors)
		return true
	})
}

// Hard-coding the path is ugly but it skips an extra service discovery cycle for now
const messengerPath string = "/messenger/log/message"

func sendLogMessage(h string, b []byte) error {
	u, err := url.Parse("http://" + h + messengerPath)
	if err != nil {
		return err
	}
	resp, err := sendHTTPReq("POST", u.String(), b)
	if err != nil {
		return err
	}
	_ = resp.Body.Close() // Don't care about the body or any errors it might cause
	return nil
}
