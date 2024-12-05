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

// function Ontology provides a semantic model of a system running on a host and exposing the functionality of asset
func Ontology(w http.ResponseWriter, req *http.Request, sys components.System) {
	fmt.Println("Printing the ontology")
	rdf := "@prefix temp: <http://www.arrowhead.eu/temp#> .\n"
	rdf += "@prefix afo: <http://www.synecdoque.com/afo-core.ttl> .\n"
	rdf += "@prefix owl: <http://www.w3.org/2002/07/owl#> .\n"
	rdf += "@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .\n"
	rdf += "@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .\n"
	rdf += "@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .\n\n"

	// Add the system instance
	rdf += fmt.Sprintf(":%s a afo:System ;\n", sys.Name)
	rdf += fmt.Sprintf("    afo:runOnHost :%s ;\n", sys.Host.Name)

	for assetName := range sys.UAssets {
		rdf += fmt.Sprintf("    afo:hasUnitAsset :%s ;\n", assetName)
	}
	details := sys.Husk.Details
	for key, values := range details {
		rdf += "    afo:hasAttribute [\n"
		rdf += "        a afo:Attribute ;\n"
		rdf += fmt.Sprintf("        afo:hasName \"%s\" ;\n", key)
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
	protoLen := len(sys.Husk.ProtoPort)
	protoCount := 0
	for protocol := range sys.Husk.ProtoPort {
		rdf += fmt.Sprintf("    afo:hasProtocol \"%s\"", protocol)
		protoCount++
		if protoCount < protoLen {
			rdf += " ;\n"
		}
	}
	rdf += ".\n\n"

	// Add the protocol-port instances
	// protoCount = 0
	for protocol, port := range sys.Husk.ProtoPort {
		rdf += fmt.Sprintf(":%s a afo:Protocol ;\n", protocol)
		rdf += fmt.Sprintf("    afo:usesPort %d .\n", port)
		// protoCount++
		// if protoCount < protoLen {
		// 	rdf += " ;\n"
		// }
	}
	rdf += "\n"

	// Add the host instance
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

	// Add the unit asset instances
	for assetName, asset := range sys.UAssets {
		rdf += fmt.Sprintf(":%s a afo:UnitAsset ;\n", assetName)
		rdf += fmt.Sprintf("    afo:hasName \"%s\" ;\n", assetName)

		details := (*asset).GetDetails()
		for key, values := range details {
			rdf += "    afo:hasAttribute [\n"
			rdf += "        a afo:Attribute ;\n"
			rdf += fmt.Sprintf("        afo:hasName \"%s\" ;\n", key)
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
		services := (*asset).GetServices()
		servicesLen := len(services)
		serviceCount := 0
		for _, service := range services {
			rdf += fmt.Sprintf("    afo:hasService :%s_%s", assetName, service.Definition)
			serviceCount++
			if serviceCount < servicesLen {
				rdf += " ;\n"
			}
		}
		rdf += " .\n\n"
	}

	// Add the service instances
	for assetName, asset := range sys.UAssets {
		services := (*asset).GetServices()
		for _, service := range services {
			rdf += fmt.Sprintf(":%s_%s a afo:Service ;\n", assetName, service.Definition)
			rdf += fmt.Sprintf("    afo:hasName \"%s/%s\" ;\n", assetName, service.Definition)
			rdf += fmt.Sprintf("    afo:hasServiceDefinition \"%s\" ;\n", service.Definition)
			details = service.Details
			for key, values := range details {
				rdf += "    afo:hasAttribute [\n"
				rdf += "        a afo:Attribute ;\n"
				rdf += fmt.Sprintf("        afo:hasName \"%s\" ;\n", key)
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
			rdf += fmt.Sprintf("    afo:hasRegistrationPeriod %d .", service.RegPeriod)
		}
	}

	// Set the content type to text/turtle
	w.Header().Set("Content-Type", "text/turtle")
	w.Write([]byte(rdf))
}
