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

package forms

import (
	"encoding/xml"
	"reflect"
	"time"
)

// SignalA is a single analog signal that has been digitized at a given time (timestamp)
type SignalA_v1a struct {
	XMLName   xml.Name  `json:"-" xml:"SignalA"`
	Value     float64   `json:"value" xml:"value"`
	Unit      string    `json:"unit" xml:"unit"`
	Timestamp time.Time `json:"timestamp" xml:"timestamp"`
	Version   string    `json:"version" xml:"version"`
}

// NewForm creates a new form of type SignalA
func (sig *SignalA_v1a) NewForm() Form {
	sig.Version = "SignalA_v1.0"
	return sig
}

// FormVersion returns the version of the form
func (sig *SignalA_v1a) FormVersion() string {
	return sig.Version
}

// Register SignalA_v1a in the formTypeMap
func init() {
	FormTypeMap["SignalA_v1.0"] = reflect.TypeOf(SignalA_v1a{})
}

// SignalB is a single binary signal recorded at a given time (timestamp)
type SignalB_v1a struct {
	XMLName   xml.Name  `json:"-" xml:"SignalB"`
	Value     bool      `json:"value" xml:"value"`
	Timestamp time.Time `json:"timestamp" xml:"timestamp"`
	Version   string    `json:"version" xml:"version"`
}

// NewForm creates a new form of type SignalB
func (sig *SignalB_v1a) NewForm() Form {
	sig.Version = "SignalB_v1.0"
	return sig
}

// FormVersion returns the version of the form
func (sig *SignalB_v1a) FormVersion() string {
	return sig.Version
}

// Register SignalB_v1a in the formTypeMap
func init() {
	FormTypeMap["SignalB_v1.0"] = reflect.TypeOf(SignalB_v1a{})
}
