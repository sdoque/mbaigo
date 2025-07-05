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
	"encoding/xml"
	"fmt"
	"reflect"
	"strings"
	"unicode"

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

	// Heuristic handling for text/plain with possible charset
	if strings.Contains(contentType, "text/plain") {
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) > 0 {
			switch trimmed[0] {
			case '{', '[':
				contentType = "application/json"
			case '<':
				contentType = "application/xml"
			default:
				return nil, fmt.Errorf("plain text content is neither valid JSON nor XML")
			}
		} else {
			return nil, fmt.Errorf("empty payload with content type text/plain")
		}
	}

	// Unmarshal to get the form version
	switch {
	case strings.Contains(contentType, "application/json"):
		if err := json.Unmarshal(data, &rawData); err != nil {
			return nil, fmt.Errorf("error unmarshalling JSON: %w", err)
		}
	case strings.Contains(contentType, "application/xml"):
		if err := xml.Unmarshal(data, &rawData); err != nil {
			return nil, fmt.Errorf("error unmarshalling XML: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported content type")
	}

	// Retrieve form version
	formVersion, ok := rawData["version"].(string)
	if !ok {
		return nil, fmt.Errorf("'version' key not found in data")
	}

	// Look up the form type in the map
	formType, exists := forms.FormTypeMap[formVersion]
	if !exists {
		return nil, fmt.Errorf("unsupported form version: %s", formVersion)
	}

	// Create a new instance of the form
	formInstance := reflect.New(formType).Interface().(forms.Form)

	// Unmarshal the full data into the form instance
	switch {
	case strings.Contains(contentType, "application/json"):
		if err := json.Unmarshal(data, formInstance); err != nil {
			return nil, fmt.Errorf("error unmarshalling JSON into form: %w", err)
		}
	case strings.Contains(contentType, "application/xml"):
		if err := xml.Unmarshal(data, formInstance); err != nil {
			return nil, fmt.Errorf("error unmarshalling XML into form: %w", err)
		}
	}

	return formInstance, nil
}

// ------- Naming Conventions Tools -------

// ToCamel converts PascalCase to camelCase.
func ToCamel(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// ToPascal converts camelCase to PascalCase.
func ToPascal(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// IsFirstLetterUpper returns true if the first rune is uppercase.
func IsFirstLetterUpper(s string) bool {
	if s == "" {
		return false
	}
	return unicode.IsUpper([]rune(s)[0])
}

// IsFirstLetterLower returns true if the first rune is lowercase.
func IsFirstLetterLower(s string) bool {
	if s == "" {
		return false
	}
	return unicode.IsLower([]rune(s)[0])
}

// IsPascalCase returns true if the string starts with an uppercase letter.
func IsPascalCase(s string) bool {
	return IsFirstLetterUpper(s)
}

// IsCamelCase returns true if the string starts with a lowercase letter.
func IsCamelCase(s string) bool {
	return IsFirstLetterLower(s)
}
