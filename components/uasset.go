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

import (
	"fmt"
	"net/http"
	"strings"
)

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

// A unit asset's Mission is a coarse classification of what the asset is *for*.
// It is the axis along which the authorizer evaluates policy, so the vocabulary
// is closed: a mission outside this set is a configuration error rather than a
// free-text label. The taxonomy is specified in systems/authorizer/MISSIONS.md,
// which is the normative document; this file is its encoding.
const (
	// MissionMeasurement observes physical or digital state without changing it.
	MissionMeasurement = "measurement"
	// MissionActuation changes physical or digital state.
	MissionActuation = "actuation"
	// MissionState is the internal mode, schedule or configuration of a system.
	MissionState = "state"
	// MissionEvent is an ephemeral notification, alarm or transition.
	MissionEvent = "event"
	// MissionAggregation is a value derived or computed from other assets' output.
	MissionAggregation = "aggregation"
	// MissionLogging is a write-mostly sink for audit trails or data.
	MissionLogging = "logging"
	// MissionControl is a bidirectional loop that both observes and acts.
	MissionControl = "control"
	// MissionTransaction is a business record or exchange: an order, a
	// maintenance notification, a confirmation.
	MissionTransaction = "transaction"
	// MissionCore is framework infrastructure: registrar, orchestrator, CA,
	// authorizer, maitreD.
	MissionCore = "core"
)

// Missions is the taxonomy in the order it is documented. Used to render the
// permitted values in configuration errors, so an operator does not have to find
// the specification to fix a typo.
var Missions = []string{
	MissionMeasurement,
	MissionActuation,
	MissionState,
	MissionEvent,
	MissionAggregation,
	MissionLogging,
	MissionControl,
	MissionTransaction,
	MissionCore,
}

// ValidMission reports whether m belongs to the taxonomy.
func ValidMission(m string) bool {
	for _, known := range Missions {
		if m == known {
			return true
		}
	}
	return false
}

// EffectiveMission returns the mission a service is authorized under: the
// service's own when it declares one, otherwise the unit asset's.
//
// An asset's mission is the right granularity when the asset is a thing — a
// sensor, a valve, a controller. It is too coarse when the asset is an
// *interface* to things: a Modbus or OPC UA front end, an MQTT bridge, a ZigBee
// gateway. There the mission belongs to what is behind each service — a
// read-only register observes, a writable one acts — and an MQTT topic path
// discloses neither, so it has to be declared.
func EffectiveMission(ua *UnitAsset, serv *Service) string {
	if serv != nil && serv.Mission != "" {
		return serv.Mission
	}
	if ua == nil {
		return ""
	}
	return ua.Mission
}

// ValidateMission rejects an absent or unrecognized mission, naming the asset and
// listing the permitted values.
//
// An asset declares exactly one mission, and declaring none is not allowed. Any
// default would be either permissive enough to be a hole or restrictive enough to
// be worked around, and an optional field is one a commissioning technician can
// leave blank — which is how information models end up carrying no usable
// metadata. Refusing to start is what keeps the mission trustworthy enough to
// authorize against.
func ValidateMission(assetName, m string) error {
	if m == "" {
		return fmt.Errorf("unit asset %q declares no mission: expected one of %s",
			assetName, strings.Join(Missions, ", "))
	}
	if !ValidMission(m) {
		return fmt.Errorf("unit asset %q declares an unknown mission %q: expected one of %s",
			assetName, m, strings.Join(Missions, ", "))
	}
	return nil
}
