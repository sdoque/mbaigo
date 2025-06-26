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

type mockForm struct {
	XMLName xml.Name `json:"-" xml:"testName"`
	Value   float64  `json:"value" xml:"value"`
	Unit    string   `json:"unit" xml:"unit"`
	Version string   `json:"version" xml:"version"`
}

// NewForm creates a new form
func (f mockForm) NewForm() forms.Form {
	f.Version = "testVersion"
	return f
}

// FormVersion returns the version of the form
func (f mockForm) FormVersion() string {
	return f.Version
}

// Returns a form containing test values
func createTestForm() (f mockForm) {
	form := mockForm{
		XMLName: xml.Name{},
		Value:   123,
		Unit:    "testUnit",
		Version: "testVersion",
	}
	return form
}

type brokenMockForm struct {
	XMLName xml.Name  `json:"-" xml:"testName"`
	Value   complex64 `json:"value" xml:"value"`
	Unit    string    `json:"unit" xml:"unit"`
	Version string    `json:"version" xml:"version"`
}

// NewForm creates a new form
func (f brokenMockForm) NewForm() forms.Form {
	f.Version = "testVersion"
	return f
}

// FormVersion returns the version of the form
func (f brokenMockForm) FormVersion() string {
	return f.Version
}

// Returns a form containing complex numbers, which xml and json can't marshal
func createBrokenTestForm() (f brokenMockForm) {
	form := brokenMockForm{
		XMLName: xml.Name{},
		Value:   complex(1, 2),
		Unit:    "testUnit",
		Version: "testVersion",
	}
	return form
}

type packParams struct {
	contentType   string
	form          forms.Form
	expectedError bool
	testCase      string
}

// Returns an error containing a list values who was missing/wrong
func assurePackData(byteArr []byte, contentType string, expectedError bool) (err error) {
	data := string(byteArr)
	if contentType == "application/xml" {
		missingData := []string{}
		correctName := strings.Contains(data, "<testName>")
		if correctName != true {
			missingData = append(missingData, "XMLName")
		}
		if expectedError == false {
			correctValue := strings.Contains(data, "<value>123</value>")
			if correctValue != true {
				missingData = append(missingData, "Value")
			}
		} else {
			correctValue := strings.Contains(data, "<value>(1+2i)</value>")
			if correctValue != true {
				missingData = append(missingData, "Value")
			}
		}
		correctUnit := strings.Contains(data, "<unit>testUnit</unit>")
		if correctUnit != true {
			missingData = append(missingData, "Unit")
		}
		correctVersion := strings.Contains(data, "<version>testVersion</version>")
		if correctVersion != true {
			missingData = append(missingData, "Version")
		}
		if len(missingData) != 0 {
			return fmt.Errorf("missing data: %s", missingData)
		}
	} else {
		missingData := []string{}
		if expectedError == false {
			correctValue := strings.Contains(data, `"value": 123`)
			if correctValue != true {
				missingData = append(missingData, "Value")
			}
		} else {
			correctValue := strings.Contains(data, `"value": (1+2i)`)
			if correctValue != true {
				missingData = append(missingData, "Value")
			}
		}
		correctUnit := strings.Contains(data, `"unit": "testUnit"`)
		if correctUnit != true {
			missingData = append(missingData, "Unit")
		}
		correctVersion := strings.Contains(data, `"version": "testVersion"`)
		if correctVersion != true {
			missingData = append(missingData, "Version")
		}
		if len(missingData) != 0 {
			return fmt.Errorf("missing data: %s", missingData)
		}
	}
	return nil
}

func TestPack(t *testing.T) {
	params := []packParams{
		{"application/xml", createTestForm(), false, "Best case, xml"},
		{"application/json", createTestForm(), false, "Best case, json"},
		{"application/xml", createBrokenTestForm(), true, "Bad case, xml"},
		{"application/json", createBrokenTestForm(), true, "Bad case, json"},
	}
	for _, c := range params {
		data, err := Pack(c.form, c.contentType)
		if c.expectedError == false {
			if err != nil {
				t.Errorf("failed in testcase '%s' with error: %v", c.testCase, err)
			}
			err = assurePackData(data, c.contentType, c.expectedError)
			if err != nil {
				t.Errorf("error from assureData: %v", err)
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
		/* {false, "Best case, xml", "text/plain", func() (data []byte, err error) {
			var f forms.SignalA_v1a
			f.NewForm()
			data, err = xml.Marshal(f)
			return
		}}, */
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
		// Can't reach second unmarshal for json to break it this way, moving on.
		/* {true, "Bad case, broken unmarshal in xml", "application/xml", func() (data []byte, err error) {
			data = append(data, byte(math.NaN()))
			return data, err
		}}, */
		// Can't reach second unmarshal for xml to break it this way, moving on.
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
