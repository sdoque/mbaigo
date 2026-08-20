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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// UnitAsset is the shared struct that every system's asset is built from.
// The system-specific configuration is held in Traits (any), and the HTTP
// dispatch logic is wired in via ServingFunc at construction time.
type UnitAsset struct {
	Name    string  `json:"name"`
	Mission Mission `json:"mission,omitempty"`
	Owner   *System `json:"-"`
	// Details is metadata about the asset — the open slot, as on a Service.
	// Anything a system wants said that has no field of its own goes here,
	// reaches the registrar, and becomes a predicate in the knowledge graph.
	//
	// Conventional keys, all optional:
	//
	//	FunctionalLocation  where the asset is, as an IRI or a name
	//	Mobility            whether the asset could run on another host
	//	TetheredTo          what a tethered asset must still be able to reach
	//
	// Mobility is what a load balancer needs and cannot derive. The graph says
	// which host each system runs on, so what is where is already known — what
	// is missing is whether any of it could be somewhere else. A ds18b20 reads a
	// 1-wire device on this machine's GPIO and can never move; a kgrapher can
	// move anywhere. Without the distinction, the first proposal a balancer
	// makes is to relocate the sensor.
	//
	// See MobilityFixed, MobilityTethered and MobilityMovable for the values and
	// what each obliges.
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

// Mission is a coarse classification of what a unit asset or service is *for*.
// It is the axis along which the authorizer evaluates policy, so the vocabulary
// is closed: a mission outside this set is a configuration error rather than a
// free-text label. The taxonomy is specified in systems/authorizer/MISSIONS.md,
// which is the normative document; this file is its encoding.
//
// A struct with an unexported field rather than a named string type, because
// only this rejects a mission nobody defined at compile time. Go assigns an
// untyped string constant to a named string type without complaint, so
// `Mission: "web_dashboard"` would still have built — and it did build, in four
// systems, each declaring something that reads like a mission and is not one.
// Three of them could not start at all and one seeded a configuration file the
// framework refuses; all four passed every test, every vet and every build.
//
// The cost is that a mission cannot be written as a literal. That is the point:
// the values below are the vocabulary, and anything arriving from outside the
// program — a configuration file, a registration record — comes through
// MissionFromString, where an unknown one is an error at the boundary it
// entered by rather than a string carried into the authorizer's reasoning.
type Mission struct {
	// name is unexported so that Mission{"anything"} cannot be written outside
	// this package, and the zero value means "none declared".
	name string
}

// String returns the mission as it appears in configuration files, registration
// records and the knowledge graph.
func (m Mission) String() string { return m.name }

// IsZero reports whether no mission has been declared. The zero value is not a
// mission: an asset that declares none is a configuration error, not a default.
func (m Mission) IsZero() bool { return m.name == "" }

// MarshalJSON writes the mission as the plain string the wire has always
// carried, so a Mission field and the string field it replaced are
// indistinguishable to anything reading the JSON.
func (m Mission) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.name)
}

// UnmarshalJSON reads a mission from JSON, rejecting one outside the taxonomy.
//
// The boundary is the right place for this. A value that arrives here comes
// from a configuration file an operator wrote or a record another system
// registered, and refusing it at the point it enters names the file and the
// field; carrying it inward turns a typo into an authorization question much
// later, where the message is about policy.
//
// An absent mission is not rejected here — a missing field and a wrong one are
// different mistakes, and ValidateMission is where the first is reported, with
// the asset's name to hand.
func (m *Mission) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}
	if name == "" {
		m.name = ""
		return nil
	}
	parsed, err := MissionFromString(name)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// MissionFromString turns a mission that arrived as text into one the rest of
// the program can hold, or reports that no such mission exists.
func MissionFromString(name string) (Mission, error) {
	for _, known := range Missions {
		if known.name == name {
			return known, nil
		}
	}
	return Mission{}, fmt.Errorf("unknown mission %q: expected one of %s", name, MissionNames())
}

var (
	// MissionMeasurement observes physical or digital state without changing it.
	MissionMeasurement = Mission{"measurement"}
	// MissionActuation changes physical or digital state.
	MissionActuation = Mission{"actuation"}
	// MissionState is the internal mode, schedule or configuration of a system.
	MissionState = Mission{"state"}
	// MissionEvent is an ephemeral notification, alarm or transition.
	MissionEvent = Mission{"event"}
	// MissionAggregation is a value derived or computed from other assets' output.
	MissionAggregation = Mission{"aggregation"}
	// MissionLogging is a write-mostly sink for audit trails or data.
	MissionLogging = Mission{"logging"}
	// MissionControl is a bidirectional loop that both observes and acts.
	MissionControl = Mission{"control"}
	// MissionTransaction is a business record or exchange: an order, a
	// maintenance notification, a confirmation.
	MissionTransaction = Mission{"transaction"}
	// MissionCore is framework infrastructure: registrar, orchestrator, CA,
	// authorizer, maitreD.
	MissionCore = Mission{"core"}
)

// Mobility says whether a unit asset could run on a different host, which is
// the question a load balancer asks and the graph cannot answer on its own.
//
// Three values rather than two, because the interesting middle case is the
// common one: most assets in this repository speak to a device over the network
// — Modbus TCP, OPC UA, MQTT, a Zigbee bridge — and so can move to any host that
// can still reach it. Collapsing that into "fixed" would freeze a cloud that is
// mostly relocatable; collapsing it into "movable" would license a move that
// silently breaks the connection it depended on.
const (
	// MobilityFixed is bound to this machine's hardware: GPIO, 1-wire, a serial
	// port, a USB device — or, in maitreD's case, bound by its purpose, since it
	// attests the host it runs on. Moving it is not a slower operation, it is a
	// different deployment.
	MobilityFixed = "fixed"

	// MobilityTethered can move to any host that can still reach what it talks
	// to. An asset declaring this owes the reader what it is tethered *to*, in a
	// TetheredTo detail beside it: a balancer must verify reachability before
	// proposing the move, and cannot do that against an unnamed dependency. A
	// tethered asset that names nothing should be read as fixed, because a move
	// nobody can check is a move nobody should make.
	MobilityTethered = "tethered"

	// MobilityMovable needs nothing of the machine it is on. It reads the
	// network and computes; it can run wherever the cloud has room.
	MobilityMovable = "movable"
)

// Mobilities is the vocabulary, for rendering permitted values in an error.
var Mobilities = []string{MobilityFixed, MobilityTethered, MobilityMovable}

// Missions is the taxonomy in the order it is documented. Used to render the
// permitted values in configuration errors, so an operator does not have to find
// the specification to fix a typo.
var Missions = []Mission{
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

// MissionNames lists the taxonomy as text, for the messages an operator reads.
func MissionNames() string {
	names := make([]string, 0, len(Missions))
	for _, m := range Missions {
		names = append(names, m.name)
	}
	return strings.Join(names, ", ")
}

// ValidMission reports whether m is a mission from the taxonomy rather than the
// zero value. A Mission cannot be anything else, so this is now a question about
// whether one was declared at all.
func ValidMission(m Mission) bool {
	return !m.IsZero()
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
func EffectiveMission(ua *UnitAsset, serv *Service) Mission {
	if serv != nil && !serv.Mission.IsZero() {
		return serv.Mission
	}
	if ua == nil {
		return Mission{}
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
// A Mission can no longer hold a value outside the taxonomy, so what is left to
// check is whether one was declared at all. An unknown one is refused earlier,
// where the text came in.
func ValidateMission(assetName string, m Mission) error {
	if m.IsZero() {
		return fmt.Errorf("unit asset %q declares no mission: expected one of %s",
			assetName, MissionNames())
	}
	return nil
}
