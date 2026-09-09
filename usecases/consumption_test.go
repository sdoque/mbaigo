package usecases

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

type stateParams struct {
	testCer          *components.Cervice
	setCer           *components.Cervice // if non-nil, used instead of testCer in TestSetState
	testSys          components.System
	bodyBytes        []byte
	body             func() *http.Response
	mockTransportErr int
	errHTTP          error
	expectedfForm    forms.Form
	expectedErr      error
	testCase         string
}

func newTestCerviceWithNodes() *components.Cervice {
	return &components.Cervice{
		IReferentce: "test",
		Definition:  "A test Cervice with nodes",
		Details:     map[string][]string{"Forms": {"SignalA_v1a"}},
		// Discovered for both, since this fixture serves the GET and the PUT
		// suites alike. A cervice used for both actions holds a token for each.
		Nodes: map[string][]components.NodeInfo{"test": {{
			URL:    "https://testSystem/testUnitAsset/test",
			Tokens: map[string]string{"read": "", "write": ""},
		}}},
		Protos: []string{"http"},
	}
}

func newTestCerviceWithoutNodes() *components.Cervice {
	return &components.Cervice{
		IReferentce: "test",
		Definition:  "A test Cervice without nodes",
		Details:     map[string][]string{"Forms": {"SignalA_v1a"}},
		Nodes:       make(map[string][]components.NodeInfo),
		Protos:      []string{"http"},
	}
}

func newTestCerviceWithBrokenUrl() *components.Cervice {
	return &components.Cervice{
		IReferentce: "test",
		Definition:  "A test Cervice with nodes",
		Details:     map[string][]string{"Forms": {"SignalA_v1a"}},
		Nodes:       map[string][]components.NodeInfo{"test": {readNode(brokenUrl)}},
		Protos:      []string{"http"},
	}
}

// readNode is a node already discovered for a read, which is what a consumer
// holds after a GET-mode search. The action key has to be there: a node with no
// token for the action being performed is one that has not been discovered for
// it, and the consumer re-orchestrates rather than present the wrong token.
func readNode(url string) components.NodeInfo {
	return components.NodeInfo{URL: url, Tokens: map[string]string{"read": ""}}
}

var form forms.SignalA_v1a

var errEmptyRespBody = errors.New("got empty response body")

var errUnpack = errors.New("problem unpacking response body")

func createTestBytes() []byte {
	return []byte("{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": " +
		"\"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}")
}

func createWorkingHttpResp() func() *http.Response {
	httpResp := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n " +
				" \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"))),
		}
	}
	return httpResp
}

// This function creates two different http responses with a different body,
// since some tests build on receiving multiple correct http responses
func createDoubleHttpResp() func() *http.Response {
	f := createServicePointTestForm()
	// Create mock response from orchestrator
	fakeBody, err := json.Marshal(f)
	if err != nil {
		log.Println("Fail Marshal at start of test")
	}
	count := 0
	return func() *http.Response {
		count++
		if count == 1 || count == 3 {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string(fakeBody))),
			}
		}
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n " +
				" \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"))),
		}
	}
}

func createEmptyHttpResp() func() *http.Response {
	httpResp := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(""))),
		}
	}
	return httpResp
}

func createStatusErrorHttpResp() func() *http.Response {
	httpResp := func() *http.Response {
		return &http.Response{
			Status:     "300 NAK",
			StatusCode: 300,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n " +
				" \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"))),
		}
	}
	return httpResp
}

func createErrorReaderHttpResp() func() *http.Response {
	httpResp := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(errorReader{}),
		}
	}
	return httpResp
}

func createUnpackErrorHttpResp() func() *http.Response {
	httpResp := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"Wrong content type"}},
			Body: io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n " +
				" \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"))),
		}
	}
	return httpResp
}

var testStateParams = []stateParams{
	{testCer: newTestCerviceWithNodes(), testSys: createTestSystem(false), bodyBytes: createTestBytes(),
		body: createWorkingHttpResp(), mockTransportErr: 0, errHTTP: nil, expectedfForm: form.NewForm(), expectedErr: nil, testCase: "No errors with nodes"},
	{testCer: newTestCerviceWithoutNodes(), setCer: newTestCerviceWithoutNodes(), testSys: createTestSystem(false), bodyBytes: createTestBytes(),
		body: createDoubleHttpResp(), mockTransportErr: 0, errHTTP: nil, expectedfForm: form.NewForm(), expectedErr: nil, testCase: "No errors without nodes"},
	{testCer: newTestCerviceWithNodes(), testSys: createTestSystem(false), bodyBytes: nil,
		body: createEmptyHttpResp(), mockTransportErr: 0, errHTTP: nil, expectedfForm: nil, expectedErr: errEmptyRespBody, testCase: "Empty response body error"},
	{testCer: newTestCerviceWithoutNodes(), testSys: createTestSystem(false), bodyBytes: createTestBytes(),
		body: createWorkingHttpResp(), mockTransportErr: 1, errHTTP: errHTTP, expectedfForm: nil, expectedErr: errHTTP, testCase: "Search4Services error"},
	{testCer: newTestCerviceWithBrokenUrl(), testSys: createTestSystem(false), bodyBytes: createTestBytes(),
		body: createWorkingHttpResp(), mockTransportErr: 2, errHTTP: errHTTP, expectedfForm: nil, expectedErr: errHTTP, testCase: "NewRequest() error"},
	{testCer: newTestCerviceWithNodes(), testSys: createTestSystem(false), bodyBytes: createTestBytes(),
		body: createStatusErrorHttpResp(), mockTransportErr: 2, errHTTP: errHTTP, expectedfForm: nil, expectedErr: errHTTP, testCase: "Status code error"},
	{testCer: newTestCerviceWithNodes(), testSys: createTestSystem(false), bodyBytes: createTestBytes(),
		body: createErrorReaderHttpResp(), mockTransportErr: 0, errHTTP: nil, expectedfForm: nil, expectedErr: errBodyRead, testCase: "io.ReadAll() error"},
	{testCer: newTestCerviceWithNodes(), testSys: createTestSystem(false), bodyBytes: createTestBytes(),
		body: createUnpackErrorHttpResp(), mockTransportErr: 0, errHTTP: nil, expectedfForm: nil, expectedErr: errUnpack, testCase: "Unpack() error"},
	{testCer: newTestCerviceWithNodes(), setCer: newTestCerviceWithNodes(), testSys: createTestSystem(false), bodyBytes: createTestBytes(),
		body: createWorkingHttpResp(), mockTransportErr: 1, errHTTP: errHTTP, expectedfForm: nil, expectedErr: errHTTP, testCase: "DefaultClient.Do() error"},
}

func TestGetState(t *testing.T) {
	for _, test := range testStateParams {
		newMockTransport(t, test.body, test.mockTransportErr, test.errHTTP)
		res, err := GetState(test.testCer, &test.testSys)

		if test.expectedfForm != nil {
			expected := test.expectedfForm.(*forms.SignalA_v1a)
			actual, ok := res.(*forms.SignalA_v1a)
			if !ok {
				t.Fatalf("Test case: %s, got %v, expected a forms.Form",
					test.testCase, res,
				)
			}
			if expected.Value != actual.Value || expected.Unit != actual.Unit ||
				expected.Timestamp != actual.Timestamp || expected.Version != actual.Version ||
				err != test.expectedErr {
				t.Errorf("Test case: %s got error: %v. \nExpected form: \n%+v\n, got: \n%+v",
					test.testCase, err, expected, actual)
			}
		} else if err == nil {
			t.Errorf("Test case: %s got error: %v:", test.testCase, err)
		}
	}
}

func TestSetState(t *testing.T) {
	for _, test := range testStateParams {
		newMockTransport(t, test.body, test.mockTransportErr, test.errHTTP)

		cer := test.testCer
		if test.setCer != nil {
			cer = test.setCer
		}
		res, err := SetState(cer, &test.testSys, test.bodyBytes)

		if test.expectedfForm != nil {
			expected := test.expectedfForm.(*forms.SignalA_v1a)
			actual, ok := res.(*forms.SignalA_v1a)
			if !ok {
				t.Fatalf("Test case: %s, got %v, expected a forms.Form",
					test.testCase, res,
				)
			}
			if expected.Value != actual.Value || expected.Unit != actual.Unit ||
				expected.Timestamp != actual.Timestamp || expected.Version != actual.Version ||
				err != test.expectedErr {
				t.Errorf("Test case: %s got error: %v. \nExpected form: \n%+v\n, got: \n%+v",
					test.testCase, err, expected, actual)
			}
		} else if err == nil {
			t.Errorf("Test case: %s got error: %v:", test.testCase, err)
		}
	}
}

func createServRecListTestForm(amount int) (servRecList forms.ServiceRecordList_v1) {
	servRecList.NewForm()
	servRecList.List = make([]forms.ServiceRecord_v1, amount)
	for i := range amount {
		servRecList.List[i].IPAddresses = []string{"123.456.789"}
		servRecList.List[i].ProtoPort = map[string]int{"http": 123}
		// Distinct providers, not the same one repeated. Every record used to
		// carry identical fields, so all three resolved to one ServLocation and
		// the list exercised duplication rather than the multiple providers it
		// is named for.
		servRecList.List[i].SystemName = fmt.Sprintf("provider%d", i)
	}
	return servRecList
}

// Use this one if a mock response from an orchestrator is needed
func createDoubleHttpRespWithServRecList(amount int, empty bool, statusErr bool,
	readErr bool, unpackErr bool) func() *http.Response {
	f := createServRecListTestForm(amount)
	// Create mock response from orchestrator
	fakeBody, err := json.Marshal(f)
	if err != nil {
		log.Println("Fail Marshal at start of test")
	}
	count := 0
	return func() *http.Response {
		resp := &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n " +
				" \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"))),
		}
		count++
		if count == 1 {
			resp.Body = io.NopCloser(strings.NewReader(string(fakeBody)))
			return resp
		}
		if empty == true {
			resp.Body = io.NopCloser(strings.NewReader(string("")))
			return resp
		}
		if statusErr == true {
			resp.Status = "300 NAK"
			resp.StatusCode = 300
			return resp
		}
		if readErr == true {
			resp.Body = io.NopCloser(errorReader{})
			return resp
		}
		if unpackErr == true {
			resp.Header = http.Header{"Content-Type": []string{"Wrong content type"}}
			return resp
		}
		return resp
	}
}

func formsEqual(a, b []forms.Form) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] == nil && b[i] == nil {
			continue
		}
		aForm, ok := a[i].(*forms.SignalA_v1a)
		if !ok {
			return false
		}
		bForm, ok := b[i].(*forms.SignalA_v1a)
		if !ok {
			return false
		}
		if aForm.Value != bForm.Value || aForm.Unit != bForm.Unit ||
			aForm.Timestamp != bForm.Timestamp || aForm.Version != bForm.Version {
			return false
		}
	}
	return true
}

func errEqual(a, b []error) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if (a[i] != nil && b[i] == nil) || (a[i] == nil && b[i] != nil) {
			return false
		}
	}
	return true
}

type getStatesTestStruct struct {
	body             func() *http.Response
	mockTransportErr int
	errHTTP          error
	expectedForm     []forms.Form
	expectedErr      []error
	testName         string
}

var (
	threeForms    = []forms.Form{form.NewForm(), form.NewForm(), form.NewForm()}
	oneNilForm    = []forms.Form{form.NewForm(), form.NewForm(), nil}
	nilForms      = []forms.Form{nil, nil, nil}
	singleNilForm = []forms.Form{nil}
	threeErr      = []error{fmt.Errorf("Error"), fmt.Errorf("Error"), fmt.Errorf("Error")}
	oneErr        = []error{nil, nil, fmt.Errorf("Error")}
	nilErr        = []error{nil, nil, nil}
	singleErr     = []error{fmt.Errorf("Error")}
)

var getStatesTestParams = []getStatesTestStruct{
	{createDoubleHttpRespWithServRecList(3, false, false, false, false), 0, nil, threeForms,
		nilErr, "No errors without nodes"},
	{createDoubleHttpRespWithServRecList(3, false, false, false, false), 4, errHTTP, oneNilForm,
		oneErr, "Error in one of the services"},
	{createDoubleHttpRespWithServRecList(3, true, false, false, false), 0, nil, nilForms,
		threeErr, "Empty response body error"},
	{createWorkingHttpResp(), 1, errHTTP, singleNilForm,
		singleErr, "Search4Services error"},
	{createDoubleHttpRespWithServRecList(3, false, true, false, false), 0, nil, nilForms,
		threeErr, "Status code error"},
	{createDoubleHttpRespWithServRecList(3, false, false, true, false), 0, nil, nilForms,
		threeErr, "io.ReadAll() error"},
	{createDoubleHttpRespWithServRecList(3, false, false, false, true), 0, nil, nilForms,
		threeErr, "Unpack() error"},
}

func TestGetStates(t *testing.T) {
	for _, testCase := range getStatesTestParams {
		testCer := newTestCerviceWithoutNodes()
		testSys := createTestSystem(false)
		newMockTransport(t, testCase.body, testCase.mockTransportErr, testCase.errHTTP)

		res, err := GetStates(testCer, &testSys)

		if !formsEqual(res, testCase.expectedForm) || !errEqual(err, testCase.expectedErr) {
			t.Errorf("Test case: %s\nExpected forms: %+v\nGot: %+v\nExpected error: %v, Got error: %v",
				testCase.testName, testCase.expectedForm, res, testCase.expectedErr, err)
		}
	}

	// Special case: No errors with existing nodes
	cerWithNodes := components.Cervice{
		IReferentce: "test",
		Definition:  "A test Cervice with nodes",
		Details:     map[string][]string{"Forms": {"SignalA_v1a"}},
		Nodes:       map[string][]components.NodeInfo{"test": {readNode("test1"), readNode("test2"), readNode("test3")}},
		Protos:      []string{"http"},
	}
	testSys := createTestSystem(false)
	newMockTransport(t, createWorkingHttpResp(), 0, nil)

	res, err := GetStates(&cerWithNodes, &testSys)
	expectedForm := []forms.Form{form.NewForm(), form.NewForm(), form.NewForm()}
	expectedErr := []error{nil, nil, nil}

	if !formsEqual(res, expectedForm) || !errEqual(err, expectedErr) {
		t.Errorf("Test case: No errors with nodes \nExpected forms: %v\nGot: %v\nExpected error: %v, Got error: %v",
			expectedForm, res, expectedErr, err)
	}

	// Special case: Error with a broken url in nodes
	cerWithBrokenUrlNode := components.Cervice{
		IReferentce: "test",
		Definition:  "A test Cervice with nodes",
		Details:     map[string][]string{"Forms": {"SignalA_v1a"}},
		Nodes:       map[string][]components.NodeInfo{"test": {readNode("test1"), readNode(brokenUrl), readNode("test3")}},
		Protos:      []string{"http"},
	}
	testSys = createTestSystem(false)
	newMockTransport(t, createWorkingHttpResp(), 0, nil)

	res, err = GetStates(&cerWithBrokenUrlNode, &testSys)
	expectedForm = []forms.Form{form.NewForm(), nil, form.NewForm()}
	expectedErr = []error{nil, fmt.Errorf("Error"), nil}

	if !formsEqual(res, expectedForm) || !errEqual(err, expectedErr) {
		t.Errorf("Test case: Error with broken url \nExpected forms: %v\nGot: %v\nExpected error: %v, Got error: %v",
			expectedForm, res, expectedErr, err)
	}
}

type logTransportMock struct {
	t           *testing.T
	errResponse error
}

func newLogTransportMock(t *testing.T) *logTransportMock {
	lt := &logTransportMock{t, nil}
	useTransport(t, lt)
	return lt
}

func (mock *logTransportMock) setError(err error) {
	mock.errResponse = err
}

// This mock transport also verifies that the system message forms are valid.
func (mock *logTransportMock) RoundTrip(req *http.Request) (res *http.Response, err error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		mock.t.Errorf("unexpected error while reading request body: %v", err)
		return
	}
	defer req.Body.Close()
	form, err := Unpack(body, req.Header.Get("Content-Type"))
	if err != nil {
		mock.t.Errorf("unexpected error from unpack: %v", err)
		return
	}
	message, ok := form.(*forms.SystemMessage_v1)
	if !ok {
		mock.t.Error("unexpected form")
		return
	}
	if message.System != testLogSys || message.Body != testLogMsg {
		mock.t.Errorf("unexpected message: %v", message)
	}

	if mock.errResponse != nil {
		return nil, mock.errResponse
	}
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusOK)
	return rec.Result(), nil
}

const testLogHost = "host"
const testLogSys = "test system"
const testLogMsg = "test msg"

// NOTE: this test also covers sendLogMessage function

func TestLog(t *testing.T) {
	mock := newLogTransportMock(t)
	mock.setError(fmt.Errorf("mock err"))
	sys := components.NewSystem(testLogSys, context.Background())
	sys.Husk = &components.Husk{
		Messengers: make(map[string]int),
	}

	// Case: increase error count by one
	sys.Husk.Messengers[testLogHost] = 0
	Log(&sys, forms.LevelDebug, testLogMsg)
	if got, want := sys.Husk.Messengers[testLogHost], 1; got != want {
		t.Errorf("expected error count %d, got %d", want, got)
	}

	// Case: removes messenger after too many errors
	sys.Husk.Messengers[testLogHost] = messengerMaxErrors
	Log(&sys, forms.LevelDebug, testLogMsg)
	_, found := sys.Husk.Messengers[testLogHost]
	if found {
		t.Errorf("expected messenger being removed")
	}

	// Case: transfer ok
	mock.setError(nil)
	sys.Husk.Messengers[testLogHost] = 0
	Log(&sys, forms.LevelDebug, testLogMsg)
	if got, want := sys.Husk.Messengers[testLogHost], 0; got != want {
		t.Errorf("expected error count %d, got %d", want, got)
	}
}

// TestAnExpiredTokenDoesNotRebindTheConsumer is the regression for three
// thermostats that spent an afternoon heating a cottage against the outdoor
// temperature.
//
// A consumer that has chosen a provider — a thermostat paired to the thermometer
// in its own room — asks the orchestrator only for a new credential when its
// token expires. The orchestrator cannot know which provider was meant, and
// answers with whichever candidate it likes. Taking that answer silently moves
// the consumer to a different thing, and it goes on controlling: plausibly,
// continuously, and against the wrong measurement.
func TestAnExpiredTokenDoesNotRebindTheConsumer(t *testing.T) {
	// The provider this cervice is bound to. It refuses with an expired token,
	// which is an answer: it is exactly where the consumer thinks it is.
	bound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "the token expired at 2026-08-24T13:11:06+02:00", http.StatusForbidden)
	}))
	defer bound.Close()

	// Another provider of the same service definition — the outdoor sensor.
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer other.Close()

	// An orchestrator that always names the other one.
	orchestrator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var sp forms.ServicePoint_v1
		sp.NewForm()
		sp.ServiceDefinition = "temperature"
		sp.ProviderName = "meteorologue"
		sp.ServNode = "OutdoorModule"
		sp.ServLocation = other.URL
		sp.Token = "a-fresh-token-for-the-wrong-sensor"
		body, _ := Pack(&sp, "application/json")
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer orchestrator.Close()

	sys := components.NewSystem("ethermostat", context.Background())
	sys.Husk = &components.Husk{
		ProtoPort: map[string]int{"http": 20196, "https": 0, "coap": 0},
		CoreS: []*components.CoreSystem{
			{Name: "orchestrator", Url: orchestrator.URL + "/orchestrator/orchestration"},
		},
	}

	cer := &components.Cervice{
		Definition: "temperature",
		Protos:     []string{"http"},
		Mode:       "get",
		Nodes: map[string][]components.NodeInfo{
			"IndoorModule": {{
				URL:    bound.URL,
				Tokens: map[string]string{"read": "an-expired-token"},
			}},
		},
	}

	// First call: refused with an expired token. The binding must survive.
	if _, err := GetState(cer, &sys); err == nil {
		t.Fatal("a 403 was reported as success")
	}
	if urls := knownURLs(cer); !urls[bound.URL] {
		t.Fatal("the provider was forgotten because it answered with an error")
	}

	// Second call: the refresh path runs, the orchestrator names the wrong
	// provider, and that answer must be refused rather than adopted.
	_, err := GetState(cer, &sys)
	if err == nil {
		t.Fatal("the consumer was silently re-bound and reported success")
	}

	urls := knownURLs(cer)
	if urls[other.URL] {
		t.Errorf("the cervice was re-bound to %s — a different provider of the same service", other.URL)
	}
	if !urls[bound.URL] {
		t.Errorf("the original binding was lost; cervice now holds %v", urls)
	}
}

// TestAnUnreachableProviderIsForgotten keeps the fix from becoming a cervice
// that can never move. A provider that does not answer at all may genuinely be
// gone, and that is the one path where re-discovery should bind freely.
func TestAnUnreachableProviderIsForgotten(t *testing.T) {
	cer := &components.Cervice{
		Definition: "temperature",
		Protos:     []string{"http"},
		Mode:       "get",
		Nodes: map[string][]components.NodeInfo{
			// A port nothing listens on: a transport failure, not an answer.
			"gone": {{URL: "http://127.0.0.1:1", Tokens: map[string]string{"read": "t"}}},
		},
	}
	sys := components.NewSystem("ethermostat", context.Background())
	sys.Husk = &components.Husk{ProtoPort: map[string]int{"http": 20196}}

	if _, err := GetState(cer, &sys); err == nil {
		t.Fatal("an unreachable provider was reported as success")
	}
	if len(knownURLs(cer)) != 0 {
		t.Error("an unreachable provider was kept; the next discovery cannot rebind")
	}
}

// TestSharedTokensAreNotWrittenInPlace reproduces a crash that took the
// cottage's heating down at 17:52 on 24 August.
//
// A NodeInfo is copied by value when a consumer pins a provider into a cervice
// of its own — ethermostat builds one cervice per heater — and copying the
// struct copies the map header, not the map. Two of the cottage's heaters read
// the same thermometer, so two feedback loops held two different cervice mutexes
// over one shared token map. Deleting from it concurrently is "fatal error:
// concurrent map writes", which is not recoverable: the whole system dies,
// control loop and servers together.
//
// Run with -race to see the read side too.
func TestSharedTokensAreNotWrittenInPlace(t *testing.T) {
	// One provider, pinned into two cervices exactly as discoverHeaters does.
	shared := components.NodeInfo{
		URL:    "http://sensor.example/temperature",
		Tokens: map[string]string{"read": "a-token", "write": "another"},
	}
	kitchen := &components.Cervice{
		Definition: "temperature",
		Nodes:      map[string][]components.NodeInfo{"IndoorModule": {shared}},
	}
	diningroom := &components.Cervice{
		Definition: "temperature",
		Nodes:      map[string][]components.NodeInfo{"IndoorModule": {shared}},
	}

	var wg sync.WaitGroup
	for _, cer := range []*components.Cervice{kitchen, diningroom} {
		wg.Add(1)
		go func(c *components.Cervice) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				forgetToken(c, shared.URL, "read")
				recordNode(c, "IndoorModule", shared.URL, nil, "read", "fresh", false)
			}
		}(cer)
	}
	wg.Wait()

	// The original map must be untouched: nothing may edit a map it shares.
	if shared.Tokens["read"] != "a-token" {
		t.Errorf("the shared token map was written in place: read = %q", shared.Tokens["read"])
	}
	if shared.Tokens["write"] != "another" {
		t.Errorf("an unrelated action was disturbed: write = %q", shared.Tokens["write"])
	}
}

// TestATokenIsRenewedBeforeItLapses covers the change from renewing by being
// refused to renewing ahead of expiry.
//
// The old design took a 403 as its signal, which works and costs one guaranteed
// refusal per binding per token lifetime — plus the reading that request was
// carrying. Forty bindings on a five-minute lifetime is a refusal every few
// seconds across a cloud, and the lost readings drew flat lines across a chart
// for five hours that looked exactly like a frozen sensor.
func TestATokenIsRenewedBeforeItLapses(t *testing.T) {
	issued := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	mint := func(life time.Duration) string {
		var claims forms.AccessToken_v1
		claims.NewForm()
		claims.Subject, claims.Action = "ethermostat", "read"
		claims.IssuedAt, claims.Expires = issued, issued.Add(life)
		payload, err := json.Marshal(claims)
		if err != nil {
			t.Fatalf("minting: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(payload) + ".c2ln"
	}

	token := mint(5 * time.Minute) // expires 08:05, renewal margin is the last minute
	cases := []struct {
		at   time.Time
		want bool
		why  string
	}{
		{issued, false, "freshly minted"},
		{issued.Add(3 * time.Minute), false, "well inside its life"},
		{issued.Add(4*time.Minute + 30*time.Second), true, "inside the final fifth"},
		{issued.Add(6 * time.Minute), true, "already lapsed"},
	}
	for _, tc := range cases {
		if got := dueForRenewal(token, tc.at); got != tc.want {
			t.Errorf("%s: dueForRenewal = %t, want %t", tc.why, got, tc.want)
		}
	}
}

// TestAnUnreadableTokenIsPresentedNotRenewed is the correction to a first
// attempt at this. Marking a token this code cannot parse as "due" looks
// cautious and is the same storm from the other side: it re-orchestrates before
// every call, for ever, because the next token is no more readable than the
// last. Renewal is an optimisation over being refused, so anything it cannot
// reason about is handed to the provider, which decides anyway.
func TestAnUnreadableTokenIsPresentedNotRenewed(t *testing.T) {
	for _, token := range []string{"", "not-a-token", "bm90anNvbg.c2ln"} {
		if dueForRenewal(token, time.Now()) {
			t.Errorf("%q would be renewed before every call", token)
		}
	}
}

// TestARenewalDoesNotDeadlockWhenTheOrchestratorPrefersAnother is the
// regression for a dining room that lost its thermometer and could not get it
// back.
//
// A bound cervice renewing its token must end up holding the provider it
// started with. Asking the singular quest cannot guarantee that: the
// orchestrator answers with whichever candidate it prefers, the stranger is
// discarded, and the cervice is left bound to a provider with no token. Nothing
// recovers it, because a provider that is up never produces the transport
// failure that would permit re-binding — so the consumer repeats "no read token
// for the provider this cervice is bound to" every tick, for ever.
func TestARenewalDoesNotDeadlockWhenTheOrchestratorPrefersAnother(t *testing.T) {
	const ours = "http://indoor.example/temperature"
	const theirs = "http://outdoor.example/temperature"

	// An orchestrator that prefers the other provider, and lists both when
	// asked for everything — which is what a real one does.
	orchestrator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/squests") {
			fmt.Fprintf(w, `{"list":[
			  {"serviceURL":%q,"serviceNode":"OutdoorModule","token":"tok-outdoor","version":"ServicePoint_v1"},
			  {"serviceURL":%q,"serviceNode":"IndoorModule","token":"tok-indoor","version":"ServicePoint_v1"}
			],"version":"ServicePointList_v1"}`, theirs, ours)
			return
		}
		fmt.Fprintf(w, `{"serviceURL":%q,"serviceNode":"OutdoorModule","token":"tok-outdoor","version":"ServicePoint_v1"}`, theirs)
	}))
	defer orchestrator.Close()

	sys := components.NewSystem("ethermostat", context.Background())
	sys.Husk = &components.Husk{
		ProtoPort: map[string]int{"http": 20196},
		CoreS: []*components.CoreSystem{
			{Name: "orchestrator", Url: orchestrator.URL + "/orchestrator/orchestration"},
		},
	}

	// Bound to the indoor sensor, with no token for this action — the state a
	// consumer is in the moment after a stale credential is dropped.
	cer := &components.Cervice{
		Definition: "temperature",
		Protos:     []string{"http"},
		Mode:       "get",
		Nodes: map[string][]components.NodeInfo{
			"IndoorModule": {{URL: ours, Tokens: map[string]string{}}},
		},
	}

	url, token, err := resolveProvider(cer, &sys, "read")
	if err != nil {
		t.Fatalf("a bound cervice could not renew: %v", err)
	}
	if url != ours {
		t.Errorf("renewed onto %s; the cervice is bound to %s", url, ours)
	}
	if token != "tok-indoor" {
		t.Errorf("token %q; want the one minted for the bound provider", token)
	}
	if knownURLs(cer)[theirs] {
		t.Error("the renewal widened the binding to another provider")
	}
}

// A 404 means the provider is reachable but no longer has what was asked for —
// a renamed asset, a withdrawn service, a different system on the same address.
// The binding has to be dropped so the next call re-discovers, or the consumer
// asks a dead URL for ever. Found on the lab cloud by renaming a sensor's unit
// asset under a running thermostat: it logged 404s and said it would keep
// asking "until it resumes", and only a restart fixed it.
func TestA404ClearsTheBindingSoDiscoveryRuns(t *testing.T) {
	gone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Resource not found", http.StatusNotFound)
	}))
	defer gone.Close()
	sys := components.NewSystem("thermostat", context.Background())
	sys.Husk = &components.Husk{ProtoPort: map[string]int{"http": 20152, "https": 0, "coap": 0}}
	// The node carries a token, or resolveProvider goes to the orchestrator for
	// one and the provider is never contacted at all — which is how the first
	// version of this test passed without exercising anything.
	cer := &components.Cervice{
		Definition: "temperature", Protos: []string{"http"}, Mode: "get",
		Nodes: map[string][]components.NodeInfo{"sensor": {{
			URL: gone.URL, Tokens: map[string]string{"read": "a-valid-token"},
		}}},
	}

	if _, err := GetState(cer, &sys); err == nil {
		t.Fatal("a 404 was reported as success")
	}
	if urls := knownURLs(cer); urls[gone.URL] {
		t.Error("the binding survived a 404; the next call would ask the same dead URL for ever")
	}
}

// The opposite case, and the reason this is not simply "any error re-binds". A
// provider answering 503 is present and temporarily unable — a rangefinder that
// cannot see, a sensor with no reading yet. Dropping that binding would re-run
// discovery on every blind cycle, and could re-bind to a different provider of
// the same definition that happens to be answering.
func TestA503KeepsTheBinding(t *testing.T) {
	blind := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no valid reading yet", http.StatusServiceUnavailable)
	}))
	defer blind.Close()

	sys := components.NewSystem("thermostat", context.Background())
	sys.Husk = &components.Husk{ProtoPort: map[string]int{"http": 20152, "https": 0, "coap": 0}}
	cer := &components.Cervice{
		Definition: "temperature", Protos: []string{"http"}, Mode: "get",
		Nodes: map[string][]components.NodeInfo{"sensor": {{
			URL: blind.URL, Tokens: map[string]string{"read": "a-valid-token"},
		}}},
	}

	if _, err := GetState(cer, &sys); err == nil {
		t.Fatal("a 503 was reported as success")
	}
	if urls := knownURLs(cer); !urls[blind.URL] {
		t.Error("the binding was dropped on a 503; a provider that is present but blind must stay bound")
	}
}

func TestVanishedCoversOnlyGoneResources(t *testing.T) {
	for code, want := range map[int]bool{
		http.StatusNotFound:            true,
		http.StatusGone:                true,
		http.StatusServiceUnavailable:  false,
		http.StatusUnauthorized:        false,
		http.StatusForbidden:           false,
		http.StatusInternalServerError: false,
		http.StatusBadRequest:          false,
	} {
		if got := (&ProviderRefusal{StatusCode: code}).Vanished(); got != want {
			t.Errorf("Vanished(%d) = %v, want %v", code, got, want)
		}
	}
}
