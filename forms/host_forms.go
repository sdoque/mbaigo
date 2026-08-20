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

// How loaded a machine is, as the machine itself sees it.

package forms

import (
	"reflect"
	"time"
)

// HostLoad_v1 is what one host reports about its own capacity.
//
// Two kinds of field, deliberately. Headroom is the single comparable number,
// and only the host can compute it: it knows its core count and its thermal
// ceiling, so it is the only party that can say what a load average means. The
// raw figures beneath it are what let a reader disagree — a balancer applying
// its own policy needs to see the trend, not just the verdict.
//
// What is *not* here is as considered as what is. There is no recommendation:
// reporting is one job and deciding is another, and a ShouldMigrate field would
// scatter the balancing policy across every maitreD in the cloud instead of
// keeping it in the one system that balances. There is no per-process list
// either: it would be a far richer disclosure than a load average, and nothing
// needs it, since the knowledge graph already says which systems run where.
// Load from the host, placement from the graph, mobility from the asset — three
// sources, each saying only what it is the authority on.
type HostLoad_v1 struct {
	// Host is the machine's name, so a reading can be matched to the placement
	// the graph describes.
	Host string `json:"host"`

	// Headroom is what remains, from 0 (saturated) to 1 (idle). The one figure
	// that compares a Raspberry Pi with a server, because each host normalizes
	// against its own capacity before reporting.
	Headroom float64 `json:"headroom"`

	// LoadNormalized is the one-minute load average divided by the core count,
	// so 1.0 means "as much runnable work as there are cores" on any machine.
	LoadNormalized float64 `json:"loadNormalized"`

	// Load1, Load5 and Load15 are the raw averages. A reader wanting to tell a
	// spike from a trend needs all three; headroom alone cannot say which it is.
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`

	// Cores is what the normalization divided by, stated so the arithmetic can
	// be checked rather than trusted.
	Cores int `json:"cores"`

	// MemAvailableMB is the kernel's own estimate of what a new process could
	// obtain — not "free", which counts none of the reclaimable cache and so
	// makes every healthy Linux machine look exhausted.
	MemAvailableMB int `json:"memAvailableMB"`
	MemTotalMB     int `json:"memTotalMB"`

	// CPUTempC and Throttled matter more on an edge deployment than on a server.
	// A Raspberry Pi under sustained load derates rather than queues, so a host
	// can read as unbusy while delivering half its clock — and a balancer that
	// ignores this moves work *onto* a degraded machine.
	//
	// ThrottledNow is the current state; ThrottledSince is the sticky flag the
	// firmware sets on the first occurrence since boot. They answer different
	// questions: whether to avoid this host today, and whether it has a cooling
	// problem worth fixing.
	CPUTempC       float64 `json:"cpuTempC,omitempty"`
	ThrottledNow   bool    `json:"throttledNow"`
	ThrottledSince bool    `json:"throttledSince"`

	// StallCPU, StallIO and StallMemory are the Linux pressure-stall figures:
	// the fraction of the last ten seconds in which work was delayed waiting for
	// a resource. This is what "too loaded" actually means, and it is a better
	// signal than load average — a load of 4.0 saturates a four-core Pi and
	// idles a sixteen-core server.
	//
	// Optional because a kernel built without CONFIG_PSI does not expose them,
	// which is the case on stock Raspberry Pi OS until psi=1 is added to the
	// boot command line. A fleet will be mixed, so a reader should prefer these
	// when present and fall back to the load average when they are not.
	StallCPU    *float64 `json:"stallCPU,omitempty"`
	StallIO     *float64 `json:"stallIO,omitempty"`
	StallMemory *float64 `json:"stallMemory,omitempty"`

	// SampledAt is when the host looked, not when the caller asked. The two
	// differ by design: the reading is taken on a timer and served from cache,
	// so that following this service costs the host nothing per subscriber.
	SampledAt time.Time `json:"sampledAt"`

	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

func (f *HostLoad_v1) NewForm() Form {
	f.Version = "HostLoad_v1"
	return f
}

func (f *HostLoad_v1) FormVersion() string {
	return f.Version
}

func init() {
	FormTypeMap["HostLoad_v1"] = reflect.TypeOf(HostLoad_v1{})
}
