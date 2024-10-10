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
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"reflect"

	"github.com/sdoque/mbaigo/forms"
)

// Pack serializes a form to a byte array for payolad shipment with serializaton format (sf) request
func Pack(f forms.Form, contentType string) (data []byte, err error) {
	switch contentType {
	case "application/xml":
		data, err = xml.MarshalIndent(f, "", "  ")
		if err != nil {
			err = fmt.Errorf("error encoding XML: %w", err)
			return
		}
	default:
		data, err = json.MarshalIndent(f, "", "  ")
		if err != nil {
			err = fmt.Errorf("error encoding JSON: %w", err)
			return
		}
	}
	return
}

// Unpack function to deserialize data into appropriate form structs
func Unpack(data []byte, contentType string) (forms.Form, error) {
	var rawData map[string]interface{}

	// Unmarshal to get the form version
	switch contentType {
	case "application/json":
		if err := json.Unmarshal(data, &rawData); err != nil {
			log.Printf("Error unmarshaling JSON: %v", err)
			return nil, err
		}
	case "application/xml":
		if err := xml.Unmarshal(data, &rawData); err != nil {
			log.Printf("Error unmarshaling XML: %v", err)
			return nil, err
		}
	default:
		return nil, errors.New("unsupported content type")
	}

	// Retrieve form version
	formVersion, ok := rawData["version"].(string)
	if !ok {
		return nil, errors.New("'version' key not found in data")
	}

	// Look up the form type in the map
	formType, exists := forms.FormTypeMap[formVersion]
	if !exists {
		return nil, errors.New("unsupported form version: " + formVersion)
	}

	// Create a new instance of the form
	formInstance := reflect.New(formType).Interface().(forms.Form)

	// Unmarshal the full data into the form instance
	switch contentType {
	case "application/json":
		if err := json.Unmarshal(data, formInstance); err != nil {
			log.Printf("Error unmarshaling JSON into form: %v", err)
			return nil, err
		}
	case "application/xml":
		if err := xml.Unmarshal(data, formInstance); err != nil {
			log.Printf("Error unmarshaling XML into form: %v", err)
			return nil, err
		}
	}

	return formInstance, nil
}
