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
 ***************************************************************************SDG*/

// SModeling generates a SysML v2 textual representation of a running system,
// covering Block Definition Diagrams (BDD) and Internal Block Diagrams (IBD).
//
// Mapping from Arrowhead concepts to SysML v2:
//   - System          → part def '<name>System' (BDD) + instantiated part (IBD)
//   - UnitAsset       → part def '<name>Block' with in/out ports
//   - Provided service → out port '<def>' : ~'<def>'   (conjugated = provided)
//   - Consumed service → in port '<def>' : '<def>'     (required)
//   - Cervice Nodes   → connect statements in the IBD

package usecases

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sdoque/mbaigo/components"
)

// SModeling writes a SysML v2 textual model of the system to the HTTP response.
func SModeling(w http.ResponseWriter, req *http.Request, sys *components.System) {
	model := sysmlPackage(sys)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, err := w.Write([]byte(model))
	if err != nil {
		log.Println("Failed to write SModeling information:", err)
	}
}

// sysmlPackage wraps the full model in a SysML v2 package block.
func sysmlPackage(sys *components.System) string {
	pkgName := sys.Husk.Host.Name + "_" + sys.Name

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("package '%s' {\n\n", pkgName))
	sb.WriteString(sysmlPortDefs(sys))
	sb.WriteString(sysmlBlockDefs(sys))
	sb.WriteString(sysmlIBD(sys))
	sb.WriteString("}\n")
	return sb.String()
}

// sysmlPortDefs emits one 'port def' per unique service definition across all unit assets.
// Each service definition becomes a named port type used by both providers and consumers.
func sysmlPortDefs(sys *components.System) string {
	seen := make(map[string]bool)
	var out strings.Builder

	out.WriteString("    // ── Port Definitions ─────────────────────────────────────────────────────\n")

	for _, ua := range sys.UAssets {
		for _, svc := range ua.GetServices() {
			if !seen[svc.Definition] {
				seen[svc.Definition] = true
				out.WriteString(fmt.Sprintf("    port def '%s';\n", svc.Definition))
			}
		}
		for _, cerv := range ua.GetCervices() {
			if !seen[cerv.Definition] {
				seen[cerv.Definition] = true
				out.WriteString(fmt.Sprintf("    port def '%s';\n", cerv.Definition))
			}
		}
	}

	out.WriteString("\n")
	return out.String()
}

// sysmlBlockDefs emits the BDD: one 'part def' for the overall system and one per unit asset.
func sysmlBlockDefs(sys *components.System) string {
	var out strings.Builder

	out.WriteString("    // ── Block Definitions (BDD) ──────────────────────────────────────────────\n")

	// System-level block
	sysBlockName := sysmlName(sys.Name) + "System"
	out.WriteString(fmt.Sprintf("    part def '%s' {\n", sysBlockName))
	out.WriteString(fmt.Sprintf("        attribute name : String = \"%s\";\n", sys.Name))
	for assetName := range sys.UAssets {
		out.WriteString(fmt.Sprintf("        part '%s' : '%s';\n", assetName, sysmlName(assetName)+"Block"))
	}
	out.WriteString("    }\n\n")

	// Per-asset blocks
	for assetName, ua := range sys.UAssets {
		out.WriteString(fmt.Sprintf("    part def '%s' {\n", sysmlName(assetName)+"Block"))
		if ua.Mission != "" {
			out.WriteString(fmt.Sprintf("        attribute mission : String = \"%s\";\n", ua.Mission))
		}

		for _, svc := range ua.GetServices() {
			out.WriteString(fmt.Sprintf("        out port '%s' : ~'%s';  // provided service\n",
				svc.Definition, svc.Definition))
		}
		for _, cerv := range ua.GetCervices() {
			out.WriteString(fmt.Sprintf("        in port '%s' : '%s';  // consumed service\n",
				cerv.Definition, cerv.Definition))
		}

		out.WriteString("    }\n\n")
	}

	return out.String()
}

// sysmlIBD emits the IBD: the instantiated system part containing asset parts and their connections.
func sysmlIBD(sys *components.System) string {
	var out strings.Builder
	sysBlockName := sysmlName(sys.Name) + "System"

	out.WriteString("    // ── Internal Block Diagram (IBD) ─────────────────────────────────────────\n")
	out.WriteString(fmt.Sprintf("    part '%s' : '%s' {\n", sys.Name, sysBlockName))
	out.WriteString(fmt.Sprintf("        attribute host : String = \"%s\";\n", sys.Husk.Host.Name))

	if len(sys.Husk.Host.IPAddresses) > 0 {
		out.WriteString(fmt.Sprintf("        attribute ipAddress : String = \"%s\";\n",
			sys.Husk.Host.IPAddresses[0]))
	}
	for proto, port := range sys.Husk.ProtoPort {
		if port != 0 {
			out.WriteString(fmt.Sprintf("        attribute %sPort : Integer = %d;\n", proto, port))
		}
	}

	for assetName, ua := range sys.UAssets {
		out.WriteString(fmt.Sprintf("\n        part '%s' : '%s' {\n", assetName, sysmlName(assetName)+"Block"))

		// Comment each provided service URL
		if len(sys.Husk.Host.IPAddresses) > 0 {
			for _, svc := range ua.GetServices() {
				for proto, port := range sys.Husk.ProtoPort {
					if port == 0 {
						continue
					}
					url := proto + "://" + sys.Husk.Host.IPAddresses[0] + ":" +
						strconv.Itoa(port) + "/" + sys.Name + "/" + assetName + "/" + svc.SubPath
					out.WriteString(fmt.Sprintf("            // provides: %s\n", url))
				}
			}
		}

		// connect statements for each known service provider
		for _, cerv := range ua.GetCervices() {
			if len(cerv.Nodes) == 0 {
				out.WriteString(fmt.Sprintf("            // '%s' not yet connected (no registered provider)\n",
					cerv.Definition))
				continue
			}
			for providerName, nodes := range cerv.Nodes {
				for _, ni := range nodes {
					out.WriteString(fmt.Sprintf("            connect '%s' to '%s'; // %s\n",
						cerv.Definition, providerName, ni.URL))
				}
			}
		}

		out.WriteString("        }\n")
	}

	out.WriteString("    }\n\n")
	return out.String()
}

// sysmlName converts a name to a SysML-compatible identifier by replacing
// characters that would be awkward outside quoted names (hyphens, spaces, dots).
func sysmlName(name string) string {
	r := strings.NewReplacer("-", "_", " ", "_", ".", "_")
	return r.Replace(name)
}
