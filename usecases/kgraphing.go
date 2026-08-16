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

// The "forms" package is designed to define structured schemas, known as "structs,"
// which represent the format and organization of documents intended for data exchange.
// These structs are utilized to create forms that are populated with data, acting as
// standardized payloads for transmission between different systems. This ensures that
// the data exchanged maintains a consistent structure, facilitating seamless
// integration and processing across system boundaries.
// Basic forms include the service registration and the service query forms.
// The form version is used for backward compatibility.

// the ontology forms are used to generate a semantic model of the system, device it is running on, its unit assets and services they offer

package usecases

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sdoque/mbaigo/components"
)

// function KGraphing provides a semantic model of a system running on a host and exposing the functionality of asset
func KGraphing(w http.ResponseWriter, req *http.Request, sys *components.System) {

	rdf := prefixes()
	rdf += modelSystem(sys)
	rdf += modelHusk(sys)
	rdf += modelEndpoints(sys)
	rdf += modelHost(sys)
	rdf += modelSecurity(sys)
	rdf += modelUAsset(sys)

	w.Header().Set("Content-Type", "text/turtle")
	_, err := w.Write([]byte(rdf))
	if err != nil {
		log.Println("Failed to write KGraphing information: ", err)
	}
}

func prefixes() (description string) {
	description = "@prefix alc: <http://www.synecdoque.com/lcloud/> .\n"
	description += "@prefix afo: <http://www.synecdoque.com/2025/afo#> .\n"
	description += "@prefix owl: <http://www.w3.org/2002/07/owl#> .\n"
	description += "@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .\n"
	description += "@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .\n"
	description += "@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .\n\n"
	return
}

// rdfObject renders a detail value as a valid Turtle object: a full IRI if the
// caller supplied one (`<...>`), an `alc:<local>` prefixed name if the value
// is a legal PN_LOCAL, or a double-quoted string literal otherwise.
//
// This prevents values like "SysML v2" (space), "mm/h" (slash), or "W/m²"
// (non-ASCII superscript) from being emitted as "alc:SysML v2" etc., which
// strict Turtle parsers reject. Such values become string literals because
// they are semantically values, not identifiers.
func rdfObject(value string) string {
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		return value
	}
	if isValidPNLocal(value) {
		return "alc:" + value
	}
	// Escape backslashes and double quotes for a Turtle string literal.
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// afoDefined names the predicates the Arrowhead Framework Ontology defines.
//
// It exists so that afo: means what it says. Everything this file writes with
// that prefix is a term someone declared in the ontology, with a domain, a range
// and a comment; everything else goes to alc:, the local cloud's own namespace,
// which is ours to mint in.
//
// The distinction is not pedantry. Writing afo:hasCost invents vocabulary in a
// namespace this project does not own: no reasoner can interpret it, the
// ontology's SHACL shapes cannot validate it, and a consumer that dereferences
// the prefix to find out what it means finds nothing. Worse, the husk detail
// loop below turns any configuration key into a predicate, so an operator could
// mint afo: terms by editing a JSON file.
//
// This list is also the agenda. A term under alc: that ought to be shared
// vocabulary is one to propose for the next ontology release, and moving it is
// one line here once it lands. hasFunctionalLocation went the other way: it was
// always written as afo: because the AFO-IDO, DEXPI and STEP alignment
// ontologies bridge it and the authorizer matches policy on it, so the ontology
// is where it belongs and version 1.2.0 is where it is being proposed.
//
// Kept in sync with ontology version 1.2.0.
var afoDefined = map[string]bool{
	// Object properties
	"communicatesOver": true, "consumes": true, "consumesService": true,
	"consumingFromSystem": true, "contains": true, "endpointOfHusk": true,
	"hasDeveloper": true, "hasHusk": true, "hasSecurityPosture": true,
	"hasServer": true, "hasUnitAsset": true, "hostedOnEndpoint": true,
	"hostingEndpoint": true, "isContainedIn": true, "isHostOf": true,
	"isHuskOf": true, "isServerOf": true, "isUnitAssetOf": true,
	"onHost": true, "providesService": true, "providingToSystem": true,
	"runsOnHost": true, "serviceConsumedBy": true, "serviceProvidedBy": true,

	// Datatype properties
	"acceptsPlaintext": true, "canVerifyPeers": true, "hasIPAddress": true,
	"hasMission": true, "hasName": true, "hasRegistrationPeriod": true,
	"hasSecurityLevel": true, "hasServiceDefinition": true, "hasUrl": true,
	"isIdentified": true, "isSubscribable": true, "namesAuthorizer": true,
	"hasFunctionalLocation": true, "namesCertificateAuthority": true,
	"offersTLS": true, "usesPort": true,
	"usesProtocol": true, "verifiesTokens": true,
}

// predicate returns the prefixed name to write for a property, in the ontology's
// namespace when the ontology defines it and in the local cloud's otherwise.
func predicate(local string) string {
	if afoDefined[local] {
		return "afo:" + local
	}
	return "alc:" + local
}

// detailPredicate turns a configuration detail key into a predicate, and reports
// whether it can be written at all.
//
// The key reaches the predicate position of a triple, where a value would have
// been escaped or quoted by rdfObject. A key of "Functional Location" produced
//
//	afo:hasFunctional Location alc:Kitchen .
//
// which is not Turtle at all — so one detail key with a space in an operator's
// systemconfig.json made the whole cloud's graph unparseable, and the triple
// store rejected the lot rather than the line.
func detailPredicate(key string) (string, bool) {
	local := "has" + key
	if !isValidPNLocal(local) {
		return "", false
	}
	return predicate(local), true
}

// isValidPNLocal applies a conservative subset of Turtle's PN_LOCAL rule:
// non-empty, does not start with a digit, and contains only ASCII letters,
// digits, underscore, or hyphen. This excludes spaces, slashes, dots in
// value position, and any non-ASCII character — all of which would need
// explicit escaping to be legal after the "alc:" prefix.
func isValidPNLocal(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r == '_', r == '-':
			// always OK
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// finalizeBlock removes any trailing semicolon from a block of predicate/object
// lines and appends the final " .\n\n" so the Turtle is syntactically correct.
func finalizeBlock(block string) string {
	// Remove trailing whitespace first
	block = strings.TrimRight(block, " \t\r\n")

	// If it ends with ';', remove it and any trailing spaces before it
	if strings.HasSuffix(block, ";") {
		block = strings.TrimSuffix(block, ";")
		block = strings.TrimRight(block, " \t")
	}

	return block + " .\n\n"
}

// endpointLocalName builds a local name for an Endpoint instance based on
// host, system, protocol, and port, so we can refer to the same endpoint
// from Husk and Service descriptions.
func endpointLocalName(sys *components.System, protocol string, port int) string {
	return fmt.Sprintf("%s_%s_%s_%d_Endpoint",
		sys.Husk.Host.Name,
		sys.Name,
		protocol,
		port,
	)
}

// modelSystem creates a knowledge graph of the system that aggregates the husk and unit assets
func modelSystem(sys *components.System) (systemModel string) {
	sName := sys.Husk.Host.Name + "_" + sys.Name
	systemModel = fmt.Sprintf("alc:%s a afo:System ;\n", sName)
	systemModel += fmt.Sprintf("    afo:hasName \"%s\" ;\n", sys.Name)

	// The Husk instance is in the alc: namespace, not afo:
	systemModel += fmt.Sprintf("    afo:hasHusk alc:%s_Husk ;\n", sName)

	// --- NEW: LocalCloud is stored in the Husk details for ServiceRegistrar systems ---
	// It is expected that only the ServiceRegistrar systems have this key and all
	// have the same name (if not, the KGrapher will use the first one it finds).
	if values, ok := sys.Husk.Details["LocalCloud"]; ok && len(values) > 0 {
		v := values[0]
		if !(strings.HasPrefix(v, "<") && strings.HasSuffix(v, ">")) && !strings.HasPrefix(v, "alc:") {
			v = "alc:" + v
		}
		systemModel += fmt.Sprintf("    afo:isContainedIn %s ;\n", v)
	}
	// --- END NEW ---

	for assetName := range sys.UAssets {
		systemModel += fmt.Sprintf("    afo:hasUnitAsset alc:%s_%s ;\n", sName, assetName)
	}

	systemModel += fmt.Sprintf("    afo:hasSecurityPosture alc:%s_Security ;\n", sName)

	systemModel = finalizeBlock(systemModel)
	return
}

// modelSecurity states how this system is actually protected.
//
// It belongs in the graph rather than only in a log because the question an
// operator asks is about the cloud, not about one system: which systems are
// enrolled, which are still reachable in the clear, which name an authorizer
// they cannot reach. Each system reports only what it observes about itself, and
// the graph is where those observations add up.
//
// Every property is a fact, not a setting. afo:namesAuthorizer says an
// authorizer is configured; afo:verifiesTokens says its key was obtained. A
// system where those disagree intends to authorize and currently cannot — it is
// refusing every request with 503 — and that is precisely the state worth being
// able to query for.
func modelSecurity(sys *components.System) string {
	sName := sys.Husk.Host.Name + "_" + sys.Name
	p := Posture(sys)

	m := fmt.Sprintf("alc:%s_Security a afo:SecurityPosture ;\n", sName)
	m += fmt.Sprintf("    afo:hasSecurityLevel \"%s\" ;\n", p.Level)
	for _, f := range []struct {
		predicate string
		value     bool
	}{
		{"namesCertificateAuthority", p.NamesCA},
		{"namesAuthorizer", p.NamesAuthorizer},
		{"isIdentified", p.Identified},
		{"canVerifyPeers", p.CanVerifyPeers},
		{"verifiesTokens", p.VerifiesTokens},
		{"offersTLS", p.OffersTLS},
		{"acceptsPlaintext", p.AcceptsPlaintext},
	} {
		m += fmt.Sprintf("    %s \"%t\"^^xsd:boolean ;\n", predicate(f.predicate), f.value)
	}

	m = finalizeBlock(m)
	return m
}

// modelHusk creates a knowledge graph of the husk that wraps the unit assets
func modelHusk(sys *components.System) string {
	sName := sys.Husk.Host.Name + "_" + sys.Name
	huskModel := fmt.Sprintf("alc:%s_Husk a afo:Husk ;\n", sName)

	// Host IRI is just alc:<HostName>, not alc:<HostName>_Host
	huskModel += fmt.Sprintf("    afo:runsOnHost alc:%s ;\n", sys.Husk.Host.Name)

	// For each protocol/port pair, link the Husk to an Endpoint instance
	for protocol, port := range sys.Husk.ProtoPort {
		if port == 0 {
			continue
		}
		eName := endpointLocalName(sys, protocol, port)
		huskModel += fmt.Sprintf("    afo:communicatesOver alc:%s ;\n", eName)
	}

	details := sys.Husk.Details
	for key, values := range details {
		// LocalCloud is now handled on the System level
		if key == "LocalCloud" {
			continue
		}
		pred, ok := detailPredicate(key)
		if !ok {
			log.Printf("kgraph: the detail %q cannot be written as a predicate and is left out of the graph; use a name of letters, digits, underscores or hyphens\n", key)
			continue
		}
		for _, value := range values {
			huskModel += fmt.Sprintf("    %s %s ;\n", pred, rdfObject(value))
		}
	}

	huskModel = finalizeBlock(huskModel)
	return huskModel
}

// modelHost creates a knowledge graph of the hosting computer
func modelHost(sys *components.System) string {
	hostModel := fmt.Sprintf("alc:%s a afo:Host ;\n", sys.Husk.Host.Name)
	hostModel += fmt.Sprintf("    afo:hasName \"%s\" ;\n", sys.Husk.Host.Name)

	ipaLen := len(sys.Husk.Host.IPAddresses)
	ipaCount := 0
	for _, ipa := range sys.Husk.Host.IPAddresses {
		hostModel += fmt.Sprintf("    afo:hasIPAddress \"%s\"", ipa)
		ipaCount++
		if ipaCount < ipaLen {
			hostModel += " ;\n"
		}
	}

	hostModel = finalizeBlock(hostModel)
	return hostModel
}

// modelEndpoints creates a knowledge graph of the (host, protocol, port)
// combinations as first-class afo:Endpoint instances.
func modelEndpoints(sys *components.System) string {
	var endpointModels string

	for protocol, port := range sys.Husk.ProtoPort {
		if port == 0 {
			continue
		}

		eName := endpointLocalName(sys, protocol, port)
		var endpointModel string

		endpointModel += fmt.Sprintf("alc:%s a afo:Endpoint ;\n", eName)
		endpointModel += fmt.Sprintf("    afo:usesProtocol \"%s\" ;\n", protocol)
		endpointModel += fmt.Sprintf("    afo:usesPort %d ;\n", port)
		endpointModel += fmt.Sprintf("    afo:onHost alc:%s ;\n", sys.Husk.Host.Name)
		// Optional: base path if you want it (/%system name%)
		// endpointModel += fmt.Sprintf("    afo:hasBasePath \"/%s\" ;\n", sys.Name)

		endpointModel = finalizeBlock(endpointModel)
		endpointModels += endpointModel
	}

	return endpointModels
}

// modelUAsset creates a knowledge graph of each unit assets and its consumed and provided services
func modelUAsset(sys *components.System) string {
	sName := sys.Husk.Host.Name + "_" + sys.Name
	var assetModels string

	for assetName, asset := range sys.UAssets {
		var assetModel string

		assetModel += fmt.Sprintf("alc:%s_%s a afo:UnitAsset ;\n", sName, assetName)
		assetModel += fmt.Sprintf("    afo:hasName \"%s\" ;\n", assetName)
		if !(*asset).Mission.IsZero() {
			assetModel += fmt.Sprintf("    afo:hasMission \"%s\" ;\n", (*asset).Mission)
		}

		details := (*asset).GetDetails()
		for key, values := range details {
			// FunctionalLocation lands in the AFO namespace because the
			// AFO-IDO/DEXPI/STEP alignment ontologies bridge it to the
			// upstream vocabularies; other detail keys stay local until
			// they too have an alignment target. That decision lives in
			// afoDefined now rather than in an exception here, and the key
			// is checked before it reaches the predicate position.
			pred, ok := detailPredicate(key)
			if !ok {
				log.Printf("kgraph: the detail %q cannot be written as a predicate and is left out of the graph; use a name of letters, digits, underscores or hyphens\n", key)
				continue
			}
			for _, value := range values {
				assetModel += fmt.Sprintf("    %s %s ;\n", pred, rdfObject(value))
			}
		}

		cervices := (*asset).GetCervices()
		for _, cervice := range cervices {
			assetModel += fmt.Sprintf("    afo:consumesService alc:%s_%s_%s ;\n", sName, assetName, cervice.Definition)
		}

		services := (*asset).GetServices()
		servicesLen := len(services)
		serviceCount := 0
		for _, service := range services {
			//`` Use service.Definition for the IRI, so it matches the Service block
			assetModel += fmt.Sprintf("    afo:providesService alc:%s_%s_%s", sName, assetName, service.Definition)
			serviceCount++
			if serviceCount < servicesLen {
				assetModel += " ;\n"
			}
		}

		assetModel = finalizeBlock(assetModel)
		assetModels += assetModel

		assetModels += modelCervices(sName, asset)
		assetModels += modelServices(sName, asset, sys)
	}

	return assetModels
}

// modelCervices creates a knowledge graph of the consumed services of a unit asset
func modelCervices(sName string, ua *components.UnitAsset) string {
	var cervicesModel string
	asset := *ua
	cervices := asset.GetCervices()

	for _, cervice := range cervices {
		var cerviceModel string

		cerviceModel += fmt.Sprintf("alc:%s_%s_%s a afo:ConsumedService ;\n",
			sName, asset.GetName(), cervice.Definition)
		cerviceModel += fmt.Sprintf("    afo:consumes \"%s\" ;\n", cervice.Definition)
		if cervice.Mode != "" {
			cerviceModel += fmt.Sprintf("    "+predicate("hasMode")+" \"%s\" ;\n", cervice.Mode)
		}

		details := cervice.Details
		for key, values := range details {
			for _, value := range values {
				cerviceModel += fmt.Sprintf("    alc:has%s %s ;\n", key, rdfObject(value))
			}
		}

		for pName, nodes := range cervice.Nodes {
			cerviceModel += fmt.Sprintf("    afo:consumes alc:%s ;\n", pName)
			for _, ni := range nodes {
				cerviceModel += fmt.Sprintf("    "+predicate("fromUrl")+" <%s> ;\n", ni.URL)
			}
		}

		cerviceModel = finalizeBlock(cerviceModel)

		// FIX: accumulate this block
		cervicesModel += cerviceModel
	}

	return cervicesModel
}

// modelServices creates a knowledge graph of the services provided by a unit asset
func modelServices(sName string, ua *components.UnitAsset, sys *components.System) string {
	var servicesModel string
	asset := *ua
	assetName := asset.GetName()
	services := asset.GetServices()

	for _, service := range services {
		var serviceModel string

		// IRI is based on service.Definition, matching UnitAsset's providesService
		serviceModel += fmt.Sprintf("alc:%s_%s_%s a afo:Service ;\n", sName, assetName, service.Definition)
		serviceModel += fmt.Sprintf("    afo:hasName \"%s/%s\" ;\n", assetName, service.Definition)
		serviceModel += fmt.Sprintf("    afo:hasServiceDefinition \"%s\" ;\n", service.Definition)

		// For each protocol/port, link to the Endpoint and give a URL
		for protocol, port := range sys.Husk.ProtoPort {
			if port == 0 {
				continue
			}

			eName := endpointLocalName(sys, protocol, port)
			serviceModel += fmt.Sprintf("    afo:hostedOnEndpoint alc:%s ;\n", eName)

			addr := protocol + "://" + sys.Husk.Host.IPAddresses[0] + ":" +
				strconv.Itoa(port) + "/" + sys.Name + "/" + assetName + "/" + service.SubPath
			serviceModel += fmt.Sprintf("    afo:hasUrl <%s> ;\n", addr)
		}

		// Additional details
		details := service.Details
		for key, values := range details {
			for _, value := range values {
				serviceModel += fmt.Sprintf("    alc:has%s  %s ;\n", key, rdfObject(value))
			}
		}

		serviceModel += fmt.Sprintf("    afo:isSubscribable \"%t\"^^xsd:boolean ;\n", service.SubscribeAble)
		if service.CFootprint != 0 {
			serviceModel += fmt.Sprintf("    "+predicate("hasCarbonFootprint")+" \"%.6f\"^^xsd:decimal ;\n", service.CFootprint)
		}
		if service.CUnit != "" {
			serviceModel += fmt.Sprintf("    "+predicate("hasCost")+" \"%.2f\"^^xsd:decimal ;\n", service.ACost)
			serviceModel += fmt.Sprintf("    "+predicate("hasCostUnit")+" \"%s\"^^xsd:string ;\n", service.CUnit)
		}
		serviceModel += fmt.Sprintf("    afo:hasRegistrationPeriod %d ;\n", service.RegPeriod)

		// Let finalizeBlock remove the trailing ';' and close the block with " ."
		serviceModel = finalizeBlock(serviceModel)
		servicesModel += serviceModel
	}

	return servicesModel
}
