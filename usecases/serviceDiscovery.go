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

// ServRegForms returns the list of forms that the service registration handles
func ServQuestForms() []string {
	return []string{"ServiceQuest_v1", "ServicePoint_v1"}
}

// FillQuestForm described the sought service (e.g., RemoteSignal)
func FillQuestForm(sys *components.System, res components.UnitAsset, sDef, protocol string) forms.ServiceQuest_v1 {
	var f forms.ServiceQuest_v1
	f.NewForm()
	f.RequesterName = sys.Name
	f.ServiceDefinition = sDef
	// TODO: known bug on commit
	// f.Protocol = append()
	f.Protocol = protocol
	f.Details = res.GetDetails()
	return f
}

// ExtractQuestForm is used by the Service Registrar and Orchestrator when they receive a service query from a consumer system
func ExtractQuestForm(bodyBytes []byte) (rec forms.ServiceQuest_v1, err error) {
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
	case "ServiceQuest_v1":
		var f forms.ServiceQuest_v1
		err = json.Unmarshal(bodyBytes, &f)
		if err != nil {
			log.Println("Unable to extract the discovery form request ")
			return
		}
		rec = f
	default:
		err = errors.New("unsupported service registrattion form version")
	}
	return
}

// Search4Service requests from the core systems the address of resources's services that meet the need
func Search4Service(qf forms.ServiceQuest_v1, sys *components.System) (servLocation forms.ServicePoint_v1, err error) {

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // Create a new context, with a 2-second timeout
	defer cancel()
	// Create a new HTTP request to the Orchestrator system (for now the Service Registrar)
	var orchestratorPointer *components.CoreSystem
	for _, cSys := range sys.CoreS {
		if cSys.Name == "orchestrator" {
			orchestratorPointer = cSys
		}
	}

	// prepare the payload to perform a service quest
	oURL := orchestratorPointer.Url + "/squest"
	jsonQF, err := json.MarshalIndent(qf, "", "  ")
	if err != nil {
		log.Printf("problem encountered when marshalling the service quest to the Orchestrator at %s\n", oURL)
		return servLocation, err
	}
	// prepare the request
	req, err := http.NewRequest(http.MethodPost, oURL, bytes.NewBuffer(jsonQF))
	if err != nil {
		return servLocation, err
	}
	req.Header.Set("Content-Type", "application/json") // set the Content-Type header
	req = req.WithContext(ctx)                         // associate the cancellable context with the request

	// Send the request /////////////////////////////////
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return servLocation, err
	}

	defer resp.Body.Close()
	// Read the response /////////////////////////////////
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return servLocation, err
	}
	servLocation, err = ExtractDiscoveryForm(body)
	if err != nil {
		return servLocation, err
	}
	return servLocation, err
}

// Search4Services requests from the core systems the address of resources' services that meet the need
func Search4Services(cer *components.Cervice, sys *components.System) (err error) {
	// instantiate the service quest form
	questForm := forms.ServiceQuest_v1{
		SysId:             0,
		RequesterName:     sys.Name,
		ServiceDefinition: cer.Name,
		Protocol:          "http",
		Details:           cer.Details,
		Version:           "ServiceQuest_v1",
	}

	//pack the service quest form
	qf, err := Pack(&questForm, "application/json")
	if err != nil {
		return err
	}

	// Search for an Orchestrator system within the local cloud
	var orchestratorPointer *components.CoreSystem
	for _, cSys := range sys.CoreS {
		if cSys.Name == "orchestrator" {
			orchestratorPointer = cSys
		}
	}
	if orchestratorPointer == nil {
		err = errors.New("failed to locate an Orchestrator")
		return err
	}
	oURL := orchestratorPointer.Url + "/squest"

	// Prepare the request to the Orchestrator
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // Create a new context, with a 2-second timeout
	defer cancel()
	req, err := http.NewRequest(http.MethodPost, oURL, bytes.NewBuffer(qf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json") // set the Content-Type header
	req = req.WithContext(ctx)                         // associate the cancellable context with the request

	// Send the request /////////////////////////////////
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	// Check if the status code indicates an error (anything outside the 200–299 range)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("received non-2xx status code: %d, response: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	// Read the response /////////////////////////////////
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	headerContentTtype := resp.Header.Get("Content-Type")
	discoveryForm, err := Unpack(bodyBytes, headerContentTtype)
	if err != nil {
		log.Printf("error extracting the discovery request %v\n", err)
	}

	// Perform a type assertion to convert the returned Form to ServicePoint_v1
	df, ok := discoveryForm.(*forms.ServicePoint_v1)
	if !ok {
		fmt.Println("Problem unpacking the service discovery request form")
		return
	}

	cer.Url = append(cer.Url, df.ServLocation)
	return err
}

// FillDiscoveredServices returns a json data byte array with a slice of matching services (e.g., Service Registrar)
func FillDiscoveredServices(dsList []forms.ServiceRecord_v1, version string) (f forms.Form, err error) {
	switch version {
	case "ServiceRecordList_v1":
		dslForm := &forms.ServiceRecordList_v1{} // pointer to struct
		f = dslForm.NewForm()
		for _, rec := range dsList {
			sf := rec.NewForm().(*forms.ServiceRecord_v1) // create new form and cast it to *ServiceRecord_v1
			dslForm.List = append(dslForm.List, *sf)
		}
	default:
		err = errors.New("unsupported service registration form version")
		return
	}
	return
}

// ExtractDiscoveryForm is used by the Orchestrator and the authorized consumer system
func ExtractDiscoveryForm(bodyBytes []byte) (sLoc forms.ServicePoint_v1, err error) {
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
	case "ServicePoint_v1":
		var f forms.ServicePoint_v1
		f.NewForm()
		err = json.Unmarshal(bodyBytes, &f)
		if err != nil {
			log.Println("Unable to extract registration request ")
			return
		}
		sLoc = f
	default:
		err = errors.New("unsupported service discovery form version")
	}
	return
}
