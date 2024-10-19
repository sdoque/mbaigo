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
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// // RSignal is a remote signal, which has to be fetched from a service provider
// type RSignal struct {
// 	Value       float64
// 	Unit        string
// 	Timestamp   time.Time
// 	Address     string
// 	QuestForm   forms.ServiceQuest_v1
// 	ServiceList forms.ServicePoint_v1
// 	Sys         *components.System
// }

// GetState request the current state of a unit asset (via the asset's service)
func GetState(cer *components.Cervice, sys *components.System) (f forms.Form, err error) {
	// get the address of the informing service of the target asset via the Orchestrator
	if len(cer.Url) == 0 {
		err := Search4Services(cer, sys)
		if err != nil {
			return f, err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // Create a new context, with a 2-second timeout
	defer cancel()
	// Create a new HTTP request
	req, err := http.NewRequest(http.MethodGet, cer.Url[0], nil)
	if err != nil {
		return f, err
	}
	// Associate the cancellable context with the request
	req = req.WithContext(ctx)
	// Send the request /////////////////////////////////
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		cer.Url = []string{} // failed to get the resource at that location: reset address field (could pop the first elemen [1:] in a for loop until it is empty)
		return f, err
	}
	defer resp.Body.Close()

	// Check if the status code indicates an error (anything outside the 200–299 range)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return f, fmt.Errorf("received non-2xx status code: %d, response: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("GetRValue-Error reading registration response body: %v", err)
		return
	}

	headerContentTtype := resp.Header.Get("Content-Type")
	f, err = Unpack(bodyBytes, headerContentTtype)
	if err != nil {
		fmt.Printf("error unpacking the service response: %s", err)
	}
	return f, nil
}

// SetState puts a request to change the state of a unit asset (via the asset's service)
func SetState(cer *components.Cervice, sys *components.System, bodyBytes []byte) (err error) {
	// get the address of the informing service of the target asset via the Orchestrator
	if len(cer.Url) == 0 {
		err := Search4Services(cer, sys)
		if err != nil {
			return err
		}
	}
	// Create a new context, with a 2-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Create a new HTTP request
	req, err := http.NewRequest(http.MethodPut, cer.Url[0], bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	// Set the Content-Type header
	req.Header.Set("Content-Type", "application/json")
	// Associate the cancellable context with the request
	req = req.WithContext(ctx)

	// Send the request /////////////////////////////////
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		cer.Url = []string{} // failed to get the resource at that location: reset address field (could pop the first elemen [1:] in a for loop until it is empty)
		return err
	}
	defer resp.Body.Close()

	// Check if the status code indicates an error (anything outside the 200–299 range)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("received non-2xx status code: %d, response: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	return nil
}
