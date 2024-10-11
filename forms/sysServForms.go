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

type ServiceRecord_v1 struct {
	Id                int                 `json:"registryID"`
	ServiceDefinition string              `json:"definition"`
	SystemName        string              `json:"systemName"`
	IPAddresses       []string            `json:"ipAddresses"`
	ProtoPort         map[string]int      `json:"protoPort"`
	Details           map[string][]string `json:"Details"`
	Certificate       string              `json:"certificate"`
	SubPath           string              `json:"subpath"`
	RegLife           int                 `json:"registrationLife"`
	Version           string              `json:"version"`
	Created           string              `json:"created"`
	Updated           string              `json:"updated"`
	EndOfValidity     string              `json:"endOfValidity"`
	SubscribeAble     bool                `json:"subscribeAble"`
	ACost             float64             `json:"activityCost"`
	CUnit             string              `json:"costUnit"`
}

func (f *ServiceRecord_v1) NewForm() Form {
	f.Version = "ServiceRecord_v1"
	return f
}

func (f *ServiceRecord_v1) FormVersion() string {
	return f.Version
}

// Register ServiceRecord_v1 in the formTypeMap
func init() {
	FormTypeMap["ServiceRecord_v1"] = reflect.TypeOf(ServiceRecord_v1{})
}

///////////////////////////////////////////////////////////////////////////////

type ServiceRecordList_v1 struct {
	List    []ServiceRecord_v1
	Version string
}

func (f *ServiceRecordList_v1) NewForm() Form {
	f.Version = "ServiceRecordList_v1"
	return f
}

func (f *ServiceRecordList_v1) FormVersion() string {
	return f.Version
}

// Register ActivityCostForm_v1 in the formTypeMap
func init() {
	FormTypeMap["ServiceRecordList_v1"] = reflect.TypeOf(ServiceRecordList_v1{})
}

///////////////////////////////////////////////////////////////////////////////

type SystemRecord_v1 struct {
	SystemName  string   `json:"systemName"`
	IPAddresses []string `json:"ipAddresses"`
	Port        int      `json:"protoPort"`
	Version     string   `json:"version"`
}

func (f *SystemRecord_v1) NewForm() Form {
	f.Version = "SystemRecord_v1"
	return f
}

func (f *SystemRecord_v1) FormVersion() string {
	return f.Version
}

// Register SystemRecord_v1 in the formTypeMap
func init() {
	FormTypeMap["SystemRecord_v1"] = reflect.TypeOf(SystemRecord_v1{})
}

///////////////////////////////////////////////////////////////////////////////

type SystemRecordList_v1 struct {
	List    []SystemRecord_v1
	Version string
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
