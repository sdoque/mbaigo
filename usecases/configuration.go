/*******************************************************************************
 * Copyright (c) 2025 Synecdoque
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
	"encoding/json"
	"fmt"
	"os"

	"github.com/sdoque/mbaigo/components"
)

// configurableAsset is a struct that contains the name of the asset and its
// configurable details and services
type ConfigurableAsset struct {
	Name     string               `json:"name"`
	Details  map[string][]string  `json:"details"`
	Services []components.Service `json:"services"`
	Traits   []json.RawMessage    `json:"traits"`
}

// templateOut is the struct used to prepare the systemconfig.json file
type templateOut struct {
	CName     string                  `json:"systemname"`
	Assets    []ConfigurableAsset     `json:"unit_assets"`
	Protocols map[string]int          `json:"protocolsNports"`
	CCoreS    []components.CoreSystem `json:"coreSystems"`
}

// configFileIn is used to extract out the information of the systemconfig.json file
// Since it does not know about the details of the Thing, it does not unmarsahll this
// information
type configFileIn struct {
	CName        string                  `json:"systemname"`
	rawResources []json.RawMessage       `json:"-"`
	Protocols    map[string]int          `json:"protocolsNports"`
	CCoreS       []components.CoreSystem `json:"coreSystems"`
}

// Configure reads the system configuration JSON file to get the deployment details.
// If the file is missing, it generates a default systemconfig.json file and shuts down the system
func Configure(sys *components.System) ([]json.RawMessage, error) {
	// prepare content of configuration file
	var defaultConfig templateOut

	// var servicesList []components.Service // this is the list of services for each unit asset

	var assetTemplate components.UnitAsset
	for _, ua := range sys.UAssets {
		assetTemplate = *ua // this creates a copy (value, not reference)
		break               // stop after the first entry
	}
	servicesTemplate := getServicesList(assetTemplate)

	confAsset := ConfigurableAsset{
		Name:     assetTemplate.GetName(),
		Details:  assetTemplate.GetDetails(),
		Services: servicesTemplate,
	}

	// If the asset exposes traits, serialize them and store as raw JSON
	if assetWithTraits, ok := assetTemplate.(components.HasTraits); ok {
		if traits := assetWithTraits.GetTraits(); traits != nil {
			traitJSON, err := json.Marshal(traits)
			if err == nil {
				confAsset.Traits = []json.RawMessage{traitJSON}
			} else {
				fmt.Println("Warning: could not marshal traits:", err)
			}
		}
	}

	defaultConfig.CName = sys.Name
	defaultConfig.Protocols = sys.Husk.ProtoPort
	defaultConfig.Assets = []ConfigurableAsset{confAsset} // this is a list of unit assets

	servReg := components.CoreSystem{
		Name: "serviceregistrar",
		Url:  "http://localhost:20102/serviceregistrar/registry",
	}
	orches := components.CoreSystem{
		Name: "orchestrator",
		Url:  "http://localhost:20103/orchestrator/orchestration",
	}
	ca := components.CoreSystem{
		Name: "ca",
		Url:  "http://localhost:20100/ca/certification",
	}
	maitreD := components.CoreSystem{
		Name: "maitreD",
		Url:  "http://localhost:20101/maitreD/maitreD",
	}
	// add the core systems to the configuration file
	// the system is part of a local cloud with mandatory core systems
	coreSystems := []components.CoreSystem{servReg, orches, ca, maitreD}
	defaultConfig.CCoreS = coreSystems

	var rawBytes []json.RawMessage // the mbaigo library does not know about the unit asset's structure (defined in the file thing.go and not part of the library)

	systemConfigFile, err := os.Open("systemconfig.json")
	if err != nil { // could not find the systemconfig.json so a default one is being created
		defaultConfigFile, err := os.Create("systemconfig.json")
		if err != nil {
			return rawBytes, err
		}
		defer defaultConfigFile.Close()

		enc := json.NewEncoder(defaultConfigFile) // Create an encoder that allows writing to a file
		enc.SetIndent("", "     ")                // Set proper indentation
		err = enc.Encode(defaultConfig)           // Write defaultConfig template to file
		if err != nil {
			return nil, fmt.Errorf("jsonEncode: %w", err)
		}
		return nil, fmt.Errorf("a new configuration file has been created. Please update it and restart the system")
	}

	// the system configuration file could be open, read the configurations and pass them on to the system
	defer systemConfigFile.Close()
	configBytes, err := os.ReadFile("systemconfig.json")
	if err != nil {
		return rawBytes, err
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
		return rawBytes, err
	}
	if len(aux.Resources) > 0 {
		configurationIn.rawResources = aux.Resources
	} else {
		var rawMessages []json.RawMessage
		for _, s := range defaultConfig.Assets {
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
	sys.Husk.ProtoPort = configurationIn.Protocols
	for _, ccore := range configurationIn.CCoreS {
		newCore := ccore
		sys.CoreS = append(sys.CoreS, &newCore)
	}

	return configurationIn.rawResources, nil
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

// MakeServiceMap() creates a map of services from a slice of services
// The map is indexed by the service subpath
func MakeServiceMap(services []components.Service) map[string]*components.Service {
	serviceMap := make(map[string]*components.Service)
	for i := range services {
		svc := services[i] // take the address of the element in the slice
		serviceMap[svc.SubPath] = &svc
	}
	return serviceMap
}
