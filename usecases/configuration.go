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
	"errors"
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
	CName     string                  `json:"systemname"`
	Protocols map[string]int          `json:"protocolsNports"`
	CCoreS    []components.CoreSystem `json:"coreSystems"`
	Resources []json.RawMessage       `json:"unit_assets"`
}

var ErrNewConfig = errors.New("A new configuration file has been created. Please update it and restart the system")

func setupDefaultConfig(sys *components.System) (defaultConfig templateOut, err error) {
	var assetTemplate components.UnitAsset
	if sys.UAssets == nil {
		return templateOut{}, fmt.Errorf("unitAssets missing")
	}

	for _, ua := range sys.UAssets {
		assetTemplate = *ua // this creates a copy (value, not reference)
		break
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
			if err != nil {
				return templateOut{}, fmt.Errorf("couldn't marshal traits: %v", err)
			}
			confAsset.Traits = []json.RawMessage{traitJSON}
		}
	}

	// prepare content of configuration file
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
	return defaultConfig, nil
}

// Configure reads the system configuration JSON file to get the deployment details.
// If the file is missing, it generates a default systemconfig.json file and shuts down the system
func Configure(sys *components.System) ([]json.RawMessage, error) {
	defaultConfig, err := setupDefaultConfig(sys)
	if err != nil {
		return nil, fmt.Errorf("couldn't create default config: %v", err)
	}

	// 0600 allows user Read/Write permission (secure config file), but no R/W for groups and others, 0644 to allow R/W on sudo and only R on groups/others, 0666 for R/W permissions for everyone
	systemConfigFile, err := os.OpenFile("systemconfig.json", os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("error while opening/creating systemconfig file, check permissions on config file")
	}
	defer systemConfigFile.Close()

	fileInfo, err := systemConfigFile.Stat() // *.Stat() returns fileInfo/stats
	if err != nil {
		return nil, fmt.Errorf("error occurred while getting config file stats")
	}
	if fileInfo.Size() == 0 { // *.Size() returns the filesize (number bytes) as an int, 0 is an empty file
		enc := json.NewEncoder(systemConfigFile)
		enc.SetIndent("", "    ")
		err = enc.Encode(defaultConfig) // Write default values into systemconfig since file was empty
		if err != nil {
			return nil, fmt.Errorf("error writing default values to system config: %v", err)
		}
		return nil, ErrNewConfig
	}

	var configurationIn configFileIn
	err = json.NewDecoder(systemConfigFile).Decode(&configurationIn) // Read the contents of systemconfig into configurationIn
	if err != nil {
		return nil, fmt.Errorf("error reading systemconfig: %v", err)
	}

	var rawResources []json.RawMessage
	if len(configurationIn.Resources) > 0 { // If unit assets was present in systemconfig file, send those
		rawResources = configurationIn.Resources
	} else {
		for _, s := range defaultConfig.Assets { // Otherwise send the system default
			jsonBytes, err := json.Marshal(s)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal struct: %v", err)
			}
			rawResources = append(rawResources, json.RawMessage(jsonBytes))
		}
	}

	sys.Name = configurationIn.CName
	sys.Husk.ProtoPort = configurationIn.Protocols
	for _, ccore := range configurationIn.CCoreS {
		newCore := ccore
		sys.CoreS = append(sys.CoreS, &newCore)
	}

	return rawResources, nil
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
