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

// Package "components" addresses the structures and behaviors of the components that
// are aggregated to form Arrowhead compliant systems in a local cloud.
// An Arrowhead local cloud is a system of systems, which are made up of a capsule
// (a.k.a. a shell) and a thing (a.k.a. an asset). The capsule runs on a device,
// and exposes the thing's resources' skills as services.

package components

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// An Arrowhead Service has specific properties that exposes a unit asset's functionality
type Service struct {
	ID         int     `json:"-"`                 // Id assigned by the Service Registrar
	Definition string  `json:"definition"`        // Service definition or purpose
	SubPath    string  `json:"subpath"`           // The URL subpath after the resource's
	Mission    Mission `json:"mission,omitempty"` // Overrides the unit asset's mission, where the asset's is too coarse (see EffectiveMission)
	// Details is metadata about the service, and is the framework's open slot:
	// anything a system wants said about a service that has no field of its own
	// goes here, reaches the registrar, and is written into the knowledge graph
	// as a predicate.
	//
	// Conventional keys, all optional:
	//
	//	Forms         the payload form, e.g. "SignalA_v1a"
	//	Unit          a QUDT unit IRI, in angle brackets
	//	QuantityKind  a QUDT quantity kind IRI, in angle brackets
	//	Measure       "interval" or "point", where the difference matters
	//	Methods       the HTTP methods this service answers (see below)
	//
	// Methods says which methods the service accepts, as W3C HTTP method IRIs:
	//
	//	"Methods": {"<http://www.w3.org/2011/http-methods#GET>",
	//	            "<http://www.w3.org/2011/http-methods#PUT>"}
	//
	// Only worth stating when the service accepts more than a read, because a
	// consumer reasonably assumes GET. Until this existed, nothing outside the
	// provider's own `serving` switch knew a service could be written to: the
	// registration form did not carry it and the graph did not say it, so the
	// only record that a setpoint was settable was the English in Description.
	// A consumer generating a client, or an AAS describing the interface, had
	// no way to find out except by trying a PUT and seeing what happened.
	//
	// IRIs rather than the bare strings "GET" and "PUT" because a detail value
	// that looks like a name is written into the graph as an entity in the
	// local cloud's namespace — alc:GET, a thing this cloud invented — whereas
	// the W3C HTTP vocabulary already has these and they dereference. It is the
	// same reason Unit carries a QUDT IRI instead of the word "Celsius".
	Details       map[string][]string `json:"details"`
	RegPeriod     int                 `json:"registrationPeriod"` // The period until the registrar is expecting a sign of life
	RegTimestamp  string              `json:"-"`                  // the creation date in the Service Registry to ensure that reRegistration is with the same record
	RegExpiration string              `json:"-"`                  // The actual time when the service record will expire if not refreshed
	Description   string              `json:"-"`                  // This is used in the service description in /doc
	// SubscribeAble says a consumer may follow this service's value rather than
	// ask for it repeatedly. Configurable, which it was not: the field carried
	// `json:"-"`, so a systemconfig.json could not turn it on and nothing but
	// code ever could.
	SubscribeAble bool `json:"subscribable,omitempty"`
	// Heartbeat is how long this service may go without saying anything to a
	// subscriber, in seconds. A subscriber that hears nothing for a few of these
	// treats the publisher as gone, so it is the liveness contract as much as a
	// refresh rate. Zero means the framework's default.
	Heartbeat int `json:"heartbeat,omitempty"`
	// Threshold is how much the value must move, in this service's own unit,
	// before it is worth telling anyone. Zero means any change is.
	//
	// In this service's unit, which matters: a consumer reading in °F and asking
	// for 0.5 is asking for something other than it thinks, so a proposal
	// carries its unit and is converted.
	Threshold float64 `json:"threshold,omitempty"`
	// FastestHeartbeat and FinestThreshold bound what a subscriber may ask for.
	// A consumer knows what it needs and the provider knows what it can honour —
	// a threshold below the sensor's resolution is meaningless and a heartbeat
	// faster than the sampling period is impossible — so a proposal is clamped
	// to these and the subscriber is told what it actually got.
	FastestHeartbeat int     `json:"fastestHeartbeat,omitempty"`
	FinestThreshold  float64 `json:"finestThreshold,omitempty"`
	// Stream carries this service's value to whoever is following it, and is nil
	// until the framework prepares one for a service that declares itself
	// subscribable.
	//
	// An interface rather than the publisher itself, because the publisher lives
	// in usecases and usecases already imports this package. What a service needs
	// to know about it is only whether it can be followed and how to answer
	// somebody who wants to.
	Stream ValueStream `json:"-"`

	ACost      float64 `json:"-"`        // activity cost to execute the service
	CUnit      string  `json:"costUnit"` // cost unit
	CFootprint float64 `json:"-"`        // carbon footprint in metric tonnes when executing the service
}

// Remember stores a value a subscription delivered, and the terms it arrived
// under, and wakes whoever is waiting for one.
func (c *Cervice) Remember(payload []byte, mediaType string, heartbeat time.Duration) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	c.followed = payload
	c.followedType = mediaType
	c.followedAt = time.Now()
	if heartbeat > 0 {
		c.heartbeat = heartbeat
	}

	floor := c.WakeFloor
	if floor <= 0 {
		floor = DefaultWakeFloor
	}
	if !c.lastWake.IsZero() && time.Since(c.lastWake) < floor {
		return
	}
	c.lastWake = time.Now()
	c.wakeChan()
	select {
	case c.updates <- struct{}{}:
	default: // one is already pending, and one wake is as good as three
	}
}

// DefaultWakeFloor is how often a followed value may wake a consumer when the
// consumer has not said. A second is short enough that a control loop feels
// immediate to somebody holding a sensor, and long enough that an actuator is
// not driven by the noise of a value that is merely being reported finely.
const DefaultWakeFloor = time.Second

// Updated is closed-over-time notification that a followed value has arrived.
//
// A control loop selects on it beside its own ticker: the ticker is the
// guarantee that the loop runs at all, and this is what makes it run *now* when
// something has changed. Waiting on it alone would leave a loop asleep for as
// long as its provider had nothing to say — and a publisher that has died says
// nothing at all.
//
// Safe on a cervice that does not exist. A consumer whose configuration names no
// provider for something holds a nil cervice, and a nil channel in a select
// simply never fires — which is the right answer and, more to the point, is not
// a panic in a control loop. Calling this in the one line that sets a feedback
// loop going must not be the thing that stops a plant.
func (c *Cervice) Updated() <-chan struct{} {
	if c == nil {
		return nil
	}
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	c.wakeChan()
	return c.updates
}

// wakeChan makes the channel on first use. Cervices are built as struct
// literals all over this project, so nothing can be assumed to have run a
// constructor. Callers hold the lock.
func (c *Cervice) wakeChan() {
	if c.updates == nil {
		c.updates = make(chan struct{}, 1)
	}
}

// Recall returns the value a subscription last delivered, if there is one and it
// is recent enough to be believed.
//
// Recent enough means within three heartbeats. The publisher promised to say
// something every heartbeat whether the value moved or not, so silence past a
// few of those is the publisher being gone — and a controller fed the last
// temperature of a sensor that died an hour ago is worse off than one told to go
// and ask.
func (c *Cervice) Recall() ([]byte, string, bool) {
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()
	if len(c.followed) == 0 {
		return nil, "", false
	}
	stale := 3 * c.heartbeat
	if c.heartbeat <= 0 {
		stale = 90 * time.Second
	}
	if time.Since(c.followedAt) > stale {
		return nil, "", false
	}
	return c.followed, c.followedType, true
}

// Forget drops a followed value, so the next read asks the provider instead.
func (c *Cervice) Forget() {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	c.followed, c.followedType = nil, ""
	c.following = false
}

// StartFollowing claims the right to keep this cervice's subscription up, and
// reports whether the caller got it. Only one follower per cervice: a second
// would open a second connection to the same provider and overwrite the same
// value.
func (c *Cervice) StartFollowing() bool {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	if c.following {
		return false
	}
	c.following = true
	return true
}

// ValueStream is a service's value as something to follow rather than to ask
// for.
type ValueStream interface {
	// Subscribable reports whether this service is meant to be followed. It is
	// safe on a nil stream, which is what a service that declares nothing has.
	Subscribable() bool
	// ServeStream answers a request to follow the value and holds the connection
	// open until the caller goes away.
	ServeStream(w http.ResponseWriter, r *http.Request)
}

// type Services is a collection of service structs
type Services map[string]*Service

// Merge method is used in the configuration use case to prevent the subpath or description to be changed or "configured"
func (s *Service) Merge(originalS *Service) {
	s.Definition = originalS.Definition
	s.SubPath = originalS.SubPath
	s.Description = originalS.Description
}

// DeepCopy creates a deep copy of the Service instance
func (s Service) DeepCopy() *Service {
	// Copy the map
	detailsCopy := make(map[string][]string)
	for key, value := range s.Details {
		// Copy each slice individually
		sliceCopy := make([]string, len(value))
		copy(sliceCopy, value)
		detailsCopy[key] = sliceCopy
	}

	// Create and return a new instance of Service with copied values
	return &Service{
		ID:            s.ID,
		Definition:    s.Definition,
		SubPath:       s.SubPath,
		Details:       detailsCopy,
		RegPeriod:     s.RegPeriod,
		RegTimestamp:  s.RegTimestamp,
		RegExpiration: s.RegExpiration,
		Description:   s.Description,
		SubscribeAble: s.SubscribeAble,
		Heartbeat:     s.Heartbeat,
		Threshold:     s.Threshold,

		FastestHeartbeat: s.FastestHeartbeat,
		FinestThreshold:  s.FinestThreshold,

		ACost: s.ACost,
		CUnit: s.CUnit,
	}
}

// DeepCopy creates a deep copy of the Services map
func CloneServices(sTemplates []Service) Services {
	services := make(map[string]*Service)
	for _, sTemplate := range sTemplates {
		newService := sTemplate.DeepCopy()
		serviceName := newService.Definition
		services[serviceName] = newService
	}
	return services
}

// Function to merge two Details maps
func MergeDetails(map1, map2 map[string][]string) map[string][]string {
	// Create a new map to hold the merged result
	result := make(map[string][]string)

	// Add all elements from map1
	for key, value := range map1 {
		result[key] = append([]string{}, value...)
	}

	// Add all elements from map2
	for key, value := range map2 {
		if existing, found := result[key]; found {
			// If the key exists, merge the slices
			result[key] = append(existing, value...)
		} else {
			// If the key does not exist, just add it
			result[key] = append([]string{}, value...)
		}
	}

	return result
}

// ---------------------------------------------------

// NodeInfo holds the URL and registered metadata for a single discovered service endpoint.
type NodeInfo struct {
	URL     string
	Details map[string][]string
	// SubscribeAble says this provider will let the value be followed rather
	// than asked for repeatedly.
	SubscribeAble bool
	// Tokens are the access tokens the orchestrator obtained for this provider,
	// keyed by the action each was minted for — "read", "write" or "invoke".
	//
	// One per action, because a token names the action it permits and the
	// provider recomputes that action from the HTTP method it receives. A read
	// token presented on a PUT is refused, so a cervice used for both a GET and
	// a POST needs one of each — which a single token string could not hold.
	//
	// An entry with an empty value is meaningful: it records that this action was
	// discovered and the cloud issued no token, which is what an unauthorized
	// cloud does. Without it a consumer would re-orchestrate before every call.
	//
	// A token outlives none of the requests it is presented on: when it expires
	// the provider refuses, the node cache is cleared, and the next call
	// re-orchestrates for a fresh one.
	Tokens map[string]string
}

// TokenFor returns the token minted for one action, and whether this node has
// been discovered for that action at all. The two are distinct: a discovered
// action with no token is an unauthorized cloud, not a missing discovery.
func (ni NodeInfo) TokenFor(action string) (token string, discovered bool) {
	if ni.Tokens == nil {
		return "", false
	}
	token, discovered = ni.Tokens[action]
	return token, discovered
}

// A Cervice is a consumed service
type Cervice struct {
	IReferentce string // Internal reference when consuming more than one service of the same type
	Definition  string // Service definition or purpose
	Details     map[string][]string
	Nodes       map[string][]NodeInfo
	Protos      []string
	Mode        string // "get" for GetState, "set" for SetState, "" for unspecified

	// followed is the value a subscription last delivered, kept as the bytes that
	// arrived rather than as a parsed form.
	//
	// The bytes, so that a value read from here and a value read over the network
	// travel exactly the same path afterwards: unpacked by the same code and
	// converted into the consumer's unit by the same code. A second path would be
	// a second place for a unit to be got wrong, and a controller that is fed a
	// number in the wrong unit does not fail, it does the wrong thing quietly.
	followed     []byte
	followedType string
	followedAt   time.Time
	// heartbeat is what the publisher agreed to, which is how long the value may
	// be quiet before silence means the publisher is gone rather than the reading
	// being steady.
	heartbeat time.Duration
	// following says a subscription is already being kept up, so a second
	// discovery does not start a second one.
	following bool

	// updates carries "a new value has arrived" to whoever is waiting on it, so
	// a control loop can act on a reading instead of finding it on its next
	// tick. Buffered by one and never blocked on: the point is to wake somebody,
	// and one pending wake is as good as three.
	updates chan struct{}
	// lastWake is when the last one was sent, so a value that moves constantly
	// does not wake a control loop constantly.
	lastWake time.Time
	// WakeFloor is the shortest interval between wakes. Zero means the default.
	//
	// A threshold is chosen for what is worth reporting; this is for what is
	// worth acting on, and they are not the same question. A tenth of a degree
	// is worth telling a data logger about and not worth moving a valve for —
	// so a servo that chased every reported change would chatter and wear for no
	// improvement in control.
	//
	// Suppressing a wake never loses the value: it is already in the cache, and
	// the loop's own ticker remains the guarantee that it is acted upon.
	WakeFloor time.Duration

	// Mutex guards Nodes and the tokens inside it.
	//
	// Discovery replaces entries and now deletes them, consumption reads them to
	// dispatch and writes back to forget a token, and a unit asset with more
	// than one goroutine polls the same cervice from each. A map written during
	// a range over it is not a race that corrupts a value — it is `fatal error:
	// concurrent map iteration and map write`, which no recover reaches and
	// which takes the system down.
	//
	// Exported like System.Mutex, because some systems reach into Nodes
	// themselves. Hold it for the map work only: the point of the snapshots in
	// the consumption path is that a request to an unresponsive provider must
	// not be made while holding a lock every other goroutine wants.
	Mutex sync.RWMutex
}

// Providers returns the discovered providers as a snapshot.
//
// A copy, so the caller can make its requests without holding the lock. A
// consuming round asks every provider in turn, each request bounded only by its
// own timeout, and a discovery on another goroutine must not have to wait for
// an unresponsive sensor before it can record what it found.
func (c *Cervice) Providers() []NodeInfo {
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	var providers []NodeInfo
	for _, nodes := range c.Nodes {
		providers = append(providers, nodes...)
	}
	return providers
}

// ProviderCount reports how many providers have been discovered.
func (c *Cervice) ProviderCount() int {
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()

	n := 0
	for _, nodes := range c.Nodes {
		n += len(nodes)
	}
	return n
}

// Cervises is a collection of "Cervice" structs
type Cervices map[string]*Cervice

// HTTPMethodVocabulary is the W3C HTTP Vocabulary in RDF, where the methods a
// service accepts are already named and already dereferenceable.
const HTTPMethodVocabulary = "http://www.w3.org/2011/http-methods#"

// HTTPMethods renders method names as the Details["Methods"] value expects, so
// a system states what it accepts without writing the vocabulary IRI out four
// times:
//
//	Details: map[string][]string{"Methods": components.HTTPMethods("GET", "PUT")},
//
// A service that only answers GET need say nothing; that is what a consumer
// assumes of a service it has been told nothing about.
func HTTPMethods(names ...string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, "<"+HTTPMethodVocabulary+strings.ToUpper(name)+">")
	}
	return out
}
