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
	inputW      http.ResponseWriter
	inputR      *http.Request
	inputF      forms.Form
	expectedErr error
	testName    string
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

func createHttpResponse() func() *http.Response {
	httpResp := func() *http.Response {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"))),
		}
	}
	return httpResp
}

func createEmptyFormVersion() forms.Form {
	form.NewForm()
	form.Version = ""
	return &form
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

// TODO: Uncomment this test if we get approval to make it so that HTTPProcessGetRequest returns an error, also make sure the expectedErr gets it proper value then
/*
var httpProcessGetRequestParams = []httpProcessGetRequestStruct{
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test123", nil), form.NewForm(), nil, "Good case"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test123", nil), nil, mockError, "Bad case, form is nil"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test123", nil), createEmptyFormVersion(), mockError, "Bad case, form version is empty"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test123", nil), createBrokenForm(), mockError, "Bad case, form value is invalid"},
	{&mockResponseWriter{}, httptest.NewRequest(http.MethodGet, "/test123", nil), form.NewForm(), mockError, "Bad case, Write fails"},
}

func TestHTTPProcessGetRequest(t *testing.T) {
	for _, testCase := range httpProcessGetRequestParams {
		err := HTTPProcessGetRequest(testCase.inputW, testCase.inputR, testCase.inputF)

		if err != testCase.expectedErr {
			t.Errorf("Expected %v, got: %v", testCase.expectedErr, err)
		}
	}
}
*/

type httpProcessSetRequestStruct struct {
	inputW       http.ResponseWriter
	inputR       *http.Request
	expectedErr  error
	expectedForm forms.SignalA_v1a
	testName     string
}

func createForm() forms.SignalA_v1a {
	form.NewForm()
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

func errorIsSame(err1 error, err2 error) bool {
	switch {
	case err1 == nil && err2 == nil:
		return true
	case err1 == nil || err2 == nil:
		return false
	default:
		return true
	}
}

var httpProcessSetRequestParams = []httpProcessSetRequestStruct{
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/test123", createBody()), nil, createForm(), "Good case"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/test123", io.NopCloser(errorReader{})), errBodyRead, forms.SignalA_v1a{}, "Bad case, ReadAll returns error"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/test123", createBrokenBody()), errBodyRead, forms.SignalA_v1a{}, "Bad case, Unmarshal returns error"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/test123", createBodyWithNoformVersion()), nil, forms.SignalA_v1a{}, "Bad case, version key missing"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/test123", createBodyWithWrongForm()), errBodyRead, forms.SignalA_v1a{}, "Bad case, Second Unmarshal breaks"},
	{httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/test123", createBodyWithWrongFormVersion()), errBodyRead, forms.SignalA_v1a{}, "Bad case, version is wrong"},
}

func TestHTTPProcessSetRequest(t *testing.T) {
	for _, testCase := range httpProcessSetRequestParams {
		f, err := HTTPProcessSetRequest(testCase.inputW, testCase.inputR)

		if f != testCase.expectedForm || !errorIsSame(err, testCase.expectedErr) {
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
