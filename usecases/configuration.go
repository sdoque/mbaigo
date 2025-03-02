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

	"github.com/sdoque/mbaigo/components"
)

// templateOut is the stuct used to prepare the systemconfig.json file
type templateOut struct {
	CName      string                  `json:"systemname"`
	UAsset     []components.UnitAsset  `json:"unit_assets"`
	CServices  []components.Service    `json:"services"`
	Protocols  map[string]int          `json:"protocolsNports"`
	PKIdetails pkix.Name               `json:"distinguishedName"`
	CCoreS     []components.CoreSystem `json:"coreSystems"`
}

// configFileIn is used to extact out the information of the systemconfig.json file
// Since it does not know about the details of the Thing, it does not unmarsahll this
// information
type configFileIn struct {
	CName        string                  `json:"systemname"`
	rawResources []json.RawMessage       `json:"-"`
	CServices    []components.Service    `json:"services"`
	Protocols    map[string]int          `json:"protocolsNports"`
	PKIdetails   pkix.Name               `json:"distinguishedName"`
	CCoreS       []components.CoreSystem `json:"coreSystems"`
}

// Configure read the system configuration JSON file to get the deployment details.
// If the file is missing, it generates a default systemconfig.json file and shuts down the system
func Configure(sys *components.System) ([]json.RawMessage, []components.Service, error) {

	var rawBytes []json.RawMessage        // the mbaigo library does not know about the unit asset's structure (defined in the file thing.go and not part of the library)
	var servicesList []components.Service // this is the list of services for each unit asset
	// prepare content of configuration file
	var defaultConfig templateOut

	defaultConfig.CName = sys.Name
	defaultConfig.Protocols = sys.Husk.ProtoPort
	defaultConfig.UAsset = getFirstAsset(sys.UAssets)
	originalSs := getServicesList(defaultConfig.UAsset[0])
	defaultConfig.CServices = originalSs

	defaultConfig.PKIdetails.CommonName = "arrowhead.eu"
	defaultConfig.PKIdetails.Country = []string{"SE"}
	defaultConfig.PKIdetails.Province = []string{"Norrbotten"}
	defaultConfig.PKIdetails.Locality = []string{"Luleaa"}
	defaultConfig.PKIdetails.Organization = []string{"Luleaa University of Technology"}
	defaultConfig.PKIdetails.OrganizationalUnit = []string{"CPS"}

	serReg := components.CoreSystem{
		Name:        "serviceregistrar",
		Url:         "http://localhost:20102/serviceregistrar/registry",
		Certificate: ".X509pubKey",
	}
	orches := components.CoreSystem{
		Name:        "orchestrator",
		Url:         "http://localhost:20103/orchestrator/orchestration",
		Certificate: ".X509pubKey",
	}
	ca := components.CoreSystem{
		Name:        "ca",
		Url:         "http://localhost:20100/ca/certification",
		Certificate: ".X509pubKey",
	}
	coreSystems := []components.CoreSystem{serReg, orches, ca}
	defaultConfig.CCoreS = coreSystems

	// open the configuration file or create one with the default content prepared above
	systemConfigFile, err := os.Open("systemconfig.json")

	if err != nil { // could not find the systemconfig.json so a default one is being created
		defaultConfigFile, err := os.Create("systemconfig.json")
		if err != nil {
			return rawBytes, servicesList, err
		}
		defer defaultConfigFile.Close()
		systemconfigjson, err := json.MarshalIndent(defaultConfig, "", "   ")
		if err != nil {
			return rawBytes, servicesList, err
		}
		nBytes, err := defaultConfigFile.Write(systemconfigjson)
		if err != nil {
			return rawBytes, servicesList, err
		}
		return rawBytes, servicesList, fmt.Errorf("a new configuration file has been written with %d bytes. Please update it and restart the system", nBytes)
	}

	// the system configuration file could be open, read the configurations and pass them on to the system
	defer systemConfigFile.Close()
	configBytes, err := os.ReadFile("systemconfig.json")
	if err != nil {
		return rawBytes, servicesList, err
	}

	// the challenge is that the definition of the unit asset is unknown to the mbaigo library and only known to the system that invokes the library
	var configurationIn configFileIn
	// extract the information related to the system separately from the unit_assets (i.e., the resources)
	type Alias configFileIn
	aux := &struct {
		Resources []json.RawMessage `json:"unit_assets"`
		*Alias
	}{
		Alias: (*Alias)(&configurationIn),
	}
	if err := json.Unmarshal(configBytes, aux); err != nil {
		return rawBytes, servicesList, err
	}
	if len(aux.Resources) > 0 {
		configurationIn.rawResources = aux.Resources
	} else {
		var rawMessages []json.RawMessage
		for _, s := range defaultConfig.UAsset {
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

	// update the services (e.g., re-registration period, costs, or units)
	for i := range configurationIn.CServices {
		for _, originalService := range originalSs {
			if originalService.Definition == configurationIn.CServices[i].Definition {
				configurationIn.CServices[i].Merge(&originalService) // keep the original definition and subpath as the original ones
			}
		}
	}
	servicesList = configurationIn.CServices

	return configurationIn.rawResources, servicesList, nil
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
