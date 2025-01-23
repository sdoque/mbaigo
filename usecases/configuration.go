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

// Package "usecases" addresses system actions in given use cases such as configuration,
// registration, authentication, orchestration, ...
package usecases

import (
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/sdoque/mbaigo/components"
)

// Sys2ConfigFile is the stuct used to prepare the systemconfig.json file
type Sys2ConfigFile struct {
	CName      string                  `json:"systemname"`
	UAsset     []components.UnitAsset  `json:"unit_assets"`
	CServices  []components.Service    `json:"services"`
	Protocols  map[string]int          `json:"protocolsNports"`
	PKIdetails pkix.Name               `json:"distinguishedName"`
	CCoreS     []components.CoreSystem `json:"coreSystems"`
}

// ConfigFile2Sys is used to extact out the information of the systemconfig.json file
// Since it does not know about the details of the Thing, it does not unmarsahll this
// information
type ConfigFile2Sys struct {
	CName        string                  `json:"systemname"`
	rawResources []json.RawMessage       `json:"-"`
	CServices    []components.Service    `json:"services"`
	Protocols    map[string]int          `json:"protocolsNports"`
	PKIdetails   pkix.Name               `json:"distinguishedName"`
	CCoreS       []components.CoreSystem `json:"coreSystems"`
}

// Configure read the system configuration JSON file to get the deployment details.
// If the file is missing, it generates a systemconfig file and shuts down the system
func Configure(sys *components.System) ([]json.RawMessage, []components.Service, error) {

	var rawBytes []json.RawMessage // the mbaigo library does not know about the Thing's structure (defined in the file thing.go and not part of the library)
	var serviceList []components.Service
	// prepare content of configuration file
	var sys2file Sys2ConfigFile

	sys2file.CName = sys.Name
	sys2file.Protocols = sys.Husk.ProtoPort
	sys2file.UAsset = getFirstAsset(sys.UAssets)
	originalSs := getServicesList(sys2file.UAsset[0])
	sys2file.CServices = originalSs

	sys2file.PKIdetails.CommonName = "arrowhead.eu"
	sys2file.PKIdetails.Country = []string{"SE"}
	sys2file.PKIdetails.Province = []string{"Norrbotten"}
	sys2file.PKIdetails.Locality = []string{"Luleaa"}
	sys2file.PKIdetails.Organization = []string{"Luleaa University of Technology"}
	sys2file.PKIdetails.OrganizationalUnit = []string{"CPS"}

	serReg := components.CoreSystem{
		Name:        "serviceregistrar",
		Url:         "http://localhost:8443/serviceregistrar/registry",
		Certificate: ".X509pubKey",
	}
	orches := components.CoreSystem{
		Name:        "orchestrator",
		Url:         "http://localhost:8445/orchestrator/orchestration",
		Certificate: ".X509pubKey",
	}
	ca := components.CoreSystem{
		Name:        "ca",
		Url:         "http://localhost:9000/ca/certification",
		Certificate: ".X509pubKey",
	}
	coreSystems := []components.CoreSystem{serReg, orches, ca}
	sys2file.CCoreS = coreSystems

	// open the configuration file or created with the content prepared above
	systemconfigfile, err := os.Open("systemconfig.json")
	if err != nil { // could not find the systemconfig.json so a default one is being created
		systemconfigfile, err := os.Create("systemconfig.json")
		if err != nil {
			return rawBytes, serviceList, err
		}
		defer systemconfigfile.Close()
		systemconfigjson, err := json.MarshalIndent(sys2file, "", "   ")
		if err != nil {
			return rawBytes, serviceList, err
		}
		nbytes, err := systemconfigfile.Write(systemconfigjson)
		if err != nil {
			return rawBytes, serviceList, err
		}
		return rawBytes, serviceList, fmt.Errorf("a new configuration file has been written with %d bytes. Please update it and restart the system", nbytes)
	}

	// the system configuration file could be open, read the configurations and pass them on
	defer systemconfigfile.Close()
	configBytes, err := os.ReadFile("systemconfig.json")
	if err != nil {
		return rawBytes, serviceList, err
	}

	// the challenge is that the definition of the unit asset is unknown to the mbaigo library and only known to the system that invokes the library
	var configurationIn ConfigFile2Sys
	// extract the information related to the system separately from the unit_assets (i.e., the resources)
	type Alias ConfigFile2Sys
	aux := &struct {
		Resources []json.RawMessage `json:"unit_assets"`
		*Alias
	}{
		Alias: (*Alias)(&configurationIn),
	}
	if err := json.Unmarshal(configBytes, aux); err != nil {
		return rawBytes, serviceList, err
	}
	if len(aux.Resources) > 0 {
		configurationIn.rawResources = aux.Resources
	} else {
		var rawMessages []json.RawMessage
		for _, s := range sys2file.UAsset {
			// convert the struct to JSON-encoded byte array
			jsonBytes, err := json.Marshal(s)
			if err != nil {
				fmt.Println("Failed to marshal struct:", err)
			}
			rawMessages = append(rawMessages, json.RawMessage(jsonBytes)) // append the json.RawMessage to the slice
		}
		configurationIn.rawResources = rawMessages
	}

	sys.Name = configurationIn.CName
	sys.Husk.DName = configurationIn.PKIdetails
	sys.Husk.ProtoPort = configurationIn.Protocols
	for _, ccore := range configurationIn.CCoreS {
		newCore := ccore
		sys.CoreS = append(sys.CoreS, &newCore)
	}

	// update the services (e.g., re-registration period)
	// (the slices might have their order jumbled, so have to sort them so the indices matches correctly)
	sortServicesList(configurationIn.CServices)
	sortServicesList(originalSs)
	for i := range configurationIn.CServices {
		(configurationIn.CServices)[i].Merge(&originalSs[i])
	}
	serviceList = configurationIn.CServices

	return configurationIn.rawResources, serviceList, nil
}

// getFirstAsset returns the first key-value pair in the Assets map
func getFirstAsset(assetMap map[string]*components.UnitAsset) []components.UnitAsset {
	var assetList []components.UnitAsset
	for key := range assetMap {
		assetList = append(assetList, *assetMap[key])
		return assetList
	}
	return assetList
}

// getServicesList() returns the original list of services
func getServicesList(uat components.UnitAsset) []components.Service {
	var serviceList []components.Service
	services := uat.GetServices()
	for s := range services {
		serviceList = append(serviceList, *services[s])
	}
	return serviceList
}

// sortServicesList sorts a list of services, based on the definition of each service
func sortServicesList(list []components.Service) {
	sort.Slice(list, func(i, j int) bool { return list[i].Definition < list[j].Definition })
}
