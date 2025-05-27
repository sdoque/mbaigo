/*******************************************************************************
 * Copyright (c) 2025 Synecdoque
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
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// System struct aggregates an Arrowhead compliant system
type System struct {
	Name          string                `json:"systemName"`
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
	Name string `json:"coreSystem"`
	Url  string `json:"url"`
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

// GetRunningCoreSystemURL returns the URL of a running core system based on the provided type.
// When systemType is "serviceregistrar", it verifies the service is the lead registrar by checking
// its /status endpoint response. For other core system types, it simply tests that the URL is accessible.
func GetRunningCoreSystemURL(sys *System, systemType string) (string, error) {
	for _, core := range sys.CoreS {
		if core.Name == systemType {
			// Special logic for the service registrar: check the status endpoint
			if systemType == "serviceregistrar" {
				statusURL := core.Url + "/status"
				resp, err := http.Get(statusURL)
				if err != nil {
					fmt.Printf("error checking service registrar status at %s: %v\n", statusURL, err)
					continue // Try the next core system instance, if any.
				}
				bodyBytes, err := io.ReadAll(resp.Body)
				resp.Body.Close() // Always close the response body when done.
				if err != nil {
					fmt.Printf("error reading response from %s: %v\n", statusURL, err)
					continue
				}
				// Verify status response
				if strings.HasPrefix(string(bodyBytes), "lead Service Registrar since") {
					fmt.Printf("Lead service registrar found at: %s\n", core.Url)
					return core.Url, nil
				}
			} else {
				// For other core systems, verify that the service is accessible.
				resp, err := http.Get(core.Url)
				if err != nil {
					fmt.Printf("error checking %s at %s: %v\n", systemType, core.Url, err)
					continue
				}
				resp.Body.Close()
				return core.Url, nil
			}
		}
	}
	return "", fmt.Errorf("failed to locate running core system of type %s", systemType)
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
