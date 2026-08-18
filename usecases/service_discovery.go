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

// Package "usecases" addresses system behaviors and actions in given use cases
// such as configuration, registration, authentication, orchestration, ...

package usecases

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// ServRegForms returns the list of forms that the service registration handles
func ServQuestForms() []string {
	return []string{"ServiceQuest_v1", "ServicePoint_v1", "ServicePointList_v1"}
}

// FillQuestForm described the sought service (e.g., RemoteSignal)
func FillQuestForm(sys *components.System, res components.UnitAsset, sDef, protocol string) forms.ServiceQuest_v1 {
	var f forms.ServiceQuest_v1
	f.NewForm()
	f.RequesterName = sys.Name
	f.ServiceDefinition = sDef
	f.Protocol = protocol
	f.Details = res.GetDetails()
	return f
}

// ExtractQuestForm is used by the Service Registrar and Orchestrator when they receive a service query from a consumer system
func ExtractQuestForm(bodyBytes []byte) (rec forms.ServiceQuest_v1, err error) {
	var jsonData map[string]interface{}
	err = json.Unmarshal(bodyBytes, &jsonData)
	if err != nil {
		err = fmt.Errorf("unmarshalling JSON data: %v", err)
		return
	}
	formVersion, ok := jsonData["version"].(string)
	if !ok {
		err = fmt.Errorf("'version' key not found in JSON data")
		return
	}

	switch formVersion {
	case "ServiceQuest_v1":
		var f forms.ServiceQuest_v1
		err = json.Unmarshal(bodyBytes, &f)
		if err != nil {
			err = fmt.Errorf("unable to extract the discovery form request ")
			return
		}
		rec = f
	default:
		err = fmt.Errorf("unsupported service registration form version")
	}
	return
}

// Search4Service requests from the core systems the address of resources's services that meet the need
func Search4Service(qf forms.ServiceQuest_v1, sys *components.System) (servLocation forms.ServicePoint_v1, err error) {
	// Create a new HTTP request to the Orchestrator system (for now the Service Registrar)
	orURL, err := components.GetRunningCoreSystemURL(sys, "orchestrator")
	if err != nil {
		return
	}
	// prepare the payload to perform a service quest
	orURL = orURL + "/squest"
	jsonQF, err := json.MarshalIndent(qf, "", "  ")
	if err != nil {
		return
	}
	resp, err := sendHTTPReq(http.MethodPost, orURL, jsonQF)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	// Read the response /////////////////////////////////
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	return ExtractDiscoveryForm(body)
}

// Search4Services requests from the core systems the address of resources' services that meet the need
func Search4Services(cer *components.Cervice, sys *components.System) (err error) {
	return Search4ServicesAs(cer, sys, ActionForMode(cer.Mode))
}

// Search4ServicesAs is Search4Services for one named action.
//
// The action the token is minted for has to be the action the request will
// perform: the provider recomputes it from the HTTP method, so a token minted
// for anything else is refused. Deriving it from Cervice.Mode alone was not
// enough — Mode is metadata a consumer may leave unset, and unset reads as
// "read", so a cervice that only ever writes got a read token and every PUT
// through it was refused.
func Search4ServicesAs(cer *components.Cervice, sys *components.System, action string) (err error) {
	// instantiate the service quest form
	questForm := forms.ServiceQuest_v1{
		SysId:             0,
		RequesterName:     sys.Name,
		ServiceDefinition: cer.Definition,
		Action:            action,
		Protocol:          preferredProtocol(cer.Protos),
		Details:           questDetails(cer.Details),
		Version:           "ServiceQuest_v1",
	}
	//pack the service quest form
	qf, err := Pack(&questForm, "application/json")
	if err != nil {
		return err
	}
	// Search for an Orchestrator system within the local cloud
	orURL, err := components.GetRunningCoreSystemURL(sys, "orchestrator")
	if err != nil {
		return err
	}
	if orURL == "" {
		return fmt.Errorf("failed to locate an orchestrator")
	}
	orURL = orURL + "/squest"
	// Prepare the request to the orchestrator
	resp, err := sendHTTPReq(http.MethodPost, orURL, qf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Read the response /////////////////////////////////
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	headerContentType := resp.Header.Get("Content-Type")
	discoveryForm, err := Unpack(bodyBytes, headerContentType)
	if err != nil {
		return err
	}
	// Perform a type assertion to convert the returned Form to ServicePoint_v1
	df, ok := discoveryForm.(*forms.ServicePoint_v1)
	if !ok {
		return fmt.Errorf("unable to unpack discovery request form")
	}
	recordNode(cer, df.ServNode, df.ServLocation, df.Details, action, df.Token, df.SubscribeAble)
	return nil
}

// recordNode files a discovered endpoint under the action it was discovered for.
//
// It merges rather than appends: a cervice used for both a GET and a PUT is
// discovered twice, once per action, and appending would leave two entries for
// the same provider — the caller would then poll it twice per round and, worse,
// might pick the copy carrying the wrong token.
func recordNode(cer *components.Cervice, node, url string, details map[string][]string, action, token string, subscribable bool) {
	cer.Mutex.Lock()
	defer cer.Mutex.Unlock()

	for i, ni := range cer.Nodes[node] {
		if ni.URL != url {
			continue
		}
		if ni.Tokens == nil {
			ni.Tokens = make(map[string]string)
		}
		ni.Tokens[action] = token
		ni.Details = details
		cer.Nodes[node][i] = ni
		return
	}
	cer.Nodes[node] = append(cer.Nodes[node], components.NodeInfo{
		URL:           url,
		Details:       details,
		Tokens:        map[string]string{action: token},
		SubscribeAble: subscribable,
	})
}

func Search4MultipleServices(cer *components.Cervice, sys *components.System) (err error) {
	return Search4MultipleServicesAs(cer, sys, ActionForMode(cer.Mode))
}

// Search4MultipleServicesAs is Search4MultipleServices for one named action.
// See Search4ServicesAs for why the action is not taken from Cervice.Mode.
func Search4MultipleServicesAs(cer *components.Cervice, sys *components.System, action string) (err error) {
	questForm := forms.ServiceQuest_v1{
		SysId:             0,
		RequesterName:     sys.Name,
		ServiceDefinition: cer.Definition,
		Action:            action,
		Protocol:          preferredProtocol(cer.Protos),
		Details:           questDetails(cer.Details),
		Version:           "ServiceQuest_v1",
	}
	// Pack the service quest form
	qf, err := Pack(&questForm, "application/json")
	if err != nil {
		return err
	}
	// Search for an Orchestrator system within the local cloud
	orURL, err := components.GetRunningCoreSystemURL(sys, "orchestrator")
	if err != nil {
		return err
	}
	if orURL == "" {
		return fmt.Errorf("failed to locate an orchestrator")
	}
	orURL = orURL + "/squests"
	// Prepare the request to the orchestrator
	resp, err := sendHTTPReq(http.MethodPost, orURL, qf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Read the response /////////////////////////////////
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	headerContentType := resp.Header.Get("Content-Type")
	discoveryForm, err := Unpack(bodyBytes, headerContentType)
	if err != nil {
		return err
	}
	points, err := servicePoints(discoveryForm)
	if err != nil {
		return err
	}
	registered := make(map[string]bool, len(points))
	for _, sp := range points {
		if sp.ServLocation == "" {
			// Nothing to call, so nothing to record. Recording it would put a
			// node with an empty URL in the list, which the consuming round then
			// skips on every poll and which pruning would keep alive because it
			// was "returned".
			continue
		}
		recordNode(cer, sp.ServNode, sp.ServLocation, sp.Details, action, sp.Token, sp.SubscribeAble)
		registered[sp.ServLocation] = true
	}
	// A discovery for several providers returns everything currently registered
	// under this definition, so anything cached and not returned is gone. Adding
	// without ever removing left a sensor in the list after the registrar had
	// stopped listing it — retried every round, paying its own timeout, for as
	// long as the consumer ran.
	//
	// Only here. Search4ServicesAs asks the orchestrator for one provider, so
	// what it does not return says nothing about the others.
	pruneNodes(cer, registered, action)
	return nil
}

// servicePoints reads the orchestrator's answer to a multi-provider quest.
//
// Two forms are accepted because the two ends are upgraded separately. A
// ServicePointList_v1 carries a token per provider; a ServiceRecordList_v1 is
// what an orchestrator that predates this answers with, and has nowhere to put
// one — so a consumer talking to an older orchestrator still discovers its
// providers, and is refused by them in an authorized cloud exactly as it was
// before. Refusing to parse it instead would take the cloud down on an upgrade
// rather than at the moment the tokens start being needed.
func servicePoints(f forms.Form) ([]forms.ServicePoint_v1, error) {
	switch list := f.(type) {
	case *forms.ServicePointList_v1:
		return list.List, nil
	case *forms.ServiceRecordList_v1:
		olderOrchestrator.Do(func() {
			log.Printf("the orchestrator answers multi-provider discovery with %s, "+
				"which carries no access token: requests to those providers will be "+
				"refused in an authorized cloud until it is upgraded\n",
				list.FormVersion())
		})
		points := make([]forms.ServicePoint_v1, 0, len(list.List))
		for _, rec := range list.List {
			points = append(points, ConvertToServicePoint(rec))
		}
		return points, nil
	default:
		return nil, fmt.Errorf("unable to unpack discovery request form: got %T", f)
	}
}

// olderOrchestrator reports the tokenless answer once rather than on every
// discovery, which for a polling consumer is every few seconds.
var olderOrchestrator sync.Once

// pruneNodes drops what a discovery did not return, for the action it was made
// under.
//
// The quest carries an action, so the orchestrator's answer is what this
// consumer may do *for that action* — and a cervice used for both a GET and a
// PUT is discovered twice, once per action. Deleting every provider absent from
// one action's answer therefore threw away providers that were perfectly good
// for the other: two temperature providers, one read-write and one read-only, a
// later write discovery returning only the first, and the read-only one gone.
// Every subsequent read then reached one sensor instead of two, with nothing to
// say so.
//
// So a provider loses only this action's token here. It is removed outright
// when it has none left for any action, because then nothing discovered it at
// all and it is no longer a provider of anything.
func pruneNodes(cer *components.Cervice, registered map[string]bool, action string) {
	// Deleting during a range over the same map is what makes this the crash
	// rather than the race: `fatal error: concurrent map iteration and map
	// write` is not recoverable, and two polling goroutines of one unit asset
	// share this cervice.
	cer.Mutex.Lock()
	defer cer.Mutex.Unlock()

	for node, nodes := range cer.Nodes {
		kept := nodes[:0]
		for _, ni := range nodes {
			if !registered[ni.URL] {
				delete(ni.Tokens, action)
				if len(ni.Tokens) == 0 {
					continue // discovered for nothing: no longer a provider here
				}
			}
			kept = append(kept, ni)
		}
		if len(kept) == 0 {
			delete(cer.Nodes, node)
			continue
		}
		cer.Nodes[node] = kept
	}
}

// ConvertToServicePoint turns a registration record into the service point handed
// to a consumer. The endpoint URL is built with preferredProtoPort, so a provider
// that has bound HTTPS is reached over HTTPS and the consumer's request carries
// its client certificate. Building the URL by hand instead loses the caller's
// identity at the provider, however well enrolled both ends are.
func ConvertToServicePoint(sr forms.ServiceRecord_v1) (sp forms.ServicePoint_v1) {
	rec := sr
	sp.NewForm()
	sp.ProviderName = rec.SystemName
	sp.ServiceDefinition = rec.ServiceDefinition
	sp.Details = rec.Details
	proto, port := preferredProtoPort(rec.ProtoPort)
	// A record with no address leaves nothing to build a URL from, and indexing
	// it panicked the poll goroutine of every system consuming that definition.
	// The address comes off the wire from the registrar, which copies whatever
	// the provider registered, and a host whose interface enumeration found
	// nothing registers an empty list. One such provider should cost its own
	// discovery, not everyone else's.
	if len(rec.IPAddresses) == 0 {
		log.Printf("the registrar lists %s of %s with no address, so there is nothing to call\n",
			rec.ServiceDefinition, rec.SystemName)
		return sp
	}
	sp.ServLocation = proto + "://" + rec.IPAddresses[0] + ":" + strconv.Itoa(port) + "/" + rec.SystemName + "/" + rec.SubPath
	sp.ServNode = rec.ServiceNode
	sp.SubscribeAble = rec.SubscribeAble
	return
}

// ActionForMode translates a cervice's mode into the action the authorizer
// reasons about. An unspecified mode is taken as a read: it is the least a
// consumer could mean, and asking for more than is intended would widen what a
// policy has to permit.
func ActionForMode(mode string) string {
	switch mode {
	case "set":
		return "write"
	case "do":
		return "invoke"
	default:
		return "read"
	}
}

// questDetails narrows a cervice's details to what the registrar should match on.
//
// A consumer that names a QuantityKind is asking for a temperature, not for
// Celsius, so the unit is dropped from the quest: the registrar compares strings,
// and a consumer wanting degrees Celsius would never be paired with a sensor
// reporting Fahrenheit if the unit were part of the query. The unit is still what
// the reading is converted into once a provider is chosen — it is a conversion
// target, not a search key.
//
// Measure is dropped for the same reason: whether the consumer treats values as
// points or intervals says nothing about which provider suits it.
func questDetails(details map[string][]string) map[string][]string {
	matched := make(map[string][]string, len(details))
	relaxUnit := len(details["QuantityKind"]) > 0
	for key, values := range details {
		if key == "Measure" || (key == "Unit" && relaxUnit) {
			continue
		}
		matched[key] = values
	}
	return matched
}

// preferredProtocol returns "https" if the cervice supports it, otherwise "http".
func preferredProtocol(protos []string) string {
	for _, p := range protos {
		if p == "https" {
			return "https"
		}
	}
	return "http"
}

// preferredProtoPort picks the best protocol and its port from a ProtoPort map,
// preferring "https" over "http" when both are available and non-zero.
func preferredProtoPort(protoPort map[string]int) (proto string, port int) {
	if p, ok := protoPort["https"]; ok && p != 0 {
		return "https", p
	}
	return "http", protoPort["http"]
}

// FillDiscoveredServices returns a json data byte array with a slice of matching services (e.g., Service Registrar)
func FillDiscoveredServices(dsList []forms.ServiceRecord_v1, version string) (f forms.Form, err error) {
	switch version {
	case "ServiceRecordList_v1":
		dslForm := &forms.ServiceRecordList_v1{} // pointer to struct
		f = dslForm.NewForm()
		for _, rec := range dsList {
			sf := rec.NewForm().(*forms.ServiceRecord_v1) // create new form and cast it to *ServiceRecord_v1
			dslForm.List = append(dslForm.List, *sf)
		}
	default:
		err = fmt.Errorf("unsupported service registration form version")
		return
	}
	return
}

// ExtractDiscoveryForm is used by the Orchestrator and the authorized consumer system
func ExtractDiscoveryForm(bodyBytes []byte) (sLoc forms.ServicePoint_v1, err error) {
	var jsonData map[string]interface{}
	err = json.Unmarshal(bodyBytes, &jsonData)
	if err != nil {
		err = fmt.Errorf("unmarshalling JSON data: %v", err)
		return
	}
	formVersion, ok := jsonData["version"].(string)
	if !ok {
		err = fmt.Errorf("'version' key not found in JSON data")
		return
	}
	switch formVersion {
	case "ServicePoint_v1":
		var f forms.ServicePoint_v1
		f.NewForm()
		err = json.Unmarshal(bodyBytes, &f)
		if err != nil {
			err = fmt.Errorf("unmarshalling JSON data: %v", err)
			return
		}
		sLoc = f
	default:
		err = fmt.Errorf("unsupported service discovery form version")
	}
	return
}
