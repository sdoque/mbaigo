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

// Package "usecases" addresses system behaviors and actions in given use cases
// such as configuration, registration, authentication, orchestration, ...

package usecases

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// RegisterServices keeps trach of the leading Service Registrar and keeps all services registered
func RegisterServices(sys *components.System) {

	var leadingRegistrar *components.CoreSystem
	// Create a buffered channel for the pointer to the leading service registrar
	registrarStream := make(chan *components.CoreSystem, 1)

	// Goroutine looking for leading service registrar every 5 seconds
	go func() {
		defer close(registrarStream)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			if leadingRegistrar != nil {
				resp, err := http.Get(leadingRegistrar.Url + "/status")
				if err != nil {
					log.Println("lost leading registrar status:", err)
					leadingRegistrar = nil
					continue // Skip to the next iteration of the loop
				}

				// Read from resp.Body and then close it directly after
				bodyBytes, err := io.ReadAll(resp.Body)
				resp.Body.Close() // Close the body directly after reading from it
				if err != nil {
					log.Println("\rError reading response from leading registrar:", err)
					leadingRegistrar = nil
					continue // Skip to the next iteration of the loop
				}

				if !strings.HasPrefix(string(bodyBytes), "lead Service Registrar since") {
					leadingRegistrar = nil
					log.Println("lost previous leading registrar")
				}
			} else {
				for _, cSys := range sys.CoreS {
					core := cSys
					if core.Name == "serviceregistrar" {
						resp, err := http.Get(core.Url + "/status")
						if err != nil {
							fmt.Println("error checking service registar status:", err)
							continue // Skip to the next iteration of the loop
						}

						// Read from resp.Body and then close it directly after
						bodyBytes, err := io.ReadAll(resp.Body)
						resp.Body.Close() // Close the body directly after reading from it
						if err != nil {
							fmt.Println("Error reading service registrar response body:", err)
							continue // Skip to the next iteration of the loop
						}

						if strings.HasPrefix(string(bodyBytes), "lead Service Registrar since") {
							leadingRegistrar = core
							fmt.Printf("\nlead registrar found at: %s\n", leadingRegistrar.Url)
						}
					}
				}
			}

			select {
			case <-ticker.C:
			case <-sys.Ctx.Done():
				return
			}
		}
	}()
	assetList := &sys.UAssets
	for _, aResource := range *assetList {
		servs := (*aResource).GetServices()
		for _, service := range servs {
			// service := (*servs)[j] // Correctly dereference the slice pointer and access the element
			go func(theUnitAsset *components.UnitAsset, theService *components.Service) {
				delay := 1 * time.Second
				for {
					timer := time.NewTimer(delay)
					select {
					case <-timer.C:
						if leadingRegistrar != nil {
							delay = registerService(sys, theUnitAsset, theService, leadingRegistrar)
						} else {
							delay = 15 * time.Second
						}
					case <-sys.Ctx.Done():
						deregisterService(leadingRegistrar, theService)
						return
					}
				}
			}(aResource, service)
		}
	}
}

// registerService makes a POST or PUT request to register or regegister individual services
func registerService(sys *components.System, ua *components.UnitAsset, ser *components.Service, registrar *components.CoreSystem) (delay time.Duration) {

	delay = 15 * time.Second
	// Prepare request
	reqPayload, err := serviceRegistrationForm(sys, ua, ser, "ServiceRecord_v1")
	if err != nil {
		log.Println("Registration marshall error, ", err)
		return
	}
	registrationurl := registrar.Url + "/register"

	var req *http.Request // Declare req outside the blocks
	if ser.ID == 0 {
		req, err = http.NewRequest("POST", registrationurl, bytes.NewBuffer(reqPayload))
		if err != nil {
			log.Printf("unable to register service %s with lead registrar\n", ser.Definition)
			return
		}
	} else {
		req, err = http.NewRequest("PUT", registrationurl, bytes.NewBuffer(reqPayload))
		if err != nil {
			log.Printf("unable to confirm the %s service with lead registar", ser.Definition)
			return
		}
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	client := &http.Client{Timeout: time.Second * 5}
	resp, err := client.Do(req) // execute the request and get the reply
	if err != nil {
		switch err := err.(type) {
		case net.Error:
			if err.Timeout() {
				log.Printf("registry timeout with lead registrar %s\n", registrationurl)
			} else {
				log.Printf("unable to (re-)register service %s with lead registrar\n", ser.Definition)
			}
		default:
			log.Printf("registration request error with %s, and error %s\n", registrationurl, err)
		}
		registrar = nil
		ser.ID = 0 // if re-registration failed, a complete new one should be made (POST)
	}

	// Handle response ------------------------------------------------

	if resp != nil {
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body) // Use io.ReadAll instead of ioutil.ReadAll
		if err != nil {
			log.Printf("Error reading registration response body: %v", err)
			return
		}
		rRecord, err := servRegRespExtract(bodyBytes)
		if err != nil {
			log.Print("Error extracting registration reply", err)
			return
		}
		ser.ID = rRecord.Id
		ser.RegTimestamp = rRecord.Created
		ser.RegExpiration = rRecord.EndOfValidity
		parsedTime, err := time.Parse(time.RFC3339, rRecord.EndOfValidity)
		if err != nil {
			log.Printf("Error parsing input: %s", err)
			return
		}
		delay = time.Until(parsedTime.Add(-5 * time.Second)) // should not wait until the deadline to start to confirrm live status
	}

	return
}

// deregisterService deletes a service from the database based on its service id
func deregisterService(registrar *components.CoreSystem, ser *components.Service) {
	if registrar == nil {
		return // there is no need to deregister if there is no leading registrar
	}
	client := &http.Client{}
	deRegServURL := registrar.Url + "/unregister/" + strconv.Itoa(ser.ID)
	fmt.Printf("Trying to unregiseter %s\n", deRegServURL)
	req, err := http.NewRequest("DELETE", deRegServURL, nil) // create a new request using http
	if err != nil {
		log.Println(err)
		return
	}
	resp, err := client.Do(req) // make the request
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()
	fmt.Printf("service %s deleted from the service registrar with HTTP Response Status: %d, %s\n", ser.Definition, resp.StatusCode, http.StatusText(resp.StatusCode))
}

// serviceRegistrationForm returrns a json data byte array with the data of the service to be registered
// in the form of choice [Sending @ Application system]
func serviceRegistrationForm(sys *components.System, res *components.UnitAsset, ser *components.Service, version string) (payload []byte, err error) {
	var f forms.Form
	switch version {
	case "ServiceRecord_v1":
		var sf forms.ServiceRecord_v1 // declare a new service form
		sf.NewForm()
		sf.Id = ser.ID
		sf.ServiceDefinition = ser.Definition
		sf.SystemName = sys.Name
		sf.IPAddresses = sys.Host.IPAddresses
		sf.ProtoPort = sys.Husk.ProtoPort
		sf.Details = deepCopyMap((*res).GetDetails())
		for key, valueSlice := range ser.Details {
			sf.Details[key] = append(sf.Details[key], valueSlice...)
		}
		sf.Certificate = sys.Husk.Certificate
		resName := (*res).GetName()
		sf.SubPath = resName + "/" + ser.SubPath

		if ser.RegPeriod != 0 {
			sf.RegLife = ser.RegPeriod
		} else {
			sf.RegLife = 30
		}
		sf.Created = ser.RegTimestamp
		f = &sf
	default:
		err = errors.New("unsupported service registration form version")
		return
	}
	payload, err = json.MarshalIndent(f, "", "  ")
	return
}

// deepCopyMap is necessary to prevent adding values to the original map at every re-registration
func deepCopyMap(m map[string][]string) map[string][]string {
	newMap := make(map[string][]string)
	for k, v := range m {
		newValue := make([]string, len(v))
		copy(newValue, v)
		newMap[k] = newValue
	}
	return newMap
}

// ServiceRegistrationFormsList returns the list of foms that the service registration handles
func ServiceRegistrationFormsList() []string {
	return []string{"ServiceRecord_v1"}
}

// servRegReqExtract determines the corrrecet forrm in to which the raw json data
// is used to update the service based on entry into the data base [Receving @ ServiceRegistry]
func ServRegReqExtract(bodyBytes []byte) (rec forms.ServiceRecord_v1, err error) {
	var jsonData map[string]interface{}
	err = json.Unmarshal(bodyBytes, &jsonData)
	if err != nil {
		log.Printf("Error unmarshaling JSON data: %v", err)
		return
	}
	formVersion, ok := jsonData["version"].(string)
	if !ok {
		log.Printf("Error: 'version' key not found in JSON data")
		return
	}
	switch formVersion {
	case "ServiceRecord_v1":
		var f forms.ServiceRecord_v1
		err = json.Unmarshal(bodyBytes, &f)
		if err != nil {
			log.Println("Unable to extract registration request ")
			return
		}
		rec = f
	default:
		err = errors.New("unsupported service registrattion form version")
	}
	return
}

// ServRegRespFillIn returrns a json data byte array with the data of the service to be registered
// in the form of choice [Sending @ ServiceRegistry]
func ServRegRespFillIn(rec forms.ServiceRecord_v1, version string) (payload []byte, err error) {
	var f forms.Form
	switch version {
	case "ServiceRecord_v1":
		var sf forms.ServiceRecord_v1
		sf.NewForm()
		f = &rec
	default:
		err = errors.New("unsupported service registrattion form version")
		return
	}
	payload, err = json.MarshalIndent(f, "", "  ")
	return
}

// servRegRespExtract determines the corrrecet forrm in to which the raw json data
// is used to update the service based on entry into the data base [Receiving @ Application system]
func servRegRespExtract(bodyBytes []byte) (rec forms.ServiceRecord_v1, err error) {
	var jsonData map[string]interface{}
	err = json.Unmarshal(bodyBytes, &jsonData)
	if err != nil {
		log.Printf("Error unmarshaling JSON data: %v", err)
		return
	}
	formVersion, ok := jsonData["version"].(string)
	if !ok {
		log.Printf("Error: 'version' key not found in JSON data")
		return
	}
	switch formVersion {
	case "ServiceRecord_v1":
		var f forms.ServiceRecord_v1
		err = json.Unmarshal(bodyBytes, &f)
		if err != nil {
			log.Println("Unable to extract registration reply")
			return
		}
		rec = f
	default:
		err = errors.New("unsupported service registrattion form version")
	}
	return
}
