package usecases

import (
	"encoding/json"
	"fmt"
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

func createConfigHasTraits(sys *components.System) (err error) {
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
				return err
			}
		}
	}
	defaultConfig.Assets = []ConfigurableAsset{confAsset} // this is a list of unit assets

	leadingRegistrar := components.CoreSystem{
		Name: "serviceregistrar",
		Url:  "http://localhost:20102/serviceregistrar/registry",
	}
	orchestrator := components.CoreSystem{
		Name: "orchestrator",
		Url:  "http://localhost:20103/orchestrator/orchestration",
	}
	ca := components.CoreSystem{
		Name: "ca",
		Url:  "http://localhost:20100/ca/certification",
	}
	maitreD := components.CoreSystem{
		Name: "maitreD",
		Url:  "http://localhost:20101/maitreD/maitreD",
	}

	defaultConfig.CCoreS = []components.CoreSystem{leadingRegistrar, orchestrator, ca, maitreD}
	defaultConfig.CName = sys.Name
	defaultConfig.Protocols = sys.Husk.ProtoPort
	defaultConfigFile, err := os.Create("systemconfig.json")
	if err != nil {
		return fmt.Errorf("encountered error while creating default config file: %v", err)
	}
	defer defaultConfigFile.Close()

	enc := json.NewEncoder(defaultConfigFile) // Create an encoder that allows writing to a file
	enc.SetIndent("", "     ")
	err = enc.Encode(defaultConfig) // Write defaultConfig template to file
	if err != nil {
		return fmt.Errorf("jsonEncode: %v", err)
	}
	return
}

func createConfigNoTraits(sys *components.System, assetAmount int) (err error) {
	var defaultConfig templateOut

	if assetAmount == 1 {
		setTest := components.Service{
			ID:            1,
			Definition:    "test",
			SubPath:       "test",
			Details:       map[string][]string{"Forms": {"SignalA_v1a"}},
			Description:   "A test service",
			RegPeriod:     45,
			RegTimestamp:  "now",
			RegExpiration: "45",
		}
		servList := []components.Service{setTest}
		mua := ConfigurableAsset{
			Name:     "testUnitAsset",
			Details:  map[string][]string{"Test": {"Test"}},
			Services: servList,
		}
		defaultConfig.Assets = append(defaultConfig.Assets, mua)
	} else {
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
		Url:  "http://localhost:20102/serviceregistrar/registry",
	}
	orchestrator := components.CoreSystem{
		Name: "orchestrator",
		Url:  "http://localhost:20103/orchestrator/orchestration",
	}
	ca := components.CoreSystem{
		Name: "ca",
		Url:  "http://localhost:20100/ca/certification",
	}
	maitreD := components.CoreSystem{
		Name: "maitreD",
		Url:  "http://localhost:20101/maitreD/maitreD",
	}

	defaultConfig.CCoreS = []components.CoreSystem{leadingRegistrar, orchestrator, ca, maitreD}
	defaultConfig.CName = sys.Name
	defaultConfig.Protocols = sys.Husk.ProtoPort
	defaultConfigFile, err := os.Create("systemconfig.json")
	if err != nil {
		return fmt.Errorf("encountered error while creating config file: %v", err)
	}
	defer defaultConfigFile.Close()

	enc := json.NewEncoder(defaultConfigFile) // Create an encoder that allows writing to a file
	enc.SetIndent("", "     ")
	err = enc.Encode(defaultConfig) // Write defaultConfig template to file
	if err != nil {
		return fmt.Errorf("jsonEncode: %v", err)
	}
	return
}

// This is the config in string form from the original Configure()
var expectedConf string = `map[coreSystems:[map[coreSystem:serviceregistrar url:http://localhost:20102/serviceregistrar/registry] map[coreSystem:orchestrator url:http://localhost:20103/orchestrator/orchestration] map[coreSystem:ca url:http://localhost:20100/ca/certification] map[coreSystem:maitreD url:http://localhost:20101/maitreD/maitreD]] protocolsNports:map[coap:0 http:1234 https:0] systemname:testSystem unit_assets:[map[details:map[Test:[Test]] name:testUnitAsset services:[map[costUnit: definition:test details:map[Forms:[SignalA_v1a]] registrationPeriod:45 subpath:test]] traits:<nil>]]]`

type setupDefConfigParams struct {
	expectError bool
	testCase    string
	setup       func(*components.System) (err error)
	cleanup     func() (err error)
}

func TestSetupDefaultConfig(t *testing.T) {
	testParams := []setupDefConfigParams{
		// {expectError, testCase, setup(), cleanup()}
		{
			false,
			"Best case",
			func(sys *components.System) (err error) {
				err = createConfigNoTraits(sys, 1)
				return err
			},
			func() (err error) { return cleanup() },
		},
		{
			false,
			"Good case, asset has traits",
			func(sys *components.System) (err error) {
				err = createConfigHasTraits(sys)
				return err
			},
			func() (err error) { return cleanup() },
		},
	}

	// Start of test
	for _, c := range testParams {
		testSys := createTestSystem(false)

		// Setup
		err := c.setup(&testSys)
		if err != nil {
			t.Errorf("setup failed: %v", err)
		}

		// Test
		_, err = setupDefaultConfig(&testSys)
		if c.expectError != true {
			if err != nil {
				t.Errorf("Expected no errors in testcase '%s', got: %v", c.testCase, err)
			}
		} else {
			if err == nil {
				t.Errorf("expected errors in testcase '%s', got none", c.testCase)
			}
		}
		// Cleanup
		err = c.cleanup()
		if err != nil {
			t.Errorf("failed to remove 'systemconfig.json' in testcase '%s'", c.testCase)
		}
	}
}

// This test is to ensure that setupDefaultConfig() doesnt change the behaviour of of Config()
func TestSetupDefaultConfigCorrectness(t *testing.T) {
	testSys := createTestSystem(false)

	// Setup a default config with setupDefaultConfig() func
	defConf, err := setupDefaultConfig(&testSys)
	if err != nil {
		t.Errorf("error in setupDefaultConfig() in test: %v", err)
	}

	def, err := os.OpenFile("systemconfig.json", os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		t.Errorf("error while opening/creating systemconfig.json in test")
	}
	defer def.Close()
	// Write our defaultConfig to file with correct indent
	enc := json.NewEncoder(def)
	enc.SetIndent("", "     ")
	enc.Encode(defConf)

	// Decode to defaultConfig so we can compare the created default- and expected config
	var defaultConfig any
	def.Seek(0, 0)
	json.NewDecoder(def).Decode(&defaultConfig)
	os.Remove("systemconfig.json")

	// Check if defaultConfig converted to a string is the same as the expectedConf
	if fmt.Sprint(defaultConfig) != expectedConf {
		t.Errorf("systemconfig not equal")
	}
}

func cleanup() error {
	err := os.Remove("systemconfig.json")
	if err != nil {
		return err
	}
	return nil
}

type configureParams struct {
	expectError bool
	testCase    string
	setup       func(*components.System) (err error)
	cleanup     func() (err error)
}

func TestConfigure(t *testing.T) {
	testParams := []configureParams{
		// {expectError, testCase, setup(), cleanup()}
		{
			false,
			"Best case, one asset",
			func(sys *components.System) (err error) {
				err = createConfigNoTraits(sys, 1)
				return
			},
			func() (err error) { return cleanup() },
		},
		{
			true,
			"Can't open/create config",
			func(sys *components.System) (err error) {
				_, err = os.OpenFile("systemconfig.json", os.O_RDWR|os.O_CREATE, 0000)
				return
			},
			func() (err error) { return cleanup() },
		},
		{
			true,
			"Config missing",
			func(sys *components.System) (err error) { return nil },
			func() (err error) { return cleanup() },
		},
		{
			false,
			"No Assets in config",
			func(sys *components.System) (err error) {
				err = createConfigNoTraits(sys, 0)
				return
			},
			func() (err error) { return cleanup() },
		},
		{
			false,
			"Multiple Assets in config",
			func(sys *components.System) (err error) {
				err = createConfigNoTraits(sys, 3)
				return
			},
			func() (err error) { return cleanup() },
		},
	}

	// Start of test
	for _, testCase := range testParams {
		testSys := createTestSystem(false)

		// Setup
		err := testCase.setup(&testSys)
		if err != nil {
			t.Errorf("failed during setup: %v", err)
		}

		// Test
		_, err = Configure(&testSys)
		if testCase.expectError == false {
			if err != nil {
				t.Errorf("Expected no errors in '%s', got: %v", testCase.testCase, err)
			}
		} else {
			if err == nil {
				t.Errorf("Expected errors in testcase '%s' got none", testCase.testCase)
			}
		}

		//Cleanup
		err = testCase.cleanup()
		if err != nil {
			t.Errorf("failed to remove 'systemconfig.json' in testcase '%s'", testCase.testCase)
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
