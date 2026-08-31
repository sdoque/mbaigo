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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// stubRegistry answers /query with the core systems of a two-host cloud.
func stubRegistry(t *testing.T) *httptest.Server {
	t.Helper()
	rec := func(system, sub, def string, host string, httpPort, httpsPort int) forms.ServiceRecord_v1 {
		return forms.ServiceRecord_v1{
			SystemName: system, SubPath: sub, ServiceDefinition: def,
			IPAddresses: []string{host},
			ProtoPort:   map[string]int{"http": httpPort, "https": httpsPort},
		}
	}
	byDef := map[string][]forms.ServiceRecord_v1{
		"registry":  {rec("serviceregistrar", "registry/registry", "registry", "10.0.0.33", 20102, 30102), rec("serviceregistrar", "registry/registry", "registry", "10.0.0.34", 20102, 30102)},
		"squest":    {rec("orchestrator", "orchestration/squest", "squest", "10.0.0.33", 20103, 30103)},
		"certify":   {rec("ca", "certification/certify", "certify", "10.0.0.33", 20100, 30100)},
		"authorize": {rec("authorizer", "authorization/authorize", "authorize", "10.0.0.33", 20104, 30104)},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q forms.ServiceQuest_v1
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		list := forms.ServiceRecordList_v1{}
		list.NewForm()
		list.List = byDef[q.ServiceDefinition]
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	}))
}

func learningSystem(t *testing.T, authorizerURL string) *components.System {
	t.Helper()
	sys := components.NewSystem("learner", context.Background())
	sys.Husk = &components.Husk{CoreS: []*components.CoreSystem{
		{Name: components.ServiceRegistrarName, Url: "http://10.0.0.33:20102/serviceregistrar/registry"},
		{Name: "ca", Url: "http://10.0.0.33:20100/ca/certification"},
		{Name: AuthorizerName, Url: authorizerURL},
	}}
	return &sys
}

func urlsOf(list []*components.CoreSystem, name string) []string {
	var out []string
	for _, cs := range list {
		if cs.Name == name {
			out = append(out, cs.Url)
		}
	}
	return out
}

// A file naming one registrar and the CA is enough: the second host's
// registrar and the orchestrator are learned, behind what the file says, with
// the scheme the template would have chosen.
func TestLearnCoreSystems(t *testing.T) {
	t.Chdir(t.TempDir())
	reg := stubRegistry(t)
	defer reg.Close()
	sys := learningSystem(t, "")

	added, err := LearnCoreSystems(sys, reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	all := sys.CoreSystems()

	regs := urlsOf(all, components.ServiceRegistrarName)
	if len(regs) != 2 || regs[0] != "http://10.0.0.33:20102/serviceregistrar/registry" || regs[1] != "http://10.0.0.34:20102/serviceregistrar/registry" {
		t.Errorf("registrars = %v; want the file's first and the other host's second, both over http", regs)
	}
	if orch := urlsOf(all, "orchestrator"); len(orch) != 1 || orch[0] != "https://10.0.0.33:30103/orchestrator/orchestration" {
		t.Errorf("orchestrator = %v; want learned over https", orch)
	}
	if cas := urlsOf(all, "ca"); len(cas) != 1 {
		t.Errorf("ca = %v; the file's CA is the same one the registry reports, so nothing new", cas)
	}
	// The file names no authorizer, so none is adopted from the cloud.
	for _, cs := range sys.LearnedCoreSystems() {
		if cs.Name == AuthorizerName {
			t.Errorf("an authorizer was learned though the file names none: %s", cs.Url)
		}
	}
	if added != 2 {
		t.Errorf("added = %d, want 2 (a registrar and an orchestrator)", added)
	}

	// The cache holds only what was learned, and a fresh system reads it back.
	if _, err := os.Stat(CoreCacheFileName); err != nil {
		t.Fatalf("no cache written: %v", err)
	}
	again := learningSystem(t, "")
	LoadCoreCache(again)
	if got := urlsOf(again.LearnedCoreSystems(), components.ServiceRegistrarName); len(got) != 1 || got[0] != "http://10.0.0.34:20102/serviceregistrar/registry" {
		t.Errorf("restored registrars = %v", got)
	}

	// Learning again adds nothing: the list does not grow with every refresh.
	if added, _ := LearnCoreSystems(sys, reg.URL); added != 0 {
		t.Errorf("a second lesson added %d", added)
	}
}

// An authorizer is learned only as an alternate to one the file names —
// naming one is what turns enforcement on, and that is the operator's call.
func TestAuthorizerIsLearnedOnlyAsAnAlternate(t *testing.T) {
	t.Chdir(t.TempDir())
	reg := stubRegistry(t)
	defer reg.Close()
	sys := learningSystem(t, "https://10.0.0.34:30104/authorizer/authorization")

	if _, err := LearnCoreSystems(sys, reg.URL); err != nil {
		t.Fatal(err)
	}
	auth := urlsOf(sys.CoreSystems(), AuthorizerName)
	if len(auth) != 2 || auth[0] != "https://10.0.0.34:30104/authorizer/authorization" || auth[1] != "https://10.0.0.33:30104/authorizer/authorization" {
		t.Errorf("authorizers = %v; want the file's first and the learned one second", auth)
	}
}

// A core URL is the service URL without the service: the client appends what
// it wants. A record with no asset segment cannot be turned into one.
func TestCoreURLOf(t *testing.T) {
	rec := forms.ServiceRecord_v1{SystemName: "ca", SubPath: "certification/certify",
		IPAddresses: []string{"h"}, ProtoPort: map[string]int{"http": 20100, "https": 30100}}
	if got := coreURLOf("ca", rec); got != "http://h:20100/ca/certification" {
		t.Errorf("ca = %q; the CA is reached before there is a certificate, so http", got)
	}
	if got := coreURLOf("orchestrator", rec); got != "https://h:30100/ca/certification" {
		t.Errorf("orchestrator = %q; want https where it is bound", got)
	}
	// An authorizer not yet serving TLS is not learned yet: it registered
	// before it enrolled, and learned then it would be reached in the clear.
	rec.ProtoPort = map[string]int{"http": 20104}
	if got := coreURLOf(AuthorizerName, rec); got != "" {
		t.Errorf("an unenrolled authorizer was learned as %q", got)
	}
	rec.ProtoPort = map[string]int{"http": 20100, "https": 30100}
	rec.SubPath = "certify"
	if got := coreURLOf("ca", rec); got != "" {
		t.Errorf("a record with no asset segment produced %q", got)
	}
}
