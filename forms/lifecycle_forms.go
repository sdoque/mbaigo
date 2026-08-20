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

// Lifecycle: what an activity costs to run, in money and in carbon.

import (
	"reflect"
	"time"
)

// ActivityCostForm_v1 struct defines the schema for activity cost data
type ActivityCostForm_v1 struct {
	Activity  string    `json:"activity"`
	Cost      float64   `json:"cost"`
	Unit      string    `json:"unit"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

func (f *ActivityCostForm_v1) NewForm() Form {
	f.Version = "ActivityCostForm_v1"
	return f
}

func (f *ActivityCostForm_v1) FormVersion() string {
	return f.Version
}

// Register ActivityCostForm_v1 in the formTypeMap
func init() {
	FormTypeMap["ActivityCostForm_v1"] = reflect.TypeOf(ActivityCostForm_v1{})
}

///////////////////////////////////////////////////////////////////////////////

// CarbonFootprintForm_v1 struct defines the schema for carbon footprint data
type CarbonFootprintForm_v1 struct {
	Activity  string    `json:"activity"`
	Footprint float64   `json:"footprint"` // in metric tonnes
	Unit      string    `json:"unit"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

func (f *CarbonFootprintForm_v1) NewForm() Form {
	f.Version = "CarbonFootprintForm_v1"
	return f
}

func (f *CarbonFootprintForm_v1) FormVersion() string {
	return f.Version
}

// Register CarbonFootprintForm_v1 in the formTypeMap
func init() {
	FormTypeMap["CarbonFootprintForm_v1"] = reflect.TypeOf(CarbonFootprintForm_v1{})
}

///////////////////////////////////////////////////////////////////////////////
