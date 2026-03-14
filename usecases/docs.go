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

// doc forms are HTML templates used to describe the systems, resources and services

package usecases

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sdoque/mbaigo/components"
)

// System Documentation (based on HATEOAS) provides an initial documentation on the system's web server of with hyperlinks to the services for browsers
// HATEOAS is the acronym for Hypermedia as the Engine of Application State, using hyperlinks to navigate the API
func SysHateoas(w http.ResponseWriter, req *http.Request, sys components.System) {
	text := "<!DOCTYPE html><html><body>\n"
	text += "<h1>System Description</h1>\n"
	text += "<p>The system <b>" + html.EscapeString(sys.Name) + "</b> " + html.EscapeString(sys.Husk.Description) + "</p><br>\n"
	text += "<a href=\"" + html.EscapeString(sys.Husk.InfoLink) + "\">Online Documentation</a></p>\n"
	text += "<p> The resource list is </p><ul>\n"

	assetList := &sys.UAssets
	for _, unitasset := range *assetList {
		metaservice := ""
		for key, values := range (*unitasset).GetDetails() {
			metaservice += html.EscapeString(key) + ": " + html.EscapeString(fmt.Sprintf("%v", values)) + " "
		}
		text += "<li><b><a href=\"http://" + sys.Husk.Host.IPAddresses[0] + ":" + strconv.Itoa(sys.Husk.ProtoPort["http"]) + "/" + html.EscapeString(sys.Name) + "/" + html.EscapeString((*unitasset).GetName()) + "/doc" + "\">" + html.EscapeString((*unitasset).GetName()) + "</a></b> with details " + metaservice + "</li>\n"
	}

	// This part of the code is commented out because it is not used in the current implementation because the assets on a PLC might have different services
	// ======================================
	// text = "</ul> having the following services:<ul>"
	// w.Write([]byte(text))
	// servicesList := getServicesList(getFirstAsset(*assetList)[0])
	// for _, service := range servicesList {
	// 	metaservice := ""
	// 	for key, values := range service.Details {
	// 		metaservice += key + ": " + fmt.Sprintf("%v", values) + " "
	// 	}
	// 	serviceURI := "<li><b>" + service.Definition + "</b> with details: " + metaservice + "</li>"
	// 	w.Write([]byte(serviceURI))
	// }

	text += "</ul> <p> The services can be accessed using the following protocols with their respective bound ports:</p><ul>\n"
	for protocol, port := range sys.Husk.ProtoPort {
		text += "<li> Protocol <b>" + html.EscapeString(protocol) + "</b> using port <em>" + strconv.Itoa(port) + "</em></li>\n"
	}

	text += "</ul> <p> of the device whose IP addresses are (upon startup):</p><ul>\n"
	for _, IPAddre := range sys.Husk.Host.IPAddresses {
		text += "<li> " + html.EscapeString(IPAddre) + "</em></li>\n"
	}

	text += "</ul></body></html>"
	_, err := w.Write([]byte(text))
	if err != nil {
		log.Printf("Error while writing to response body for SysHateoas: %v", err)
	}
}

// ResHateoas provides information about the unit asset(s) and each service and is accessed via the system's web server
func ResHateoas(w http.ResponseWriter, req *http.Request, ua components.UnitAsset, sys components.System) {
	text := "<!DOCTYPE html><html></head><body>\n"
	text += "<h1>Unit Asset Description</h1>\n"

	uaName := ua.GetName()
	metaservice := ""
	for key, values := range ua.GetDetails() {
		metaservice += html.EscapeString(key) + ": " + html.EscapeString(fmt.Sprintf("%v", values)) + " "
	}
	text += "The resource <b>" + html.EscapeString(uaName) + "</b> belongs to system <b>" + html.EscapeString(sys.Name) + "</b> and has the details " + metaservice + " with the following services:" + "<ul>\n"

	services := ua.GetServices()
	for _, service := range services {
		metaservice := ""
		for key, values := range service.Details {
			metaservice += html.EscapeString(key) + ": " + html.EscapeString(fmt.Sprintf("%v", values)) + " "
		}
		text += "<li><a href=\"http://" + sys.Husk.Host.IPAddresses[0] + ":" + strconv.Itoa(sys.Husk.ProtoPort["http"]) + "/" + html.EscapeString(sys.Name) + "/" + html.EscapeString(uaName) + "/" + html.EscapeString(service.SubPath) + "/doc\">" + html.EscapeString(service.Definition) + "</a> with details: " + metaservice + "</li>\n"
	}

	text += "</ul></body></html>"
	_, err := w.Write([]byte(text))
	if err != nil {
		log.Printf("Error while writing response body for ResHateoas: %v", err)
	}
}

// ServiceHateoas provides information about the service and is accessed via the system's web server
func ServiceHateoas(w http.ResponseWriter, req *http.Request, serv components.Service, sys components.System) {
	parts := strings.Split(req.URL.Path, "/")
	uaName := parts[2]
	text := "<!DOCTYPE html><html></head><body>\n"
	text += "<h1>Service Description</h1>\n"

	metaservice := ""
	for key, values := range serv.Details {
		metaservice += html.EscapeString(key) + ": " + html.EscapeString(fmt.Sprintf("%v", values)) + " "
	}
	text += "The service <b><a href=\"http://" + sys.Husk.Host.IPAddresses[0] + ":" + strconv.Itoa(sys.Husk.ProtoPort["http"]) + "/" + html.EscapeString(sys.Name) + "/" + html.EscapeString(uaName) + "/" + html.EscapeString(serv.SubPath) + "\">" + html.EscapeString(serv.Definition) + "</a> </b> " + html.EscapeString(serv.Description) + " and has the details " + metaservice
	_, err := w.Write([]byte(text))
	if err != nil {
		log.Printf("Error while writing response body for ServiceHateoas: %v", err)
	}
}

// // getFirstAsset returns the first key-value pair in the Assets map
// func getFirstAsset(assetMap map[string]*components.UnitAsset) []components.UnitAsset {
// 	var assetList []components.UnitAsset
// 	for key := range assetMap {
// 		assetList = append(assetList, *assetMap[key])
// 		return assetList
// 	}
// 	return assetList
// }

// // getServicesList() returns the original list of services
// func getServicesList(uat components.UnitAsset) []components.Service {
// 	var serviceList []components.Service
// 	services := uat.GetServices()
// 	for s := range services {
// 		serviceList = append(serviceList, *services[s])
// 	}
// 	return serviceList
// }
