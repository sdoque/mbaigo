/*******************************************************************************
 * Copyright (c) 2026 Synecdoque
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, subject to the following conditions:
 *
 * The software is licensed under the MIT License. See the LICENSE file in this
 * repository for details.
 *
 * Contributors:
 *   Jan A. van Deventer, Luleå - initial implementation
 ***************************************************************************SDG*/

package forms

import (
	"encoding/xml"
	"fmt"
	"math"
	"reflect"
	"time"
)

// VectorA_v1a is a quantity with three components: an acceleration, an angular
// velocity, a magnetic field.
//
// One form rather than three SignalA services, for the reason PolarA is one
// form: the components are only meaningful together. Asked for one at a time
// they are three samples from three moments, and a vehicle that turned between
// the requests gets an acceleration whose x is from before the turn and whose y
// is from after it.
//
// All three components share one unit, the QUDT unit IRI, as a SignalA's does.
//
// # The frame is not optional
//
// A vector's components are only numbers until it is said along which axes,
// and the conventions in use differ in ways that raise no error: a chip's axes
// are where its maker printed them, and an aircraft's z points down where a
// car's points up. So Frame names the axes, and a consumer should refuse a
// frame it does not know rather than assume one. The frame defined here:
//
//	VectorVehicle  ISO 8855: x forward, y to the left, z up, fixed to the body
//	               the sensor is mounted on. Right-handed, so a rotation is
//	               positive counter-clockwise looking down the axis: a positive
//	               z angular velocity is a turn to the left.
//
// Another frame is added here, with its convention, when something needs it.
type VectorA_v1a struct {
	XMLName xml.Name `json:"-" xml:"VectorA"`

	X    float64 `json:"x" xml:"x"`
	Y    float64 `json:"y" xml:"y"`
	Z    float64 `json:"z" xml:"z"`
	Unit string  `json:"unit" xml:"unit"`

	Frame string `json:"frame" xml:"frame"`

	Timestamp time.Time `json:"timestamp" xml:"timestamp"`
	Version   string    `json:"version" xml:"version"`
}

// The frames a VectorA_v1a's components may be expressed in.
const (
	VectorVehicle = "vehicle"
)

// NewForm creates a new form of type VectorA
func (v *VectorA_v1a) NewForm() Form {
	v.Version = "VectorA_v1.0"
	return v
}

// FormVersion returns the version of the form
func (v *VectorA_v1a) FormVersion() string {
	return v.Version
}

// Check refuses what a consumer could only misread: a component that is not a
// number, and a frame it cannot know the axes of.
func (v *VectorA_v1a) Check() error {
	for _, c := range []struct {
		axis  string
		value float64
	}{{"x", v.X}, {"y", v.Y}, {"z", v.Z}} {
		if math.IsNaN(c.value) || math.IsInf(c.value, 0) {
			return fmt.Errorf("the %s component is %v", c.axis, c.value)
		}
	}
	if v.Frame != VectorVehicle {
		return fmt.Errorf("frame %q is not one whose axes are defined: %s", v.Frame, VectorVehicle)
	}
	return nil
}

// DistanceFrom is how far this vector is from an earlier one: the length of
// the difference, in the unit both are in. Two vectors in different units or
// different frames cannot be compared without a conversion this form does not
// know, so the answer is then that they cannot.
func (v *VectorA_v1a) DistanceFrom(earlier Form) (float64, bool) {
	e, ok := earlier.(*VectorA_v1a)
	if !ok || e.Unit != v.Unit || e.Frame != v.Frame {
		return 0, false
	}
	dx, dy, dz := v.X-e.X, v.Y-e.Y, v.Z-e.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz), true
}

// Register VectorA_v1a in the formTypeMap
func init() {
	FormTypeMap["VectorA_v1.0"] = reflect.TypeOf(VectorA_v1a{})
}
