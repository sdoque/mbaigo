package usecases

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

type httpProcessGetRequestStruct struct {
	inputW       http.ResponseWriter
	inputBody    string
	inputF       forms.Form
	expectedBody string
	testName     string
}

type mockResponseWriter struct {
	http.ResponseWriter
}

func (e *mockResponseWriter) Write(b []byte) (int, error) {
	return 0, fmt.Errorf("Forced write error")
}

func (e *mockResponseWriter) WriteHeader(statusCode int) {}

func (e *mockResponseWriter) Header() http.Header {
	return make(http.Header)
}

func createEmptyFormVersion() mockForm {
	form := mockForm{
		XMLName: xml.Name{},
		Value:   0,
		Unit:    "testUnit",
		Version: "",
	}
	return form
}

func createBrokenForm() mockForm {
	form := mockForm{
		XMLName: xml.Name{},
		Value:   complex(1, 2),
		Unit:    "testUnit",
		Version: "SignalA_v1.0",
	}
	return form
}

var httpProcessGetRequestParams = []httpProcessGetRequestStruct{
	{httptest.NewRecorder(), "{\n  \"value\": 0,\n  \"unit\": \"\",\n}", form.NewForm(),
		"{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n " +
			" \"version\": \"SignalA_v1.0\"\n}", "Good case"},
	{httptest.NewRecorder(), "<form><value>0</value><unit></unit></form>", nil,
		"No payload found.\n", "Bad case, form is nil"},
	{httptest.NewRecorder(), "\n", createEmptyFormVersion(),
		"No payload information found.\n", "Bad case, form version is empty"},
	{httptest.NewRecorder(), "", createBrokenForm(),
		"Error packing response.\n", "Bad case, form value is invalid"},
	{&mockResponseWriter{}, "", form.NewForm(),
		"", "Bad case, Write fails"},
}

func TestHTTPProcessGetRequest(t *testing.T) {
	for _, testCase := range httpProcessGetRequestParams {
		inputR := httptest.NewRequest(http.MethodGet, "/test123", io.NopCloser(strings.NewReader(testCase.inputBody)))
		HTTPProcessGetRequest(testCase.inputW, inputR, testCase.inputF)

		if testCase.testName == "Bad case, Write fails" {
			if _, ok := testCase.inputW.(*mockResponseWriter); !ok {
				t.Errorf("Expected inputW to be of type *mockResponseWriter")
			}
		}
		recorder, ok := testCase.inputW.(*httptest.ResponseRecorder)
		if ok {
			if recorder.Body.String() != testCase.expectedBody {
				t.Errorf("Expected %s, got: %s", testCase.expectedBody, recorder.Body.String())
			}
		}
	}
}

type httpProcessSetRequestStruct struct {
	inputW       http.ResponseWriter
	inputBody    string
	expectedErr  bool
	expectedForm forms.SignalA_v1a
	testName     string
}

func createForm() forms.SignalA_v1a {
	form.NewForm()
	form.Value = 0
	return form
}

var httpProcessSetRequestParams = []httpProcessSetRequestStruct{
	{httptest.NewRecorder(), "{\n  \"value\": 0,\n  \"unit\": \"\",\n " +
		" \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}",
		false, createForm(), "Good case"},
	{httptest.NewRecorder(), "\n", true, forms.SignalA_v1a{}, "Bad case, Unmarshal returns error"},
	{httptest.NewRecorder(), "{\n  \"value\": 0,\n  \"unit\": \"\",\n " +
		" \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"bersion\": \"SignalA_v1.0\"\n}",
		true, forms.SignalA_v1a{}, "Bad case, version key missing"},
	{httptest.NewRecorder(), "{\n  \"value\": \"not-a-number\",\n  \"unit\": \"\",\n " +
		" \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}",
		true, forms.SignalA_v1a{}, "Bad case, Second Unmarshal breaks"},
	{httptest.NewRecorder(), "{\n  \"value\": 0,\n  \"unit\": \"\",\n " +
		" \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalB_v1.0\"\n}",
		true, forms.SignalA_v1a{}, "Bad case, version is wrong"},
	{httptest.NewRecorder(), "{\n  \"value\": false,\n " +
		" \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalB_v1.0\"\n}",
		true, forms.SignalA_v1a{}, "Bad case, form version is SignalB_v1a"},
}

func TestHTTPProcessSetRequest(t *testing.T) {
	for _, testCase := range httpProcessSetRequestParams {
		inputR := httptest.NewRequest(http.MethodPut, "/test123", io.NopCloser(strings.NewReader(testCase.inputBody)))
		inputR.Header.Set("Content-Type", "application/json")
		f, err := HTTPProcessSetRequest(testCase.inputW, inputR)

		if f != testCase.expectedForm || (err == nil && testCase.expectedErr == true) ||
			(err != nil && testCase.expectedErr == false) {
			t.Errorf("Expected %v and %v, got: %v and %v", testCase.expectedForm, testCase.expectedErr, f, err)
		}
	}

	// Special case
	specialRequest := httptest.NewRequest(http.MethodPut, "/test123", io.NopCloser(errorReader{}))
	specialRequest.Header.Set("Content-Type", "application/json")
	expectedForm := forms.SignalA_v1a{}
	f, err := HTTPProcessSetRequest(httptest.NewRecorder(), specialRequest)

	if f != expectedForm || err == nil {
		t.Errorf("Expected %v, got: %v", expectedForm, f)
	}
}

type getBestContentTypeStruct struct {
	acceptHeaderInput     string
	bestContentTypeOutput string
	testName              string
}

var getBestContentTypeParams = []getBestContentTypeStruct{
	{"", "application/json",
		"Good case, no accept header provided"},
	{"application/xml", "application/xml",
		"Good case, accept header provided without q-values"},
	{"application/xml;q=0.7, application/json;q=0.9", "application/json",
		"Good case, accept header provided with q-values"},
	{"application/xml;q=wrong, application/json;q=1.1", "application/json",
		"Good case, xml gets skipped"},
	{"application/xml;q=0.9, application/json;q=0.9", "application/xml",
		"Good case, equal q-values selects the first one"},
	{"application/xml;q=-0.9", "application/json",
		"Good case, no MIME type found"},
}

func TestGetBestContentType(t *testing.T) {
	for _, testCase := range getBestContentTypeParams {
		res := getBestContentType(testCase.acceptHeaderInput)

		if res != testCase.bestContentTypeOutput {
			t.Errorf("Expected %v, got: %v in test case: %s", testCase.bestContentTypeOutput, res, testCase.testName)
		}
	}
}

const testMessenger string = "testmessenger"
const testRegMesForm string = `{"version": "MessengerRegistration_v1", "host": "` + testMessenger + `"}`

func TestRegisterMessenger(t *testing.T) {
	table := []struct {
		method         string
		contentType    string
		body           io.ReadCloser
		expectedStatus int
	}{
		// Bad method
		{http.MethodGet, "application/json", nil, http.StatusMethodNotAllowed},
		// Bad body
		{http.MethodPost, "application/json", errReader(0), http.StatusInternalServerError},
		// Bad unpack
		{http.MethodPost, "bad type", nil, http.StatusBadRequest},
		// Bad form
		{http.MethodPost, "application/json",
			io.NopCloser(strings.NewReader(`{"version": "SystemMessage_v1"}`)),
			http.StatusBadRequest,
		},
		// Missing host
		{http.MethodPost, "application/json",
			io.NopCloser(strings.NewReader(`{"version": "MessengerRegistration_v1"}`)),
			http.StatusBadRequest,
		},
		// All good
		// WARN: this case is expected to be the last one in this table, as its
		// result is being used in the special cases!
		{http.MethodPost, "application/json",
			io.NopCloser(strings.NewReader(testRegMesForm)),
			http.StatusOK,
		},
	}

	sys := components.NewSystem("testsys", context.Background())
	testFunc := func(method, content string, body io.ReadCloser) *http.Response {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/msg", body)
		req.Header.Set("Content-Type", content)
		RegisterMessenger(rec, req, &sys)

		return rec.Result()
	}
	for _, test := range table {
		res := testFunc(test.method, test.contentType, test.body)
		if got, want := res.StatusCode, test.expectedStatus; got != want {
			t.Errorf("expected status %d, got %d", want, got)
		}
	}

	// Verify the messenger was registered from the last test case
	errors, found := sys.Messengers[testMessenger]
	if errors != 0 || found == false {
		t.Errorf("expected registered messenger, found none")
	}

	// Verify duplicate registration doesn't lose error count
	errCount := -1
	sys.Messengers[testMessenger] = errCount
	res := testFunc(http.MethodPost, "application/json",
		io.NopCloser(strings.NewReader(testRegMesForm)),
	)
	if got, want := res.StatusCode, http.StatusOK; got != want {
		t.Errorf("expected status %d, got %d", want, got)
	}
	if got, want := sys.Messengers[testMessenger], errCount; got != want {
		t.Errorf("expected error count %d, got %d", want, got)
	}
}
