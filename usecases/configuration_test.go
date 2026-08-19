package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

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
	mua := &components.UnitAsset{
		Name:        "testUnitAsset",
		Details:     map[string][]string{"Test": {"Test"}},
		ServicesMap: *ServicesMap,
		Traits:      map[string][]string{"Trait": {"testTrait"}},
	}
	sys.UAssets[mua.GetName()] = mua

	// If the asset exposes traits, serialize them and store as raw JSON
	if traits := assetTemplate.GetTraits(); traits != nil {
		traitJSON, err := json.Marshal(traits)
		if err == nil {
			confAsset.Traits = []json.RawMessage{traitJSON}
		} else {
			return err
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
	defaultConfig.IPAddresses = sys.Husk.Host.IPAddresses
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
	defaultConfig.IPAddresses = sys.Husk.Host.IPAddresses
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

// createConfigEmptyIPs writes a config file with an explicit empty ipAddresses
// list, simulating an old config created before IP persistence was added.
func createConfigEmptyIPs(sys *components.System) error {
	type minimalConfig struct {
		CName     string                  `json:"systemname"`
		Protocols map[string]int          `json:"protocolsNports"`
		CCoreS    []components.CoreSystem `json:"coreSystems"`
		Assets    []ConfigurableAsset     `json:"unit_assets"`
	}
	cfg := minimalConfig{
		CName:     sys.Name,
		Protocols: sys.Husk.ProtoPort,
		CCoreS: []components.CoreSystem{
			{Name: "serviceregistrar", Url: "http://localhost:20102/serviceregistrar/registry"},
		},
		Assets: []ConfigurableAsset{{Name: "testUnitAsset"}},
	}
	f, err := os.Create("systemconfig.json")
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "    ")
	return enc.Encode(cfg)
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

// TestConfigureIPAddresses verifies that IP addresses are correctly persisted
// and restored through the config file.
func TestConfigureIPAddresses(t *testing.T) {
	t.Run("IP addresses from config override discovered IPs", func(t *testing.T) {
		testSys := createTestSystem(false)

		// Write a config that includes the system's discovered IPs.
		if err := createConfigNoTraits(&testSys, 1); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer cleanup()

		// Simulate a network change by replacing the in-memory IPs with a sentinel value.
		testSys.Husk.Host.IPAddresses = []string{"9.9.9.9"}

		if _, err := Configure(&testSys); err != nil {
			t.Fatalf("Configure returned unexpected error: %v", err)
		}

		// The IPs should have been restored from the config, not the sentinel.
		for _, ip := range testSys.Husk.Host.IPAddresses {
			if ip == "9.9.9.9" {
				t.Error("expected config IPs to override discovered IPs, but sentinel value survived")
			}
		}
	})

	t.Run("Config without ipAddresses keeps discovered IPs", func(t *testing.T) {
		testSys := createTestSystem(false)

		// Write a config that has no ipAddresses field (old-style config).
		if err := createConfigEmptyIPs(&testSys); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer cleanup()

		discovered := make([]string, len(testSys.Husk.Host.IPAddresses))
		copy(discovered, testSys.Husk.Host.IPAddresses)

		if _, err := Configure(&testSys); err != nil && err != ErrNewConfig {
			t.Fatalf("Configure returned unexpected error: %v", err)
		}

		if len(testSys.Husk.Host.IPAddresses) != len(discovered) {
			t.Errorf("expected %d IPs, got %d", len(discovered), len(testSys.Husk.Host.IPAddresses))
		}
		for i, ip := range testSys.Husk.Host.IPAddresses {
			if ip != discovered[i] {
				t.Errorf("IP[%d]: expected %s, got %s", i, discovered[i], ip)
			}
		}
	})
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
	mua := components.UnitAsset{
		Name:        "test",
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

// The seeded core-system URLs name the host by its own address, not localhost.
//
// Both mean the same machine on a single-host cloud, and localhost was fine
// while these URLs were only ever http. They are not: a consumer that reaches
// the orchestrator over https presents a client certificate, and the certificate
// the CA issues carries the host's addresses — so https://localhost:30103 fails
// to verify against a certificate that never mentions localhost. On the running
// testbed this was the whole of why authorization refused everything: the quest
// arrived over http, the subject was the empty string, and no policy can name
// "".
func TestSeededCoreSystemsNameTheHostNotLocalhost(t *testing.T) {
	sys := components.NewSystem("ds18b20", context.Background())
	sys.Husk = &components.Husk{
		Host:      &components.HostingDevice{IPAddresses: []string{"192.168.1.10", "127.0.0.1"}},
		ProtoPort: map[string]int{"http": 20150, "https": 30150},
		Details:   map[string][]string{},
	}
	sys.UAssets["sensor"] = &components.UnitAsset{Name: "sensor", Mission: components.MissionMeasurement}

	config, err := setupDefaultConfig(&sys)
	if err != nil {
		t.Fatalf("building the default configuration: %v", err)
	}

	seen := map[string]string{}
	for _, core := range config.CCoreS {
		seen[core.Name] = core.Url
		if strings.Contains(core.Url, "localhost") {
			t.Errorf("%s is seeded as %q; a certificate does not mention localhost", core.Name, core.Url)
		}
	}
	for _, name := range []string{"serviceregistrar", "orchestrator", "ca"} {
		url, present := seen[name]
		if !present {
			t.Errorf("%s is not in the generated configuration", name)
			continue
		}
		if !strings.HasPrefix(url, "http://192.168.1.10:") {
			t.Errorf("%s is seeded as %q; want the host's own address", name, url)
		}
	}

	// The authorizer has a slot and no URL: visible to an operator, inert until
	// filled. A real URL here would make every generated system refuse to serve
	// anything but its core services until an authorizer existed.
	url, present := seen["authorizer"]
	if !present {
		t.Fatal("the authorizer has no slot, so an operator must know the JSON shape to add one")
	}
	if url != "" {
		t.Errorf("the authorizer is seeded as %q; adoption is per deployment", url)
	}
}
