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

// Package "components" addresses the structures of the components that
// are aggregated to form Arrowhead compliant systems in a local cloud.
// An Arrowhead local cloud is a system of systems, which are made up of a husk
// (a.k.a. a shell) and a unit-asset (a.k.a. an asset or a thing). The husk runs on a device,
// and exposes the unit assets' functionalities as services.

package components

import "net/http"

// UnitAsset is the shared struct that every system's asset is built from.
// The system-specific configuration is held in Traits (any), and the HTTP
// dispatch logic is wired in via ServingFunc at construction time.
type UnitAsset struct {
	Name        string                                           `json:"name"`
	Mission     string                                           `json:"mission,omitempty"`
	Owner       *System                                          `json:"-"`
	Details     map[string][]string                              `json:"details"`
	ServicesMap Services                                         `json:"-"`
	CervicesMap Cervices                                         `json:"-"`
	Traits      any                                              `json:"traits,omitempty"`
	ServingFunc func(http.ResponseWriter, *http.Request, string) `json:"-"`
}

// GetName returns the name of the unit asset.
func (ua *UnitAsset) GetName() string { return ua.Name }

// GetServices returns the services exposed by the unit asset.
func (ua *UnitAsset) GetServices() Services { return ua.ServicesMap }

// GetCervices returns the services consumed by the unit asset.
func (ua *UnitAsset) GetCervices() Cervices { return ua.CervicesMap }

// GetDetails returns the metadata details of the unit asset.
func (ua *UnitAsset) GetDetails() map[string][]string { return ua.Details }

// GetTraits returns the system-specific traits of the unit asset.
func (ua *UnitAsset) GetTraits() any { return ua.Traits }

// Serving dispatches an incoming HTTP request to the system-specific handler.
func (ua *UnitAsset) Serving(w http.ResponseWriter, r *http.Request, servicePath string) {
	if ua.ServingFunc != nil {
		ua.ServingFunc(w, r, servicePath)
	}
}
