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

package forms

// Service discovery: what a consumer asks the orchestrator for, and the service point it is given.

import "reflect"

// ServiceQuest_v1 asks the registrar which services match.
//
// RequesterName and ProviderName are opposite ends of the question and are easy
// to confuse: RequesterName is who is asking, ProviderName filters on whose
// records are wanted — the same sense as ServicePoint_v1.ProviderName in the
// answer. Neither is an identity claim the registrar acts on; authorization uses
// the certificate CN of the connection, never a name carried in a form.
//
// ProviderName is what lets the authorizer ask "what does system X provide", so
// it can read that system's own attributes — its functional location, say — from
// the registry rather than from anything the caller asserted. Omitting it matches
// any provider.
//
// Action states what the consumer intends — read, write or invoke — so that
// discovery can be filtered to the providers it may actually use rather than to
// every provider of the definition. The registrar ignores it; only the authorizer
// reads it. An omitted action is taken as read, the least it could mean.
type ServiceQuest_v1 struct {
	SysId             int                 `json:"systemId"`
	RequesterName     string              `json:"requesterName"`
	ProviderName      string              `json:"providerName,omitempty"`
	ServiceDefinition string              `json:"serviceDefinition"`
	Action            string              `json:"action,omitempty"`
	Protocol          string              `json:"protocol"`
	Details           map[string][]string `json:"details"`
	Version           string              `json:"version"`
}

func (f *ServiceQuest_v1) NewForm() Form {
	f.Version = "ServiceQuest_v1"
	return f
}

func (f *ServiceQuest_v1) FormVersion() string {
	return f.Version
}

// Register ServiceQuest_v1 in the formTypeMap
func init() {
	FormTypeMap["ServiceQuest_v1"] = reflect.TypeOf(ServiceQuest_v1{})
}

///////////////////////////////////////////////////////////////////////////////

type ServicePoint_v1 struct {
	ServiceID         int                 `json:"serviceId"`
	ProviderName      string              `json:"providerName"`
	ServiceDefinition string              `json:"definition"`
	Details           map[string][]string `json:"details"`
	ServLocation      string              `json:"serviceURL"`
	ServNode          string              `json:"serviceNode"`
	Token             string              `json:"token"`
	// SubscribeAble says this provider will let a consumer follow the value
	// rather than ask for it repeatedly. Carried here because it is the consumer
	// that decides whether to follow, and this is what the consumer is handed.
	SubscribeAble bool   `json:"subscribeAble,omitempty"`
	Version       string `json:"version"`
}

func (f *ServicePoint_v1) NewForm() Form {
	f.Version = "ServicePoint_v1"
	return f
}

func (f *ServicePoint_v1) FormVersion() string {
	return f.Version
}

// ServicePointList_v1 is what a consumer receives when it asks for every
// provider of a service rather than one.
//
// A list of service points rather than of registration records, because the two
// carry different things. A record is what the registrar holds about a service;
// a service point is that plus what this consumer needs in order to use it —
// the endpoint chosen for the protocols it speaks, and the access token minted
// for it.
//
// The multi-provider answer used to be a ServiceRecordList_v1, which has
// nowhere to put a token. So every request from that path went out with an
// empty one and was refused by any provider in an authorized cloud, and the
// orchestrator did not filter the list by policy either — the authorizer was
// never consulted on that path at all.
type ServicePointList_v1 struct {
	List    []ServicePoint_v1 `json:"list"`
	Version string            `json:"version"`
}

func (f *ServicePointList_v1) NewForm() Form {
	f.Version = "ServicePointList_v1"
	return f
}

func (f *ServicePointList_v1) FormVersion() string {
	return f.Version
}

// Register the service point forms in the formTypeMap
func init() {
	FormTypeMap["ServicePoint_v1"] = reflect.TypeOf(ServicePoint_v1{})
	FormTypeMap["ServicePointList_v1"] = reflect.TypeOf(ServicePointList_v1{})
}
