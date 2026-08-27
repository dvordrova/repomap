package report

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestBrowserTargetPayloadProjectionIsTypedDeterministicAndTrimmed(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	canonicalBefore, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}

	first, err := ProjectBrowserTargetPayload(&data)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProjectBrowserTargetPayload(&data)
	if err != nil {
		t.Fatal(err)
	}
	firstRaw, err := EncodeBrowserTargetPayload(first)
	if err != nil {
		t.Fatal(err)
	}
	secondRaw, err := EncodeBrowserTargetPayload(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstRaw, secondRaw) {
		t.Fatal("target projection is not deterministic")
	}
	canonicalAfter, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonicalBefore, canonicalAfter) {
		t.Fatal("target projection mutated canonical ReportData")
	}

	decoded, err := DecodeBrowserTargetPayload(firstRaw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, first) {
		t.Fatalf("target payload round trip changed value\nwant: %#v\n got: %#v", first, decoded)
	}
	if decoded.Features.Core == nil || decoded.Features.Entrypoints == nil ||
		decoded.Features.Integrations == nil || decoded.Features.ActivityPaths == nil {
		t.Fatal("complete semantic feature family was not projected")
	}
	for _, used := range []string{
		`"features":`, `"program":`, `"objects":`, `"relations":`, `"projection":`,
		`"refined_core":`, `"refined_groups":`, `"entrypoints":`, `"dependencies":`,
		`"routes":`, `"outcomes":`,
	} {
		if !bytes.Contains(firstRaw, []byte(used)) {
			t.Errorf("browser-used field %s is absent", used)
		}
	}
	for _, dead := range []string{
		"format_version", "program_portfolio", "analysis_target", "cube_map_view",
		"runtime_portfolio", "target_outcome_portfolio", "captured_input_count",
		"index_sha256", "program_index_sha256", "semantic_state", "index_coverage",
		"targets_observed", "targets_indexed", "targets_omitted", "visibility",
		"baseline_core", "declared_candidates", "declaration_coverage",
	} {
		if bytes.Contains(firstRaw, []byte(`"`+dead+`"`)) {
			t.Errorf("browser-dead field %q leaked into target payload", dead)
		}
	}
}

func TestBrowserRepositoryPayloadProjectsSharedIndexAndAuthorizedRoutes(t *testing.T) {
	data, navigation := targetNavigationFixture(t)
	payload, err := ProjectBrowserRepositoryPayload(data, navigation)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Targets) != len(navigation.Targets) || payload.Runtime != nil {
		t.Fatalf("repository shared projection = %#v", payload)
	}
	for index := range payload.Targets {
		payload.Targets[index].Href = "?target=" + string(rune('0'+index)) + "#/program"
	}
	if err := payload.Validate(); err != nil {
		t.Fatalf("standalone target routes were rejected: %v", err)
	}
	raw, err := EncodeBrowserRepositoryPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBrowserRepositoryPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, payload) {
		t.Fatalf("repository payload round trip changed value\nwant: %#v\n got: %#v", payload, decoded)
	}
	for _, targetLocal := range []string{
		`"features":`, `"program":`, `"objects":`, `"relations":`,
		`"refined_core":`, `"entrypoints":`, `"dependencies":`, `"activity_paths":`,
	} {
		if bytes.Contains(raw, []byte(targetLocal)) {
			t.Errorf("target-local field %s leaked into repository payload", targetLocal)
		}
	}
	invalid := payload
	invalid.Targets = append([]BrowserTargetIndexItem(nil), payload.Targets...)
	invalid.Targets[0].Href = "?target=01#/program"
	if err := invalid.Validate(); err == nil {
		t.Fatal("non-canonical standalone target route was accepted")
	}
}

func TestBrowserRepositoryPayloadRejectsIncompleteRuntimeTargetCoverage(t *testing.T) {
	payload := BrowserRepositoryPayload{
		Version: BrowserRepositoryPayloadVersion,
		Repository: BrowserRepository{
			Name: "fixture", CapturedRevision: strings.Repeat("a", 40),
		},
		Source:                         BrowserSource{Kind: "none"},
		LogicalDefaultSelectedTargetID: "selected-a",
		Targets: []BrowserTargetIndexItem{
			{SelectedTargetID: "selected-a", ProgramTargetID: "program-a", Language: "go", Kind: "library", DisplayName: "a", State: "analyzed", Href: "?target=0#/program"},
			{SelectedTargetID: "selected-b", ProgramTargetID: "program-b", Language: "go", Kind: "library", DisplayName: "b", State: "analyzed", Href: "?target=1#/program"},
		},
		Runtime: &BrowserRuntimeOverview{
			Roles: []BrowserRuntimeRole{{
				Name: "Runtime", Purpose: "Runs one target.", Prominence: "primary",
				RoleKind: "service", Requiredness: "required", Confidence: "high",
				Implementations: []BrowserRuntimeImplementation{{ProgramTargetID: "program-a"}},
				Evidence:        []BrowserLocation{},
			}},
			UnclassifiedTargets: []BrowserRuntimeUnclassifiedTarget{},
		},
		OpenablePaths: []string{},
	}
	if err := payload.Validate(); err == nil || !strings.Contains(err.Error(), "cover every analyzed target") {
		t.Fatalf("incomplete runtime target coverage error = %v", err)
	}
}

func TestBrowserPayloadCodecsRejectUnknownFieldsVersionsAndTrailingValues(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	payload, err := ProjectBrowserTargetPayload(&data)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeBrowserTargetPayload(payload)
	if err != nil {
		t.Fatal(err)
	}

	unknown := append([]byte(nil), raw[:len(raw)-1]...)
	unknown = append(unknown, []byte(`,"unknown":true}`)...)
	if _, err := DecodeBrowserTargetPayload(unknown); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	wrongVersion := bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":2`), 1)
	if _, err := DecodeBrowserTargetPayload(wrongVersion); err == nil ||
		!strings.Contains(err.Error(), "unsupported version") {
		t.Fatalf("wrong version error = %v", err)
	}
	if _, err := DecodeBrowserTargetPayload(append(raw, []byte(` {}`)...)); err == nil ||
		!strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("trailing value error = %v", err)
	}
}

func TestBrowserTargetProjectionRejectsSourceOutsideCanonicalOpenability(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	data.OpenablePaths = []string{}
	if _, err := ProjectBrowserTargetPayload(&data); err == nil ||
		!strings.Contains(err.Error(), "outside canonical openability") {
		t.Fatalf("missing canonical source authority error = %v", err)
	}
}

func TestBrowserTargetProjectionKeepsCoreRepresentativesOutsideBoundedProgramView(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	if data.CoreMapView == nil || len(data.CoreMapView.RefinedCore) == 0 ||
		len(data.CoreMapView.RefinedCore[0].RepresentativeSymbols) == 0 {
		t.Fatal("fixture lacks a core representative")
	}
	representative := &data.CoreMapView.RefinedCore[0].RepresentativeSymbols[0]
	representative.Symbol.NodeID = "program-object-omitted-from-bounded-view"

	payload, err := ProjectBrowserTargetPayload(&data)
	if err != nil {
		t.Fatal(err)
	}
	if got := payload.Features.Core.RefinedCore[0].RepresentativeSymbols[0].Symbol.NodeID; got != representative.Symbol.NodeID {
		t.Fatalf("representative identity = %q", got)
	}
}
