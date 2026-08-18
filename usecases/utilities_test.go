package usecases

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sdoque/mbaigo/forms"
)

type packParams struct {
	contentType     string
	expectedError   bool
	form            mockForm
	expectedValue   string
	expectedVersion string
	testCase        string
}

func TestPack(t *testing.T) {
	params := []packParams{
		{"application/xml", false, mockForm{Value: 123, Version: "testVersion"}, "<value>123</value>", "<version>testVersion</version>", "Best case, xml"},
		{"application/json", false, mockForm{Value: 123, Version: "testVersion"}, `"value": 123`, `"version": "testVersion"`, "Best case, json"},
		{"application/xml", true, mockForm{Value: complex(1, 2), Version: "testVersion"}, "", "", "Bad case, xml"},
		{"application/json", true, mockForm{Value: complex(1, 2), Version: "testVersion"}, "", "", "Bad case, json"},
	}
	for _, c := range params {
		data, err := Pack(c.form, c.contentType)
		if c.expectedError == false {
			if err != nil {
				t.Errorf("failed in testcase '%s' with error: %v", c.testCase, err)
			}
			if strings.Contains(string(data), c.expectedValue) != true {
				t.Errorf("value missing or wrong in testcase '%s'", c.testCase)
			}
			if strings.Contains(string(data), c.expectedVersion) != true {
				t.Errorf("version missing or wrong in testcase '%s'", c.testCase)
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
		{true, "Bad case, broken unmarshal in xml", "application/xml", func() (data []byte, err error) {
			data = append(data, byte(math.NaN()))
			return data, err
		}},
		// TODO: Refactor code so we can do another test: currently can't reach second unmarshal for json to break it this way, moving on.
		// TODO: Refactor code so we can do another test: currently can't reach second unmarshal for xml to break it this way, moving on.
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

// TestForLogCannotForgeALogLine: a request path and a certificate common name
// are chosen by the caller, and a newline in either lets that caller write log
// entries of their own. A forged line is indistinguishable from a real one once
// it is in the file, and the log is what an operator reads to work out what
// happened.
func TestForLogCannotForgeALogLine(t *testing.T) {
	forged := "/thermostat/setpoint\n2026-08-08 09:00:00 first request to ca from peer \"root\""
	got := ForLog(forged)

	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("a newline survived: %q", got)
	}
	if strings.Contains(got, "\x00") {
		t.Errorf("a NUL survived: %q", got)
	}

	// An ordinary path is untouched, including non-ASCII: a Swedish place name
	// in a functional location is not an attack.
	// Luleå rather than a Swedish common noun: the point is that non-ASCII
	// survives, and the spell checker in CI has its own opinion about Swedish.
	for _, ordinary := range []string{"/thermostat/Kitchen/setpoint", "/thermostat/Luleå/setpoint"} {
		if ForLog(ordinary) != ordinary {
			t.Errorf("ForLog(%q) = %q, want it unchanged", ordinary, ForLog(ordinary))
		}
	}

	// And a caller does not get to choose how long a log line is.
	if long := ForLog(strings.Repeat("a", 5000)); len(long) > 300 {
		t.Errorf("a 5000-character path produced a %d-character log field", len(long))
	}
}

// TestForLogTruncatesOnARuneBoundary is follow-up finding N13. The length check
// counted bytes and the slice cut bytes, so a run of multi-byte characters was
// cut mid-rune — putting back exactly the invalid UTF-8 the map above it exists
// to strip.
func TestForLogTruncatesOnARuneBoundary(t *testing.T) {
	got := ForLog(strings.Repeat("€", 400))
	if !utf8.ValidString(got) {
		t.Error("truncation produced invalid UTF-8")
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("a 400-character run was not truncated: %d characters", utf8.RuneCountInString(got))
	}
}

// A log viewer gives meaning to more than the control characters: the line and
// paragraph separators some honour as newlines, and the bidirectional overrides,
// which can make a line read in the opposite direction from how it was written.
func TestForLogStripsWhatALogViewerActsOn(t *testing.T) {
	for name, r := range map[string]rune{
		"line separator U+2028":      ' ',
		"paragraph separator U+2029": ' ',
		"right-to-left override":     '\u202E',
		"zero-width joiner":          '‍',
	} {
		got := ForLog("before" + string(r) + "after")
		if strings.ContainsRune(got, r) {
			t.Errorf("%s survived: %q", name, got)
		}
		if got != "beforeafter" {
			t.Errorf("%s: got %q, want %q", name, got, "beforeafter")
		}
	}
}
