package usecases

import (
	"encoding/xml"
	"fmt"
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
