package usecases

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

func createDefaultConfig(sys components.System) {
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

	defaultConfig.CName = sys.Name
	defaultConfig.Protocols = sys.Husk.ProtoPort
	defaultConfig.Assets = []ConfigurableAsset{confAsset} // this is a list of unit assets

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
	os.Remove("systemconfig.json")
	testSys := createTestSystem(false)
	createDefaultConfig(testSys)
	_, err := Configure(&testSys)
	if err != nil {
		t.Errorf("Expected no errors in best case, got: %#v", err)
	}

	// 2, Missing config
	os.Remove("systemconfig.json")
	_, err = Configure(&testSys)
	if err == nil {
		t.Errorf("Expected error because of missing config")
	}

	// 2.1, Missing config ( Breaking os.Create() )
	os.Remove("systemconfig.json")
	// Create a file that noone has permissions to open, should break os.Open()
	os.OpenFile("systemconfig.json", os.O_RDWR|os.O_CREATE, 0000)
	_, err = Configure(&testSys)
	if err == nil {
		t.Errorf("Expected error while creating config")
	}

	// 2.2, Missing config ( Breaking encode to file )
	// Allow read, but not write?
	
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
