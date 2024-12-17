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

package forms

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/sdoque/mbaigo/components"
)

// function SModel provides a semantic model of a system running on a host and exposing the functionality of asset
func SModel(w http.ResponseWriter, req *http.Request, sys *components.System) {

	rdf := prefixes()
	rdf += modelSystem(sys)
	rdf += modelHost(sys)
	rdf += modelUAsset(sys)

	w.Header().Set("Content-Type", "text/turtle")
	w.Write([]byte(rdf))
}

func prefixes() (description string) {
	description = "@prefix : <http://www.synecdoque.com/example#> .\n"
	description += "@prefix afo: <http://www.synecdoque.com/afo-core.ttl> .\n"
	description += "@prefix owl: <http://www.w3.org/2002/07/owl#> .\n"
	description += "@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .\n"
	description += "@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .\n"
	description += "@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .\n\n"
	return
}

func modelSystem(sys *components.System) (systemModel string) {
	sName := sys.Host.Name + "_" + sys.Name
	systemModel = fmt.Sprintf(":%s a afo:System ;\n", sName)
	systemModel += fmt.Sprintf("    afo:hasName \"%s\" ;\n", sys.Name)
	systemModel += fmt.Sprintf("    afo:runsOnHost :%s ;\n", sys.Host.Name)

	for assetName := range sys.UAssets {
		systemModel += fmt.Sprintf("    afo:hasUnitAsset :%s_%s ;\n", sName, assetName)
	}
	details := sys.Husk.Details
	for key, values := range details {
		systemModel += fmt.Sprintf("    afo:has%s [\n", key)
		valuesCount := 0
		valuesLen := len(values)
		for _, value := range values {
			systemModel += fmt.Sprintf("        afo:hasValue \"%s\"", value)
			valuesCount++
			if valuesCount < valuesLen {
				systemModel += " ;\n"
			}
		}
		systemModel += "\n    ] ;\n"
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

func modelHost(sys *components.System) string {
	hostModel := fmt.Sprintf(":%s a afo:Host ;\n", sys.Host.Name)
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

func modelUAsset(sys *components.System) string {
	sName := sys.Host.Name + "_" + sys.Name
	var assetModels string
	for assetName, asset := range sys.UAssets {
		assetModels += fmt.Sprintf(":%s_%s a afo:UnitAsset ;\n", sName, assetName)
		assetModels += fmt.Sprintf("    afo:hasName \"%s\" ;\n", assetName)

		details := (*asset).GetDetails()
		for key, values := range details {
			assetModels += fmt.Sprintf("    afo:has%s [\n", key)
			// rdf += fmt.Sprintf("        afo:hasName \"%s\" ;\n", key)
			valuesCount := 0
			valuesLen := len(values)
			for _, value := range values {
				assetModels += fmt.Sprintf("        afo:hasValue \"%s\"", value)
				valuesCount++
				if valuesCount < valuesLen {
					assetModels += " ;\n"
				}
			}
			assetModels += "\n    ] ;\n"
		}

		cervices := (*asset).GetCervices()
		// cervicesLen := len(cervices)
		// if cervicesLen > 0 {
		// 	assetModels += " ;\n"
		// }
		cerviceCount := 0
		for _, cervice := range cervices {
			assetModels += fmt.Sprintf("    afo:consumesService :%s_%s_%s", sName, assetName, cervice.Name)
			cerviceCount++
			// if cerviceCount < cervicesLen {
			assetModels += " ;\n"
			// }
		}

		services := (*asset).GetServices()
		servicesLen := len(services)
		serviceCount := 0
		for _, service := range services {
			assetModels += fmt.Sprintf("    afo:hasService :%s_%s_%s", sName, assetName, service.Definition)
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

func modelCervices(sName string, ua *components.UnitAsset) string {
	var cervicesModel string
	asset := *ua
	cervices := asset.GetCervices()
	for _, cervice := range cervices {

		cervicesModel += fmt.Sprintf(":%s_%s_%s a afo:ConsumedService ;\n", sName, asset.GetName(), cervice.Name)
		cervicesModel += fmt.Sprintf("    afo:hasName \"%s\" ;\n", cervice.Name)
		details := cervice.Details
		detailsCount := 0
		detailsLen := len(details)
		for key, values := range details {
			cervicesModel += fmt.Sprintf("    afo:has%s [\n", key)
			detailsCount++
			valuesCount := 0
			valuesLen := len(values)
			for _, value := range values {
				cervicesModel += fmt.Sprintf("\t\tafo:hasValue \"%s\"", value)
				valuesCount++
				if valuesCount < valuesLen {
					cervicesModel += " ;\n"
				}
				cervicesModel += "\n\t]"
				if detailsCount < detailsLen {
					cervicesModel += " ;\n"
				}
			}
		}
		pAddressCount := 0
		pAddressLen := len(cervice.Url)
		if pAddressLen > 0 {
			cervicesModel += " ;\n"
		}
		for _, address := range cervice.Url {
			cervicesModel += fmt.Sprintf("    afo:consumes <%s>", address)
			pAddressCount++
			if pAddressCount < pAddressLen {
				cervicesModel += " ;\n"
			}
		}
		cervicesModel += " .\n\n"
	}
	return cervicesModel
}

func modelServices(sName string, ua *components.UnitAsset, sys *components.System) string {
	var servicesModel string
	asset := *ua
	assetName := asset.GetName()
	services := asset.GetServices()
	for _, service := range services {
		servicesModel += fmt.Sprintf(":%s_%s_%s a afo:Service ;\n", sName, assetName, service.Definition)
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
			servicesModel += fmt.Sprintf("    afo:has%s [\n", key)
			// rdf += "        a afo:Attribute ;\n"
			// rdf += fmt.Sprintf("        afo:hasName \"%s\" ;\n", key)
			valuesCount := 0
			valuesLen := len(values)
			for _, value := range values {
				servicesModel += fmt.Sprintf("        afo:hasValue \"%s\"", value)
				valuesCount++
				if valuesCount < valuesLen {
					servicesModel += " ;\n"
				}
			}
			servicesModel += "\n    ] ;\n"
		}
		servicesModel += fmt.Sprintf("    afo:isSubscribAble %t ;\n", service.SubscribeAble)
		if service.CUnit != "" {
			servicesModel += fmt.Sprintf("    afo:hasCost %.2f ;\n", service.ACost)
			servicesModel += fmt.Sprintf("    afo:hasCostUnit \"%s\" ;\n", service.CUnit)
		}
		servicesModel += fmt.Sprintf("    afo:hasRegistrationPeriod %d .\n\n", service.RegPeriod)
	}

	return servicesModel
}
