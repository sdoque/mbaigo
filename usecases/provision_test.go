package usecases

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"

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
		Version: "testVersion",
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
