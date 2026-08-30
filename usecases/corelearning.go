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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// A system learns the cloud's core systems from the registry instead of being
// told each of them by hand.
//
// Every core system registers what it provides with the lead registrar, so the
// registrar already knows where every orchestrator, authorizer, CA and standby
// registrar is. Configuring that list into every system's file on every host
// was the deployer's job, and on a second host it was where the mistakes were
// made. Now a file needs the address of one registrar — by default this host's
// own — and the CA; the rest is asked for once the system has enrolled.
//
// The CA stays in the file on purpose. A system reaches the CA before it holds
// a certificate, so a CA learned from the network would be a trust root learned
// from an unauthenticated source: whoever could answer as a registrar on the LAN
// could point every new system at their own CA. What is learned is learned over
// mTLS, after enrollment, and appended behind whatever the file says — the
// operator's word wins over the cloud's.

// coreDefinitions maps a core system's name to the service definition that
// identifies it in the registry. These are the framework's own definitions,
// stable across deployments, which is what makes the lookup safe to hard-code.
var coreDefinitions = map[string]string{
	components.ServiceRegistrarName: "registry",
	"orchestrator":                  "squest",
	"ca":                            "certify",
	AuthorizerName:                  "authorize",
}

// CoreCacheFileName is where learned core systems are kept between runs.
//
// A cache and not the configuration file. The file is what the operator wrote,
// and a system rewriting it would leave the operator unable to tell their own
// statement from the cloud's. The cache exists for one case: a restart while
// the lead is down, when the file alone would leave this system unable to find
// the standby it learned about last week. The same reason and the same shape
// as the maitreD's whitelist cache.
const CoreCacheFileName = "coresystems.cache.json"

// LearnCoreSystems asks the lead registrar for every core system it knows and
// records the ones this system had not heard of. It returns how many were new.
func LearnCoreSystems(sys *components.System, registrar string) (int, error) {
	// Naming an authorizer is a decision — it turns enforcement on — and not a
	// discovery. A file that names none has not adopted authorization, and
	// learning one from the cloud would adopt it on the operator's behalf. So an
	// authorizer is learned only as an alternate to one the file already names.
	learnAuthorizer := false
	for _, cs := range sys.Husk.CoreS {
		if cs.Name == AuthorizerName && strings.TrimSpace(cs.Url) != "" {
			learnAuthorizer = true
		}
	}

	added := 0
	for name, definition := range coreDefinitions {
		if name == AuthorizerName && !learnAuthorizer {
			continue
		}
		records, err := queryRegistry(sys, registrar, definition)
		if err != nil {
			return added, fmt.Errorf("asking the registrar for %s: %w", definition, err)
		}
		for _, rec := range records {
			coreURL := coreURLOf(name, rec)
			if coreURL == "" {
				continue
			}
			if sys.AddLearnedCoreSystem(components.CoreSystem{Name: name, Url: coreURL}) {
				log.Printf("%s: learned from the registry that %s is at %s\n", sys.Name, name, coreURL)
				added++
			}
		}
	}
	if added > 0 {
		if err := saveCoreCache(sys); err != nil {
			log.Printf("%s: could not write %s: %v\n", sys.Name, CoreCacheFileName, err)
		}
	}
	return added, nil
}

// coreURLOf turns a service record into the core URL a client dereferences:
// the service's own URL with the last path segment removed, since a core URL
// names the system and its asset and the client appends the service.
//
// The scheme follows the same line the configuration template draws. The
// registrar and the CA are reached before this system has a certificate, so
// they are reached in the clear; the orchestrator and the authorizer are reached
// after, over TLS if the provider bound it.
func coreURLOf(name string, rec forms.ServiceRecord_v1) string {
	if len(rec.IPAddresses) == 0 {
		return ""
	}
	proto, port := preferredProtoPort(rec.ProtoPort)
	if name == components.ServiceRegistrarName || name == "ca" {
		proto, port = "http", rec.ProtoPort["http"]
	} else if proto != "https" {
		// A core system reached after enrollment is learned only in its
		// enrolled form. The authorizer registers before it holds a certificate,
		// so its first record names an http port and nothing else; learned then,
		// it would be an authorizer whose key is fetched in the clear. The next
		// lesson, a minute on, finds the record renewed with TLS bound.
		return ""
	}
	if port == 0 {
		return ""
	}
	sub := strings.Trim(rec.SubPath, "/")
	if i := strings.LastIndex(sub, "/"); i >= 0 {
		sub = sub[:i]
	} else {
		return "" // no asset segment to keep: not a URL a client could use
	}
	return proto + "://" + rec.IPAddresses[0] + ":" + strconv.Itoa(port) + "/" + rec.SystemName + "/" + sub
}

// queryRegistry asks the lead for every provider of one definition.
func queryRegistry(sys *components.System, registrar, definition string) ([]forms.ServiceRecord_v1, error) {
	quest := forms.ServiceQuest_v1{
		RequesterName:     sys.Name,
		ServiceDefinition: definition,
		Protocol:          "https",
		Details:           map[string][]string{},
		Version:           "ServiceQuest_v1",
	}
	body, err := json.Marshal(quest)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, registrar+"/query", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second, Transport: http.DefaultClient.Transport}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		reason, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(reason)))
	}
	var list forms.ServiceRecordList_v1
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	return list.List, nil
}

func saveCoreCache(sys *components.System) error {
	data, err := json.MarshalIndent(sys.LearnedCoreSystems(), "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(CoreCacheFileName, data, 0o644)
}

// LoadCoreCache restores what an earlier run learned, behind the file's own
// entries. A missing cache is the ordinary first run and not an error.
func LoadCoreCache(sys *components.System) {
	data, err := os.ReadFile(CoreCacheFileName)
	if err != nil {
		return
	}
	var cached []components.CoreSystem
	if err := json.Unmarshal(data, &cached); err != nil {
		log.Printf("%s: ignoring %s: %v\n", sys.Name, CoreCacheFileName, err)
		return
	}
	restored := 0
	for _, cs := range cached {
		if sys.AddLearnedCoreSystem(cs) {
			restored++
		}
	}
	if restored > 0 {
		log.Printf("%s: %d core system(s) remembered from %s\n", sys.Name, restored, CoreCacheFileName)
	}
}
