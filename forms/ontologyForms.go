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

	"github.com/sdoque/mbaigo/components"
)

// function SModel provides a semantic model of a system running on a host and exposing the functionality of asset
func SModel(w http.ResponseWriter, req *http.Request, sys components.System) {
	fmt.Println("Processing the semantic model")
	rdf := "@prefix temp: <http://www.arrowhead.eu/temp#> .\n"
	rdf += "@prefix afo: <http://www.synecdoque.com/afo-core.ttl> .\n"
	rdf += "@prefix owl: <http://www.w3.org/2002/07/owl#> .\n"
	rdf += "@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .\n"
	rdf += "@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .\n"
	rdf += "@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .\n\n"

	// Add the system instance ----------------------------------------------------------------------
	sName := sys.Host.Name + "_" + sys.Name
	rdf += fmt.Sprintf(":%s a afo:System ;\n", sName)
	rdf += fmt.Sprintf("    afo:hasName :\"%s\" ;\n", sys.Name)
	rdf += fmt.Sprintf("    afo:runsOnHost :%s ;\n", sys.Host.Name)

	for assetName := range sys.UAssets {
		rdf += fmt.Sprintf("    afo:hasUnitAsset :%s_%s ;\n", sName, assetName)
	}
	details := sys.Husk.Details
	for key, values := range details {
		rdf += fmt.Sprintf("    afo:has%s [\n", key)
		valuesCount := 0
		valuesLen := len(values)
		for _, value := range values {
			rdf += fmt.Sprintf("        afo:hasValue \"%s\"", value)
			valuesCount++
			if valuesCount < valuesLen {
				rdf += " ;\n"
			}
		}
		rdf += "\n    ] ;\n"
	}

	protoCount := 0
	ppStart := false
	for protocol := range sys.Husk.ProtoPort {
		if sys.Husk.ProtoPort[protocol] != 0 {
			if protoCount > 0 && ppStart {
				rdf += " ;\n"
			}
			ppStart = true
			rdf += "    afo:communicatesOver [\n"
			rdf += fmt.Sprintf("        afo:usesProtocol \"%s\" ;\n", protocol)
			rdf += fmt.Sprintf("        afo:usesPort %d \n", sys.Husk.ProtoPort[protocol])
			rdf += "    ]"
		}
		protoCount++
	}
	rdf += ".\n\n"

	// Add the host instance ----------------------------------------------------------------------
	rdf += fmt.Sprintf(":%s a afo:Host ;\n", sys.Host.Name)
	rdf += fmt.Sprintf("    afo:hasName \"%s\" ;\n", sys.Host.Name)
	ipaLen := len(sys.Host.IPAddresses)
	ipaCount := 0
	for _, ipa := range sys.Host.IPAddresses {
		rdf += fmt.Sprintf("    afo:hasIPaddress \"%s\"", ipa)
		ipaCount++
		if ipaCount < ipaLen {
			rdf += " ;\n"
		}
	}
	rdf += " .\n\n"

	// Add the unit asset instances ----------------------------------------------------------------------
	for assetName, asset := range sys.UAssets {
		rdf += fmt.Sprintf(":%s_%s a afo:UnitAsset ;\n", sName, assetName)
		rdf += fmt.Sprintf("    afo:hasName \"%s\" ;\n", assetName)

		details := (*asset).GetDetails()
		for key, values := range details {
			rdf += fmt.Sprintf("    afo:has%s [\n", key)
			// rdf += fmt.Sprintf("        afo:hasName \"%s\" ;\n", key)
			valuesCount := 0
			valuesLen := len(values)
			for _, value := range values {
				rdf += fmt.Sprintf("        afo:hasValue \"%s\"", value)
				valuesCount++
				if valuesCount < valuesLen {
					rdf += " ;\n"
				}
			}
			rdf += "\n    ] ;\n"
		}
		cervices := (*asset).GetCervices()
		// cervicesLen := len(cervices)
		cerviceCount := 0
		for _, cervice := range cervices {
			rdf += fmt.Sprintf("    afo:consumesService :%s_%s_%s", sName, assetName, cervice.Name)
			cerviceCount++
			// if cerviceCount < cervicesLen {
			rdf += " ;\n"
			// }
		}

		services := (*asset).GetServices()
		servicesLen := len(services)
		serviceCount := 0
		for _, service := range services {
			rdf += fmt.Sprintf("    afo:hasService :%s_%s_%s", sName, assetName, service.Definition)
			serviceCount++
			if serviceCount < servicesLen {
				rdf += " ;\n"
			}
		}
		rdf += " .\n\n"
	}

	// Add the consumed services instances ----------------------------------------------------------------------
	for assetName, asset := range sys.UAssets {

		cervices := (*asset).GetCervices()
		for _, cervice := range cervices {
			rdf += fmt.Sprintf(":%s_%s_%s a afo:ConsumedService ;\n", sName, assetName, cervice.Name)
			rdf += fmt.Sprintf("    afo:hasName \"%s\" ;\n", cervice.Name)
			details = cervice.Details
			detailsCount := 0
			detailsLen := len(details)
			for key, values := range details {
				rdf += fmt.Sprintf("    afo:has%s [\n", key)
				detailsCount++
				valuesCount := 0
				valuesLen := len(values)
				for _, value := range values {
					rdf += fmt.Sprintf("        afo:hasValue \"%s\"", value)
					valuesCount++
					if valuesCount < valuesLen {
						rdf += " ;\n"
					}
					rdf += "\n    ]"
					if detailsCount < detailsLen {
						rdf += " ;\n"
					}
				}
			}
			pAddressCount := 0
			pAddressLen := len(cervice.Url)
			if pAddressLen > 0 {
				rdf += " \n"
			}
			for _, address := range cervice.Url {
				rdf += fmt.Sprintf("        afo:hasProvider \"%s\"", address)
				pAddressCount++
				if pAddressCount < pAddressLen {
					rdf += " ;\n"
				}
			}
			rdf += " .\n\n"
		}

		// Add the provided services instances ----------------------------------------------------------------------
		services := (*asset).GetServices()
		for _, service := range services {
			rdf += fmt.Sprintf(":%s_%s_%s a afo:Service ;\n", sName, assetName, service.Definition)
			rdf += fmt.Sprintf("    afo:hasName \"%s/%s\" ;\n", assetName, service.Definition)
			rdf += fmt.Sprintf("    afo:hasServiceDefinition \"%s\" ;\n", service.Definition)
			details = service.Details
			for key, values := range details {
				rdf += fmt.Sprintf("    afo:has%s [\n", key)
				// rdf += "        a afo:Attribute ;\n"
				// rdf += fmt.Sprintf("        afo:hasName \"%s\" ;\n", key)
				valuesCount := 0
				valuesLen := len(values)
				for _, value := range values {
					rdf += fmt.Sprintf("        afo:hasValue \"%s\"", value)
					valuesCount++
					if valuesCount < valuesLen {
						rdf += " ;\n"
					}
				}
				rdf += "\n    ] ;\n"
			}
			rdf += fmt.Sprintf("    afo:isSubscribAble %t ;\n", service.SubscribeAble)
			if service.CUnit != "" {
				rdf += fmt.Sprintf("    afo:hasCost %.2f ;\n", service.ACost)
				rdf += fmt.Sprintf("    afo:hasCostUnit \"%s\" ;\n", service.CUnit)
			}
			rdf += fmt.Sprintf("    afo:hasRegistrationPeriod %d .\n\n", service.RegPeriod)
		}

	}

	// Set the content type to text/turtle
	w.Header().Set("Content-Type", "text/turtle")
	w.Write([]byte(rdf))
}
