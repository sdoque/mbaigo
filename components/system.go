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
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// System struct aggragates an Arrowhead compliant system
type System struct {
	Name          string                `json:"systemname"`
	Host          *HostingDevice        // the system runs on a device
	Husk          *Husk                 // the system aggregates a "husk" (a wrapper or a shell)
	UAssets       map[string]*UnitAsset // the system aggregates "asset", which is made up of one or more unit-asset
	CoreS         []*CoreSystem         // the system is part of a local cloud with mandatory core systems
	Ctx           context.Context       // create a context that can be cancelled
	Sigs          chan os.Signal        // channel to initiate a graceful shutdown when Ctrl+C is pressed
	RegistrarChan chan *CoreSystem      // channel for the lead service registrar
}

// CoreSystem struct holds details about the core system included in the configuration file
type CoreSystem struct {
	Name        string `json:"coresystem"`
	Url         string `json:"url"`
	Certificate string `json:"-"`
}

// NewSystem instantiates the new system and gathers the host information
func NewSystem(name string, ctx context.Context) System {
	getBuildInfo()
	newSystem := System{Name: name}
	newSystem.Ctx = ctx
	newSystem.Sigs = make(chan os.Signal, 1)
	signal.Notify(newSystem.Sigs, syscall.SIGINT)
	newSystem.RegistrarChan = make(chan *CoreSystem, 1)
	newSystem.Host = NewDevice()
	newSystem.UAssets = make(map[string]*UnitAsset) // initialize UAsset as an empty map
	return newSystem
}

// The following code is used only for issues support on GitHub @sdoque --------------------------
var (
	AppName   string
	Version   string
	BuildDate string
	BuildHash string
)

func getBuildInfo() {
	if AppName != "" {
		fmt.Printf("System: %s - %s\n", AppName, Version)
		fmt.Printf("Build date: %s\n", BuildDate)
		fmt.Printf("Build hash: %s\n", BuildHash)
	}
}
