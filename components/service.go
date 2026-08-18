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

// Package "components" addresses the structures and behaviors of the components that
// are aggregated to form Arrowhead compliant systems in a local cloud.
// An Arrowhead local cloud is a system of systems, which are made up of a capsule
// (a.k.a. a shell) and a thing (a.k.a. an asset). The capsule runs on a device,
// and exposes the thing's resources' skills as services.

package components

import (
	"net/http"
	"sync"
)

// An Arrowhead Service has specific properties that exposes a unit asset's functionality
type Service struct {
	ID            int                 `json:"-"`                  // Id assigned by the Service Registrar
	Definition    string              `json:"definition"`         // Service definition or purpose
	SubPath       string              `json:"subpath"`            // The URL subpath after the resource's
	Mission       Mission             `json:"mission,omitempty"`  // Overrides the unit asset's mission, where the asset's is too coarse (see EffectiveMission)
	Details       map[string][]string `json:"details"`            // Metadata or details about the service
	RegPeriod     int                 `json:"registrationPeriod"` // The period until the registrar is expecting a sign of life
	RegTimestamp  string              `json:"-"`                  // the creation date in the Service Registry to ensure that reRegistration is with the same record
	RegExpiration string              `json:"-"`                  // The actual time when the service record will expire if not refreshed
	Description   string              `json:"-"`                  // This is used in the service description in /doc
	// SubscribeAble says a consumer may follow this service's value rather than
	// ask for it repeatedly. Configurable, which it was not: the field carried
	// `json:"-"`, so a systemconfig.json could not turn it on and nothing but
	// code ever could.
	SubscribeAble bool `json:"subscribable,omitempty"`
	// Heartbeat is how long this service may go without saying anything to a
	// subscriber, in seconds. A subscriber that hears nothing for a few of these
	// treats the publisher as gone, so it is the liveness contract as much as a
	// refresh rate. Zero means the framework's default.
	Heartbeat int `json:"heartbeat,omitempty"`
	// Threshold is how much the value must move, in this service's own unit,
	// before it is worth telling anyone. Zero means any change is.
	//
	// In this service's unit, which matters: a consumer reading in °F and asking
	// for 0.5 is asking for something other than it thinks, so a proposal
	// carries its unit and is converted.
	Threshold float64 `json:"threshold,omitempty"`
	// FastestHeartbeat and FinestThreshold bound what a subscriber may ask for.
	// A consumer knows what it needs and the provider knows what it can honour —
	// a threshold below the sensor's resolution is meaningless and a heartbeat
	// faster than the sampling period is impossible — so a proposal is clamped
	// to these and the subscriber is told what it actually got.
	FastestHeartbeat int     `json:"fastestHeartbeat,omitempty"`
	FinestThreshold  float64 `json:"finestThreshold,omitempty"`
	// Stream carries this service's value to whoever is following it, and is nil
	// until the framework prepares one for a service that declares itself
	// subscribable.
	//
	// An interface rather than the publisher itself, because the publisher lives
	// in usecases and usecases already imports this package. What a service needs
	// to know about it is only whether it can be followed and how to answer
	// somebody who wants to.
	Stream ValueStream `json:"-"`

	ACost      float64 `json:"-"`        // activity cost to execute the service
	CUnit      string  `json:"costUnit"` // cost unit
	CFootprint float64 `json:"-"`        // carbon footprint in metric tonnes when executing the service
}

// ValueStream is a service's value as something to follow rather than to ask
// for.
type ValueStream interface {
	// Subscribable reports whether this service is meant to be followed. It is
	// safe on a nil stream, which is what a service that declares nothing has.
	Subscribable() bool
	// ServeStream answers a request to follow the value and holds the connection
	// open until the caller goes away.
	ServeStream(w http.ResponseWriter, r *http.Request)
}

// type Services is a collection of service structs
type Services map[string]*Service

// Merge method is used in the configuration use case to prevent the subpath or description to be changed or "configured"
func (s *Service) Merge(originalS *Service) {
	s.Definition = originalS.Definition
	s.SubPath = originalS.SubPath
	s.Description = originalS.Description
}

// DeepCopy creates a deep copy of the Service instance
func (s Service) DeepCopy() *Service {
	// Copy the map
	detailsCopy := make(map[string][]string)
	for key, value := range s.Details {
		// Copy each slice individually
		sliceCopy := make([]string, len(value))
		copy(sliceCopy, value)
		detailsCopy[key] = sliceCopy
	}

	// Create and return a new instance of Service with copied values
	return &Service{
		ID:            s.ID,
		Definition:    s.Definition,
		SubPath:       s.SubPath,
		Details:       detailsCopy,
		RegPeriod:     s.RegPeriod,
		RegTimestamp:  s.RegTimestamp,
		RegExpiration: s.RegExpiration,
		Description:   s.Description,
		SubscribeAble: s.SubscribeAble,
		Heartbeat:     s.Heartbeat,
		Threshold:     s.Threshold,

		FastestHeartbeat: s.FastestHeartbeat,
		FinestThreshold:  s.FinestThreshold,

		ACost: s.ACost,
		CUnit: s.CUnit,
	}
}

// DeepCopy creates a deep copy of the Services map
func CloneServices(sTemplates []Service) Services {
	services := make(map[string]*Service)
	for _, sTemplate := range sTemplates {
		newService := sTemplate.DeepCopy()
		serviceName := newService.Definition
		services[serviceName] = newService
	}
	return services
}

// Function to merge two Details maps
func MergeDetails(map1, map2 map[string][]string) map[string][]string {
	// Create a new map to hold the merged result
	result := make(map[string][]string)

	// Add all elements from map1
	for key, value := range map1 {
		result[key] = append([]string{}, value...)
	}

	// Add all elements from map2
	for key, value := range map2 {
		if existing, found := result[key]; found {
			// If the key exists, merge the slices
			result[key] = append(existing, value...)
		} else {
			// If the key does not exist, just add it
			result[key] = append([]string{}, value...)
		}
	}

	return result
}

// ---------------------------------------------------

// NodeInfo holds the URL and registered metadata for a single discovered service endpoint.
type NodeInfo struct {
	URL     string
	Details map[string][]string
	// Tokens are the access tokens the orchestrator obtained for this provider,
	// keyed by the action each was minted for — "read", "write" or "invoke".
	//
	// One per action, because a token names the action it permits and the
	// provider recomputes that action from the HTTP method it receives. A read
	// token presented on a PUT is refused, so a cervice used for both a GET and
	// a POST needs one of each — which a single token string could not hold.
	//
	// An entry with an empty value is meaningful: it records that this action was
	// discovered and the cloud issued no token, which is what an unauthorized
	// cloud does. Without it a consumer would re-orchestrate before every call.
	//
	// A token outlives none of the requests it is presented on: when it expires
	// the provider refuses, the node cache is cleared, and the next call
	// re-orchestrates for a fresh one.
	Tokens map[string]string
}

// TokenFor returns the token minted for one action, and whether this node has
// been discovered for that action at all. The two are distinct: a discovered
// action with no token is an unauthorized cloud, not a missing discovery.
func (ni NodeInfo) TokenFor(action string) (token string, discovered bool) {
	if ni.Tokens == nil {
		return "", false
	}
	token, discovered = ni.Tokens[action]
	return token, discovered
}

// A Cervice is a consumed service
type Cervice struct {
	IReferentce string // Internal reference when consuming more than one service of the same type
	Definition  string // Service definition or purpose
	Details     map[string][]string
	Nodes       map[string][]NodeInfo
	Protos      []string
	Mode        string // "get" for GetState, "set" for SetState, "" for unspecified

	// Mutex guards Nodes and the tokens inside it.
	//
	// Discovery replaces entries and now deletes them, consumption reads them to
	// dispatch and writes back to forget a token, and a unit asset with more
	// than one goroutine polls the same cervice from each. A map written during
	// a range over it is not a race that corrupts a value — it is `fatal error:
	// concurrent map iteration and map write`, which no recover reaches and
	// which takes the system down.
	//
	// Exported like System.Mutex, because some systems reach into Nodes
	// themselves. Hold it for the map work only: the point of the snapshots in
	// the consumption path is that a request to an unresponsive provider must
	// not be made while holding a lock every other goroutine wants.
	Mutex sync.RWMutex
}

// Providers returns the discovered providers as a snapshot.
//
// A copy, so the caller can make its requests without holding the lock. A
// consuming round asks every provider in turn, each request bounded only by its
// own timeout, and a discovery on another goroutine must not have to wait for
// an unresponsive sensor before it can record what it found.
func (c *Cervice) Providers() []NodeInfo {
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	var providers []NodeInfo
	for _, nodes := range c.Nodes {
		providers = append(providers, nodes...)
	}
	return providers
}

// ProviderCount reports how many providers have been discovered.
func (c *Cervice) ProviderCount() int {
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	n := 0
	for _, nodes := range c.Nodes {
		n += len(nodes)
	}
	return n
}

// Cervises is a collection of "Cervice" structs
type Cervices map[string]*Cervice
