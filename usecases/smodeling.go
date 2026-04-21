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
	"sort"
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
	sb.WriteString(sysmlBehaviorDefs(sys))
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
// Per-asset type names are qualified with the system name to prevent collisions
// when two systems happen to use the same asset name (e.g. both kgrapher and
// modeler defining an "assembler" asset).
func sysmlBlockDefs(sys *components.System) string {
	var out strings.Builder

	out.WriteString("    // ── Block Definitions (BDD) ──────────────────────────────────────────────\n")

	// System-level block. Specialises from an mAF abstract type so core
	// systems (ServiceRegistrar, Orchestrator, CertificateAuthority) are
	// structurally distinguishable from domain systems. Attributes declared
	// here (host, ports) let the LocalCloud IBD instance assign
	// per-deployment values via 'redefines'.
	sysBlockName := sysmlName(sys.Name) + "System"
	out.WriteString(fmt.Sprintf("    part def '%s' :> %s {\n", sysBlockName, systemBaseType(sys.Name)))
	out.WriteString(fmt.Sprintf("        attribute name : String = \"%s\";\n", sys.Name))
	out.WriteString("        attribute host : String;\n")
	for proto, port := range sys.Husk.ProtoPort {
		if port != 0 {
			out.WriteString(fmt.Sprintf("        attribute %sPort : Integer;\n", proto))
		}
	}
	for assetName := range sys.UAssets {
		out.WriteString(fmt.Sprintf("        part '%s' : '%s';\n", assetName, assetTypeName(sys, assetName)))
	}
	out.WriteString("    }\n\n")

	// Per-asset blocks: each asset specialises from mAF::UnitAsset.
	for assetName, ua := range sys.UAssets {
		out.WriteString(fmt.Sprintf("    part def '%s' :> UnitAsset {\n", assetTypeName(sys, assetName)))
		if ua.Mission != "" {
			out.WriteString(fmt.Sprintf("        attribute mission : String = \"%s\";\n", ua.Mission))
		}

		for _, svc := range ua.GetServices() {
			out.WriteString(fmt.Sprintf("        out port '%s' : '%s';  // provided service\n",
				svc.Definition, svc.Definition))
		}
		for _, cerv := range ua.GetCervices() {
			out.WriteString(fmt.Sprintf("        in port '%s' : '%s';  // consumed service\n",
				cerv.Definition, cerv.Definition))
		}

		// Link the asset to its behaviour when cervices carry Mode tags.
		// The "perform action <local-name> : <type>" form is the SysML v2
		// canonical way to declare that a part performs an action def.
		if assetHasBehavior(ua) {
			out.WriteString(fmt.Sprintf("        perform action behave : '%s';\n", behaviorTypeName(sys, assetName)))
		}

		out.WriteString("    }\n\n")
	}

	return out.String()
}

// sysmlIBD emits the IBD: the instantiated system part with host metadata, provided
// service URLs, and @connect annotations for consumed services.
//
// Nested part usages are intentionally omitted here: they are already declared in the
// BDD part def, so redeclaring them inside the instance would cause a redefinition
// conflict in any conforming SysML v2 tool.  Cross-system connect statements belong
// at the package level and are generated by the modeler when it assembles all systems.
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

	if len(sys.Husk.Host.IPAddresses) > 0 {
		for assetName, ua := range sys.UAssets {
			for _, svc := range ua.GetServices() {
				for proto, port := range sys.Husk.ProtoPort {
					if port == 0 {
						continue
					}
					url := proto + "://" + sys.Husk.Host.IPAddresses[0] + ":" +
						strconv.Itoa(port) + "/" + sys.Name + "/" + assetName + "/" + svc.SubPath
					// Path format "<asset>.<definition>" lets the modeler resolve
					// @connect URLs back to provider ports when building the
					// LocalCloud IBD's connect statements.
					out.WriteString(fmt.Sprintf("        // provides %s.%s at %s\n",
						assetName, svc.Definition, url))
				}
			}
			for _, cerv := range ua.GetCervices() {
				if len(cerv.Nodes) == 0 {
					out.WriteString(fmt.Sprintf("        // @connect %s.%s → (no registered provider)\n",
						assetName, cerv.Definition))
					continue
				}
				for _, nodes := range cerv.Nodes {
					for _, ni := range nodes {
						out.WriteString(fmt.Sprintf("        // @connect %s.%s → %s\n",
							assetName, cerv.Definition, ni.URL))
					}
				}
			}
		}
	}

	out.WriteString("    }\n\n")
	return out.String()
}

// sysmlBehaviorDefs emits abstract action defs and per-asset behavior action defs
// for unit assets whose cervices have Mode set to "get" or "set".
// A compute step is inserted between gets and sets when both are present.
func sysmlBehaviorDefs(sys *components.System) string {
	type assetBehavior struct {
		name string
		gets []string // cervice definitions with Mode=="get", sorted
		sets []string // cervice definitions with Mode=="set", sorted
	}

	assetNames := make([]string, 0, len(sys.UAssets))
	for name := range sys.UAssets {
		assetNames = append(assetNames, name)
	}
	sort.Strings(assetNames)

	var behaviors []assetBehavior

	for _, assetName := range assetNames {
		ua := sys.UAssets[assetName]
		var gets, sets []string
		for _, c := range ua.GetCervices() {
			switch c.Mode {
			case "get":
				gets = append(gets, c.Definition)
			case "set":
				sets = append(sets, c.Definition)
			}
		}
		if len(gets) == 0 && len(sets) == 0 {
			continue
		}
		sort.Strings(gets)
		sort.Strings(sets)
		behaviors = append(behaviors, assetBehavior{name: assetName, gets: gets, sets: sets})
	}

	if len(behaviors) == 0 {
		return ""
	}

	// GetState / SetState / Compute are declared in the mAF library emitted
	// by the modeler, so we don't re-declare them here. We only emit the
	// per-asset behaviour defs that reference them.
	var out strings.Builder
	out.WriteString("    // ── Behaviour Definitions ────────────────────────────────────────────────\n")
	for _, ab := range behaviors {
		out.WriteString(fmt.Sprintf("    action def '%s' {\n", behaviorTypeName(sys, ab.name)))

		for _, g := range ab.gets {
			out.WriteString(fmt.Sprintf("        action 'get_%s' : GetState;\n", g))
		}
		hasCompute := len(ab.gets) > 0 && len(ab.sets) > 0
		if hasCompute {
			out.WriteString("        action compute : Compute;\n")
		}
		for _, s := range ab.sets {
			out.WriteString(fmt.Sprintf("        action 'set_%s' : SetState;\n", s))
		}

		var steps []string
		for _, g := range ab.gets {
			steps = append(steps, fmt.Sprintf("'get_%s'", g))
		}
		if hasCompute {
			steps = append(steps, "compute")
		}
		for _, s := range ab.sets {
			steps = append(steps, fmt.Sprintf("'set_%s'", s))
		}

		if len(steps) > 1 {
			out.WriteString("\n")
			for i := 0; i < len(steps)-1; i++ {
				out.WriteString(fmt.Sprintf("        first %s then %s;\n", steps[i], steps[i+1]))
			}
		}

		out.WriteString("    }\n\n")
	}

	return out.String()
}

// sysmlName converts a name to a SysML-compatible identifier by replacing
// characters that would be awkward outside quoted names (hyphens, spaces, dots).
func sysmlName(name string) string {
	r := strings.NewReplacer("-", "_", " ", "_", ".", "_")
	return r.Replace(name)
}

// assetTypeName builds the SysML v2 type name for a unit asset, qualified by
// the system that owns it so that two systems with an identically named asset
// do not produce conflicting 'part def' declarations in the merged package.
func assetTypeName(sys *components.System, assetName string) string {
	return sysmlName(sys.Name) + "_" + sysmlName(assetName) + "UnitAsset"
}

// behaviorTypeName builds the SysML v2 name for a unit asset's behaviour
// action def, qualified by the system for the same collision-avoidance reason
// as assetTypeName.
func behaviorTypeName(sys *components.System, assetName string) string {
	return sysmlName(sys.Name) + "_" + sysmlName(assetName) + "Behavior"
}

// assetHasBehavior reports whether a unit asset has at least one cervice
// tagged with Mode "get" or "set" — the precondition for generating a
// behaviour action def and a matching perform reference.
func assetHasBehavior(ua *components.UnitAsset) bool {
	for _, c := range ua.GetCervices() {
		if c.Mode == "get" || c.Mode == "set" {
			return true
		}
	}
	return false
}

// systemBaseType maps an Arrowhead system name to the mAF abstract type it
// should specialise from. Core infrastructure systems get role-specific
// types so a SysML v2 tool can answer "which part def is a ServiceRegistrar?"
// with a simple type query.
func systemBaseType(name string) string {
	switch name {
	case "serviceregistrar":
		return "ServiceRegistrar"
	case "orchestrator":
		return "Orchestrator"
	case "ca":
		return "CertificateAuthority"
	default:
		return "ArrowheadSystem"
	}
}
