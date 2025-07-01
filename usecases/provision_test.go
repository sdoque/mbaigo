package usecases

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/forms"
)

type httpProcessGetRequestStruct struct {
	inputW       http.ResponseWriter
	inputR       *http.Request
	inputF       forms.Form
	expectedBody string
	testName     string
}

type wrongVersionForm struct {
	XMLName xml.Name  `json:"-" xml:"testName"`
	Value   complex64 `json:"value" xml:"value"`
	Unit    string    `json:"unit" xml:"unit"`
	Version string    `json:"version" xml:"version"`
}

type brokenTestValueForm struct {
	XMLName xml.Name  `json:"-" xml:"testName"`
	Value   complex64 `json:"value" xml:"value"`
	Unit    string    `json:"unit" xml:"unit"`
	Version string    `json:"version" xml:"version"`
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

func createNewBodyString() string {
	return "{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"
}

func (f wrongVersionForm) NewForm() forms.Form {
	f.Version = ""
	return f
}

func (f wrongVersionForm) FormVersion() string {
	return f.Version
}

func createEmptyFormVersion() wrongVersionForm {
	form := wrongVersionForm{
		XMLName: xml.Name{},
		Value:   0,
		Unit:    "testUnit",
		Version: "",
	}
	return form
}

func (f brokenTestValueForm) NewForm() forms.Form {
	f.Version = "testVersion"
	return f
}

func (f brokenTestValueForm) FormVersion() string {
	return f.Version
}

func createBrokenForm() brokenTestValueForm {
	form := brokenTestValueForm{
		XMLName: xml.Name{},
		Value:   complex(1, 2),
		Unit:    "testUnit",
		Version: "SignalA_v1.0",
	}
	return form
}

var mockError error = fmt.Errorf("A mock error")

var httpProcessGetRequestParams = []httpProcessGetRequestStruct{
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test123", nil), form.NewForm(), createNewBodyString(), "Good case"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test123", nil), nil, "No payload found.\n", "Bad case, form is nil"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test123", nil), createEmptyFormVersion(), "No payload information found.\n", "Bad case, form version is empty"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test123", nil), createBrokenForm(), "Error packing response: error encoding JSON: json: unsupported type: complex64\n", "Bad case, form value is invalid"},
	{&mockResponseWriter{}, httptest.NewRequest(http.MethodGet, "/test123", nil), form.NewForm(), "", "Bad case, Write fails"},
}

func TestHTTPProcessGetRequest(t *testing.T) {
	for _, testCase := range httpProcessGetRequestParams {
		HTTPProcessGetRequest(testCase.inputW, testCase.inputR, testCase.inputF)

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
	inputR       *http.Request
	expectedErr  bool
	expectedForm forms.SignalA_v1a
	testName     string
}

func createForm() forms.SignalA_v1a {
	form.NewForm()
	form.Value = 0
	return form
}

func createBody() io.ReadCloser {
	return io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}")))
}

func createBrokenBody() io.ReadCloser {
	return io.NopCloser(strings.NewReader(string([]byte{0})))
}

func createBodyWithNoformVersion() io.ReadCloser {
	return io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"bersion\": \"SignalA_v1.0\"\n}")))
}

func createBodyWithWrongForm() io.ReadCloser {
	return io.NopCloser(strings.NewReader(string("{\n  \"value\": \"not-a-number\",\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}")))
}

func createBodyWithWrongFormVersion() io.ReadCloser {
	return io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalB_v1.0\"\n}")))
}

func createBodyWithSignalBForm() io.ReadCloser {
	return io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalB_v1.0\"\n}")))
}

var errReadAll error = fmt.Errorf("forced read error")

var errUnmarshal error = fmt.Errorf("invalid character '\\x00' looking for beginning of value")

var errVersion error = fmt.Errorf("unsupported service set request form version")

var httpProcessSetRequestParams = []httpProcessSetRequestStruct{
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/test123", createBody()), false, createForm(), "Good case"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/test123", io.NopCloser(errorReader{})), true, forms.SignalA_v1a{}, "Bad case, ReadAll returns error"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/test123", createBrokenBody()), true, forms.SignalA_v1a{}, "Bad case, Unmarshal returns error"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/test123", createBodyWithNoformVersion()), true, forms.SignalA_v1a{}, "Bad case, version key missing"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/test123", createBodyWithWrongForm()), true, forms.SignalA_v1a{}, "Bad case, Second Unmarshal breaks"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/test123", createBodyWithWrongFormVersion()), true, forms.SignalA_v1a{}, "Bad case, version is wrong"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/test123", createBodyWithSignalBForm()), true, forms.SignalA_v1a{}, "Bad case, form version is SignalB_v1a"},
}

func TestHTTPProcessSetRequest(t *testing.T) {
	for _, testCase := range httpProcessSetRequestParams {
		testCase.inputR.Header.Set("Content-Type", "application/json")
		f, err := HTTPProcessSetRequest(testCase.inputW, testCase.inputR)

		if f != testCase.expectedForm || (err == nil && testCase.expectedErr == true) || (err != nil && testCase.expectedErr == false) {
			t.Errorf("Expected %v and %v, got: %v and %v", testCase.expectedForm, testCase.expectedErr, f, err)
		}
	}
}

type getBestContentTypeStruct struct {
	acceptHeaderInput     string
	bestContentTypeOutput string
	testName              string
}

var getBestContentTypeParams = []getBestContentTypeStruct{
	{"", "application/json", "Good case, no accept header provided"},
	{"application/xml", "application/xml", "Good case, accept header provided without q-values"},
	{"application/xml;q=0.7, application/json;q=0.9", "application/json", "Good case, accept header provided with q-values"},
	{"application/xml;q=wrong, application/json;q=1.1", "application/json", "Good case, xml gets skipped"},
	{"application/xml;q=0.9, application/json;q=0.9", "application/xml", "Good case, equal q-values selects the first one"},
	{"application/xml;q=-0.9", "application/json", "Good case, no MIME type found"},
}

func TestGetBestContentType(t *testing.T) {
	for _, testCase := range getBestContentTypeParams {
		res := getBestContentType(testCase.acceptHeaderInput)

		if res != testCase.bestContentTypeOutput {
			t.Errorf("Expected %v, got: %v in test case: %s", testCase.bestContentTypeOutput, res, testCase.testName)
		}
	}
}
