package goldenmechanism

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const listingFixture = `package demo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
)

type listing struct {
	Items []string
}

type service struct{}

func anchorOne() {}

func anchorTwo() {}

func (s *service) Serve(w http.ResponseWriter, r *http.Request) error {
	listing, err := s.load(".")
	if err != nil {
		return err
	}
	sortParam := r.URL.Query().Get("sort")
	listing.applySortAndLimit(sortParam, "3", "1")
	if err := render(w, listing); err != nil {
		return err
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func (s *service) load(path string) (*listing, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	result := &listing{}
	for _, entry := range entries {
		result.Items = append(result.Items, entry.Name())
	}
	return result, nil
}

func (l *listing) applySortAndLimit(sortParam, limitParam, offsetParam string) {
	sort.Strings(l.Items)
	offset, _ := strconv.Atoi(offsetParam)
	if offset > 0 {
		l.Items = l.Items[offset:]
	}
	limit, _ := strconv.Atoi(limitParam)
	if limit > 0 {
		l.Items = l.Items[:limit]
	}
}

func render(w http.ResponseWriter, listing *listing) error {
	if err := json.NewEncoder(w).Encode(listing.Items); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, "done")
	return err
}
`

func TestProbeExtractsBoundedMechanismSyntax(t *testing.T) {
	repo := writeFixture(t)
	result, err := Probe(context.Background(), repo, fixturePlan())
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result.Partial {
		t.Fatalf("Probe() unexpectedly partial: %s", result.StopReason)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	functions := make(map[string]Function, len(result.Functions))
	for _, function := range result.Functions {
		functions[function.Symbol] = function
	}
	for _, symbol := range []string{
		"anchorOne",
		"anchorTwo",
		"service.Serve",
		"service.load",
		"listing.applySortAndLimit",
		"render",
	} {
		if _, exists := functions[symbol]; !exists {
			t.Errorf("missing function %q; got %v", symbol, sortedFunctionSymbols(result.Functions))
		}
	}
	for _, symbol := range []string{"service.load", "listing.applySortAndLimit", "render"} {
		if function := functions[symbol]; function.Seed || function.Depth != 1 || len(function.ReachedFromIDs) == 0 {
			t.Errorf("expanded function %q = seed %v depth %d reached_from %v", symbol, function.Seed, function.Depth, function.ReachedFromIDs)
		}
	}

	wantedOperations := map[string]bool{
		"direct_local_call":   false,
		"local_error_handoff": false,
		"url_query_get":       false,
		"read_directory":      false,
		"append":              false,
		"sort":                false,
		"slice":               false,
		"json_encode":         false,
		"plain_format":        false,
		"write_header":        false,
		"error_return":        false,
		"lexical_order":       false,
	}
	targets := make(map[string]bool)
	capabilities := make(map[semanticdiscovery.Capability]bool)
	for _, observation := range result.Observations {
		if _, wanted := wantedOperations[observation.Operation]; wanted {
			wantedOperations[observation.Operation] = true
		}
		if observation.TargetSymbol != "" {
			targets[observation.TargetSymbol] = true
		}
		capabilities[observation.Capability] = true
		for _, ref := range observation.Evidence {
			if ref.Location.Path != "listing.go" || ref.Location.Line <= 0 || ref.Location.Column <= 0 {
				t.Errorf("observation %q has non-local or incomplete evidence: %+v", observation.ID, ref.Location)
			}
		}
	}
	for operation, found := range wantedOperations {
		if !found {
			t.Errorf("missing operation %q", operation)
		}
	}
	for _, target := range []string{"service.load", "listing.applySortAndLimit", "render"} {
		if !targets[target] {
			t.Errorf("missing direct-call target %q; got %v", target, targets)
		}
	}
	for _, capability := range []semanticdiscovery.Capability{
		semanticdiscovery.CapabilityStatic,
		semanticdiscovery.CapabilityDirectCall,
		semanticdiscovery.CapabilitySequence,
		semanticdiscovery.CapabilityBranch,
		semanticdiscovery.CapabilityDataRead,
		semanticdiscovery.CapabilityDataWrite,
		semanticdiscovery.CapabilityDataTransformation,
		semanticdiscovery.CapabilityOutputEffect,
		semanticdiscovery.CapabilityErrorPath,
	} {
		if !capabilities[capability] {
			t.Errorf("missing capability %q", capability)
		}
	}
}

func TestProbeReturnsValidatedPartialResultAtFunctionLineBudget(t *testing.T) {
	repo := writeFixture(t)
	plan := fixturePlan()
	plan.Limits.MaxFunctionLines = 2

	result, err := Probe(context.Background(), repo, plan)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if !result.Partial || result.StopReason != StopFunctionLineLimit {
		t.Fatalf("partial = %v, stop_reason = %q; want true/%q", result.Partial, result.StopReason, StopFunctionLineLimit)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("partial Validate() error = %v", err)
	}
	for _, function := range result.Functions {
		if len(function.Source) > 2 {
			t.Errorf("function %q retained %d lines, limit 2", function.Symbol, len(function.Source))
		}
	}
}

func TestProbeReturnsValidatedPartialResultAtFunctionCountBudget(t *testing.T) {
	repo := writeFixture(t)
	plan := fixturePlan()
	plan.Limits.MaxFunctions = 1

	result, err := Probe(context.Background(), repo, plan)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if !result.Partial || result.StopReason != StopFunctionLimit {
		t.Fatalf("partial = %v, stop_reason = %q; want true/%q", result.Partial, result.StopReason, StopFunctionLimit)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("partial Validate() error = %v", err)
	}
	if result.Budget.ResolvedSeedCount != 1 {
		t.Fatalf("resolved seeds = %d, want 1", result.Budget.ResolvedSeedCount)
	}
	for _, resolution := range result.Seeds {
		if resolution.Status != SeedResolved && resolution.Status != SeedSkippedFunctionLimit {
			t.Errorf("seed %q status = %q", resolution.Seed.Symbol, resolution.Status)
		}
	}
}

func TestProbeAllowsOneExactSeed(t *testing.T) {
	repo := writeFixture(t)
	plan := fixturePlan()
	plan.Seeds = plan.Seeds[:1]
	result, err := Probe(context.Background(), repo, plan)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result.Budget.SeedCount != 1 || result.Budget.ResolvedSeedCount != 1 {
		t.Fatalf("seed budget = %+v", result.Budget)
	}
}

func TestProbeRetainsPlannerAssignedSeedDepth(t *testing.T) {
	repo := writeFixture(t)
	plan := fixturePlan()
	plan.Seeds = []Seed{{
		OriginFactID: "fact-load", OriginEvidenceID: "evidence-load",
		Path: "listing.go", Symbol: "service.load",
		Depth: 1, ReachedFromEvidenceID: "evidence-call-load",
	}}
	result, err := Probe(context.Background(), repo, plan)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result.Budget.MaxDepthReached != 1 {
		t.Fatalf("max depth = %d, want 1", result.Budget.MaxDepthReached)
	}
	if len(result.Functions) != 1 || result.Functions[0].Depth != 1 ||
		!equalStrings(result.Functions[0].PlannedFromEvidenceIDs, []string{"evidence-call-load"}) {
		t.Fatalf("planned function = %+v", result.Functions)
	}
}

func TestSeedPlannerFieldsAreBackwardCompatibleAndValidated(t *testing.T) {
	encoded, err := json.Marshal(Seed{
		OriginFactID: "fact", OriginEvidenceID: "evidence",
		Path: "listing.go", Symbol: "anchorOne",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "depth") || strings.Contains(string(encoded), "reached_from") {
		t.Fatalf("zero-value planner fields changed legacy json: %s", encoded)
	}

	plan := fixturePlan()
	plan.Seeds = []Seed{{
		OriginFactID: "fact", OriginEvidenceID: "evidence",
		Path: "listing.go", Symbol: "anchorOne", Depth: 1,
	}}
	if _, err := plan.normalized(); err == nil {
		t.Fatal("normalized() accepted a frontier seed without reached-from evidence")
	}
	plan.Seeds[0].Depth = 0
	plan.Seeds[0].ReachedFromEvidenceID = "unexpected"
	if _, err := plan.normalized(); err == nil {
		t.Fatal("normalized() accepted reached-from evidence on a root seed")
	}
}

func TestProbeIDsAreStable(t *testing.T) {
	repo := writeFixture(t)
	first, err := Probe(context.Background(), repo, fixturePlan())
	if err != nil {
		t.Fatalf("first Probe() error = %v", err)
	}
	second, err := Probe(context.Background(), repo, fixturePlan())
	if err != nil {
		t.Fatalf("second Probe() error = %v", err)
	}
	if got, want := artifactIDs(second), artifactIDs(first); !equalStrings(got, want) {
		t.Fatalf("artifact ids changed across replay\n got: %v\nwant: %v", got, want)
	}
}

func TestProbeRejectsInvalidPathAndExactSymbol(t *testing.T) {
	repo := writeFixture(t)

	invalidPath := fixturePlan()
	invalidPath.Seeds[0].Path = "../outside.go"
	if _, err := Probe(context.Background(), repo, invalidPath); err == nil {
		t.Fatal("Probe() accepted a path outside the repository")
	}

	invalidSymbol := fixturePlan()
	invalidSymbol.Seeds[0].Symbol = "missingFunction"
	if _, err := Probe(context.Background(), repo, invalidSymbol); err == nil {
		t.Fatal("Probe() accepted an absent exact seed symbol")
	}
}

func fixturePlan() Plan {
	return Plan{
		MechanismID: "listing-mechanism",
		Seeds: []Seed{
			{OriginFactID: "fact-serve", OriginEvidenceID: "evidence-serve", Path: "listing.go", Symbol: "(*service).Serve"},
			{OriginFactID: "fact-anchor-one", OriginEvidenceID: "evidence-anchor-one", Path: "listing.go", Symbol: "anchorOne"},
			{OriginFactID: "fact-anchor-two", OriginEvidenceID: "evidence-anchor-two", Path: "listing.go", Symbol: "anchorTwo"},
		},
		ExpansionAllowlist: []string{
			"service.Serve",
			"service.load",
			"listing.applySortAndLimit",
			"render",
		},
		Limits: Limits{
			MaxDepth: 2, MaxFiles: 1, MaxFunctions: 8, MaxSourceBytes: 64 * 1024,
			MaxFunctionLines: 100, MaxFunctionBytes: 16 * 1024, Timeout: time.Second,
		},
	}
}

func writeFixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "listing.go"), []byte(listingFixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return repo
}

func sortedFunctionSymbols(functions []Function) []string {
	result := make([]string, 0, len(functions))
	for _, function := range functions {
		result = append(result, function.Symbol)
	}
	sort.Strings(result)
	return result
}

func artifactIDs(result Result) []string {
	var ids []string
	for _, file := range result.Files {
		ids = append(ids, file.ID)
	}
	for _, function := range result.Functions {
		ids = append(ids, function.ID)
		for _, line := range function.Source {
			ids = append(ids, line.ID)
		}
	}
	for _, observation := range result.Observations {
		ids = append(ids, observation.ID)
		for _, ref := range observation.Evidence {
			ids = append(ids, ref.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
