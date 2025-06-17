package usecases

import (
	//"encoding/json"
	//"fmt"
	//"io"
	//"reflect"
	//"strings"
	"io"
	"net/http"
	"strings"
	"testing"

	//"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

type stateParams struct {
	testCase         string
	testCer          *components.Cervice
	testSys          *components.System
	bodyBytes        []byte
	body             func() *http.Response
	mockTransportErr int
	errHTTP          error
	expectedfForm    forms.Form
	expectedErr      error
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

var testCerviceWithNodesRefresh = components.Cervice{
	IReferentce: "test",
	Definition:  "A test Cervice with nodes",
	Details:     map[string][]string{"Forms": {"SignalA_v1a"}},
	Nodes:       map[string][]string{"test": {"https://testSystem/testUnitAsset/test"}},
	Protos:      []string{"http"},
}

var testCerviceWithoutNodes = components.Cervice{
	IReferentce: "test",
	Definition:  "A test Cervice without nodes",
	Details:     map[string][]string{"Forms": {"SignalA_v1a"}},
	Nodes:       nil,
	Protos:      []string{"http"},
}

var testCerviceWithBrokenUrl = components.Cervice{
	IReferentce: "test",
	Definition:  "A test Cervice with nodes",
	Details:     map[string][]string{"Forms": {"SignalA_v1a"}},
	Nodes:       map[string][]string{"test": {brokenUrl}},
	Protos:      []string{"http"},
}

var testSys = createTestSystem(false)

var form forms.SignalA_v1a

var testStateParams = []stateParams{
	{
		"No errors",
		newTestCerviceWithNodes(),
		&testSys,
		nil,
		func() *http.Response {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"))),
			}
		},
		0,
		nil,
		form.NewForm(),
		nil,
	},
	{
		"No errors",
		&testCerviceWithoutNodes,
		&testSys,
		nil,
		func() *http.Response {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"))),
			}
		},
		0,
		nil,
		form.NewForm(),
		nil,
	},
	{
		"Search4Services error",
		&testCerviceWithoutNodes,
		&testSys,
		nil,
		func() *http.Response {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"))),
			}
		},
		1,
		errHTTP,
		nil,
		errHTTP,
	},
	{
		"NewRequest() error",
		&testCerviceWithBrokenUrl,
		&testSys,
		nil,
		func() *http.Response {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"))),
			}
		},
		1,
		errHTTP,
		nil,
		errHTTP,
	},
	{
		"Status code error",
		newTestCerviceWithNodes(),
		&testSys,
		nil,
		func() *http.Response {
			return &http.Response{
				Status:     "300 NAK",
				StatusCode: 300,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"))),
			}
		},
		2,
		errHTTP,
		nil,
		errHTTP,
	},
	{
		"io.ReadAll() error",
		newTestCerviceWithNodes(),
		&testSys,
		nil,
		func() *http.Response {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(errorReader{}),
			}
		},
		0,
		nil,
		nil,
		nil,
	},
	{
		"Unpack() error",
		newTestCerviceWithNodes(),
		&testSys,
		nil,
		func() *http.Response {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"Wrong content type"}},
				Body:       io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"))),
			}
		},
		0,
		nil,
		nil,
		nil,
	},
	{
		"DefaultClient.Do() error",
		newTestCerviceWithNodes(),
		&testSys,
		nil,
		func() *http.Response {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string("{\n  \"value\": 0,\n  \"unit\": \"\",\n  \"timestamp\": \"0001-01-01T00:00:00Z\",\n  \"version\": \"SignalA_v1.0\"\n}"))),
			}
		},
		1,
		errHTTP,
		nil,
		errHTTP,
	},
}

func TestGetState(t *testing.T) {
	for _, test := range testStateParams {
		newMockTransport(test.body, test.mockTransportErr, test.errHTTP)

		res, err := GetState(test.testCer, test.testSys)

		// Directly compare the fields of the expected and actual forms
		if res != nil {
			expected := test.expectedfForm.(*forms.SignalA_v1a)
			actual := res.(*forms.SignalA_v1a)
			if test.testCase == "No errors" {
				if expected.Value != actual.Value || expected.Unit != actual.Unit || expected.Timestamp != actual.Timestamp || expected.Version != actual.Version || err != test.expectedErr {
					t.Errorf("Test case: %s got error: %v. \nExpected form: \n%+v\n, got: \n%+v", test.testCase, err, expected, actual)
				}
			}
		} else {
			if err == nil {
				t.Errorf("Test case: %s got error: %v:", test.testCase, err)
			}
		}
	}
}

func TestSetState(t *testing.T) {
	for _, test := range testStateParams {
		newMockTransport(test.body, test.mockTransportErr, test.errHTTP)

		if test.testCase == "DefaultClient.Do() error" {
			test.testCer = &testCerviceWithNodesRefresh
		}
		res, err := SetState(test.testCer, test.testSys, nil)

		// Directly compare the fields of the expected and actual forms
		if res != nil {
			expected := test.expectedfForm.(*forms.SignalA_v1a)
			actual := res.(*forms.SignalA_v1a)
			if test.testCase == "No errors" {
				if expected.Value != actual.Value || expected.Unit != actual.Unit || expected.Timestamp != actual.Timestamp || expected.Version != actual.Version || err != test.expectedErr {
					t.Errorf("Test case: %s got error: %v. \nExpected form: \n%+v\n, got: \n%+v", test.testCase, err, expected, actual)
				}
			}
		} else {
			if err == nil {
				t.Errorf("Test case: %s got error: %v:", test.testCase, err)
			}
		}
	}
}
