package usecases

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

// A mocked UnitAsset used for testing
type mockUnitAssetWithTraits struct {
	Name        string              `json:"name"`
	Owner       *components.System  `json:"-"`
	Details     map[string][]string `json:"details"`
	ServicesMap components.Services `json:"-"`
	CervicesMap components.Cervices `json:"-"`
	Traits      map[string][]string `json:"-"`
}

func (mua mockUnitAssetWithTraits) GetTraits() any {
	return mua.Traits
}

func (mua mockUnitAssetWithTraits) GetName() string {
	return mua.Name
}

func (mua mockUnitAssetWithTraits) GetServices() components.Services {
	return mua.ServicesMap
}

func (mua mockUnitAssetWithTraits) GetCervices() components.Cervices {
	return mua.CervicesMap
}

func (mua mockUnitAssetWithTraits) GetDetails() map[string][]string {
	return mua.Details
}

func (mua mockUnitAssetWithTraits) Serving(w http.ResponseWriter, r *http.Request, servicePath string) {
}

func createConfig(sys *components.System, assetTrait bool, assetAmount int) {
	var defaultConfig templateOut

	var assetTemplate components.UnitAsset
	for _, ua := range sys.UAssets {
		assetTemplate = *ua
		break
	}
	servicesTemplate := getServicesList(assetTemplate)

	confAsset := ConfigurableAsset{
		Name:     assetTemplate.GetName(),
		Details:  assetTemplate.GetDetails(),
		Services: servicesTemplate,
	}

	if assetTrait == true {
		setTest := &components.Service{
			ID:            1,
			Definition:    "test",
			SubPath:       "test",
			Details:       map[string][]string{"Forms": {"SignalA_v1a"}},
			Description:   "A test service",
			RegPeriod:     45,
			RegTimestamp:  "now",
			RegExpiration: "45",
		}
		ServicesMap := &components.Services{
			setTest.SubPath: setTest,
		}
		mua := &mockUnitAssetWithTraits{
			Name:        "testUnitAsset",
			Details:     map[string][]string{"Test": {"Test"}},
			ServicesMap: *ServicesMap,
			CervicesMap: nil,
			Traits:      map[string][]string{"Trait": {"testTrait"}},
		}
		var muaInterface components.UnitAsset = mua
		sys.UAssets[mua.GetName()] = &muaInterface
		// If the asset exposes traits, serialize them and store as raw JSON
		if assetWithTraits, ok := assetTemplate.(components.HasTraits); ok {
			if traits := assetWithTraits.GetTraits(); traits != nil {
				traitJSON, err := json.Marshal(traits)
				if err == nil {
					confAsset.Traits = []json.RawMessage{traitJSON}
				} else {
					fmt.Println("Warning: could not marshal traits:", err)
				}
			}
		}
		defaultConfig.Assets = []ConfigurableAsset{confAsset} // this is a list of unit assets
	}

	if assetAmount > 0 {
		for x := range assetAmount {
			setTest := components.Service{
				ID:            x,
				Definition:    fmt.Sprintf("test%d", x),
				SubPath:       fmt.Sprintf("test%d", x),
				Details:       map[string][]string{"Forms": {"SignalA_v1a"}},
				Description:   "A test service",
				RegPeriod:     45,
				RegTimestamp:  "now",
				RegExpiration: "45",
			}
			servList := []components.Service{setTest}
			mua := ConfigurableAsset{
				Name:     fmt.Sprintf("testUnitAsset%d", x),
				Details:  map[string][]string{"Test": {"Test"}},
				Services: servList,
			}
			defaultConfig.Assets = append(defaultConfig.Assets, mua)
		}
	}

	leadingRegistrar := components.CoreSystem{
		Name: "serviceregistrar",
		Url:  "https://leadingregistrar",
	}
	test := components.CoreSystem{
		Name: "test",
		Url:  "https://test",
	}

	defaultConfig.CCoreS = append(defaultConfig.CCoreS, leadingRegistrar)
	defaultConfig.CCoreS = append(defaultConfig.CCoreS, test)
	defaultConfig.CName = sys.Name
	defaultConfig.Protocols = sys.Husk.ProtoPort
	os.Remove("systemconfig.json")
	defaultConfigFile, err := os.Create("systemconfig.json")
	if err != nil {
		log.Fatalf("Encountered error while creating default config file")
	}
	defer defaultConfigFile.Close()

	enc := json.NewEncoder(defaultConfigFile) // Create an encoder that allows writing to a file
	enc.SetIndent("", "     ")
	err = enc.Encode(defaultConfig) // Write defaultConfig template to file
	if err != nil {
		log.Fatalf("jsonEncode: %v", err)
	}
}

type configureParams struct {
	assetHasTraits  bool
	assetNumber     int
	createNewConfig bool
	allowConfigRead bool
	expectError     bool
	testCase        string
}

func TestConfigure(t *testing.T) {
	testParams := []configureParams{
		// {assetHasTraits, assetNumber, createNewConfig, allowConfigRead, expectError, testCase}
		{false, 1, true, true, false, "Best case"},
		{false, 0, false, true, true, "Missing asset"},
		{true, 0, true, true, false, "Good case, asset has traits"},
		{false, 1, false, true, true, "Config missing"},
		{false, 1, false, false, true, "Config missing, cant open or create"},
		{false, 0, true, true, false, "No Assets in config"},
		{false, 3, true, true, false, "Multiple Assets in config"},
	}
	defer os.Remove("systemconfig.json")
	for _, testCase := range testParams {
		os.Remove("systemconfig.json")
		testSys := createTestSystem(false)
		if testCase.testCase == "Missing asset" {
			testSys.UAssets = nil
		}
		if testCase.createNewConfig == true {
			createConfig(&testSys, testCase.assetHasTraits, testCase.assetNumber)
		}
		if testCase.allowConfigRead == false {
			os.OpenFile("systemconfig.json", os.O_RDWR|os.O_CREATE, 0000)
		}
		_, err := Configure(&testSys)
		if testCase.expectError == false {
			if err != nil {
				t.Errorf("Expected no errors in '%s', got: %v", testCase.testCase, err)
			}
		} else {
			if err == nil {
				t.Errorf("Expected errors in testcase '%s' got none", testCase.testCase)
			}
		}
		if (testCase.createNewConfig == true) || (testCase.allowConfigRead == false) {
			err = os.Remove("systemconfig.json")
			if err != nil {
				t.Fatalf("failed while removing file")
			}
		}
		if errors.Is(err, ErrNewConfig) {
			err = os.Remove("systemconfig.json")
			if err != nil {
				t.Fatalf("failed while removing file")
			}
		}
	}
}

func TestGetServiceList(t *testing.T) {
	setTest := &components.Service{
		ID:            1,
		Definition:    "test",
		SubPath:       "test",
		Details:       map[string][]string{"Forms": {"SignalA_v1a"}},
		Description:   "A test service",
		RegPeriod:     45,
		RegTimestamp:  "now",
		RegExpiration: "45",
	}
	ServicesMap := &components.Services{
		setTest.SubPath: setTest,
	}
	mua := mockUnitAsset{
		Name:        "test",
		Owner:       nil,
		Details:     nil,
		ServicesMap: *ServicesMap,
	}
	servList := getServicesList(mua)
	if len(servList) != 1 && servList[0].Definition != "test" {
		t.Errorf("Expected length: 1, got %d\tExpected 'Definition': test, got %s", len(servList), servList[0].Definition)
	}
}
