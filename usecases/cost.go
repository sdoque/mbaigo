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
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

func GetActivitiesCost(serv *components.Service) (payload []byte, err error) {
	var f forms.ActivityCostForm_v1
	f.NewForm()
	f.Activity = serv.Definition
	f.Cost = serv.ACost
	f.Unit = serv.CUnit
	f.Timestamp = time.Now()
	payload, err = json.MarshalIndent(f, "", "  ")
	return
}

// SetActivitiesCost updates the service cost
func SetActivitiesCost(serv *components.Service, bodyBytes []byte) (err error) {
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
	var acForm forms.ActivityCostForm_v1
	switch formVersion {
	case "ActivityCostForm_v1":
		var f forms.ActivityCostForm_v1
		err = json.Unmarshal(bodyBytes, &f)
		if err != nil {
			log.Println("Unable to extract new activity costs request ")
			return
		}
		acForm = f
	default:
		err = errors.New("unsupported version of activity costs form")
		return
	}

	if serv.Definition == acForm.Activity {
		serv.ACost = acForm.Cost // update the service's cost
		log.Printf("The new service cost is %f => the service is %+v\n", acForm.Cost, serv)
	} else {
		err = errors.New("mismatch between service list order") // corrected typo
		return
	}
	return
}

// ACServices handles the http request for the cost of a service
func ACServices(w http.ResponseWriter, r *http.Request, ua *components.UnitAsset, serviceP string) {
	servicesList := (*ua).GetServices()
	serv := servicesList[serviceP]
	switch r.Method {
	case "GET":
		payload, err := GetActivitiesCost(serv)
		if err != nil {
			log.Printf("Error in getting the activity costs\n")
			http.Error(w, "Error marshaling data.", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(payload)
		return
	case "PUT":
		defer r.Body.Close()
		bodyBytes, err := io.ReadAll(r.Body) // Use io.ReadAll instead of ioutil.ReadAll
		if err != nil {
			log.Printf("Error reading registration response body: %v", err)
			return
		}
		err = SetActivitiesCost(serv, bodyBytes)
		if err != nil {
			log.Printf("there was an error updating the activittiy costs, %s\n", err)
		}
	default:
		http.Error(w, "Method is not supported.", http.StatusNotFound)
	}
}
