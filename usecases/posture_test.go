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
 *   Thomas Hedeler, Hamburg - initial implementation
 ***************************************************************************SDG*/

package usecases

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"sync"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

// postureSystem builds a system with the named core systems configured and the
// given enrolment state, without touching the network.
func postureSystem(cores []string, cert, caCert string, key *ecdsa.PublicKey, ports map[string]int) *components.System {
	sys := &components.System{
		Name:  "test",
		Mutex: &sync.Mutex{},
		Husk: &components.Husk{
			ProtoPort:   ports,
			Certificate: cert,
			CA_cert:     caCert,
			Host:        &components.HostingDevice{Name: "testhost"},
		},
	}
	if key != nil {
		sys.Husk.AuthorizerKey.Store(key)
	}
	for _, name := range cores {
		// The registrar takes a different path through GetRunningCoreSystemURL
		// (it is status-checked over the network), so the postures under test
		// name only the CA and the authorizer.
		sys.Husk.CoreS = append(sys.Husk.CoreS, &components.CoreSystem{
			Name: name,
			Url:  "http://localhost:20100/" + name,
		})
	}
	return sys
}

func TestPostureReportsWhatIsActuallyInForce(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	both := map[string]int{"http": 20100, "https": 30100}
	tlsOnly := map[string]int{"http": 0, "https": 30100}

	cases := []struct {
		name  string
		sys   *components.System
		want  string
		notes []string
	}{
		{
			// Scenario 1: nothing deployed. The cloud runs and says so.
			name: "no CA configured",
			sys:  postureSystem(nil, "", "", nil, both),
			want: PostureOpen,
		},
		{
			// A CA is named and this system has not enrolled: the state every
			// system passes through, and the one it stays in when the CA is
			// unreachable.
			name: "CA configured, no certificate yet",
			sys:  postureSystem([]string{"ca"}, "", "", nil, both),
			want: PostureEnrolling,
		},
		{
			// Scenario 2: enrolled, no authorizer. Callers are named, not
			// restricted.
			name: "enrolled, no authorizer",
			sys:  postureSystem([]string{"ca"}, "cert", "cacert", nil, both),
			want: PostureIdentified,
		},
		{
			// The 503 state: it means to authorise and cannot. This must not
			// read as "authorized".
			name:  "authorizer named, key not held",
			sys:   postureSystem([]string{"ca", "authorizer"}, "cert", "cacert", nil, both),
			want:  PostureIdentified,
			notes: []string{"key is not held"},
		},
		{
			name: "authorized",
			sys:  postureSystem([]string{"ca", "authorizer"}, "cert", "cacert", &key.PublicKey, tlsOnly),
			want: PostureAuthorized,
		},
	}

	for _, tc := range cases {
		p := Posture(tc.sys)
		if p.Level != tc.want {
			t.Errorf("%s: level %q, want %q", tc.name, p.Level, tc.want)
		}
		for _, note := range tc.notes {
			if !strings.Contains(p.String(), note) {
				t.Errorf("%s: %q does not mention %q", tc.name, p.String(), note)
			}
		}
	}
}

// TestPostureSaysWhenPlaintextIsStillOpen: a system at "authorized" that still
// listens on its HTTP port can be reached without any of the protection the
// level names. Reporting the level alone would overstate it.
func TestPostureSaysWhenPlaintextIsStillOpen(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}

	open := postureSystem([]string{"ca", "authorizer"}, "cert", "cacert", &key.PublicKey,
		map[string]int{"http": 20100, "https": 30100})
	shut := postureSystem([]string{"ca", "authorizer"}, "cert", "cacert", &key.PublicKey,
		map[string]int{"http": 0, "https": 30100})

	if p := Posture(open); !p.AcceptsPlaintext || !strings.Contains(p.String(), "without TLS") {
		t.Errorf("an open HTTP port is not reported: %q", p)
	}
	if p := Posture(shut); p.AcceptsPlaintext || strings.Contains(p.String(), "without TLS") {
		t.Errorf("plaintext reported for an HTTPS-only system: %q", p)
	}
}

// TestSecurityPostureIsInTheKnowledgeGraph: the posture belongs in the graph
// because the question is about a cloud, not one system. A level that only ever
// reached the log could not be queried across systems.
func TestSecurityPostureIsInTheKnowledgeGraph(t *testing.T) {
	sys := postureSystem([]string{"ca"}, "cert", "cacert", nil, map[string]int{"http": 20100, "https": 30100})
	sys.UAssets = make(map[string]*components.UnitAsset)

	graph := modelSecurity(sys)

	for _, want := range []string{
		"a afo:SecurityPosture",
		`afo:hasSecurityLevel "identified"`,
		`afo:namesCertificateAuthority "true"^^xsd:boolean`,
		`afo:namesAuthorizer "false"^^xsd:boolean`,
		`afo:acceptsPlaintext "true"^^xsd:boolean`,
	} {
		if !strings.Contains(graph, want) {
			t.Errorf("the graph does not carry %s:\n%s", want, graph)
		}
	}

	// The system block has to point at it, or nothing can find it.
	if !strings.Contains(modelSystem(sys), "afo:hasSecurityPosture") {
		t.Error("the system does not link to its security posture")
	}
}
