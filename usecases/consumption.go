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

	"net/http"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// GetState request the current state of a unit asset (via the asset's service)
func GetState(cer *components.Cervice, sys *components.System) (f forms.Form, err error) {
	return stateHandler(http.MethodGet, cer, sys, nil)
}

// GetStates requests the current state of certain services of a unit asset depending on requested definition and/or details
func GetStates(cer *components.Cervice, sys *components.System) (f []forms.Form, err []error) {
	return stateHandlers(http.MethodGet, cer, sys, nil)
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

	resp, err := sendHttpReq(httpMethod, serviceUrl, bodyBytes)
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

func stateHandlers(httpMethod string, cer *components.Cervice, sys *components.System, bodyBytes []byte) (f []forms.Form, err []error) {
	if len(cer.Nodes) == 0 {
		lastErr := Search4MultipleServices(cer, sys)
		if lastErr != nil {
			f = append(f, nil)
			err = append(err, lastErr)
			return f, err
		}
	}

	var serviceUrls []string
	for _, values := range cer.Nodes {
		if len(values) > 0 {
			serviceUrls = append(serviceUrls, values...)
		}
	}

	for _, serviceUrl := range serviceUrls {
		if len(serviceUrl) == 0 {
			continue
		}
		resp, lastErr := sendHttpReq(httpMethod, serviceUrl, bodyBytes)
		if lastErr != nil {
			cer.Nodes = make(map[string][]string)
			f = append(f, nil)
			err = append(err, lastErr)
			continue
		}
		defer resp.Body.Close()

		// If the response includes a payload, unpack it into a forms.Form
		bodyBytes, lastErr = io.ReadAll(resp.Body)
		if lastErr != nil {
			lastErr = fmt.Errorf("reading state response body: %w", lastErr)
			f = append(f, nil)
			err = append(err, lastErr)
			continue
		}

		if len(bodyBytes) < 1 {
			lastErr = fmt.Errorf("got empty response body")
			f = append(f, nil)
			err = append(err, lastErr)
			continue
		}

		headerContentType := resp.Header.Get("Content-Type")
		formValue, lastErr := Unpack(bodyBytes, headerContentType)
		if lastErr != nil {
			lastErr = fmt.Errorf("unpacking response body: %w", lastErr)
			f = append(f, nil)
			err = append(err, lastErr)
			continue
		}
		f = append(f, formValue)
		err = append(err, nil)
	}
	return f, err
}
