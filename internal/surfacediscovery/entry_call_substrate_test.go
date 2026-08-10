package surfacediscovery

import (
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/entrycall"
)

func TestEntryCallSubstrateReusesSurfacePassAndGroupsExternalStaticFamily(t *testing.T) {
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/entry-call\n\ngo 1.25\n")
	writeFixtureFile(t, filepath.Join(repository, "main.go"), `package main

import "net/http"

func main() { initAPI() }

func initAPI() {
	http.HandleFunc("/first", handler)
	http.HandleFunc("/second", handler)
}

func handler(http.ResponseWriter, *http.Request) {}
`)
	options := DefaultOptions(repository)
	options.CaptureEntryCallSubstrate = true
	result, err := Analyze(options)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.EntryCallSubstrate == nil || result.EntryCallSubstrate.State != entrycall.StateReady {
		t.Fatalf("entry-call substrate = %+v, want ready", result.EntryCallSubstrate)
	}
	substrate := result.EntryCallSubstrate
	if len(substrate.Roots) != 1 {
		t.Fatalf("roots = %+v, want exact process main", substrate.Roots)
	}
	foundMainConnector := false
	foundHandleFunc := false
	for _, family := range substrate.Families {
		var caller, callee entrycall.ExactNode
		for _, node := range substrate.Nodes {
			if node.ID == family.CallerID {
				caller = node
			}
			if node.ID == family.CalleeID {
				callee = node
			}
		}
		if caller.Label == "entry-call · main" && callee.Label == "entry-call · initAPI" {
			foundMainConnector = family.WitnessCount == 1
		}
		if caller.Label == "entry-call · initAPI" && callee.Label == "http · HandleFunc" {
			foundHandleFunc = family.WitnessCount == 2 && len(family.Callsites) == 2 && callee.External
		}
	}
	if !foundMainConnector || !foundHandleFunc {
		t.Fatalf("families = %+v nodes=%+v frontiers=%+v direct_coverage=%+v, want main connector plus grouped external HandleFunc x2", substrate.Families, substrate.Nodes, substrate.Frontiers, result.DirectCallIndex.Coverage)
	}
	if len(substrate.SurfaceCandidates) != 2 || substrate.Coverage.SurfaceCandidatesIndexed != 2 {
		t.Fatalf("surface candidates = %+v coverage=%+v, want two exact HandleFunc callsites", substrate.SurfaceCandidates, substrate.Coverage)
	}
	compilation, err := entrycall.Compile(substrate.Snapshot())
	if err != nil {
		t.Fatalf("Compile captured substrate: %v", err)
	}
	if compilation.AdvertisedFamilyCount() < 2 {
		t.Fatalf("advertised families = %d, want connected external family", compilation.AdvertisedFamilyCount())
	}
	withoutCapture := options
	withoutCapture.CaptureEntryCallSubstrate = false
	withoutResult, err := Analyze(withoutCapture)
	if err != nil {
		t.Fatalf("Analyze without capture: %v", err)
	}
	if withoutResult.EntryCallSubstrate != nil {
		t.Fatalf("no-consumer run retained entry-call substrate: %+v", withoutResult.EntryCallSubstrate)
	}
	if withoutResult.DirectCallIndex == nil || result.DirectCallIndex == nil ||
		withoutResult.DirectCallIndex.SHA256 != result.DirectCallIndex.SHA256 {
		t.Fatalf("capture changed authoritative DirectCallIndex: with=%+v without=%+v", result.DirectCallIndex, withoutResult.DirectCallIndex)
	}
}
