package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

func sample(value float64) *forms.SignalA_v1a {
	var f forms.SignalA_v1a
	f.NewForm()
	f.Value = value
	f.Unit = "<http://qudt.org/vocab/unit/DEG_C>"
	f.Timestamp = time.Now()
	return &f
}

func temperature(threshold float64) *components.Service {
	return &components.Service{
		Definition:    "temperature",
		SubPath:       "temp",
		SubscribeAble: true,
		Threshold:     threshold,
		Heartbeat:     30,
		Details:       map[string][]string{"Unit": {"<http://qudt.org/vocab/unit/DEG_C>"}},
	}
}

// A change worth reporting is measured from what subscribers were last told, not
// from the sample before.
//
// Measuring against the previous sample is the easy mistake and it is silent: a
// value creeping by a tenth of the threshold every second never reports a change
// however far it drifts, so a thermostat's subscribers would sit on a reading
// that is degrees out while every individual step looked negligible.
func TestAValueThatCreepsIsStillReported(t *testing.T) {
	publisher := NewPublisher(temperature(0.5))
	events := make(chan forms.Form, 32)
	sub, done := publisher.addSubscriber(terms{})
	defer done()
	go func() {
		for value := range sub.events {
			events <- value
		}
	}()

	publisher.Sample(sample(20.0)) // the first is always sent: nobody knew it
	for _, step := range []float64{20.1, 20.2, 20.3, 20.4, 20.5, 20.6} {
		publisher.Sample(sample(step))
	}
	time.Sleep(20 * time.Millisecond)

	var got []float64
	for {
		select {
		case value := <-events:
			got = append(got, value.(*forms.SignalA_v1a).Value)
			continue
		default:
		}
		break
	}

	if len(got) < 2 {
		t.Fatalf("readings sent: %v; a value that crept 0.6 past the baseline was "+
			"never reported, because each step was compared with the step before", got)
	}
	if got[0] != 20.0 {
		t.Errorf("the first reading sent was %v, want 20.0", got[0])
	}
	if got[1] != 20.5 {
		t.Errorf("the change was reported at %v; with a threshold of 0.5 from a "+
			"baseline of 20.0 it is due at 20.5", got[1])
	}
}

// A value sitting still says nothing, which is the point of a threshold.
func TestASteadyValueIsNotBroadcast(t *testing.T) {
	publisher := NewPublisher(temperature(0.5))
	sub, done := publisher.addSubscriber(terms{})
	defer done()

	publisher.Sample(sample(20.0))
	<-sub.events // the first
	for i := 0; i < 5; i++ {
		publisher.Sample(sample(20.1))
	}

	select {
	case value := <-sub.events:
		t.Errorf("a value that moved 0.1 against a threshold of 0.5 was broadcast (%v)",
			value.(*forms.SignalA_v1a).Value)
	default:
	}
}

// A heartbeat carries the present value, not the last one broadcast — otherwise
// a subscriber that joined during a drift is told the stale figure every thirty
// seconds and has no way to know it.
func TestTheCurrentValueIsTheLatestSample(t *testing.T) {
	publisher := NewPublisher(temperature(0.5))
	publisher.Sample(sample(20.0))
	publisher.Sample(sample(20.2)) // below the threshold: not broadcast

	value, known := publisher.Current()
	if !known {
		t.Fatal("no current value after two samples")
	}
	if got := value.(*forms.SignalA_v1a).Value; got != 20.2 {
		t.Errorf("the current value is %v; a heartbeat must carry the present "+
			"reading, not the one last broadcast", got)
	}
}

// The terms are negotiated: the consumer knows what it needs and the provider
// what it can honour, so a proposal is clamped rather than obeyed or ignored.
func TestASubscriberIsToldTheTermsItActuallyGot(t *testing.T) {
	service := temperature(0.5)
	service.FastestHeartbeat = 5
	service.FinestThreshold = 0.2
	publisher := NewPublisher(service)

	// Asking for more than the sensor can give.
	agreed := publisher.agree(terms{Heartbeat: time.Second, Threshold: 0.01})
	if agreed.Heartbeat != 5*time.Second {
		t.Errorf("heartbeat agreed at %s; the service cannot beat faster than 5s", agreed.Heartbeat)
	}
	if agreed.Threshold != 0.2 {
		t.Errorf("threshold agreed at %v; the service cannot resolve finer than 0.2", agreed.Threshold)
	}

	// Asking for something reasonable is honoured.
	agreed = publisher.agree(terms{Heartbeat: 20 * time.Second, Threshold: 0.3})
	if agreed.Heartbeat != 20*time.Second || agreed.Threshold != 0.3 {
		t.Errorf("a workable proposal was not honoured: %s, %v", agreed.Heartbeat, agreed.Threshold)
	}

	// Asking for nothing gets the service's own terms.
	agreed = publisher.agree(terms{})
	if agreed.Heartbeat != 30*time.Second || agreed.Threshold != 0.5 {
		t.Errorf("the service's own terms were not used: %s, %v", agreed.Heartbeat, agreed.Threshold)
	}
}

// The stream says what was agreed before it says anything else, and the current
// value immediately after, so a subscriber never waits for a heartbeat to learn
// either the terms or the state.
func TestTheStreamOpensWithTheTermsAndTheValue(t *testing.T) {
	publisher := NewPublisher(temperature(0.5))
	publisher.Sample(sample(21.5))

	r := httptest.NewRequest(http.MethodGet, "/sys/asset/temp?heartbeat=20&threshold=0.3", nil)
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Millisecond)
	defer cancel()
	w := httptest.NewRecorder()
	publisher.ServeStream(w, r.WithContext(ctx))

	body := w.Body.String()
	if !strings.Contains(body, "event: terms") {
		t.Errorf("the stream did not open with the agreed terms:\n%s", body)
	}
	if !strings.Contains(body, `"heartbeat":20`) || !strings.Contains(body, `"threshold":0.3`) {
		t.Errorf("the terms sent are not the ones agreed:\n%s", body)
	}
	if !strings.Contains(body, "event: value") || !strings.Contains(body, "21.5") {
		t.Errorf("the current value was not sent on subscribing:\n%s", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type is %q", ct)
	}
}

// A publisher carries a handful of consumers, not an unbounded number: each
// holds a connection and a buffer on a machine that is also running a control
// loop.
func TestSubscribersAreCapped(t *testing.T) {
	publisher := NewPublisher(temperature(0))
	for i := 0; i < maxSubscribers; i++ {
		if sub, _ := publisher.addSubscriber(terms{}); sub == nil {
			t.Fatalf("refused subscriber %d, below the cap of %d", i, maxSubscribers)
		}
	}
	if sub, _ := publisher.addSubscriber(terms{}); sub != nil {
		t.Errorf("accepted subscriber %d, past the cap", maxSubscribers+1)
	}
}

// One consumer that has stopped reading must not stall a sampling loop that is
// also driving a control loop.
func TestASlowSubscriberDoesNotStallTheSampler(t *testing.T) {
	publisher := NewPublisher(temperature(0))
	sub, done := publisher.addSubscriber(terms{})
	defer done()

	finished := make(chan struct{})
	go func() {
		for i := 0; i < 500; i++ { // far more than the subscriber's buffer
			publisher.Sample(sample(float64(i)))
		}
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("sampling blocked on a subscriber that was not reading")
	}
	_ = sub
}

// A service that says it is subscribable gets a publisher without its system
// writing any code to make one, so turning subscription on is configuration.
func TestASubscribableServiceIsGivenAPublisher(t *testing.T) {
	sys := components.NewSystem("ds18b20", context.Background())
	followed := temperature(0.5)
	plain := &components.Service{Definition: "jitter", SubPath: "jitter"}
	sys.UAssets["sensor"] = &components.UnitAsset{
		Name:    "sensor",
		Mission: components.MissionMeasurement,
		ServicesMap: components.Services{
			followed.SubPath: followed,
			plain.SubPath:    plain,
		},
	}

	PreparePublishers(&sys)

	if followed.Stream == nil || !followed.Stream.Subscribable() {
		t.Error("a service configured as subscribable cannot be followed")
	}
	if plain.Stream != nil {
		t.Error("a service that asked for nothing was given a publisher anyway")
	}

	// And the system publishes by handing over a sample, whether or not anyone
	// is listening and whether or not the service is subscribable.
	Publish(sys.UAssets["sensor"], "temp", sample(21.0))
	if value, known := followed.Stream.(*Publisher).Current(); !known || value.(*forms.SignalA_v1a).Value != 21.0 {
		t.Error("the sample handed to the framework did not reach the publisher")
	}
	Publish(sys.UAssets["sensor"], "jitter", sample(3.0)) // must not panic
	Publish(nil, "temp", sample(1.0))                     // nor this
}

// A followed value is answered without asking the provider, and the caller's
// loop cannot tell the difference — which is the whole point: no consumer is
// rewritten to gain a subscription.
func TestAFollowedValueIsAnsweredWithoutARequest(t *testing.T) {
	asked := 0
	useTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		asked++
		return nil, fmt.Errorf("the provider was asked, when the value was already known")
	}))

	cer := &components.Cervice{
		Definition: "temperature",
		Nodes: map[string][]components.NodeInfo{"sensor": {{
			URL: "http://sensor/temperature", SubscribeAble: true,
			Tokens: map[string]string{"read": "tok"},
		}}},
		Details: map[string][]string{"Unit": {"<http://qudt.org/vocab/unit/DEG_C>"}},
	}
	// As a subscription would have left it.
	cer.Remember([]byte(`{"value":21.5,"unit":"<http://qudt.org/vocab/unit/DEG_C>","version":"SignalA_v1.0"}`),
		"application/json", 30*time.Second)

	// Already being followed, which is the state a read finds in production: the
	// subscription is up and the value is arriving. Without this, Follow starts
	// one here and its first connection attempt is the request the test is
	// counting.
	cer.StartFollowing()

	sys := createTestSystem(false)
	form, err := stateHandler(http.MethodGet, cer, &sys, nil)
	if err != nil {
		t.Fatalf("reading a followed value: %v", err)
	}
	if asked != 0 {
		t.Errorf("the provider was asked %d times for a value already being followed", asked)
	}
	if got := form.(*forms.SignalA_v1a).Value; got != 21.5 {
		t.Errorf("read %v, want 21.5", got)
	}
}

// A value nobody is keeping current must not be served. The publisher promised
// to speak every heartbeat whether the value moved or not, so silence past a few
// of those means it is gone — and a controller fed the last reading of a sensor
// that died an hour ago is worse off than one told to go and ask.
func TestAStaleFollowedValueIsNotServed(t *testing.T) {
	cer := &components.Cervice{Definition: "temperature"}
	cer.Remember([]byte(`{"value":21.5,"version":"SignalA_v1.0"}`), "application/json", time.Millisecond)

	if _, _, fresh := cer.Recall(); !fresh {
		t.Fatal("a value just delivered was already considered stale")
	}
	time.Sleep(10 * time.Millisecond) // past three heartbeats of 1ms
	if _, _, fresh := cer.Recall(); fresh {
		t.Error("a value older than three heartbeats was still offered to a control loop")
	}
}

// The stream's terms and values reach the cervice, and the heartbeat with them —
// without it, staleness cannot be judged.
func TestFollowingKeepsTheValueCurrent(t *testing.T) {
	stream := "event: terms\ndata: {\"heartbeat\":20,\"threshold\":0.5}\n\n" +
		"event: value\ndata: {\"value\":19.0,\"unit\":\"<http://qudt.org/vocab/unit/DEG_C>\",\"version\":\"SignalA_v1.0\"}\n\n" +
		"event: value\ndata: {\"value\":19.7,\"unit\":\"<http://qudt.org/vocab/unit/DEG_C>\",\"version\":\"SignalA_v1.0\"}\n\n"

	cer := &components.Cervice{Definition: "temperature"}
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(stream))}
	if err := readValues(cer, resp); err != nil {
		t.Fatalf("reading the stream: %v", err)
	}

	payload, _, fresh := cer.Recall()
	if !fresh {
		t.Fatal("nothing was remembered from the stream")
	}
	form, err := Unpack(payload, "application/json")
	if err != nil {
		t.Fatalf("what was remembered is not readable: %v", err)
	}
	if got := form.(*forms.SignalA_v1a).Value; got != 19.7 {
		t.Errorf("the cervice holds %v; the last value on the stream was 19.7", got)
	}
}

// Only one follower per cervice: a second would open a second connection to the
// same provider and write over the same value.
func TestOnlyOneFollowerPerCervice(t *testing.T) {
	cer := &components.Cervice{Definition: "temperature"}
	if !cer.StartFollowing() {
		t.Fatal("the first follower was refused")
	}
	if cer.StartFollowing() {
		t.Error("a second follower was allowed onto the same cervice")
	}
	cer.Forget()
	if !cer.StartFollowing() {
		t.Error("after the subscription ended, nothing could take it up again")
	}
}

// TestAPublisherAndAConsumerMeet is the one test that proves the two halves fit.
//
// Everything else here exercises one side against a fixture the other side never
// wrote. This runs a real publisher behind a real HTTP server, points a cervice
// at it, and asks the question a control loop asks — with no consuming code
// written for the occasion, because the claim being tested is that none is
// needed.
func TestAPublisherAndAConsumerMeet(t *testing.T) {
	service := temperature(0.5)
	service.FastestHeartbeat = 1
	publisher := NewPublisher(service)
	service.Stream = publisher

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !wantsStream(r) {
			t.Error("the consumer asked for the value without asking to follow it")
			return
		}
		publisher.ServeStream(w, r)
	}))
	// Closed last: httptest waits for outstanding requests, and a subscription is
	// an outstanding request that never ends until the consumer stops. Cancelling
	// the consumer's context is what lets the handler return, so it has to happen
	// first — deferred later, therefore run earlier.
	defer provider.Close()

	publisher.Sample(sample(19.0))

	ctx, stopConsumer := context.WithCancel(context.Background())
	defer stopConsumer()
	sys := components.NewSystem("thermostat", ctx)
	sys.Husk = &components.Husk{ProtoPort: map[string]int{"http": 0}}
	cer := &components.Cervice{
		Definition: "temperature",
		Details:    map[string][]string{"Unit": {"<http://qudt.org/vocab/unit/DEG_C>"}},
		Nodes: map[string][]components.NodeInfo{"sensor": {{
			URL: provider.URL, SubscribeAble: true,
			Tokens: map[string]string{"read": ""},
		}}},
	}

	Follow(cer, &sys)

	// The value should arrive without anybody asking for it.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, fresh := cer.Recall(); fresh {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, _, fresh := cer.Recall(); !fresh {
		t.Fatal("the consumer subscribed and no value ever arrived")
	}

	form, err := stateHandler(http.MethodGet, cer, &sys, nil)
	if err != nil {
		t.Fatalf("reading the followed value: %v", err)
	}
	if got := form.(*forms.SignalA_v1a).Value; got != 19.0 {
		t.Fatalf("the consumer read %v, want the published 19.0", got)
	}

	// And it follows the value as it moves.
	publisher.Sample(sample(21.0))
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		form, err = stateHandler(http.MethodGet, cer, &sys, nil)
		if err == nil && form.(*forms.SignalA_v1a).Value == 21.0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("the consumer still reads %v after the publisher moved to 21.0",
		form.(*forms.SignalA_v1a).Value)
}

// TestTheRegistrationSaysAServiceCanBeFollowed is the join that was missing.
//
// A consumer decides whether to subscribe from what the registrar told it — it
// cannot read the provider's configuration file. The field was on the service
// and on the record and nothing connected them, so every service in every cloud
// registered as not subscribable whatever it declared, and consumers polled
// providers that were willing to publish. The framework never noticed because
// both ends were individually correct.
func TestTheRegistrationSaysAServiceCanBeFollowed(t *testing.T) {
	sys := components.NewSystem("ds18b20", context.Background())
	sys.Husk = &components.Husk{
		Host:      &components.HostingDevice{Name: "canbus", IPAddresses: []string{"192.0.2.1"}},
		ProtoPort: map[string]int{"http": 20150},
	}
	sys.Husk.Bound.Bind("http", 20150)

	followed := temperature(0.1)
	asset := &components.UnitAsset{
		Name: "28-00000f030344", Mission: components.MissionMeasurement,
		ServicesMap: components.Services{followed.SubPath: followed},
	}
	sys.UAssets[asset.Name] = asset

	payload, err := serviceRegistrationForm(&sys, sys.UAssets[asset.Name], followed, "ServiceRecord_v1")
	if err != nil {
		t.Fatalf("building the registration: %v", err)
	}

	var record forms.ServiceRecord_v1
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatalf("the registration is not readable: %v", err)
	}
	if !record.SubscribeAble {
		t.Error("a service that declares itself subscribable is registered as not " +
			"subscribable, so no consumer will ever follow it")
	}
}

// TestAProviderThatDoesNotPublishIsNotChasedForever separates a fact from a
// fault.
//
// "No provider offers a subscription" describes what is registered; nothing will
// change until discovery says otherwise, so there is nothing to retry. Treating
// it as a transient failure meant a consumer whose provider simply polls
// reconnected for ever and wrote a line in the log each time — which is what the
// testbed showed: a thermostat reporting every half minute, for as long as it
// ran, that a subscription it never had was over.
func TestAProviderThatDoesNotPublishIsNotChasedForever(t *testing.T) {
	attempts := 0
	useTransport(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		return nil, fmt.Errorf("no subscription should have been attempted")
	}))

	cer := &components.Cervice{
		Definition: "temperature",
		Nodes: map[string][]components.NodeInfo{"sensor": {{
			URL: "http://sensor/temperature", SubscribeAble: false, // it only answers reads
			Tokens: map[string]string{"read": ""},
		}}},
	}
	sys := components.NewSystem("thermostat", context.Background())

	for i := 0; i < 5; i++ { // as a control loop would, every cycle
		Follow(cer, &sys)
	}
	time.Sleep(50 * time.Millisecond)

	if attempts != 0 {
		t.Errorf("%d connections were attempted to a provider that does not publish", attempts)
	}
	// And nothing is left holding the claim, so the moment a publishing provider
	// is discovered the next read can follow it.
	if !cer.StartFollowing() {
		t.Error("the cervice is still marked as followed, so a provider that starts " +
			"publishing would never be picked up")
	}
}
