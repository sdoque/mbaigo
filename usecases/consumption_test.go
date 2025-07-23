package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
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
		if count == 2 || count == 5 {
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
			actual := res.(*forms.SignalA_v1a)
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
			actual := res.(*forms.SignalA_v1a)
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
	t *testing.T
}

func newLogTransportMock(t *testing.T) *logTransportMock {
	lt := &logTransportMock{t}
	http.DefaultClient.Transport = lt
	return lt
}

var logError = fmt.Errorf("mock error")

func (lt *logTransportMock) RoundTrip(req *http.Request) (res *http.Response, err error) {
	b, err := io.ReadAll(req.Body)
	if err != nil {
		lt.t.Errorf("unexpected error while reading request body: %v", err)
		return
	}
	defer req.Body.Close()
	f, err := Unpack(b, req.Header.Get("Content-Type"))
	if err != nil {
		lt.t.Errorf("unexpected error from unpack: %v", err)
		return
	}
	m, ok := f.(*forms.SystemMessage_v1)
	if !ok {
		lt.t.Error("unexpected form")
		return
	}
	if m.System != testLogSys || m.Body != testLogMsg {
		lt.t.Errorf("unexpected message: %v", m)
	}
	err = fmt.Errorf("mock error")
	return
}

const testLogHost = "host"
const testLogSys = "test system"
const testLogMsg = "test msg"

func TestLog(t *testing.T) {
	newLogTransportMock(t)
	s := components.NewSystem(testLogSys, context.Background())
	s.Messengers[testLogHost] = 0
	Log(&s, forms.LevelDebug, testLogMsg)
	if got, want := s.Messengers[testLogHost], 1; got != want {
		t.Errorf("expected error count %d, got %d", want, got)
	}

	s.Messengers[testLogHost] = messengerMaxErrors
	Log(&s, forms.LevelDebug, testLogMsg)
	_, found := s.Messengers[testLogHost]
	if found {
		t.Errorf("expected messenger being removed")
	}
}
