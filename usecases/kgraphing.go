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
	"net/http"
	"strconv"
	"strings"

	"github.com/sdoque/mbaigo/components"
)

// function KGraphing provides a semantic model of a system running on a host and exposing the functionality of asset
func KGraphing(w http.ResponseWriter, req *http.Request, sys *components.System) {

	rdf := prefixes()
	rdf += modelSystem(sys)
	rdf += modelHost(sys)
	rdf += modelUAsset(sys)

	w.Header().Set("Content-Type", "text/turtle")
	w.Write([]byte(rdf))
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

func modelSystem(sys *components.System) (systemModel string) {
	sName := sys.Host.Name + "_" + sys.Name
	systemModel = fmt.Sprintf("alc:%s a afo:System ;\n", sName)
	systemModel += fmt.Sprintf("    afo:hasName \"%s\" ;\n", sys.Name)
	systemModel += fmt.Sprintf("    afo:runsOnHost alc:%s ;\n", sys.Host.Name)

	for assetName := range sys.UAssets {
		systemModel += fmt.Sprintf("    afo:hasUnitAsset alc:%s_%s ;\n", sName, assetName)
	}
	details := sys.Husk.Details
	for key, values := range details {
		for _, value := range values {
			if !(strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")) {
				value = "alc:" + value
			}
			systemModel += fmt.Sprintf("    afo:has%s %s ;\n", key, value)
		}
	}

	protoCount := 0
	ppStart := false
	for protocol := range sys.Husk.ProtoPort {
		if sys.Husk.ProtoPort[protocol] != 0 {
			if protoCount > 0 && ppStart {
				systemModel += " ;\n"
			}
			ppStart = true
			systemModel += "    afo:communicatesOver [\n"
			systemModel += fmt.Sprintf("        afo:usesProtocol \"%s\" ;\n", protocol)
			systemModel += fmt.Sprintf("        afo:usesPort %d \n", sys.Husk.ProtoPort[protocol])
			systemModel += "    ]"
		}
		protoCount++
	}
	systemModel += ".\n\n"
	return
}

// modelHost creates a knowledge graph of the hosting computer
func modelHost(sys *components.System) string {
	hostModel := fmt.Sprintf("alc:%s a afo:Host ;\n", sys.Host.Name)
	hostModel += fmt.Sprintf("    afo:hasName \"%s\" ;\n", sys.Host.Name)
	ipaLen := len(sys.Host.IPAddresses)
	ipaCount := 0
	for _, ipa := range sys.Host.IPAddresses {
		hostModel += fmt.Sprintf("    afo:hasIPaddress \"%s\"", ipa)
		ipaCount++
		if ipaCount < ipaLen {
			hostModel += " ;\n"
		}
	}
	hostModel += " .\n\n"
	return hostModel
}

// modelUAsset creates a knowledge graph of each unit assets and its consumed and provided services
func modelUAsset(sys *components.System) string {
	sName := sys.Host.Name + "_" + sys.Name
	var assetModels string
	for assetName, asset := range sys.UAssets {
		assetModels += fmt.Sprintf("alc:%s_%s a afo:UnitAsset ;\n", sName, assetName)
		assetModels += fmt.Sprintf("    afo:hasName \"%s\" ;\n", assetName)

		details := (*asset).GetDetails()
		for key, values := range details {
			for _, value := range values {
				if !(strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")) {
					value = "alc:" + value
				}
				assetModels += fmt.Sprintf("    alc:has%s %s ;\n", key, value)
			}
		}

		cervices := (*asset).GetCervices()
		cerviceCount := 0
		for _, cervice := range cervices {
			assetModels += fmt.Sprintf("    afo:consumesService alc:%s_%s_%s", sName, assetName, cervice.Definition)
			cerviceCount++
			// if cerviceCount < cervicesLen {
			assetModels += " ;\n"
			// }
		}

		services := (*asset).GetServices()
		servicesLen := len(services)
		serviceCount := 0
		for _, service := range services {
			assetModels += fmt.Sprintf("    afo:providesService alc:%s_%s_%s", sName, assetName, service.Definition)
			serviceCount++
			if serviceCount < servicesLen {
				assetModels += " ;\n"
			}
		}
		assetModels += " .\n\n"

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

		cervicesModel += fmt.Sprintf("alc:%s_%s_%s a afo:ConsumedService ;\n", sName, asset.GetName(), cervice.Definition)
		cervicesModel += fmt.Sprintf("    afo:consumes \"%s\" ;\n", cervice.Definition)

		details := cervice.Details
		keyCounter := 0
		keysLen := len(details)
		for key, values := range details {
			valuesCounter := 0
			valuesLen := len(values)
			for _, value := range values {
				if !(strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")) {
					value = "alc:" + value
				}
				cervicesModel += fmt.Sprintf("    alc:has%s %s", key, value)
				valuesCounter++
				if valuesCounter < valuesLen {
					cervicesModel += " ;\n"
				}
			}
			keyCounter++
			if keyCounter < keysLen {
				cervicesModel += " ;\n"
			}
		}

		// list of providers
		pCounter := 0
		providersCount := len(cervice.Nodes)
		if providersCount > 0 {
			cervicesModel += " ;\n"
		}
		for pName, provider := range cervice.Nodes {
			cervicesModel += fmt.Sprintf("    afo:consumes alc:%s ;\n", pName)
			uCounter := 0
			urlCount := len(provider)
			for _, url := range provider {
				cervicesModel += fmt.Sprintf("    afo:fromUrl <%s>", url)
				uCounter++
				if uCounter < urlCount {
					cervicesModel += " ;\n"
				}
			}
			pCounter++
			if pCounter < providersCount {
				cervicesModel += " ;\n"
			}
		}
		cervicesModel += " .\n\n"
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
		servicesModel += fmt.Sprintf("alc:%s_%s_%s a afo:Service ;\n", sName, assetName, service.Definition)
		servicesModel += fmt.Sprintf("    afo:hasName \"%s/%s\" ;\n", assetName, service.Definition)
		servicesModel += fmt.Sprintf("    afo:hasServiceDefinition \"%s\" ;\n", service.Definition)
		for protocol, port := range sys.Husk.ProtoPort {
			if port != 0 {
				addr := protocol + "://" + sys.Host.IPAddresses[0] + ":" + strconv.Itoa(port) + "/" + sys.Name + "/" + assetName + "/" + service.Definition
				servicesModel += fmt.Sprintf("    afo:hasUrl <%s> ;\n", addr)
			}
		}

		details := service.Details
		for key, values := range details {
			for _, value := range values {
				if !(strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")) {
					value = "alc:" + value
				}
				servicesModel += fmt.Sprintf("    alc:has%s  %s ;\n", key, value)
			}
		}

		servicesModel += fmt.Sprintf("    afo:isSubscribAble \"%t\"^^xsd:boolean ;\n", service.SubscribeAble)
		if service.CUnit != "" {
			servicesModel += fmt.Sprintf("    afo:hasCost \"%.2f\"^^xsd:decimal ;\n", service.ACost)
			servicesModel += fmt.Sprintf("    afo:hasCostUnit \"%s\"^^xsd:string ;\n", service.CUnit)
		}
		servicesModel += fmt.Sprintf("    afo:hasRegistrationPeriod %d .\n\n", service.RegPeriod)
	}

	return servicesModel
}
