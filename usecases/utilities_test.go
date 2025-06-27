package usecases

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/forms"
)

// Returns a form containing test values
func createTestForm() (form mockForm) {
	form = mockForm{
		XMLName: xml.Name{},
		Value:   123,
		Unit:    "testUnit",
		Version: "testVersion",
	}
	return form
}

type assureParams struct {
	contentType  string
	checkName    func(string, []string) []string
	checkValue   func(string, []string) []string
	checkUnit    func(string, []string) []string
	checkVersion func(string, []string) []string
}

// Returns an error containing a list of values who was missing/wrong
func assurePackData(byteArr []byte, contentType string) (err error) {
	testParams := []assureParams{
		// Parameters and function for checking xml data
		{
			"application/xml",
			func(data string, missingList []string) []string {
				if strings.Contains(data, "<testName>") == false {
					missingList = append(missingList, "name")
				}
				return missingList
			},
			func(data string, missingList []string) []string {
				if strings.Contains(data, "<value>123</value>") == false {
					missingList = append(missingList, "value")
				}
				return missingList
			},
			func(data string, missingList []string) []string {
				if strings.Contains(data, "<unit>testUnit</unit>") == false {
					missingList = append(missingList, "unit")
				}
				return missingList
			},
			func(data string, missingList []string) []string {
				if strings.Contains(data, "<version>testVersion</version>") == false {
					missingList = append(missingList, "version")
				}
				return missingList
			},
		},
		// Parameters and functions for checking json data
		{
			"application/json",
			func(data string, missingList []string) []string { return missingList },
			func(data string, missingList []string) []string {
				if strings.Contains(data, `"value": 123`) == false {
					missingList = append(missingList, "value")
				}
				return missingList
			},
			func(data string, missingList []string) []string {
				if strings.Contains(data, `"unit": "testUnit"`) == false {
					missingList = append(missingList, "unit")
				}
				return missingList
			},
			func(data string, missingList []string) []string {
				if strings.Contains(data, `"version": "testVersion"`) == false {
					missingList = append(missingList, "version")
				}
				return missingList
			},
		},
	}
	// Loops through the param list, and checks if there's an element with the same contentType
	for _, c := range testParams {
		data := string(byteArr)
		var missingList []string
		if c.contentType == contentType {
			// If there's an element with same contentType, run checks
			missingList = c.checkName(data, missingList)
			missingList = c.checkValue(data, missingList)
			missingList = c.checkUnit(data, missingList)
			missingList = c.checkVersion(data, missingList)
		}
		if len(missingList) != 0 {
			return fmt.Errorf("fields containing wrong data: %v", missingList)
		}
	}
	return err
}

type packParams struct {
	contentType   string
	expectedError bool
	form          func() mockForm
	assureData    func([]byte, string) error
	testCase      string
}

func TestPack(t *testing.T) {
	params := []packParams{
		{"application/xml", false, func() mockForm { return createTestForm() }, func(byteArr []byte, cType string) error { return assurePackData(byteArr, cType) }, "Best case, xml"},
		{"application/json", false, func() mockForm { return createTestForm() }, func(byteArr []byte, cType string) error { return assurePackData(byteArr, cType) }, "Best case, json"},
		{"application/xml", true, func() mockForm { form := createTestForm(); form.Value = complex(1, 2); return form }, nil, "Bad case, xml"},
		{"application/json", true, func() mockForm { form := createTestForm(); form.Value = complex(1, 2); return form }, nil, "Bad case, json"},
	}
	for _, c := range params {
		data, err := Pack(c.form(), c.contentType)
		if c.expectedError == false {
			if err != nil {
				t.Errorf("failed in testcase '%s' with error: %v", c.testCase, err)
			}
			// Only assure data if we expect and get no errors from Pack() since data will be empty if we get an error
			err = c.assureData(data, c.contentType)
			if err != nil {
				t.Errorf("error from assureData in testcase '%s': %v", c.testCase, err)
			}
		} else {
			if err == nil {
				t.Errorf("expected error in testcase '%s', got none", c.testCase)
			}
		}
	}
}

// This covers the case of it having a version but is not present in the form type map
type testFormHasVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// This covers the case of it not having a version
type testFormNoVersion struct {
	Name string `json:"name"`
}

type unpackParams struct {
	expectError bool
	testCase    string
	contentType string
	setup       func() (data []byte, err error)
}

func TestUnpack(t *testing.T) {
	testParams := []unpackParams{
		//{expectError, testCase, contentType, setup()}
		{false, "Best case, json", "text/plain", func() (data []byte, err error) {
			var f forms.SignalA_v1a
			f.NewForm()
			data, err = json.Marshal(f)
			return
		}},
		// TODO: The following test can't be done because xml.Unmarshal() can't unmarshal to map[]
		// fails with "error unmarshalling XML: unknown type map[string]interface {}"
		/*{false, "Best case, xml", "text/plain", func() (data []byte, err error) {
			var f forms.SignalA_v1a
			f.NewForm()
			data, err = xml.Marshal(f)
			return
		}},*/
		{true, "Bad case, not json/xml", "text/plain", func() (data []byte, err error) { return []byte("TEST123"), nil }},
		{true, "Bad case, empty []byte", "text/plain", func() (data []byte, err error) { return []byte(""), nil }},
		{true, "Bad case, unsupported content type", "unknown", func() (data []byte, err error) { return []byte("test"), nil }},
		{true, "Bad case, missing version", "application/json", func() (data []byte, err error) {
			f := &testFormNoVersion{
				Name: "testName",
			}
			data, err = json.Marshal(f)
			return data, err
		}},
		{true, "Bad case, unsupported form version", "application/json", func() (data []byte, err error) {
			f := &testFormHasVersion{
				Name:    "testName",
				Version: "testVersion",
			}
			data, err = json.Marshal(f)
			return data, err
		}},
		{true, "Bad case, broken unmarshal in json", "application/json", func() (data []byte, err error) {
			data = append(data, byte(math.NaN()))
			return data, err
		}},
		// TODO: Refactor code so we can do this test: currently can't reach second unmarshal for json to break it this way, moving on.
		{true, "Bad case, broken unmarshal in xml", "application/xml", func() (data []byte, err error) {
			data = append(data, byte(math.NaN()))
			return data, err
		}},
		// TODO: Refactor code so we can do this test: currently can't reach second unmarshal for xml to break it this way, moving on.
	}

	for _, c := range testParams {
		// Setup
		data, err := c.setup()
		if err != nil {
			t.Errorf("unexpected error in setup of testcase '%s': %v", c.testCase, err)
		}

		// Test
		_, err = Unpack(data, c.contentType)
		if c.expectError != true {
			if err != nil {
				t.Errorf("error occurred in testcase '%s', got:\n %v", c.testCase, err)
			}
		} else {
			if err == nil {
				t.Errorf("expected errors in testcase '%s', got none", c.testCase)
			}
		}
	}
}

type toCamelParams struct {
	expectedString string
	testString     string
	testCase       string
}

func TestToCamel(t *testing.T) {
	testParams := []toCamelParams{
		{"testString", "TestString", "Best case"},
		{"", "", "Empty string"},
	}
	for _, c := range testParams {
		generatedStr := ToCamel(c.testString)
		if generatedStr != c.expectedString {
			t.Errorf("expected both strings to be %s, generated string was: %s", c.expectedString, generatedStr)
		}
	}
}

type toPascalParams struct {
	expectedString string
	testString     string
	testCase       string
}

func TestToPascal(t *testing.T) {
	testParams := []toPascalParams{
		{"TestString", "testString", "Best case"},
		{"", "", "Empty string"},
	}
	for _, c := range testParams {
		generatedStr := ToPascal(c.testString)
		if generatedStr != c.expectedString {
			t.Errorf("expected both strings to be %s in testcase '%s', generated string was: %s", c.expectedString, c.testCase, generatedStr)
		}
	}
}

type isFirstUpperParams struct {
	expectedUpper bool
	testString    string
	testCase      string
}

func TestIsFirstLetterUpper(t *testing.T) {
	testParams := []isFirstUpperParams{
		{true, "FirstUpper", "First letter is uppercase"},
		{false, "firstUpper", "First letter is not uppercase"},
		{false, "", "Empty string"},
	}
	for _, c := range testParams {
		isUpper := IsFirstLetterUpper(c.testString)
		if isUpper != c.expectedUpper {
			if c.expectedUpper == true {
				t.Errorf("expected first letter to be uppercase in testcase '%s'", c.testCase)
			} else {
				t.Errorf("expected first letter to be lowercase in testcase '%s'", c.testCase)
			}
		}
	}
}

type isFirstLowerParams struct {
	expectedLower bool
	testString    string
	testCase      string
}

func TestIsFirstLetterLower(t *testing.T) {
	testParams := []isFirstLowerParams{
		{true, "firstLower", "First letter is lowercase"},
		{false, "FirstLower", "First letter is not lowercase"},
		{false, "", "Empty string"},
	}
	for _, c := range testParams {
		isLower := IsFirstLetterLower(c.testString)
		if isLower != c.expectedLower {
			if c.expectedLower == true {
				t.Errorf("expected first letter to be lowercase in testcase '%s'", c.testCase)
			} else {
				t.Errorf("expected first letter to be uppercase in testcase '%s'", c.testCase)
			}
		}
	}
}

type isPascalCaseParams struct {
	expectedPascal bool
	testString     string
	testCase       string
}

func TestIsPascalCase(t *testing.T) {
	testParams := []isPascalCaseParams{
		{true, "IsPascal", "Is Pascal"},
		{false, "isPascal", "Not Pascal"},
	}
	for _, c := range testParams {
		isPascal := IsPascalCase(c.testString)
		if isPascal != c.expectedPascal {
			if c.expectedPascal == true {
				t.Errorf("expected first letter to be uppercase in testcase '%s'", c.testCase)
			} else {
				t.Errorf("expected first letter to be lowercase in testcase '%s'", c.testCase)
			}
		}
	}
}

type isCamelCaseParams struct {
	expectedCamel bool
	testString    string
	testCase      string
}

func TestICamelCase(t *testing.T) {
	testParams := []isCamelCaseParams{
		{true, "isCamel", "Is Camel"},
		{false, "IsCamel", "Not Camel"},
	}
	for _, c := range testParams {
		isCamel := IsCamelCase(c.testString)
		if isCamel != c.expectedCamel {
			if c.expectedCamel == true {
				t.Errorf("expected first letter to be lowercase in testcase '%s'", c.testCase)
			} else {
				t.Errorf("expected first letter to be uppercase in testcase '%s'", c.testCase)
			}
		}
	}
}
