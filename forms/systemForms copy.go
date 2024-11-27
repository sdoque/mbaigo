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

import "reflect"

type SystemRecordList_v1 struct {
	List    []string `json:"systemurl"`
	Version string   `json:"version"`
}

func (f *SystemRecordList_v1) NewForm() Form {
	f.Version = "SystemRecordList_v1"
	return f
}

func (f *SystemRecordList_v1) FormVersion() string {
	return f.Version
}

// Register SystemRecordList_v1 in the formTypeMap
func init() {
	FormTypeMap["SystemRecordList_v1"] = reflect.TypeOf(SystemRecordList_v1{})
}
