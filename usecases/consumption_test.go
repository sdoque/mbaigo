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
	sys.Messengers[testLogHost] = 0
	Log(&sys, forms.LevelDebug, testLogMsg)
	if got, want := sys.Messengers[testLogHost], 1; got != want {
		t.Errorf("expected error count %d, got %d", want, got)
	}

	// Case: removes messenger after too many errors
	sys.Messengers[testLogHost] = messengerMaxErrors
	Log(&sys, forms.LevelDebug, testLogMsg)
	_, found := sys.Messengers[testLogHost]
	if found {
		t.Errorf("expected messenger being removed")
	}

	// Case: transfer ok
	mock.setError(nil)
	sys.Messengers[testLogHost] = 0
	Log(&sys, forms.LevelDebug, testLogMsg)
	if got, want := sys.Messengers[testLogHost], 0; got != want {
		t.Errorf("expected error count %d, got %d", want, got)
	}
}
