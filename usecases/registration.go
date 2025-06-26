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
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

type registrarTracker struct {
	url   string
	mutex sync.RWMutex
}

func (rt *registrarTracker) set(url string) {
	rt.mutex.Lock()
	rt.url = url
	rt.mutex.Unlock()
}

func (rt *registrarTracker) get() string {
	rt.mutex.RLock()
	defer rt.mutex.RUnlock()
	return rt.url
}

// RegisterServices keeps track of the leading Service Registrar and keeps all services registered
func RegisterServices(sys *components.System) {
	// Keep track of the registrar URL. The URL is shared between goroutines,
	// so it must be protected from data races using a mutex.
	registrar := &registrarTracker{}

	// Goroutine looking for leading service registrar every 5 seconds
	go func() {
		ticker := time.Tick(5 * time.Second)
		for {
			newURL, err := components.GetRunningCoreSystemURL(sys, components.ServiceRegistrarName)
			registrar.set(newURL) // should be empty on error anyway
			if err != nil {
				log.Println("find lead registrar:", err)
			}

			select {
			case <-ticker:
			case <-sys.Ctx.Done():
				return
			}
		}
	}()

	// Run registration loops for each services
	assetList := &sys.UAssets
	for _, aResource := range *assetList {
		servs := (*aResource).GetServices()
		for _, service := range servs {
			go func(theUnitAsset *components.UnitAsset, theService *components.Service) {
				delay := 1 * time.Second
				for {
					select {
					case <-time.Tick(delay):
						delay = registerService(sys, registrar.get(), theUnitAsset, theService)
					case <-sys.Ctx.Done():
						err := unregisterService(registrar.get(), theService)
						if err != nil {
							log.Println("unregistering service:", err)
						}
						return
					}
				}
			}(aResource, service)
		}
	}
}

// registerService makes a POST or PUT request to register or register individual services
func registerService(sys *components.System, registrar string, ua *components.UnitAsset, serv *components.Service) (delay time.Duration) {
	delay = 15 * time.Second
	if registrar == "" {
		return delay
	}

	// Prepare request
	reqPayload, err := serviceRegistrationForm(sys, ua, serv, "ServiceRecord_v1")
	if err != nil {
		log.Println("Registration marshall error, ", err)
		return
	}
	registrationURL := registrar + "/register"

	var req *http.Request // Declare req outside the blocks
	if serv.ID == 0 {
		req, err = http.NewRequest("POST", registrationURL, bytes.NewBuffer(reqPayload))
		if err != nil {
			log.Printf("unable to register service %s with lead registrar\n", serv.Definition)
			return
		}
	} else {
		req, err = http.NewRequest("PUT", registrationURL, bytes.NewBuffer(reqPayload))
		if err != nil {
			log.Printf("unable to confirm the %s service with lead registrar", serv.Definition)
			return
		}
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := http.DefaultClient.Do(req) // execute the request and get the reply
	if err != nil {
		switch err := err.(type) {
		case net.Error:
			if err.Timeout() {
				log.Printf("registry timeout with lead registrar %s\n", registrationURL)
			} else {
				log.Printf("unable to (re-)register service %s with lead registrar\n", serv.Definition)
			}
		default:
			log.Printf("registration request error with %s, and error %s\n", registrationURL, err)
		}
		serv.ID = 0 // if re-registration failed, a complete new one should be made (POST)
		return
	}

	// Handle response ------------------------------------------------

	if resp != nil {
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body) // Use io.ReadAll instead of ioutil.ReadAll
		if err != nil {
			log.Printf("Error reading registration response body: %v", err)
			return
		}

		headerContentType := resp.Header.Get("Content-Type")
		rRecord, err := Unpack(bodyBytes, headerContentType)
		if err != nil {
			log.Printf("error extracting the registration record reply %v\n", err)
		}

		// Perform a type assertion to convert the returned Form to ServiceRecord_v1
		rr, ok := rRecord.(*forms.ServiceRecord_v1)
		if !ok {
			log.Println("Problem unpacking the service registration reply")
			return
		}

		serv.ID = rr.Id
		serv.RegTimestamp = rr.Created
		serv.RegExpiration = rr.EndOfValidity
		parsedTime, err := time.Parse(time.RFC3339, rr.EndOfValidity)
		if err != nil {
			log.Printf("Error parsing input: %s", err)
			return
		}
		// should not wait until the deadline to start to confirm live status
		delay = time.Until(parsedTime.Add(-5 * time.Second))
	}
	return
}

// unregisterService deletes a service from the database based on its service id
func unregisterService(registrar string, serv *components.Service) error {
	if registrar == "" {
		return nil // there is no need to deregister if there is no leading registrar
	}
	unregisterURL := registrar + "/unregister/" + strconv.Itoa(serv.ID)
	req, err := http.NewRequest("DELETE", unregisterURL, nil) // create a new request using http
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// serviceRegistrationForm returns a json data byte array with the data of the service to be registered
// in the form of choice [Sending @ Application system]
func serviceRegistrationForm(sys *components.System, ua *components.UnitAsset, serv *components.Service, version string) (payload []byte, err error) {
	var f forms.Form
	switch version {
	case "ServiceRecord_v1":
		resName := (*ua).GetName()
		var sr forms.ServiceRecord_v1 // declare a new service form
		sr.NewForm()
		sr.Id = serv.ID
		sr.ServiceDefinition = serv.Definition
		sr.SystemName = sys.Name
		sr.ServiceNode = sys.Host.Name + "_" + sys.Name + "_" + resName + "_" + serv.Definition
		sr.IPAddresses = sys.Host.IPAddresses
		sr.ProtoPort = make(map[string]int) // initialize the map
		for key, port := range sys.Husk.ProtoPort {
			if port != 0 { // exclude entries where the port is 0
				sr.ProtoPort[key] = port
			}
		}
		sr.Details = deepCopyMap((*ua).GetDetails())
		for key, valueSlice := range serv.Details {
			sr.Details[key] = append(sr.Details[key], valueSlice...)
		}
		sr.SubPath = resName + "/" + serv.SubPath

		if serv.RegPeriod != 0 {
			sr.RegLife = serv.RegPeriod
		} else {
			sr.RegLife = 30
		}
		sr.Created = serv.RegTimestamp
		f = &sr
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

// ServiceRegistrationFormsList returns the list of forms that the service registration handles
func ServiceRegistrationFormsList() []string {
	return []string{"ServiceRecord_v1"}
}
