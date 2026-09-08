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

// Space: sweeps, poses and maps — for services whose value is a shape rather
// than a number.
//
// Every measurement form before this one was scalar: SignalA is a float and
// SignalB a boolean. A scanning rangefinder produces neither, and collapsing
// its sweep to one number throws away the entire field of view, which is what
// the file-based integration on the mini wheel loader does today.

import (
	"encoding/xml"
	"fmt"
	"reflect"
	"time"
)

// ScanA_v1a is one sweep of a scanning rangefinder: parallel arrays of angle,
// distance and validity, taken over a span of time rather than at an instant.
//
// Valid is a separate array and not a sentinel distance, and that is the whole
// point of this form. A rangefinder that sees nothing — fog, a black surface,
// standing water, beyond range — reports either its maximum range or an error,
// and if "no valid return" reaches a consumer as "clear to 50 m" the vehicle
// drives into what it cannot see. A consumer must be unable to read this form
// without also reading whether the reading happened.
//
// SweepDuration is carried because a sweep is not instantaneous: on a moving
// vehicle the first and last points are taken from different places. Nothing
// in this framework corrects for that yet, but a consumer cannot correct for
// it later if the provider never said how long the sweep took.
type ScanA_v1a struct {
	XMLName xml.Name `json:"-" xml:"ScanA"`

	Angles    []float64 `json:"angles" xml:"angles"`       // one per point
	Distances []float64 `json:"distances" xml:"distances"` // one per point
	Valid     []bool    `json:"valid" xml:"valid"`         // one per point

	AngleUnit    string `json:"angleUnit" xml:"angleUnit"`
	DistanceUnit string `json:"distanceUnit" xml:"distanceUnit"`

	// Timestamp is when the sweep finished; SweepDuration is how long it took.
	Timestamp     time.Time     `json:"timestamp" xml:"timestamp"`
	SweepDuration time.Duration `json:"sweepDuration" xml:"sweepDuration"`

	Version string `json:"version" xml:"version"`
}

// NewForm creates a new form of type ScanA
func (s *ScanA_v1a) NewForm() Form {
	s.Version = "ScanA_v1.0"
	return s
}

// FormVersion returns the version of the form
func (s *ScanA_v1a) FormVersion() string {
	return s.Version
}

// Check reports whether the three parallel arrays agree in length.
//
// They are separate slices in JSON and nothing but this stops a provider from
// sending 271 angles and 270 distances. A consumer that indexes them in step
// would then read one point's distance against another point's angle — an
// obstacle in the wrong direction, which is worse than no obstacle at all.
func (s *ScanA_v1a) Check() error {
	if len(s.Angles) != len(s.Distances) || len(s.Angles) != len(s.Valid) {
		return fmt.Errorf("scan arrays disagree: %d angles, %d distances, %d validity flags",
			len(s.Angles), len(s.Distances), len(s.Valid))
	}
	return nil
}

// ValidCount is how many points in the sweep were actual returns.
func (s *ScanA_v1a) ValidCount() int {
	n := 0
	for _, ok := range s.Valid {
		if ok {
			n++
		}
	}
	return n
}

// Register ScanA_v1a in the formTypeMap
func init() {
	FormTypeMap["ScanA_v1.0"] = reflect.TypeOf(ScanA_v1a{})
}

// PoseA_v1a is a position and heading in a named frame.
//
// One form rather than three SignalA services, because a pose read as three
// separate requests is three samples from three moments: a consumer asking a
// moving vehicle for x, then y, then heading gets a position the vehicle was
// never in. The three numbers are only meaningful together.
//
// Frame names the coordinate system the pose is expressed in, because a pose
// without a frame is not a location. "map" is the frame a cartographer builds.
type PoseA_v1a struct {
	XMLName xml.Name `json:"-" xml:"PoseA"`

	X       float64 `json:"x" xml:"x"`
	Y       float64 `json:"y" xml:"y"`
	Heading float64 `json:"heading" xml:"heading"`

	Frame        string `json:"frame" xml:"frame"`
	DistanceUnit string `json:"distanceUnit" xml:"distanceUnit"`
	AngleUnit    string `json:"angleUnit" xml:"angleUnit"`

	Timestamp time.Time `json:"timestamp" xml:"timestamp"`
	Version   string    `json:"version" xml:"version"`
}

// NewForm creates a new form of type PoseA
func (p *PoseA_v1a) NewForm() Form {
	p.Version = "PoseA_v1.0"
	return p
}

// FormVersion returns the version of the form
func (p *PoseA_v1a) FormVersion() string {
	return p.Version
}

// Register PoseA_v1a in the formTypeMap
func init() {
	FormTypeMap["PoseA_v1.0"] = reflect.TypeOf(PoseA_v1a{})
}

// MapA_v1a is an occupancy grid: a rectangle of cells, each holding how
// strongly the evidence says that cell is occupied.
//
// Cells are one byte each and travel as base64 in JSON, which Go does for a
// []byte without being asked. The three reserved values matter more than the
// scale: a cell nothing has ever seen is Unknown, and it must not be confused
// with a cell that has been looked at and found empty. A map that reports
// unseen ground as free is the same failure as a rangefinder reporting
// no-return as clear.
//
// Origin is the world coordinate of the centre of cell (0,0), so a consumer can
// place the grid without knowing how the provider grew it.
type MapA_v1a struct {
	XMLName xml.Name `json:"-" xml:"MapA"`

	Width      int     `json:"width" xml:"width"`           // cells
	Height     int     `json:"height" xml:"height"`         // cells
	Resolution float64 `json:"resolution" xml:"resolution"` // metres per cell
	OriginX    float64 `json:"originX" xml:"originX"`
	OriginY    float64 `json:"originY" xml:"originY"`

	// Cells is row-major, Width*Height bytes: MapUnknown, or an occupancy
	// probability scaled to MapFree..MapOccupied.
	Cells []byte `json:"cells" xml:"cells"`

	Frame        string `json:"frame" xml:"frame"`
	DistanceUnit string `json:"distanceUnit" xml:"distanceUnit"`

	Timestamp time.Time `json:"timestamp" xml:"timestamp"`
	Version   string    `json:"version" xml:"version"`
}

// The three cell values a consumer must distinguish.
const (
	MapUnknown  byte = 255 // never observed — not the same as free
	MapFree     byte = 0
	MapOccupied byte = 100
)

// NewForm creates a new form of type MapA
func (m *MapA_v1a) NewForm() Form {
	m.Version = "MapA_v1.0"
	return m
}

// FormVersion returns the version of the form
func (m *MapA_v1a) FormVersion() string {
	return m.Version
}

// Check reports whether the cell count matches the declared rectangle.
func (m *MapA_v1a) Check() error {
	if m.Width < 0 || m.Height < 0 {
		return fmt.Errorf("map has negative extent: %d x %d", m.Width, m.Height)
	}
	if len(m.Cells) != m.Width*m.Height {
		return fmt.Errorf("map declares %d x %d = %d cells but carries %d",
			m.Width, m.Height, m.Width*m.Height, len(m.Cells))
	}
	return nil
}

// Register MapA_v1a in the formTypeMap
func init() {
	FormTypeMap["MapA_v1.0"] = reflect.TypeOf(MapA_v1a{})
}
