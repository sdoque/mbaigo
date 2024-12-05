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

// An Arrowhead Service has specific properties that exposes a unit asset's functionality
type Service struct {
	ID            int                 `json:"-"`                  // Id assigned by the Service Registrar
	Definition    string              `json:"servicedefinition"`  // Service definition or purpose
	SubPath       string              `json:"-"`                  // The URL subpath after the resource's
	Details       map[string][]string `json:"details"`            // Metadata or details about the service
	RegPeriod     int                 `json:"registrationPeriod"` // The period until the registrar is expecting a sign of life
	RegTimestamp  string              `json:"-"`                  // the creation date in the Service Registry to ensure that reRegistration is with the same record
	RegExpiration string              `json:"-"`                  // The actual time when the service record will expire if not refreshed
	Description   string              `json:"-"`                  // This is used in the service description in /doc
	SubscribeAble bool                `json:"-"`                  // If true, one can subscribe to this service
	ACost         float64             `json:"-"`                  // activity cost to execute the service
	CUnit         string              `json:"costUnit"`           // cost unit
}

// type Services is a collection of service stucts
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
		ACost:         s.ACost,
		CUnit:         s.CUnit,
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

// A Cervice is a consumed service
type Cervice struct {
	Name    string
	Details map[string][]string
	Url     []string
	Protos  []string
}

// Cervises is a collection of "Cervice" structs
type Cervices map[string]*Cervice
