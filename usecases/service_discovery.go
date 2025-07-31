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
	"fmt"
	"io"
	"net/http"
	"strconv"

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
		err = fmt.Errorf("unmarshalling JSON data: %v", err)
		return
	}
	formVersion, ok := jsonData["version"].(string)
	if !ok {
		err = fmt.Errorf("'version' key not found in JSON data")
		return
	}

	switch formVersion {
	case "ServiceQuest_v1":
		var f forms.ServiceQuest_v1
		err = json.Unmarshal(bodyBytes, &f)
		if err != nil {
			err = fmt.Errorf("unable to extract the discovery form request ")
			return
		}
		rec = f
	default:
		err = fmt.Errorf("unsupported service registration form version")
	}
	return
}

// Search4Service requests from the core systems the address of resources's services that meet the need
func Search4Service(qf forms.ServiceQuest_v1, sys *components.System) (servLocation forms.ServicePoint_v1, err error) {
	// Create a new HTTP request to the Orchestrator system (for now the Service Registrar)
	orURL, err := components.GetRunningCoreSystemURL(sys, "orchestrator")
	if err != nil {
		return
	}
	// prepare the payload to perform a service quest
	orURL = orURL + "/squest"
	jsonQF, err := json.MarshalIndent(qf, "", "  ")
	if err != nil {
		return
	}
	resp, err := sendHTTPReq(http.MethodPost, orURL, jsonQF)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	// Read the response /////////////////////////////////
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	return ExtractDiscoveryForm(body)
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
	resp, err := sendHTTPReq(http.MethodPost, orURL, qf)
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

func Search4MultipleServices(cer *components.Cervice, sys *components.System) (err error) {
	questForm := forms.ServiceQuest_v1{
		SysId:             0,
		RequesterName:     sys.Name,
		ServiceDefinition: cer.Definition,
		Protocol:          "http",
		Details:           cer.Details,
		Version:           "ServiceQuest_v1",
	}
	// Pack the service quest form
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
	orURL = orURL + "/squests"
	// Prepare the request to the orchestrator
	resp, err := sendHTTPReq(http.MethodPost, orURL, qf)
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
	srList, ok := discoveryForm.(*forms.ServiceRecordList_v1)
	if !ok {
		return fmt.Errorf("unable to unpack discovery request form")
	}
	for _, values := range srList.List {
		sp := convertToServicePoint(values)
		cer.Nodes[sp.ServNode] = append(cer.Nodes[sp.ServNode], sp.ServLocation)
	}
	return nil
}

func convertToServicePoint(sr forms.ServiceRecord_v1) (sp forms.ServicePoint_v1) {
	rec := sr
	sp.NewForm()
	sp.ProviderName = rec.SystemName
	sp.ServiceDefinition = rec.ServiceDefinition
	sp.Details = rec.Details
	sp.ServLocation = "http://" + rec.IPAddresses[0] + ":" + strconv.Itoa(rec.ProtoPort["http"]) + "/" + rec.SystemName + "/" + rec.SubPath
	sp.ServNode = rec.ServiceNode
	return
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
		err = fmt.Errorf("unmarshalling JSON data: %v", err)
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
			err = fmt.Errorf("unmarshalling JSON data: %v", err)
			return
		}
		sLoc = f
	default:
		err = fmt.Errorf("unsupported service discovery form version")
	}
	return
}
