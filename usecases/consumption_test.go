package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

type stateParams struct {
	testCer          *components.Cervice
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
		Nodes:       map[string][]string{"test": {"https://testSystem/testUnitAsset/test"}},
		Protos:      []string{"http"},
	}
}

func newTestCerviceWithoutNodes() *components.Cervice {
	return &components.Cervice{
		IReferentce: "test",
		Definition:  "A test Cervice without nodes",
		Details:     map[string][]string{"Forms": {"SignalA_v1a"}},
		Nodes:       make(map[string][]string),
		Protos:      []string{"http"},
	}
}

func newTestCerviceWithBrokenUrl() *components.Cervice {
	return &components.Cervice{
		IReferentce: "test",
		Definition:  "A test Cervice with nodes",
		Details:     map[string][]string{"Forms": {"SignalA_v1a"}},
		Nodes:       map[string][]string{"test": {brokenUrl}},
		Protos:      []string{"http"},
	}
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
	{newTestCerviceWithNodes(), createTestSystem(false), createTestBytes(),
		createWorkingHttpResp(), 0, nil, form.NewForm(), nil, "No errors with nodes"},
	{newTestCerviceWithoutNodes(), createTestSystem(false), createTestBytes(),
		createDoubleHttpResp(), 0, nil, form.NewForm(), nil, "No errors without nodes"},
	{newTestCerviceWithNodes(), createTestSystem(false), nil,
		createEmptyHttpResp(), 0, nil, nil, errEmptyRespBody, "Empty response body error"},
	{newTestCerviceWithoutNodes(), createTestSystem(false), createTestBytes(),
		createWorkingHttpResp(), 1, errHTTP, nil, errHTTP, "Search4Services error"},
	{newTestCerviceWithBrokenUrl(), createTestSystem(false), createTestBytes(),
		createWorkingHttpResp(), 2, errHTTP, nil, errHTTP, "NewRequest() error"},
	{newTestCerviceWithNodes(), createTestSystem(false), createTestBytes(),
		createStatusErrorHttpResp(), 2, errHTTP, nil, errHTTP, "Status code error"},
	{newTestCerviceWithNodes(), createTestSystem(false), createTestBytes(),
		createErrorReaderHttpResp(), 0, nil, nil, errBodyRead, "io.ReadAll() error"},
	{newTestCerviceWithNodes(), createTestSystem(false), createTestBytes(),
		createUnpackErrorHttpResp(), 0, nil, nil, errUnpack, "Unpack() error"},
	{newTestCerviceWithNodes(), createTestSystem(false), createTestBytes(),
		createWorkingHttpResp(), 1, errHTTP, nil, errHTTP, "DefaultClient.Do() error"},
}

func TestGetState(t *testing.T) {
	for _, test := range testStateParams {
		newMockTransport(test.body, test.mockTransportErr, test.errHTTP)
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
		newMockTransport(test.body, test.mockTransportErr, test.errHTTP)

		if test.testCase == "DefaultClient.Do() error" {
			test.testCer.Nodes = map[string][]string{"test": {"https://testSystem/testUnitAsset/test"}}
		}
		if test.testCase == "No errors without nodes" {
			test.testCer.Nodes = make(map[string][]string)
		}
		res, err := SetState(test.testCer, &test.testSys, test.bodyBytes)

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
		newMockTransport(testCase.body, testCase.mockTransportErr, testCase.errHTTP)

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
		Nodes:       map[string][]string{"test": {"test1", "test2", "test3"}},
		Protos:      []string{"http"},
	}
	testSys := createTestSystem(false)
	newMockTransport(createWorkingHttpResp(), 0, nil)

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
		Nodes:       map[string][]string{"test": {"test1", brokenUrl, "test3"}},
		Protos:      []string{"http"},
	}
	testSys = createTestSystem(false)
	newMockTransport(createWorkingHttpResp(), 0, nil)

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
	http.DefaultClient.Transport = lt
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
