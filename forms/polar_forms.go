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
	"strings"
	"time"
)

// PolarA_v1a is a magnitude and a direction: how much, and which way.
//
// A wind is a speed and a direction; a destination is a distance and a bearing.
// Sent as two SignalA services they are two samples from two moments, and a
// consumer that asks for the speed and then the direction of a gusting wind
// gets a speed that never blew from that direction. The two numbers are only
// meaningful together, which is the same reason a pose is one form.
//
// Each carries its own unit. The magnitude is meters for a distance and meters
// per second for a wind; the direction is degrees or radians.
//
// # The frame is not optional
//
// "Which way" has no meaning until it says from what, and turning which way —
// and the conventions in use are not merely different, they are opposite in
// ways that produce no error:
//
//   - A navigator measures a bearing from straight ahead, counter-clockwise,
//     towards where the vehicle should go (ISO 8855).
//   - A meteorologist measures a wind from north, clockwise, and from where it
//     comes: a wind of 270 blows from the west, towards the east.
//
// Read a wind in the navigator's convention and every direction is wrong by a
// reflection and a half-turn, and still a plausible number. So Frame names both
// the reference and the sense, and a consumer should refuse a frame it does not
// know rather than assume one. The frames defined here:
//
//	PolarVehicle      0 is straight ahead, positive to the left (ISO 8855),
//	                  the direction something lies or goes towards.
//	PolarMap + "..."  0 is the map's x axis, positive counter-clockwise,
//	                  towards; the rest of the string names the map,
//	                  as a PoseA's frame does.
//	PolarCompassFrom  0 is north, positive clockwise, the direction something
//	                  comes FROM. The meteorological wind.
//	PolarCompassTo    0 is north, positive clockwise, the direction something
//	                  goes TO. A course, a current.
type PolarA_v1a struct {
	XMLName xml.Name `json:"-" xml:"PolarA"`

	// Magnitude is never negative. A negative magnitude is a direction turned
	// half round and not saying so, and Check refuses it.
	Magnitude     float64 `json:"magnitude" xml:"magnitude"`
	MagnitudeUnit string  `json:"magnitudeUnit" xml:"magnitudeUnit"`

	Direction     float64 `json:"direction" xml:"direction"`
	DirectionUnit string  `json:"directionUnit" xml:"directionUnit"`

	Frame string `json:"frame" xml:"frame"`

	Timestamp time.Time `json:"timestamp" xml:"timestamp"`
	Version   string    `json:"version" xml:"version"`
}

// The frames a PolarA_v1a direction may be measured in. PolarMap is a prefix:
// a map's full frame name follows it, as in "map@2026-09-27T10:00:00Z".
const (
	PolarVehicle     = "vehicle"
	PolarMap         = "map"
	PolarCompassFrom = "compass-from"
	PolarCompassTo   = "compass-to"
)

// NewForm creates a new form of type PolarA
func (p *PolarA_v1a) NewForm() Form {
	p.Version = "PolarA_v1.0"
	return p
}

// FormVersion returns the version of the form
func (p *PolarA_v1a) FormVersion() string {
	return p.Version
}

// Check refuses what a consumer could only misread: a magnitude that is
// negative or not a number, a direction that is not a number, and a frame it
// cannot know the convention of.
func (p *PolarA_v1a) Check() error {
	if math.IsNaN(p.Magnitude) || math.IsInf(p.Magnitude, 0) {
		return fmt.Errorf("magnitude is %v", p.Magnitude)
	}
	if p.Magnitude < 0 {
		return fmt.Errorf("magnitude %v is negative; turn the direction half round instead", p.Magnitude)
	}
	if math.IsNaN(p.Direction) || math.IsInf(p.Direction, 0) {
		return fmt.Errorf("direction is %v", p.Direction)
	}
	switch {
	case p.Frame == PolarVehicle, p.Frame == PolarCompassFrom, p.Frame == PolarCompassTo:
	case p.Frame == PolarMap, strings.HasPrefix(p.Frame, PolarMap+"@"):
	default:
		return fmt.Errorf("frame %q is not one whose convention is defined: %s, %s, %s or %s@<session>",
			p.Frame, PolarVehicle, PolarCompassFrom, PolarCompassTo, PolarMap)
	}
	return nil
}

// Radians is the direction in radians, whichever unit it was sent in. A
// direction unit that is neither degrees nor radians is an error rather than a
// guess.
func (p *PolarA_v1a) Radians() (float64, error) {
	switch p.DirectionUnit {
	case "", unitDegree:
		// An empty unit is read as degrees, the form's customary one, because
		// that is what a person typing a curl command means.
		return p.Direction * math.Pi / 180, nil
	case unitRadian:
		return p.Direction, nil
	}
	return 0, fmt.Errorf("direction unit %q is neither degrees nor radians", p.DirectionUnit)
}

const (
	unitDegree = "<http://qudt.org/vocab/unit/DEG>"
	unitRadian = "<http://qudt.org/vocab/unit/RAD>"
)

// Register PolarA_v1a in the formTypeMap
func init() {
	FormTypeMap["PolarA_v1.0"] = reflect.TypeOf(PolarA_v1a{})
}
