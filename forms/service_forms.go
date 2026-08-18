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
	ServiceNode       string              `json:"serviceNode"`
	Mission           string              `json:"mission,omitempty"`
	IPAddresses       []string            `json:"ipAddresses"`
	ProtoPort         map[string]int      `json:"protoPort"`
	Details           map[string][]string `json:"details"`
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
	List    []ServiceRecord_v1 `json:"list"`
	Version string             `json:"version"`
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

// What a subscription to the service registry reports.
//
// Registration and deregistration only. A service re-registers every RegPeriod
// seconds to confirm it is still there, and that is not a change: reporting it
// would wake every subscriber more than once a second in a cloud of this size,
// each time with a list identical to the last.
const (
	// RegistryRegistered: a service is in the registry that was not there
	// before — a unit asset was added, or the system providing it has started.
	RegistryRegistered = "registered"
	// RegistryDeregistered: a service is gone, whether it was withdrawn or its
	// registration lapsed. A subscriber cannot tell those apart and does not
	// need to: the service is unavailable either way.
	RegistryDeregistered = "deregistered"
)

// RegistryEvent_v1 reports one service entering or leaving the registry.
//
// It carries the whole record rather than a summary of it. Everything a
// subscriber might act on is already described there — which system, which
// service, where to reach it, what mission it serves, what details it
// registered — and restating a few of those fields in a second struct would
// leave two definitions to keep in step for no gain.
//
// One event per service, not per system. A system starting up registers its
// services in turn and so produces several, all naming the same system. That is
// what the registry observes; collapsing them would mean guessing when a system
// had finished starting, which the registry cannot know. It is also the only
// granularity that catches the case that matters most — a unit asset added to a
// system that is already running, where the system itself neither arrives nor
// departs.
//
// A subscriber whose reaction is expensive should make it idempotent rather
// than expect the registry to coalesce: marking a system stale and rebuilding
// on demand turns any number of events into one piece of work.
type RegistryEvent_v1 struct {
	// Change is RegistryRegistered or RegistryDeregistered.
	Change string `json:"change"`
	// Record is the service registration this event concerns. On a
	// deregistration it is the record as it last stood.
	Record ServiceRecord_v1 `json:"record"`
	// Timestamp is when the registry observed the change.
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
}

func (f *RegistryEvent_v1) NewForm() Form {
	f.Version = "RegistryEvent_v1"
	return f
}

func (f *RegistryEvent_v1) FormVersion() string {
	return f.Version
}

// Register RegistryEvent_v1 in the formTypeMap
func init() {
	FormTypeMap["RegistryEvent_v1"] = reflect.TypeOf(RegistryEvent_v1{})
}
