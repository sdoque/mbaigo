package usecases

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

// A mocked UnitAsset used for testing
type mockUnitAssetWithTraits struct {
	Name        string              `json:"name"`    // Must be a unique name, ie. a sensor ID
	Owner       *components.System  `json:"-"`       // The parent system this UA is part of
	Details     map[string][]string `json:"details"` // Metadata or details about this UA
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
		assetTemplate = *ua // this creates a copy (value, not reference)
		break               // stop after the first entry
	}
	servicesTemplate := getServicesList(assetTemplate)

	confAsset := ConfigurableAsset{
		Name:     assetTemplate.GetName(),
		Details:  assetTemplate.GetDetails(),
		Services: servicesTemplate,
	}

	sys.UAssets = make(map[string]*components.UnitAsset)
	if assetTrait == true {
		// Create one asset with traits, if it's needed for test
		testCerv := &components.Cervice{
			Definition: "testCerv",
			Details:    map[string][]string{"Forms": {"SignalA_v1a"}},
			Nodes:      map[string][]string{},
		}
		CervicesMap := &components.Cervices{
			testCerv.Definition: testCerv,
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
			CervicesMap: *CervicesMap,
			Traits:      map[string][]string{"Trait": {"testTrait"}},
		}
		var muaInterface components.UnitAsset = mua
		sys.UAssets[mua.GetName()] = &muaInterface
	}

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
	for x := range assetAmount {
		// Create one asset with traits, if it's needed for test
		testCerv := &components.Cervice{
			Definition: fmt.Sprintf("testCerv%d", x),
			Details:    map[string][]string{"Forms": {"SignalA_v1a"}},
			Nodes:      map[string][]string{},
		}
		CervicesMap := &components.Cervices{
			testCerv.Definition: testCerv,
		}
		setTest := &components.Service{
			ID:            x,
			Definition:    fmt.Sprintf("test%d", x),
			SubPath:       fmt.Sprintf("test%d", x),
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
			Name:        fmt.Sprintf("testUnitAsset%d", x),
			Details:     map[string][]string{"Test": {"Test"}},
			ServicesMap: *ServicesMap,
			CervicesMap: *CervicesMap,
			Traits:      map[string][]string{"Trait": {"testTrait"}},
		}

		var muaInterface components.UnitAsset = mua
		sys.UAssets[mua.GetName()] = &muaInterface
	}

	defaultConfig.CName = sys.Name
	defaultConfig.Protocols = sys.Husk.ProtoPort
	defaultConfig.Assets = []ConfigurableAsset{confAsset} // this is a list of unit assets

	os.Remove("systemconfig.json")
	defaultConfigFile, err := os.Create("systemconfig.json")
	if err != nil {
		log.Fatalf("Encountered error while creating default config file")
	}
	defer defaultConfigFile.Close()

	enc := json.NewEncoder(defaultConfigFile) // Create an encoder that allows writing to a file
	enc.SetIndent("", "     ")                // Set proper indentation
	err = enc.Encode(defaultConfig)           // Write defaultConfig template to file
	if err != nil {
		log.Fatalf("jsonEncode: %v", err)
	}
}

func TestConfigure(t *testing.T) {
	// 1, Best case
	testSys := createTestSystem(false)
	createConfig(&testSys, false, 1)
	_, err := Configure(&testSys)
	if err != nil {
		t.Errorf("Expected no errors in best case, got: %#v", err)
	}

	// 1.1, Asset has traits, good case
	createConfig(&testSys, true, 0)
	_, err = Configure(&testSys)
	if err != nil {
		t.Errorf("Expected no errors while having traits, got: %#v", err)
	}

	// 2, Asset with traits (can't break Marshal currently)

	// 3, Missing config
	os.Remove("systemconfig.json")
	_, err = Configure(&testSys)
	if err == nil {
		t.Errorf("Expected error because of missing config")
	}

	// 3.1, Missing config ( Breaking os.Create() )
	os.Remove("systemconfig.json")
	// Create a file that noone has permissions to open, should break os.Open() & os.Create()
	os.OpenFile("systemconfig.json", os.O_RDWR|os.O_CREATE, 0000)
	_, err = Configure(&testSys)
	if err == nil {
		t.Errorf("Expected error while creating config")
	}

	// 3.2, Missing config ( Breaking encode to file )
	// This could be done by including a test hook (os.Chmod(path, 0444)) which will change
	// permission to read only, before the it starts writing with encode

	// 4, Config file could be opened, but it failed while reading
	// Could be done like above, change permission to 0000 so it cant be read

	// 5, if len(aux.Resources) == 0, under Alias part
	// no unit_assets present in the system
	createConfig(&testSys, false, 0)
	_, err = Configure(&testSys)
	if err != nil {
		t.Errorf("Expected no errors")
	}

}

func TestGetServiceList(t *testing.T) {
	// getServicesList(uat components.UnitAsset) []components.Service
	testSys := createTestSystem(false)
	ua := (*testSys.UAssets["testUnitAsset"])
	servList := getServicesList(ua)
	if len(servList) != 1 && servList[0].Definition != "test" {
		t.Errorf("Expected length: 1, got %d\tExpected 'Definition': test, got %s", len(servList), servList[0].Definition)
	}
}

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
			t.Errorf(`Expected servMap["%s"].SubPath to be "%s", with ID: "%d". Got Subpath: "%s", with ID: "%d"`, service, service, c, servMap[service].SubPath, servMap[service].ID)
		}
	}
}
