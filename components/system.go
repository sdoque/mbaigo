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
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// System struct aggregates an Arrowhead compliant system
type System struct {
	Name    string                `json:"systemName"`
	Husk    *Husk                 // the system aggregates a "husk" (a wrapper or a shell)
	UAssets map[string]*UnitAsset // the system aggregates "asset", which is made up of one or more unit-asset
	Ctx     context.Context       // create a context that can be cancelled
	Sigs    chan os.Signal        // channel to initiate a graceful shutdown when Ctrl+C is pressed
	Mutex   *sync.Mutex
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
	newSystem.UAssets = make(map[string]*UnitAsset) // initialize UAsset as an empty map
	// Since the return System isn't a pointer (incorrectly), this map needs to
	// be a pointer instead (usually not normal) and initialized (usually not needed)
	// in order to avoid linter errors.
	// The errors is due to this func returning a copy of newSystem and attempts
	// to copy the mutex too, but it's not allowed for sync objects.
	// Reference: https://stackoverflow.com/questions/37242009/function-returns-lock-by-value
	newSystem.Mutex = &sync.Mutex{}
	return newSystem
}

func verifyStatus(u *url.URL) ([]byte, error) {
	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Body must be fully drained AND closed upon returning, otherwise it might leak memory
	body, err := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("bad response: %d %s", resp.StatusCode, resp.Status)
	}
	return body, err
}

const ServiceRegistrarName string = "serviceregistrar"
const ServiceRegistrarLeader string = "lead Service Registrar since"

// GetRunningCoreSystemURL returns the URL of a running core system based on the provided type.
// When systemType is "serviceregistrar", it verifies the service is the lead registrar by checking
// its /status endpoint response. For other core system types, it simply tests that the URL is accessible.
func GetRunningCoreSystemURL(sys *System, systemType string) (string, error) {
	// Store the latest error encountered when iterating thru the system list
	// and then return this error if no matching system was found.
	var lastErr error

	for _, core := range sys.Husk.CoreS {
		// Ignore unrelated systems
		if core.Name != systemType {
			continue
		}

		coreURL, err := url.Parse(core.Url)
		if err != nil {
			lastErr = fmt.Errorf("parsing core URL: %w", err)
			continue
		}

		coreSystemURL := coreURL.String() // Preserves the original URL
		if core.Name != ServiceRegistrarName {
			return coreSystemURL, nil
		}

		// Perform extra checks on the response from a service registrar
		coreURL = coreURL.JoinPath("status")
		body, err := verifyStatus(coreURL)
		if err != nil {
			lastErr = fmt.Errorf("verifying registrar: %w", err)
			continue
		}

		// Skips non-leading registrars
		if !bytes.HasPrefix(body, []byte(ServiceRegistrarLeader)) {
			continue
		}
		return coreSystemURL, nil
	}

	err := fmt.Errorf("core system '%s' not found", systemType)
	if lastErr != nil {
		err = fmt.Errorf("core system '%s' not found: %w", systemType, lastErr)
	}
	return "", err
}

// The following code is used only for issues support on GitHub @sdoque
var (
	AppName   string
	Version   string
	BuildDate string
	BuildHash string
)

func getBuildInfo() {
	// TODO: This info should be updated when setting up version release tools
	// Leaving the fmt.Prints as is for now.
	if AppName != "" {
		fmt.Printf("System: %s - %s\n", AppName, Version)
		fmt.Printf("Build date: %s\n", BuildDate)
		fmt.Printf("Build hash: %s\n", BuildHash)
	}
}
