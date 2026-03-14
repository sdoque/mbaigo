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
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// GetActivitiesCarbon retrieves the service carbon footprint
func GetActivitiesCarbon(serv *components.Service) (payload []byte, err error) {
	var f forms.CarbonFootprintForm_v1
	f.NewForm()
	f.Activity = serv.Definition
	f.Footprint = serv.CFootprint
	if suf, ok := rateSuffix(serv.CUnit); ok {
		f.Unit = "t" + suf
	} else {
		f.Unit = "t"
	}
	f.Timestamp = time.Now()
	payload, err = json.MarshalIndent(f, "", "  ")
	return
}

// rateSuffix checks if the unit string contains a rate (e.g., "/hr") and returns the suffix if found
func rateSuffix(unit string) (string, bool) {
	unit = strings.TrimSpace(unit)

	i := strings.Index(unit, "/")
	if i == -1 {
		// No slash → not a rate
		return "", false
	}

	return unit[i:], true // e.g. "Eur/hr" → "/hr"
}

// SetActivitiesCarbon sets the service carbon footprint from the request body
func SetActivitiesCarbon(serv *components.Service, bodyBytes []byte) (err error) {
	f, err := Unpack(bodyBytes, "application/json")
	if err != nil {
		return fmt.Errorf("unmarshalling cost form: %w", err)
	}
	acForm, ok := f.(*forms.CarbonFootprintForm_v1)
	if !ok {
		return fmt.Errorf("couldn't convert to correct form")
	}
	if serv.Definition != acForm.Activity {
		return fmt.Errorf("service definition and activity cost forms activity field doesn't match")
	}
	serv.CFootprint = acForm.Footprint // update the service's cost
	return
}

// FCServices handles the carbon footprint service requests
func FCServices(w http.ResponseWriter, r *http.Request, ua *components.UnitAsset, serviceP string) {
	// Has to use (*ua) in order to reach the methods for the interface UnitAsset, since ua is a pointer to an interface
	servicesList := (*ua).GetServices()
	serv := servicesList[serviceP]
	switch r.Method {
	case "GET":
		payload, err := GetActivitiesCarbon(serv)
		if err != nil {
			http.Error(w, "Error marshaling data.", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(payload)
		if err != nil {
			http.Error(w, "Error while writing to response body"+err.Error(), http.StatusInternalServerError)
		}
	case "PUT":
		defer r.Body.Close()
		bodyBytes, err := io.ReadAll(r.Body) // Use io.ReadAll instead of ioutil.ReadAll
		if err != nil {
			http.Error(w, "Error reading registration response body"+err.Error(), http.StatusBadRequest)
			return
		}
		err = SetActivitiesCarbon(serv, bodyBytes)
		if err != nil {
			http.Error(w, "Error occurred while updating activity costs"+err.Error(), http.StatusInternalServerError)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	default:
		http.Error(w, "Method is not supported.", http.StatusNotFound)
	}
}
