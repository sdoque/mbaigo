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

// The "forms" package is designed to define structured schemas, known as "structs,"
// which represent the format and organization of documents intended for data exchange.
// These structs are utilized to create forms that are populated with data, acting as
// standardized payloads for transmission between different systems. This ensures that
// the data exchanged maintains a consistent structure, facilitating seamless
// integration and processing across system boundaries.
// Basic forms include the service registration and the service query forms.
// The form version is used for backward compatibility.

// the ontology forms are used to generate a semantic model of the system, device it is running on, its unit assets and services they offer

package usecases

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sdoque/mbaigo/components"
)

// function KGraphing provides a semantic model of a system running on a host and exposing the functionality of asset
func KGraphing(w http.ResponseWriter, req *http.Request, sys *components.System) {

	rdf := prefixes()
	rdf += modelSystem(sys)
	rdf += modelHusk(sys)
	rdf += modelEndpoints(sys)
	rdf += modelHost(sys)
	rdf += modelUAsset(sys)

	w.Header().Set("Content-Type", "text/turtle")
	_, err := w.Write([]byte(rdf))
	if err != nil {
		log.Println("Failed to write KGraphing information: ", err)
	}
}

func prefixes() (description string) {
	description = "@prefix alc: <http://www.synecdoque.com/lcloud/> .\n"
	description += "@prefix afo: <http://www.synecdoque.com/2025/afo#> .\n"
	description += "@prefix owl: <http://www.w3.org/2002/07/owl#> .\n"
	description += "@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .\n"
	description += "@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .\n"
	description += "@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .\n\n"
	return
}

// finalizeBlock removes any trailing semicolon from a block of predicate/object
// lines and appends the final " .\n\n" so the Turtle is syntactically correct.
func finalizeBlock(block string) string {
	// Remove trailing whitespace first
	block = strings.TrimRight(block, " \t\r\n")

	// If it ends with ';', remove it and any trailing spaces before it
	if strings.HasSuffix(block, ";") {
		block = strings.TrimSuffix(block, ";")
		block = strings.TrimRight(block, " \t")
	}

	return block + " .\n\n"
}

// endpointLocalName builds a local name for an Endpoint instance based on
// host, system, protocol, and port, so we can refer to the same endpoint
// from Husk and Service descriptions.
func endpointLocalName(sys *components.System, protocol string, port int) string {
	return fmt.Sprintf("%s_%s_%s_%d_Endpoint",
		sys.Husk.Host.Name,
		sys.Name,
		protocol,
		port,
	)
}

// modelSystem creates a knowledge graph of the system that aggregates the husk and unit assets
func modelSystem(sys *components.System) (systemModel string) {
	sName := sys.Husk.Host.Name + "_" + sys.Name
	systemModel = fmt.Sprintf("alc:%s a afo:System ;\n", sName)
	systemModel += fmt.Sprintf("    afo:hasName \"%s\" ;\n", sys.Name)

	// The Husk instance is in the alc: namespace, not afo:
	systemModel += fmt.Sprintf("    afo:hasHusk alc:%s_Husk ;\n", sName)

	// --- NEW: LocalCloud is stored in the Husk details for ServiceRegistrar systems ---
	// It is expected that only the ServiceRegistrar systems have this key and all
	// have the same name (if not, the KGrapher will use the first one it finds).
	if values, ok := sys.Husk.Details["LocalCloud"]; ok && len(values) > 0 {
		v := values[0]
		if !(strings.HasPrefix(v, "<") && strings.HasSuffix(v, ">")) && !strings.HasPrefix(v, "alc:") {
			v = "alc:" + v
		}
		systemModel += fmt.Sprintf("    afo:isContainedIn %s ;\n", v)
	}
	// --- END NEW ---

	for assetName := range sys.UAssets {
		systemModel += fmt.Sprintf("    afo:hasUnitAsset alc:%s_%s ;\n", sName, assetName)
	}

	systemModel = finalizeBlock(systemModel)
	return
}

// modelHusk creates a knowledge graph of the husk that wraps the unit assets
func modelHusk(sys *components.System) string {
	sName := sys.Husk.Host.Name + "_" + sys.Name
	huskModel := fmt.Sprintf("alc:%s_Husk a afo:Husk ;\n", sName)

	// Host IRI is just alc:<HostName>, not alc:<HostName>_Host
	huskModel += fmt.Sprintf("    afo:runsOnHost alc:%s ;\n", sys.Husk.Host.Name)

	// For each protocol/port pair, link the Husk to an Endpoint instance
	for protocol, port := range sys.Husk.ProtoPort {
		if port == 0 {
			continue
		}
		eName := endpointLocalName(sys, protocol, port)
		huskModel += fmt.Sprintf("    afo:communicatesOver alc:%s ;\n", eName)
	}

	details := sys.Husk.Details
	for key, values := range details {
		// LocalCloud is now handled on the System level
		if key == "LocalCloud" {
			continue
		}
		for _, value := range values {
			if !(strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")) {
				value = "alc:" + value
			}
			huskModel += fmt.Sprintf("    afo:has%s %s ;\n", key, value)
		}
	}

	huskModel = finalizeBlock(huskModel)
	return huskModel
}

// modelHost creates a knowledge graph of the hosting computer
func modelHost(sys *components.System) string {
	hostModel := fmt.Sprintf("alc:%s a afo:Host ;\n", sys.Husk.Host.Name)
	hostModel += fmt.Sprintf("    afo:hasName \"%s\" ;\n", sys.Husk.Host.Name)

	ipaLen := len(sys.Husk.Host.IPAddresses)
	ipaCount := 0
	for _, ipa := range sys.Husk.Host.IPAddresses {
		hostModel += fmt.Sprintf("    afo:hasIPaddress \"%s\"", ipa)
		ipaCount++
		if ipaCount < ipaLen {
			hostModel += " ;\n"
		}
	}

	hostModel = finalizeBlock(hostModel)
	return hostModel
}

// modelEndpoints creates a knowledge graph of the (host, protocol, port)
// combinations as first-class afo:Endpoint instances.
func modelEndpoints(sys *components.System) string {
	var endpointModels string

	for protocol, port := range sys.Husk.ProtoPort {
		if port == 0 {
			continue
		}

		eName := endpointLocalName(sys, protocol, port)
		var endpointModel string

		endpointModel += fmt.Sprintf("alc:%s a afo:Endpoint ;\n", eName)
		endpointModel += fmt.Sprintf("    afo:usesProtocol \"%s\" ;\n", protocol)
		endpointModel += fmt.Sprintf("    afo:usesPort %d ;\n", port)
		endpointModel += fmt.Sprintf("    afo:onHost alc:%s ;\n", sys.Husk.Host.Name)
		// Optional: base path if you want it (/%system name%)
		// endpointModel += fmt.Sprintf("    afo:hasBasePath \"/%s\" ;\n", sys.Name)

		endpointModel = finalizeBlock(endpointModel)
		endpointModels += endpointModel
	}

	return endpointModels
}

// modelUAsset creates a knowledge graph of each unit assets and its consumed and provided services
func modelUAsset(sys *components.System) string {
	sName := sys.Husk.Host.Name + "_" + sys.Name
	var assetModels string

	for assetName, asset := range sys.UAssets {
		var assetModel string

		assetModel += fmt.Sprintf("alc:%s_%s a afo:UnitAsset ;\n", sName, assetName)
		assetModel += fmt.Sprintf("    afo:hasName \"%s\" ;\n", assetName)
		if (*asset).Mission != "" {
			assetModel += fmt.Sprintf("    afo:hasMission \"%s\" ;\n", (*asset).Mission)
		}

		details := (*asset).GetDetails()
		for key, values := range details {
			fmt.Printf("key: %s, values: %v\n", key, values)
			if strings.HasSuffix(key, ":") {
				for _, value := range values {
					if value == "" {
						log.Printf("Warning: empty value for key '%s' in asset '%s'. Skipping.", key, assetName)
						continue
					}
					relationship := value[0] // byte
					reference := value[1:]   // string (from second character onward)

					switch relationship {
					case '=': // single quotes for byte comparison
						assetModel += fmt.Sprintf("    owl:sameAs %s ;\n", reference)
					}
				}
				continue
			}
			for _, value := range values {
				if !(strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")) {
					value = "alc:" + value
				}
				assetModel += fmt.Sprintf("    alc:has%s %s ;\n", key, value)
			}
		}

		cervices := (*asset).GetCervices()
		for _, cervice := range cervices {
			assetModel += fmt.Sprintf("    afo:consumesService alc:%s_%s_%s ;\n", sName, assetName, cervice.Definition)
		}

		services := (*asset).GetServices()
		servicesLen := len(services)
		serviceCount := 0
		for _, service := range services {
			// Use service.Definition for the IRI, so it matches the Service block
			assetModel += fmt.Sprintf("    afo:providesService alc:%s_%s_%s", sName, assetName, service.Definition)
			serviceCount++
			if serviceCount < servicesLen {
				assetModel += " ;\n"
			}
		}

		assetModel = finalizeBlock(assetModel)
		assetModels += assetModel

		assetModels += modelCervices(sName, asset)
		assetModels += modelServices(sName, asset, sys)
	}

	return assetModels
}

// modelCervices creates a knowledge graph of the consumed services of a unit asset
func modelCervices(sName string, ua *components.UnitAsset) string {
	var cervicesModel string
	asset := *ua
	cervices := asset.GetCervices()

	for _, cervice := range cervices {
		var cerviceModel string

		cerviceModel += fmt.Sprintf("alc:%s_%s_%s a afo:ConsumedService ;\n",
			sName, asset.GetName(), cervice.Definition)
		cerviceModel += fmt.Sprintf("    afo:consumes \"%s\" ;\n", cervice.Definition)
		if cervice.Mode != "" {
			cerviceModel += fmt.Sprintf("    afo:hasMode \"%s\" ;\n", cervice.Mode)
		}

		details := cervice.Details
		for key, values := range details {
			for _, value := range values {
				if !(strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")) {
					value = "alc:" + value
				}
				cerviceModel += fmt.Sprintf("    alc:has%s %s ;\n", key, value)
			}
		}

		for pName, nodes := range cervice.Nodes {
			cerviceModel += fmt.Sprintf("    afo:consumes alc:%s ;\n", pName)
			for _, ni := range nodes {
				cerviceModel += fmt.Sprintf("    afo:fromUrl <%s> ;\n", ni.URL)
			}
		}

		cerviceModel = finalizeBlock(cerviceModel)

		// FIX: accumulate this block
		cervicesModel += cerviceModel
	}

	return cervicesModel
}

// modelServices creates a knowledge graph of the services provided by a unit asset
func modelServices(sName string, ua *components.UnitAsset, sys *components.System) string {
	var servicesModel string
	asset := *ua
	assetName := asset.GetName()
	services := asset.GetServices()

	for _, service := range services {
		var serviceModel string

		// IRI is based on service.Definition, matching UnitAsset's providesService
		serviceModel += fmt.Sprintf("alc:%s_%s_%s a afo:Service ;\n", sName, assetName, service.Definition)
		serviceModel += fmt.Sprintf("    afo:hasName \"%s/%s\" ;\n", assetName, service.Definition)
		serviceModel += fmt.Sprintf("    afo:hasServiceDefinition \"%s\" ;\n", service.Definition)

		// For each protocol/port, link to the Endpoint and give a URL
		for protocol, port := range sys.Husk.ProtoPort {
			if port == 0 {
				continue
			}

			eName := endpointLocalName(sys, protocol, port)
			serviceModel += fmt.Sprintf("    afo:hostedOnEndpoint alc:%s ;\n", eName)

			addr := protocol + "://" + sys.Husk.Host.IPAddresses[0] + ":" +
				strconv.Itoa(port) + "/" + sys.Name + "/" + assetName + "/" + service.SubPath
			serviceModel += fmt.Sprintf("    afo:hasUrl <%s> ;\n", addr)
		}

		// Additional details
		details := service.Details
		for key, values := range details {
			for _, value := range values {
				if !(strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")) {
					value = "alc:" + value
				}
				serviceModel += fmt.Sprintf("    alc:has%s  %s ;\n", key, value)
			}
		}

		serviceModel += fmt.Sprintf("    afo:isSubscribAble \"%t\"^^xsd:boolean ;\n", service.SubscribeAble)
		if service.CFootprint != 0 {
			serviceModel += fmt.Sprintf("    afo:hasCarbonFootprint \"%.6f\"^^xsd:decimal ;\n", service.CFootprint)
		}
		if service.CUnit != "" {
			serviceModel += fmt.Sprintf("    afo:hasCost \"%.2f\"^^xsd:decimal ;\n", service.ACost)
			serviceModel += fmt.Sprintf("    afo:hasCostUnit \"%s\"^^xsd:string ;\n", service.CUnit)
		}
		serviceModel += fmt.Sprintf("    afo:hasRegistrationPeriod %d ;\n", service.RegPeriod)

		// Let finalizeBlock remove the trailing ';' and close the block with " ."
		serviceModel = finalizeBlock(serviceModel)
		servicesModel += serviceModel
	}

	return servicesModel
}
