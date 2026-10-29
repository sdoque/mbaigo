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
//   - Provided service → out port '<def>' : '<Def>'
//   - Consumed service → in port '<def>' : '<Def>'
//   - Cervice Nodes   → connect statements in the IBD
//
// A port whose service declares a unit carries it: the value's SysML v2
// quantity type and measurement reference, and the QUDT unit and quantity kind
// the knowledge graph states for the same service (see sysml_units.go).

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
// The port def name is derived from the service definition so that it is
// distinct from the port usage name — SysML v2 requires def and usage to use
// different identifiers. See portDefName for the naming rule.
func sysmlPortDefs(sys *components.System) string {
	seen := make(map[string]bool)
	var out strings.Builder

	out.WriteString("    // ── Port Definitions ─────────────────────────────────────────────────────\n")

	for _, ua := range sys.UAssets {
		for _, svc := range ua.GetServices() {
			if !seen[svc.Definition] {
				seen[svc.Definition] = true
				out.WriteString(fmt.Sprintf("    port def '%s' :> ServicePort;\n", portDefName(svc.Definition)))
			}
		}
		for _, cerv := range ua.GetCervices() {
			if !seen[cerv.Definition] {
				seen[cerv.Definition] = true
				out.WriteString(fmt.Sprintf("    port def '%s' :> ServicePort;\n", portDefName(cerv.Definition)))
			}
		}
	}

	out.WriteString("\n")
	return out.String()
}

// portDefName derives the SysML v2 'port def' identifier from a service
// definition name. The result must differ from the port usage name (which
// is just the service definition). We capitalise the first letter; if the
// input is already capitalised (as with "OnOff"), we append "Port" so the
// def and the usage still resolve to different identifiers.
func portDefName(def string) string {
	if len(def) == 0 {
		return def
	}
	capFirst := strings.ToUpper(def[:1]) + def[1:]
	if capFirst == def {
		return def + "Port"
	}
	return capFirst
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
	// 'name' and 'host' are inherited from ArrowheadSystem in mAF — they
	// must use 'redefines' to avoid a duplicate-feature error in a strict
	// SysML v2 tool. 'httpPort' / 'coapPort' / 'httpsPort' are new, so no
	// redefines.
	out.WriteString(fmt.Sprintf("        attribute redefines name : String = \"%s\";\n", sys.Name))
	out.WriteString("        attribute redefines host : String;\n")
	for _, proto := range sortedProtocols(sys.Husk.ProtoPort) {
		out.WriteString(fmt.Sprintf("        attribute %sPort : Integer;\n", proto))
		// The endpoint's port number is bound to the attribute the cloud's
		// IBD gives a value, so the number is stated once.
		out.WriteString(fmt.Sprintf("        attribute %sEndpoint :> endpoints { :>> protocol = %q; :>> portNumber = %sPort; }\n",
			proto, proto, proto))
	}
	out.WriteString(sysmlPosture(sys))
	for assetName := range sys.UAssets {
		out.WriteString(fmt.Sprintf("        part '%s' : '%s';\n", assetName, assetTypeName(sys, assetName)))
	}
	out.WriteString("    }\n\n")

	// Per-asset blocks: each asset specialises from mAF::UnitAsset.
	for assetName, ua := range sys.UAssets {
		out.WriteString(fmt.Sprintf("    part def '%s' :> UnitAsset {\n", assetTypeName(sys, assetName)))
		// 'mission' and 'functionalLocation' are inherited from UnitAsset —
		// they must use 'redefines'. The mission is a literal of mAF's closed
		// Mission enumeration, as it is a closed type in the framework.
		if !ua.Mission.IsZero() {
			out.WriteString(fmt.Sprintf("        attribute redefines mission = Mission::'%s';\n", ua.Mission))
		}
		if places := ua.Details["FunctionalLocation"]; len(places) > 0 {
			out.WriteString(fmt.Sprintf("        attribute redefines functionalLocation = %s;\n", sysmlStrings(places)))
		}

		for _, svc := range ua.GetServices() {
			subscribable := svc.SubscribeAble
			out.WriteString(sysmlPort("out", svc.Definition, "provided service", svc.Details,
				portFacts{regPeriod: svc.RegPeriod, subscribable: &subscribable}))
		}
		for _, cerv := range ua.GetCervices() {
			out.WriteString(sysmlPort("in", cerv.Definition, "consumed service", cerv.Details,
				portFacts{mode: cerv.Mode}))
		}

		// Link the asset to its behavior when cervices carry Mode tags.
		// The "perform action <local-name> : <type>" form is the SysML v2
		// canonical way to declare that a part performs an action def.
		if assetHasBehavior(ua) {
			out.WriteString(fmt.Sprintf("        perform action behave : '%s';\n", behaviorTypeName(sys, assetName)))
		}

		out.WriteString("    }\n\n")
	}

	return out.String()
}

// sortedProtocols lists the protocols a husk actually binds, in a stable order.
func sortedProtocols(protoPort map[string]int) []string {
	var protos []string
	for proto, port := range protoPort {
		if port != 0 {
			protos = append(protos, proto)
		}
	}
	sort.Strings(protos)
	return protos
}

// sysmlPosture states the system's security posture and what it reaches outside
// the cloud, from the same usecases.Posture the knowledge graph is written
// from — so the model and the graph cannot disagree about either.
func sysmlPosture(sys *components.System) string {
	p := Posture(sys)
	var b strings.Builder
	b.WriteString("        attribute :>> posture {\n")
	fmt.Fprintf(&b, "            attribute :>> securityLevel = %q;\n", p.Level)
	for _, f := range []struct {
		name  string
		value bool
	}{
		{"namesCertificateAuthority", p.NamesCA},
		{"namesAuthorizer", p.NamesAuthorizer},
		{"isIdentified", p.Identified},
		{"canVerifyPeers", p.CanVerifyPeers},
		{"verifiesTokens", p.VerifiesTokens},
		{"offersTLS", p.OffersTLS},
		{"acceptsPlaintext", p.AcceptsPlaintext},
	} {
		fmt.Fprintf(&b, "            attribute :>> %s = %t;\n", f.name, f.value)
	}
	b.WriteString("        }\n")
	used := make(map[string]int)
	for _, a := range p.External {
		name := a.Name
		if used[name]++; used[name] > 1 {
			name = fmt.Sprintf("%s %d", a.Name, used[name])
		}
		fmt.Fprintf(&b, "        part '%s' :> externalAttachments {\n", strings.ReplaceAll(name, "'", ""))
		fmt.Fprintf(&b, "            attribute :>> name = %q;\n", a.Name)
		if a.Protocol != "" {
			fmt.Fprintf(&b, "            attribute :>> protocol = %q;\n", a.Protocol)
		}
		fmt.Fprintf(&b, "            attribute :>> isEncrypted = %t;\n", a.Encrypted)
		fmt.Fprintf(&b, "            attribute :>> isAuthenticated = %t;\n", a.Authenticated)
		b.WriteString("        }\n")
	}
	return b.String()
}

// portFacts are what a port says beyond its details: a provided service's
// registration and subscription, a consumed service's mode.
type portFacts struct {
	mode         string
	regPeriod    int
	subscribable *bool
}

// sysmlPort emits one port usage. A service that declares a unit gets a body
// saying what its value is: typed by the SysML v2 quantity, on the unit's
// measurement reference, with the QUDT identifiers the knowledge graph uses for
// the same service. A consumer's port carries the unit it asks for, so a
// connection between a °F provider and a °C consumer shows two units — which
// is where the framework converts.
func sysmlPort(direction, definition, role string, details map[string][]string, facts portFacts) string {
	q, unitIRI, kindIRI, typed := sysmlQuantityFor(details)
	head := fmt.Sprintf("        %s port '%s' : '%s'", direction, definition, portDefName(definition))

	var body []string
	add := func(format string, args ...any) { body = append(body, fmt.Sprintf(format, args...)) }
	if mode, ok := sysmlMode(facts.mode); ok {
		add("attribute :>> mode = Mode::'%s';", mode)
	}
	if methods := details["Methods"]; len(methods) > 0 {
		iris := make([]string, len(methods))
		for i, m := range methods {
			iris[i] = bareIRI(m)
		}
		add("attribute :>> methods = %s;", sysmlStrings(iris))
	}
	if forms := details["Forms"]; len(forms) > 0 {
		add("attribute :>> forms = %s;", sysmlStrings(forms))
	}
	if def := firstDetail(details, "DefaultForm"); def != "" {
		add("attribute :>> defaultForm = %q;", def)
	}
	if facts.regPeriod > 0 {
		add("attribute :>> registrationPeriod = %d [SI::s];", facts.regPeriod)
	}
	if facts.subscribable != nil {
		add("attribute :>> subscribable = %t;", *facts.subscribable)
	}
	if unitIRI == "" && kindIRI == "" && len(body) == 0 {
		return fmt.Sprintf("%s;  // %s\n", head, role)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s {  // %s\n", head, role)
	if typed {
		fmt.Fprintf(&b, "            attribute :>> value : %s { :>> mRef = %s; }\n", q.ValueType, q.Ref)
	} else if unitIRI != "" {
		fmt.Fprintf(&b, "            // no SysML v2 unit is mapped for %s; see sysml_units.go\n", local(unitIRI))
	}
	if unitIRI != "" {
		fmt.Fprintf(&b, "            attribute :>> qudtUnit = \"%s\";\n", unitIRI)
	}
	if kindIRI != "" {
		fmt.Fprintf(&b, "            attribute :>> qudtQuantityKind = \"%s\";\n", kindIRI)
	}
	for _, line := range body {
		fmt.Fprintf(&b, "            %s\n", line)
	}
	b.WriteString("        }\n")
	return b.String()
}

// sysmlMode names a cervice's mode as mAF's Mode literal, if it is one.
func sysmlMode(mode string) (string, bool) {
	switch m := strings.ToLower(strings.TrimSpace(mode)); m {
	case "get", "set", "do":
		return m, true
	}
	return "", false
}

// sysmlStrings writes one string, or several as a SysML v2 sequence.
func sysmlStrings(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = strconv.Quote(v)
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return "(" + strings.Join(quoted, ", ") + ")"
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
	// per-asset behavior defs that reference them.
	var out strings.Builder
	out.WriteString("    // ── Behavior Definitions ────────────────────────────────────────────────\n")
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

// behaviorTypeName builds the SysML v2 name for a unit asset's behavior
// action def, qualified by the system for the same collision-avoidance reason
// as assetTypeName.
func behaviorTypeName(sys *components.System, assetName string) string {
	return sysmlName(sys.Name) + "_" + sysmlName(assetName) + "Behavior"
}

// assetHasBehavior reports whether a unit asset has at least one cervice
// tagged with Mode "get" or "set" — the precondition for generating a
// behavior action def and a matching perform reference.
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
	case "authorizer":
		return "Authorizer"
	case "maitreD":
		return "MaitreD"
	default:
		return "ArrowheadSystem"
	}
}
