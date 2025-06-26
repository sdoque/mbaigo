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
	"fmt"
	"io"
	"log"
	"net/http"

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
	f.Protocol = protocol
	f.Details = res.GetDetails()
	return f
}

// ExtractQuestForm is used by the Service Registrar and Orchestrator when they receive a service query from a consumer system
func ExtractQuestForm(bodyBytes []byte) (rec forms.ServiceQuest_v1, err error) {
	var jsonData map[string]interface{}
	err = json.Unmarshal(bodyBytes, &jsonData)
	if err != nil {
		log.Printf("Error unmarshalling JSON data: %v", err)
		return
	}
	formVersion, ok := jsonData["version"].(string)
	if !ok {
		log.Printf("'version' key not found in JSON data")
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
		err = fmt.Errorf("unsupported service registration form version")
	}
	return
}

func sendHttpReq(method string, url string, data []byte) (resp *http.Response, err error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json") // set the Content-Type header
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, fmt.Errorf("received non-2xx status code: %d, response: %s from the Orchestrator", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return
}

// Search4Service requests from the core systems the address of resources's services that meet the need
func Search4Service(qf forms.ServiceQuest_v1, sys *components.System) (servLocation forms.ServicePoint_v1, err error) {
	// Create a new HTTP request to the Orchestrator system (for now the Service Registrar)
	orURL, err := components.GetRunningCoreSystemURL(sys, "orchestrator")
	if err != nil {
		return servLocation, err
	}
	// prepare the payload to perform a service quest
	orURL = orURL + "/squest"
	jsonQF, err := json.MarshalIndent(qf, "", "  ")
	if err != nil {
		return servLocation, err
	}
	resp, err := sendHttpReq(http.MethodPost, orURL, jsonQF)
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
		ServiceDefinition: cer.Definition,
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
	orURL, err := components.GetRunningCoreSystemURL(sys, "orchestrator")
	if err != nil {
		return err
	}
	if orURL == "" {
		return fmt.Errorf("failed to locate an orchestrator")
	}
	orURL = orURL + "/squest"
	// Prepare the request to the orchestrator
	resp, err := sendHttpReq(http.MethodPost, orURL, qf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Read the response /////////////////////////////////
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	headerContentType := resp.Header.Get("Content-Type")
	discoveryForm, err := Unpack(bodyBytes, headerContentType)
	if err != nil {
		return err
	}
	// Perform a type assertion to convert the returned Form to ServicePoint_v1
	df, ok := discoveryForm.(*forms.ServicePoint_v1)
	if !ok {
		return fmt.Errorf("unable to unpack discovery request form")
	}
	cer.Nodes[df.ServNode] = append(cer.Nodes[df.ServNode], df.ServLocation)
	return nil
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
		err = fmt.Errorf("unsupported service registration form version")
		return
	}
	return
}

// ExtractDiscoveryForm is used by the Orchestrator and the authorized consumer system
func ExtractDiscoveryForm(bodyBytes []byte) (sLoc forms.ServicePoint_v1, err error) {
	var jsonData map[string]interface{}
	err = json.Unmarshal(bodyBytes, &jsonData)
	if err != nil {
		log.Printf("Error unmarshalling JSON data: %v", err)
		return
	}
	formVersion, ok := jsonData["version"].(string)
	if !ok {
		err = fmt.Errorf("'version' key not found in JSON data")
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
		err = fmt.Errorf("unsupported service discovery form version")
	}
	return
}
