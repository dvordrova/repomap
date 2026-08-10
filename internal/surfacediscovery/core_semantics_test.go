package surfacediscovery

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyFrameworkCoverageRemainsDecodable(t *testing.T) {
	var coverage SurfaceCoverage
	if err := json.Unmarshal([]byte(`{
		"cobra_descriptor_count": 3,
		"cobra_record_count": 2,
		"framework_matched": {"cobra": 2, "gin": 1}
	}`), &coverage); err != nil {
		t.Fatal(err)
	}
	if coverage.CobraDescriptorCount != 3 || coverage.CobraRecordCount != 2 ||
		coverage.FrameworkMatched["cobra"] != 2 || coverage.FrameworkMatched["gin"] != 1 {
		t.Fatalf("legacy framework coverage was not preserved: %#v", coverage)
	}
}

func TestAnalyzeCoreKeepsGenericSSAAndTypedDiscovery(t *testing.T) {
	result, err := Analyze(DefaultOptions(filepath.Join("testdata", "custom_router")))
	if err != nil {
		t.Fatal(err)
	}

	typedRoutes, netHTTPServers := 0, 0
	for _, trigger := range result.Catalog.Triggers {
		if trigger.Producer == "typed_registration_detector" && trigger.Kind == "http_route" {
			typedRoutes++
		}
		if trigger.Framework == "net/http" && trigger.Kind == "http_server" {
			netHTTPServers++
		}
		if trigger.Framework != "" && trigger.Framework != "net/http" &&
			trigger.Framework != "errgroup" && trigger.Framework != "typed" {
			t.Fatalf("non-core trigger reached ordinary analysis: %#v", trigger)
		}
	}
	if typedRoutes != 1 || netHTTPServers != 1 {
		t.Fatalf("core routes/servers = %d/%d, want 1/1: %#v", typedRoutes, netHTTPServers, result.Catalog.Triggers)
	}
	if result.DirectCallIndex == nil || result.DirectCallIndex.State != DirectCallIndexReady ||
		len(result.DirectCallIndex.Nodes) == 0 {
		t.Fatalf("core DirectCallIndex was disabled: %#v", result.DirectCallIndex)
	}
	if strings.Contains(result.Coverage.ScopeStatement, "Cobra") ||
		strings.Contains(result.Coverage.ScopeStatement, "framework") {
		t.Fatalf("core scope claims removed producers: %q", result.Coverage.ScopeStatement)
	}
}

func TestAnalyzeCoreSkipsCobraInventory(t *testing.T) {
	result, err := Analyze(DefaultOptions(filepath.Join("testdata", "cobra")))
	if err != nil {
		t.Fatal(err)
	}
	for _, trigger := range result.Catalog.Triggers {
		if trigger.Kind == "cli_command" || trigger.Framework == "cobra" {
			t.Fatalf("Cobra inventory reached ordinary analysis: %#v", trigger)
		}
	}
	if result.Coverage.CobraDescriptorCount != 0 || result.Coverage.CobraRecordCount != 0 {
		t.Fatalf("fresh Cobra coverage reached ordinary analysis: %#v", result.Coverage)
	}
	for _, phase := range result.Coverage.Phases {
		if phase.Phase == "cobra_inventory" {
			t.Fatalf("removed Cobra producer still ran: %#v", phase)
		}
	}
	if result.DirectCallIndex == nil || result.DirectCallIndex.State != DirectCallIndexReady {
		t.Fatalf("DirectCallIndex was disabled for Cobra-using source: %#v", result.DirectCallIndex)
	}
}
