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

// --------------------------------------------------------- //
// Helpfunctions that creates a default config file
// with/without any asset traits
// --------------------------------------------------------- //

// This is pretty much a copy of setupDefaultConfig() in configuration.go,
// but this also creates and writes to a systemconfig.json file
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
	defaultConfig.Assets = []ConfigurableAsset{confAsset}

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

	enc := json.NewEncoder(defaultConfigFile)
	enc.SetIndent("", "     ")
	err = enc.Encode(defaultConfig)
	if err != nil {
		return fmt.Errorf("jsonEncode: %v", err)
	}
	return
}

// This is pretty much a copy of setupDefaultConfig() in configuration.go,
// but this also creates and writes to a systemconfig.json file
func createConfigNoTraits(sys *components.System, assetAmount int) (err error) {
	var defaultConfig templateOut

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

	enc := json.NewEncoder(defaultConfigFile)
	enc.SetIndent("", "     ")
	err = enc.Encode(defaultConfig)
	if err != nil {
		return fmt.Errorf("jsonEncode: %v", err)
	}
	return
}

// --------------------------------------------------------- //
// Helpfunctions and structs for testing SetupDefaultConfig()
// --------------------------------------------------------- //

func cleanup() error {
	return os.Remove("systemconfig.json")
}

type setupDefConfigParams struct {
	expectError bool
	setup       func(*components.System) (err error)
	cleanup     func() (err error)
	testCase    string
}

func TestSetupDefaultConfig(t *testing.T) {
	testParams := []setupDefConfigParams{
		{
			false,
			func(sys *components.System) (err error) { return createConfigNoTraits(sys, 1) },
			func() (err error) { return cleanup() },
			"Best case",
		},
		{
			false,
			func(sys *components.System) (err error) { return createConfigHasTraits(sys) },
			func() (err error) { return cleanup() },
			"Good case, asset has traits",
		},
		{
			true,
			func(sys *components.System) (err error) { return createConfigHasTraits(sys) },
			func() (err error) { return cleanup() },
			"No assets in sys",
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

		if c.testCase == "No assets in sys" {
			testSys.UAssets = nil
		}

		// Test
		_, err = setupDefaultConfig(&testSys)
		if c.expectError == false && err != nil {
			t.Errorf("Expected no errors in testcase '%s', got: %v", c.testCase, err)
		}
		if c.expectError == true && err == nil {
			t.Errorf("expected errors in testcase '%s', got none", c.testCase)
		}

		// Cleanup
		err = c.cleanup()
		if err != nil {
			t.Errorf("failed to remove 'systemconfig.json' in testcase '%s': %v", c.testCase, err)
		}
	}
}

// --------------------------------------------------------- //
// Helpfunctions and structs for testing Configure()
// --------------------------------------------------------- //

type configureParams struct {
	expectError bool

	setup    func(*components.System) (err error)
	cleanup  func() (err error)
	testCase string
}

func TestConfigure(t *testing.T) {
	testParams := []configureParams{
		{
			false,
			func(sys *components.System) (err error) { return createConfigNoTraits(sys, 1) },
			func() (err error) { return cleanup() },
			"Best case, one asset",
		},
		{
			true,
			func(sys *components.System) (err error) {
				_, err = os.OpenFile("systemconfig.json", os.O_RDWR|os.O_CREATE, 0000)
				return
			},
			func() (err error) { return cleanup() },
			"Can't open/create config",
		},
		{
			true,
			func(sys *components.System) (err error) { return nil },
			func() (err error) { return cleanup() },
			"Config missing",
		},
		{
			false,
			func(sys *components.System) (err error) { return createConfigNoTraits(sys, 0) },
			func() (err error) { return cleanup() },
			"No Assets in config",
		},
		{
			false,
			func(sys *components.System) (err error) { return createConfigNoTraits(sys, 3) },
			func() (err error) { return cleanup() },
			"Multiple Assets in config",
		},
		{
			true,
			func(sys *components.System) (err error) {
				sys.UAssets = nil
				return createConfigNoTraits(sys, 1)
			},
			func() (err error) { return cleanup() },
			"No assets in sys",
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
		if testCase.expectError == false && err != nil {
			t.Errorf("Expected no errors in '%s', got: %v", testCase.testCase, err)
		}
		if testCase.expectError == true && err == nil {
			t.Errorf("Expected errors in testcase '%s'", testCase.testCase)
		}

		//Cleanup
		err = testCase.cleanup()
		if err != nil {
			t.Errorf("failed to remove 'systemconfig.json' in testcase '%s'", testCase.testCase)
		}
	}
}

// --------------------------------------------------------- //
// Testing GetServiceList()
// --------------------------------------------------------- //

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
	if len(servList) != 1 || servList[0].Definition != "test" {
		t.Errorf("Expected length: 1, got %d\tExpected 'Definition': test, got %s",
			len(servList), servList[0].Definition)
	}
}

// --------------------------------------------------------- //
// Testing MakeServiceMap()
// --------------------------------------------------------- //

func TestMakeServiceMap(t *testing.T) {
	var servList []components.Service
	for x := range 6 {
		serv := components.Service{
			ID:            x,
			Definition:    fmt.Sprintf("testDef%d", x),
			SubPath:       fmt.Sprintf("test%d", x),
			Details:       map[string][]string{"Forms": {"SignalA_v1a"}},
			Description:   fmt.Sprintf("test service %d", x),
			RegPeriod:     45,
			RegTimestamp:  "now",
			RegExpiration: "45",
		}
		servList = append(servList, serv)
	}
	servMap := MakeServiceMap(servList)
	for c := range 6 {
		service := fmt.Sprintf("test%d", c)
		if servMap[service].SubPath != service || servMap[service].ID != c {
			t.Errorf(`Expected servMap["%s"].SubPath to be "%s", with ID: "%d". Got: "%s", with ID: "%d"`,
				service, service, c, servMap[service].SubPath, servMap[service].ID)
		}
	}
}
