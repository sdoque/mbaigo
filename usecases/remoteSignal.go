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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// RSignal is a remote signal, which has to be fetched from a service provider
type RSignal struct {
	Value       float64
	Unit        string
	Timestamp   time.Time
	Address     string
	QuestForm   forms.ServiceQuest_v1
	ServiceList forms.ServicePoint_v1
	Sys         *components.System
}

// TODO: Remove
// GetValue makes a GET HTTP request to a provider to obtain a signal payload as a service.
// If the URL is unknown, it will first get it from the Service Registrar
func (sig *RSignal) GetValue() (v float64, err error) {
	// get the address of the informing service of the target asset via the Orchestrator
	if sig.Address == "" {
		resourceLocation, err := Search4Service(sig.QuestForm, sig.Sys)
		if err != nil {
			log.Printf("unable to locate the sensor resource")
			return 0, err
		}
		sig.Address = resourceLocation.ServLocation
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // Create a new context, with a 2-second timeout
	defer cancel()
	// Create a new HTTP request
	req, err := http.NewRequest(http.MethodGet, sig.Address, nil)
	if err != nil {
		sig.Address = "" // failed to get the resource at that location: reset address field
		return v, err
	}
	// Associate the cancellable context with the request
	req = req.WithContext(ctx)
	// Send the request /////////////////////////////////
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return v, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("GetRValue-Error reading registration response body: %v", err)
		return
	}
	var jsonData map[string]interface{}
	err = json.Unmarshal(bodyBytes, &jsonData)
	if err != nil {
		log.Printf("GetRValue-Error unmarshaling JSON data: %v", err)
		return
	}
	if resp.StatusCode == 404 {
		err = fmt.Errorf("received 404 not found for url: %s", sig.Address)
		return
	}
	formVersion, ok := jsonData["version"].(string)
	if !ok {
		log.Printf("Error: 'version' key not found in JSON data")
		return
	}
	switch formVersion {
	case "SignalA_v1.0":
		var rsig forms.SignalA_v1a
		err = json.Unmarshal(bodyBytes, &rsig)
		if err != nil {
			log.Println("Unable to extract registration request ")
			return
		}
		sig.Value = rsig.Value
		sig.Timestamp = rsig.Timestamp
		v = rsig.Value
	default:
		err = errors.New("unsupported service registrattion form version")
	}
	return v, nil
}

// GetState request the current state of a unit asset (via the asset's service)
func GetState(cer *components.Cervice, sys *components.System) (f forms.Form, err error) {
	// get the address of the informing service of the target asset via the Orchestrator
	if len(cer.Url) == 0 {
		err := Search4Services(cer, sys)
		if err != nil {
			log.Printf("unable to locate the derised service")
			return f, err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // Create a new context, with a 2-second timeout
	defer cancel()
	// Create a new HTTP request
	req, err := http.NewRequest(http.MethodGet, cer.Url[0], nil)
	if err != nil {
		cer.Url = []string{} // failed to get the resource at that location: reset address field (could pop the first elemen [1:] in a for loop until it is empty)
		return f, err
	}
	// Associate the cancellable context with the request
	req = req.WithContext(ctx)
	// Send the request /////////////////////////////////
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return f, err
	}
	defer resp.Body.Close()

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
			log.Printf("unable to locate the derised service")
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
		return err
	}
	defer resp.Body.Close()

	// Read the response /////////////////////////////////
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Println(body) ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	return nil
}

// DOTO: remove
// UpdateValue makes a PUT HTTP request to a service provider to update a signal payload as a service.
// If the URL is unknown, it will first get it from the Service Registrar
func (sig *RSignal) UpdateValue(value float64) (err error) {
	// get the address of the updating service of the target asset via the Orchestrator
	if sig.Address == "" {
		servLocation, err := Search4Service(sig.QuestForm, sig.Sys)
		if err != nil {
			log.Printf("unable to locate actuator resource")
			return err
		}
		sig.Address = servLocation.ServLocation
	}

	// prepare the form to send
	var f forms.SignalA_v1a
	f.NewForm()
	f.Value = value
	f.Unit = "Degrees"
	f.Timestamp = time.Now()
	jsonData, err := json.MarshalIndent(f, "", "  ")

	// Create a new context, with a 2-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Create a new HTTP request
	req, err := http.NewRequest(http.MethodPut, sig.Address, bytes.NewBuffer(jsonData))
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
		return err
	}
	defer resp.Body.Close()

	// Read the response /////////////////////////////////
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Println(body) ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	return nil
}
