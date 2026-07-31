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

package usecases

import (
	"fmt"

	"github.com/sdoque/mbaigo/components"
)

// ValidateMissions checks that every service the system is about to register
// resolves to a mission in the taxonomy, taking each service's own mission where
// it declares one and the unit asset's otherwise.
//
// This runs over the *constructed* unit assets rather than over the
// configuration file, because for a whole family of systems the mission is not
// in the file: a Modbus or OPC UA front end derives it from each register's
// access mode, and a gateway does not know its assets until it has talked to the
// device. Validating at configuration time would reject those systems for
// omitting something they are supposed to determine at instantiation.
func ValidateMissions(sys *components.System) error {
	for _, ua := range sys.UAssets {
		asset := ua
		for _, serv := range (*asset).GetServices() {
			mission := components.EffectiveMission(asset, serv)
			if err := components.ValidateMission((*asset).GetName(), mission); err != nil {
				return fmt.Errorf("service %q: %w", serv.Definition, err)
			}
		}
	}
	return nil
}
