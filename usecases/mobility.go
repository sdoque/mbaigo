/*******************************************************************************
 * Copyright (c) 2026 Synecdoque
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
 ***************************************************************************SDG*/

package usecases

import (
	"log"

	"github.com/sdoque/mbaigo/components"
)

// mobilityDetail and tetherDetail are where these two facts used to live.
const (
	mobilityDetail = "Mobility"
	tetherDetail   = "TetheredTo"
)

// AdoptMobilityDetails moves a mobility written in Details onto the field that
// now holds it, for configuration files written before it had one.
//
// Every Raspberry Pi in this project carries a systemconfig.json with
// "Mobility": ["fixed"] in an asset's details, and those files are not rewritten
// on upgrade — a system reads its configuration and keeps it. Refusing to start,
// or silently losing the declaration, would both be wrong: the operator wrote a
// true thing in the only place that existed at the time.
//
// The detail is removed once adopted, because leaving it would put the same fact
// in two places that can disagree, and because Details still reaches the
// registrar: a copy left behind would go on being matched against as a
// requirement, which is the fault this whole change exists to remove.
//
// An unknown value is dropped with a warning rather than refused. The typed
// field refuses bad input at the boundary it arrives at, but this input arrived
// before there was a boundary, and a cloud that will not start because of a
// typo in a field nothing enforces yet is worse than one that says so and runs.
func AdoptMobilityDetails(sys *components.System) {
	for _, ua := range sys.UAssets {
		asset := ua
		details := asset.Details
		if len(details) == 0 {
			continue
		}

		if values, ok := details[mobilityDetail]; ok {
			if asset.Mobility.IsZero() && len(values) > 0 {
				m, err := components.MobilityFromString(values[0])
				if err != nil {
					log.Printf("%s: ignoring the Mobility detail: %v\n", asset.Name, err)
				} else {
					asset.Mobility = m
					log.Printf("%s: Mobility is a field now, not a detail — adopted %q from the configuration file; write it as \"mobility\" to silence this\n",
						asset.Name, m)
				}
			}
			delete(details, mobilityDetail)
		}

		if values, ok := details[tetherDetail]; ok {
			if len(asset.TetheredTo) == 0 && len(values) > 0 {
				asset.TetheredTo = values
			}
			delete(details, tetherDetail)
		}
	}
}

// ValidateMobilities checks every asset's declared mobility.
//
// Unlike a mission, an absent mobility is allowed. A mission is what the
// authorizer reasons about, so leaving it blank has no safe reading; a mobility
// is what a balancer reasons about, and an asset that declares none is simply
// one nothing may propose to move. That is the conservative answer, and it is
// the right default for the thirty-odd systems that have never said.
//
// What is checked is the obligation tethered carries. See
// components.ValidateMobility.
func ValidateMobilities(sys *components.System) error {
	for _, ua := range sys.UAssets {
		asset := ua
		if err := components.ValidateMobility(asset.Name, asset.Mobility, asset.TetheredTo); err != nil {
			return err
		}
	}
	return nil
}
