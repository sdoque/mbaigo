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
	"log"
	"os"

	"github.com/sdoque/mbaigo/components"
)

// configurableAsset is a struct that contains the name of the asset and its
// configurable details and services
type ConfigurableAsset struct {
	Name string `json:"name"`
	// The validated type, not the text. A configuration naming a mission that
	// does not exist is refused as the file is read, where the message can name
	// the file and the field, rather than carried inward to be refused later as
	// an authorization question.
	Mission  components.Mission   `json:"mission,omitempty"`
	Details  map[string][]string  `json:"details"`
	Services []components.Service `json:"services"`
	Traits   []json.RawMessage    `json:"traits"`
}

// templateOut is the struct used to prepare the systemconfig.json file
type templateOut struct {
	CName       string                  `json:"systemname"`
	LocalCloud  string                  `json:"localcloud,omitempty"`
	IPAddresses []string                `json:"ipAddresses"`
	Assets      []ConfigurableAsset     `json:"unit_assets"`
	Protocols   map[string]int          `json:"protocolsNports"`
	CCoreS      []components.CoreSystem `json:"coreSystems"`
}

// configFileIn is used to extract out the information of the systemconfig.json file
// Since it does not know about the details of the Thing, it does not unmarsahll this
// information
type configFileIn struct {
	CName       string                  `json:"systemname"`
	LocalCloud  string                  `json:"localcloud,omitempty"`
	IPAddresses []string                `json:"ipAddresses"`
	Protocols   map[string]int          `json:"protocolsNports"`
	CCoreS      []components.CoreSystem `json:"coreSystems"`
	Resources   []json.RawMessage       `json:"unit_assets"`
}

var ErrNewConfig = errors.New("new config file was created")

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
		Mission:  assetTemplate.Mission,
		Details:  assetTemplate.GetDetails(),
		Services: servicesTemplate,
	}

	// If the asset exposes traits, serialize them and store as raw JSON
	if traits := assetTemplate.GetTraits(); traits != nil {
		traitJSON, err := json.Marshal(traits)
		if err != nil {
			return templateOut{}, fmt.Errorf("couldn't marshal traits: %v", err)
		}
		confAsset.Traits = []json.RawMessage{traitJSON}
	}

	// prepare content of configuration file
	defaultConfig.CName = sys.Name
	for key, values := range sys.Husk.Details { // if the system has a LocalCloud detail, add it to the config file
		if key == "LocalCloud" && len(values) > 0 {
			defaultConfig.LocalCloud = values[0]
			break
		}
	}
	defaultConfig.IPAddresses = sys.Husk.Host.IPAddresses
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

	// add the core systems to the configuration file.
	// maitreD is intentionally NOT listed here: no system dereferences a
	// maitreD URL via the coreSystems map. The CA reaches the requester's
	// host-local maitreD by combining the source IP of the inbound CSR with
	// its own MaitreDPort trait (see systems/ca/thing.go), not via this list.
	coreSystems := []components.CoreSystem{servReg, orches, ca}
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
		return nil, fmt.Errorf("error while opening/creating systemconfig file: %v", err)
	}
	defer systemConfigFile.Close()

	fileInfo, err := systemConfigFile.Stat() // *.Stat() returns fileInfo/stats
	if err != nil {
		return nil, fmt.Errorf("error occurred while getting config file stats: %s", err)
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

	// A configuration written before its system declared missions has assets
	// without one, and the framework refuses to start a system whose assets do
	// not classify themselves. Left alone, adding a mission to a system would
	// stop every deployment of it that already exists: the template is written
	// only when there is no systemconfig.json and is never merged into one that
	// is already there, so the file cannot correct itself.
	//
	// The value comes from the system author's own template, which is what the
	// file would have been seeded with had it been written today. That is a
	// different thing from defaulting a blank field: nothing is guessed, and an
	// asset the template does not know about is still refused.
	rawResources = fillMissionsFromTemplates(sys, rawResources)
	rawResources = fillServicesFromTemplates(sys, rawResources)

	sys.Name = configurationIn.CName
	// Restore IP addresses from config, allowing operators to limit which address is used.
	if len(configurationIn.IPAddresses) > 0 {
		sys.Husk.Host.IPAddresses = configurationIn.IPAddresses
	}
	// If the systemconfig file has a LocalCloud defined, add it to the system details
	if configurationIn.LocalCloud != "" {
		if sys.Husk.Details == nil {
			sys.Husk.Details = make(map[string][]string)
		}
		sys.Husk.Details["LocalCloud"] = []string{configurationIn.LocalCloud}
	}
	sys.Husk.ProtoPort = configurationIn.Protocols
	for _, ccore := range configurationIn.CCoreS {
		newCore := ccore
		sys.Husk.CoreS = append(sys.Husk.CoreS, &newCore)
	}

	return rawResources, nil
}

// soleTemplate returns the system's only template asset, or nil if it has more
// than one and so cannot speak for an asset it does not name.
func soleTemplate(sys *components.System) *components.UnitAsset {
	if len(sys.UAssets) != 1 {
		return nil
	}
	for _, ua := range sys.UAssets {
		return ua
	}
	return nil
}

// capabilities are the service fields a running system decides, not a file.
//
// Whether a service can be followed depends on whether this build's code
// publishes to it; an operator cannot make that true or false by editing JSON,
// and a configuration written before the field existed says false by omission —
// which is indistinguishable from an operator having turned it off, so the
// template is taken as the answer.
var capabilities = map[string]bool{
	"subscribable": true,
}

// fillServicesFromTemplates gives a configured service whatever its system's
// template says and its configuration file does not.
//
// A systemconfig.json is written once, when there is none, and never merged
// into afterwards. So every field added to a service since a deployment was
// commissioned is missing from that deployment's file, and every service added
// since is missing altogether. That has cost this project four separate
// deployments: a files service that never appeared, a syslist the registrar
// served without declaring, missions that stopped systems from starting, and a
// temperature that registered as unfollowable because the file predated the
// word.
//
// Two kinds of field, treated differently. A capability comes from the template,
// because it describes what the code can do. Everything else is a deployment's
// own business and the file wins wherever it says anything at all — a threshold
// or a heartbeat an operator tuned is not overwritten by a release.
func fillServicesFromTemplates(sys *components.System, raws []json.RawMessage) []json.RawMessage {
	for i, raw := range raws {
		var asset map[string]json.RawMessage
		if err := json.Unmarshal(raw, &asset); err != nil {
			continue
		}
		var name string
		_ = json.Unmarshal(asset["name"], &name)

		template, known := sys.UAssets[name]
		if !known {
			template = soleTemplate(sys)
		}
		if template == nil {
			continue
		}

		var configured []map[string]json.RawMessage
		if raw, present := asset["services"]; present {
			if err := json.Unmarshal(raw, &configured); err != nil {
				continue
			}
		}

		bySubPath := map[string]int{}
		for at, serv := range configured {
			var subPath string
			_ = json.Unmarshal(serv["subpath"], &subPath)
			bySubPath[subPath] = at
		}

		changed := false
		for _, offered := range getServicesList(*template) {
			fields, err := asFields(offered)
			if err != nil {
				continue
			}
			at, present := bySubPath[offered.SubPath]
			if !present {
				// A service this build serves and the file has never heard of.
				// Left out, it is a path the system answers on and the cloud
				// cannot discover, authorize or reason about.
				configured = append(configured, fields)
				changed = true
				continue
			}
			for key, value := range fields {
				_, stated := configured[at][key]
				if stated && !capabilities[key] {
					continue // the file has its own answer and is entitled to it
				}
				if !stated || !sameJSON(configured[at][key], value) {
					configured[at][key] = value
					changed = true
				}
			}
		}
		if !changed {
			continue
		}

		services, err := json.Marshal(configured)
		if err != nil {
			continue
		}
		asset["services"] = services
		patched, err := json.Marshal(asset)
		if err != nil {
			continue
		}
		raws[i] = patched
	}
	return raws
}

// asFields renders one template service as the fields a configuration file
// would carry for it, so the two can be compared key by key.
func asFields(serv components.Service) (map[string]json.RawMessage, error) {
	encoded, err := json.Marshal(serv)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

// sameJSON reports whether two encoded values say the same thing.
func sameJSON(a, b json.RawMessage) bool {
	return string(a) == string(b)
}

// fillMissionsFromTemplates supplies a mission to a configured asset that has
// none, taking it from the template asset of the same name.
//
// The templates are in sys.UAssets: a system puts them there before it calls
// Configure, which is what lets this be done once here rather than in each of
// the systems.
//
// Announced rather than silent. The operator's file is not rewritten — the next
// release could declare something different — so saying which asset was filled
// in, and with what, is what lets them put it in the file themselves.
func fillMissionsFromTemplates(sys *components.System, raws []json.RawMessage) []json.RawMessage {
	for i, raw := range raws {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			continue // not an object; leave it for the system to reject
		}
		var name, mission string
		_ = json.Unmarshal(fields["name"], &name)
		_ = json.Unmarshal(fields["mission"], &mission)
		if mission != "" {
			continue
		}
		template, known := sys.UAssets[name]
		if !known {
			// Named for the instance rather than the template, which is the
			// documented commissioning step for whole families of systems: a
			// ds18b20 asset is named after its 1-wire identifier, a telegrapher
			// asset after its MQTT topic. Keying on the name alone therefore
			// missed exactly the deployments this exists for, and they are the
			// ones that fail on upgrade.
			//
			// A system with one template has only one thing an asset could be,
			// so the mission is not in doubt. A system with several does not get
			// a guess: which template a renamed asset came from is not knowable
			// from here, and inventing a mission is worse than refusing to
			// start, because it is what the authorizer classifies the asset by.
			template = soleTemplate(sys)
			if template == nil {
				continue
			}
		}
		if template == nil {
			continue
		}
		from := template.Mission
		if from.IsZero() {
			continue
		}
		filled, err := json.Marshal(from)
		if err != nil {
			continue
		}
		fields["mission"] = filled
		patched, err := json.Marshal(fields)
		if err != nil {
			continue
		}
		raws[i] = patched
		log.Printf("%s: unit asset %q has no mission in systemconfig.json; using %q "+
			"from this system's template. Add \"mission\": %q to the asset to "+
			"declare it yourself.\n", sys.Name, name, from, from)
	}
	return raws
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
