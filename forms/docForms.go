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

package forms

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/sdoque/mbaigo/components"
)

// System HATEOAS provides an initial documentation as a web server of the system with hyperlinks to the services for authorized browsers
// It is the acronym for Hypermedia as the Engine of Application State
func SysHateoas(w http.ResponseWriter, req *http.Request, sys components.System) {
	text := "<!DOCTYPE html><html><body>"
	w.Write([]byte(text))
	text = "<h1>System Description</h1>"
	w.Write([]byte(text))
	text = "<p>The system <b>" + sys.Name + "</b> " + sys.Husk.Description + "</p><br>"
	w.Write([]byte(text))
	text = "<a href=\"" + sys.Husk.InfoLink + "\">Online Documentation</a></p>"
	w.Write([]byte(text))

	text = "<p> The resource list is </p><ul>"
	w.Write([]byte(text))
	assetList := &sys.UAssets
	for _, unitasset := range *assetList {
		metaservice := ""
		for key, values := range (*unitasset).GetDetails() {
			metaservice += key + ": " + fmt.Sprintf("%v", values) + " "
		}
		resourceURI := "<li><b><a href=\"http://" + sys.Host.IPAddresses[0] + ":" + strconv.Itoa(sys.Husk.ProtoPort["http"]) + "/" + sys.Name + "/" + (*unitasset).GetName() + "/doc" + "\">" + (*unitasset).GetName() + "</a></b> with details " + metaservice + "</li>"
		w.Write([]byte(resourceURI))
	}

	text = "</ul> having the following services:<ul>"
	w.Write([]byte(text))
	servicesList := getServicesList(getFirstAsset(*assetList)[0])
	for _, service := range servicesList {
		metaservice := ""
		for key, values := range service.Details {
			metaservice += key + ": " + fmt.Sprintf("%v", values) + " "
		}
		serviceURI := "<li><b>" + service.Definition + "</b> with details: " + metaservice + "</li>"
		w.Write([]byte(serviceURI))
	}

	text = "</ul> <p> The services can be accessed using the following protocols with their respective bound ports:</p><ul>"
	w.Write([]byte(text))
	for protocol, port := range sys.Husk.ProtoPort {
		protoDoor := "<li> Protocol <b>" + protocol + "</b> using port <em>" + strconv.Itoa(port) + "</em></li>"
		w.Write([]byte(protoDoor))
	}

	text = "</ul> <p> of the device whose IP addresses are (upon startup):</p><ul>"
	w.Write([]byte(text))
	for _, IPAddre := range sys.Host.IPAddresses {
		hostaddresses := "<li> " + IPAddre + "</em></li>"
		w.Write([]byte(hostaddresses))
	}

	text = "</ul></body></html>"
	w.Write([]byte(text))
}

// ResHateoas provides information about the unit asset(s) and each service and is accessed via the system's web server
func ResHateoas(w http.ResponseWriter, req *http.Request, ua components.UnitAsset, sys components.System) {
	text := "<!DOCTYPE html><html></head><body>"
	w.Write([]byte(text))

	text = "<h1>Unit Asset Description</h1>"
	w.Write([]byte(text))

	uaName := ua.GetName()
	metaservice := ""
	for key, values := range ua.GetDetails() {
		metaservice += key + ": " + fmt.Sprintf("%v", values) + " "
	}
	text = "The resource <b>" + uaName + "</b> belongs to system <b>" + sys.Name + "</b> and has the details " + metaservice + " with the following services:" + "<ul>"
	w.Write([]byte(text))
	services := ua.GetServices()
	for _, service := range services {
		metaservice := ""
		for key, values := range service.Details {
			metaservice += key + ": " + fmt.Sprintf("%v", values) + " "
		}
		serviceURI := "<li><a href=\"http://" + sys.Host.IPAddresses[0] + ":" + strconv.Itoa(sys.Husk.ProtoPort["http"]) + "/" + sys.Name + "/" + uaName + "/" + service.SubPath + "/doc\">" + service.Definition + "</a> with details: " + metaservice + "</li>"
		w.Write([]byte(serviceURI))
	}

	text = "</ul></body></html>"
	w.Write([]byte(text))
}

// ResHateoas provides information about the service and is accessed via the system's web server
func ServiceHateoas(w http.ResponseWriter, req *http.Request, ser components.Service, sys components.System) {
	parts := strings.Split(req.URL.Path, "/")
	uaName := parts[2]
	text := "<!DOCTYPE html><html></head><body>"
	w.Write([]byte(text))

	text = "<h1>Service Description</h1>"
	w.Write([]byte(text))

	metaservice := ""
	for key, values := range ser.Details {
		metaservice += key + ": " + fmt.Sprintf("%v", values) + " "
	}
	text = "The service <b><a href=\"http://" + sys.Host.IPAddresses[0] + ":" + strconv.Itoa(sys.Husk.ProtoPort["http"]) + "/" + sys.Name + "/" + uaName + "/" + ser.SubPath + "\">" + ser.Definition + "</a> </b> " + ser.Description + " and has the details " + metaservice
	w.Write([]byte(text))
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
