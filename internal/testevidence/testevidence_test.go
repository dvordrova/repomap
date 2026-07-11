package testevidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
)

type fakeFinder struct {
	locations map[string][]evidence.Location
	errors    map[string]error
}

func (f fakeFinder) References(_ context.Context, _ string, location evidence.Location) (evidence.LocationSet, error) {
	key := locationKey(location)
	if err := f.errors[key]; err != nil {
		return evidence.LocationSet{}, err
	}
	return staticLocations(f.locations[key]), nil
}

func TestCollectLinksClaimsToTestReferences(t *testing.T) {
	t.Parallel()

	structural, assessment, report := fixtures(t)
	finder := fakeFinder{
		locations: map[string][]evidence.Location{
			"server/key.go:90": {
				{Path: "server/key.go", Line: 100, Column: 2},
				{Path: "server/key_test.go", Line: 20, Column: 4},
			},
			"server/validation.go:10": {
				{Path: "server/validation_test.go", Line: 30, Column: 6},
				{Path: "server/validation_test.go", Line: 30, Column: 6},
			},
		},
		errors: map[string]error{
			"server/errors.go:20": errors.New("gopls unavailable"),
		},
	}
	bundle, err := Collect(context.Background(), finder, "/repo", structural, assessment, report, Options{MaxSearches: 3})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(bundle.Searches) != 3 {
		t.Fatalf("searches = %#v", bundle.Searches)
	}
	if bundle.Searches[1].Predicate != sourceexplain.PredicateValidatesInput ||
		bundle.Searches[2].Predicate != sourceexplain.PredicateMapsError {
		t.Fatalf("ranked searches = %#v", bundle.Searches)
	}
	if len(bundle.References) != 2 {
		t.Fatalf("references = %#v", bundle.References)
	}
	for index, reference := range bundle.References {
		if reference.EvidenceID != fmt.Sprintf("test-ref-%03d", index+1) ||
			reference.Kind != EvidenceKindTestReference ||
			len(reference.Provenance) == 0 ||
			len(reference.Scenarios) == 0 {
			t.Fatalf("reference = %#v", reference)
		}
	}
	if !hasWarning(bundle.Warnings, "searches.truncated") || !hasWarning(bundle.Warnings, "references.failed") {
		t.Fatalf("warnings = %#v", bundle.Warnings)
	}
}

func TestCollectTruncatesReferencesPerSearch(t *testing.T) {
	t.Parallel()

	structural, assessment, report := fixtures(t)
	finder := fakeFinder{locations: map[string][]evidence.Location{
		"server/key.go:90": {
			{Path: "server/a_test.go", Line: 1, Column: 1},
			{Path: "server/b_test.go", Line: 2, Column: 1},
		},
	}}
	bundle, err := Collect(context.Background(), finder, "/repo", structural, assessment, report, Options{
		MaxSearches:            1,
		MaxReferencesPerSearch: 1,
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(bundle.References) != 1 || bundle.References[0].Path != "server/a_test.go" {
		t.Fatalf("references = %#v", bundle.References)
	}
	if !hasWarning(bundle.Warnings, "references.truncated") {
		t.Fatalf("warnings = %#v", bundle.Warnings)
	}
}

func TestCollectRequiresMatchingTargets(t *testing.T) {
	t.Parallel()

	structural, assessment, report := fixtures(t)
	report.Target.Path = "other.go"
	if _, err := Collect(context.Background(), fakeFinder{}, "/repo", structural, assessment, report, Options{}); err == nil {
		t.Fatal("Collect() error = nil")
	}
}

func TestCollectRejectsReportNotValidatedAgainstAssessment(t *testing.T) {
	t.Parallel()

	structural, assessment, report := fixtures(t)
	report.Claims[0].SourceEvidenceIDs = []string{"source-999"}
	if _, err := Collect(context.Background(), fakeFinder{}, "/repo", structural, assessment, report, Options{}); err == nil {
		t.Fatal("Collect() accepted a report with invented source evidence")
	}
}

func TestBundleValidateRejectsTestSupportedOverclaim(t *testing.T) {
	t.Parallel()

	structural, assessment, report := fixtures(t)
	bundle, err := Collect(context.Background(), fakeFinder{locations: map[string][]evidence.Location{
		"server/key.go:90": {{Path: "server/key_test.go", Line: 20, Column: 4}},
	}}, "/repo", structural, assessment, report, Options{MaxSearches: 1})
	if err != nil {
		t.Fatal(err)
	}
	bundle.References[0].Kind = EvidenceKind("test_supported")
	if err := bundle.Validate(); err == nil {
		t.Fatal("Validate() accepted test-supported overclaim")
	}
}

func TestCollectRejectsUnprovenancedReferenceResult(t *testing.T) {
	t.Parallel()

	structural, assessment, report := fixtures(t)
	finder := invalidFinder{}
	bundle, err := Collect(context.Background(), finder, "/repo", structural, assessment, report, Options{MaxSearches: 1})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(bundle.References) != 0 || !hasWarning(bundle.Warnings, "references.invalid") {
		t.Fatalf("bundle = %#v", bundle)
	}
}

type invalidFinder struct{}

func (invalidFinder) References(context.Context, string, evidence.Location) (evidence.LocationSet, error) {
	return evidence.LocationSet{
		Locations: []evidence.Location{{Path: "server/key_test.go", Line: 20, Column: 4}},
		Certainty: evidence.CertaintyStatic,
	}, nil
}

func fixtures(t *testing.T) (symbol.Bundle, sourceexplain.Bundle, sourceexplain.Report) {
	t.Helper()

	targetEntity := evidence.Entity{
		ID:       "target",
		Kind:     evidence.EntityMethod,
		Name:     "kvServer.Put",
		Location: &evidence.Location{Path: "server/key.go", Line: 90, Column: 20},
	}
	structural := symbol.Bundle{
		Version: symbol.BundleVersion,
		Target: symbol.Fact{
			EvidenceID: "resolution-001",
			Entity:     targetEntity,
			Certainty:  evidence.CertaintyStatic,
		},
		OutgoingCalls: []symbol.CallFact{
			outgoingCall(targetEntity, "call-validation", "checkPutRequest", "server/validation.go", 10, 91),
			outgoingCall(targetEntity, "call-error", "togRPCError", "server/errors.go", 20, 97),
			outgoingCall(targetEntity, "call-delegate", "Put", "server/core.go", 30, 95),
			outgoingCall(targetEntity, "call-fill", "fill", "server/header.go", 40, 100),
		},
	}
	target := sourcecard.Target{
		EvidenceID: "resolution-001",
		EntityID:   "target",
		Name:       "kvServer.Put",
		Kind:       evidence.EntityMethod,
		Path:       "server/key.go",
		Line:       90,
		Column:     20,
	}
	texts := []string{
		"func (s *kvServer) Put(ctx context.Context, r *PutRequest) (*PutResponse, error) {",
		"\tif err := checkPutRequest(r); err != nil {",
		"\t\treturn nil, err",
		"\t}",
		"",
		"\tresp, err := s.kv.Put(ctx, r)",
		"\tif err != nil {",
		"\t\treturn nil, togRPCError(err)",
		"\t}",
		"",
		"\ts.hdr.fill(resp.Header)",
		"\treturn resp, nil",
		"}",
	}
	lines := make([]sourcecard.Line, 0, len(texts))
	includedBytes := 0
	for index, text := range texts {
		line := 90 + index
		lines = append(lines, sourcecard.Line{EvidenceID: fmt.Sprintf("source-%d", line), Line: line, Text: text})
		includedBytes += len(text)
		if index > 0 {
			includedBytes++
		}
	}
	card := sourcecard.Card{
		Version:    sourcecard.Version,
		Language:   "go",
		RepoName:   "repo",
		Target:     target,
		FileSHA256: strings.Repeat("a", 64),
		Window: sourcecard.Window{
			StartLine:     90,
			EndLine:       102,
			IncludedBytes: includedBytes,
			StopReason:    sourcecard.StopNextTopLevelFunc,
		},
		Lines:    lines,
		Warnings: []sourcecard.Warning{{Code: "boundary.lexical", Message: "test lexical window"}},
	}
	assessment, err := sourceexplain.Build(structural, card)
	if err != nil {
		t.Fatalf("Build() assessment error = %v", err)
	}
	type rawAssessment struct {
		QuestionID        string   `json:"question_id"`
		Verdict           string   `json:"verdict"`
		SourceEvidenceIDs []string `json:"source_evidence_ids"`
	}
	rawAssessments := make([]rawAssessment, 0, len(assessment.Questions))
	for _, question := range assessment.Questions {
		rawAssessments = append(rawAssessments, rawAssessment{
			QuestionID:        question.ID,
			Verdict:           "shown",
			SourceEvidenceIDs: append([]string{}, question.CandidateSourceEvidenceIDs...),
		})
	}
	raw, err := json.Marshal(struct {
		Assessments  []rawAssessment `json:"assessments"`
		Unknowns     []any           `json:"unknowns"`
		NextActionID string          `json:"next_action_id"`
	}{Assessments: rawAssessments, Unknowns: []any{}, NextActionID: "action-find-tests"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := sourceexplain.ParseReport(assessment, raw)
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	return structural, assessment, parsed.Report
}

func outgoingCall(caller evidence.Entity, id, name, path string, line, callsiteLine int) symbol.CallFact {
	return symbol.CallFact{
		EvidenceID: id,
		Caller:     caller,
		Callee: evidence.Entity{
			ID:       "callee-" + id,
			Kind:     evidence.EntityFunction,
			Name:     name,
			Location: &evidence.Location{Path: path, Line: line, Column: 6},
		},
		Callsite:  &evidence.Location{Path: "server/key.go", Line: callsiteLine, Column: 2},
		Certainty: evidence.CertaintyStatic,
	}
}

func locationKey(location evidence.Location) string {
	return fmt.Sprintf("%s:%d", location.Path, location.Line)
}

func staticLocations(locations []evidence.Location) evidence.LocationSet {
	return evidence.LocationSet{
		Locations:  append([]evidence.Location{}, locations...),
		Certainty:  evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{Provider: "gopls", Version: "fixture", Operation: "references"}},
		Scenarios: []evidence.Scenario{{
			ID:   "gopls-active-build",
			Name: "gopls active build configuration",
		}},
	}
}

func hasWarning(warnings []Warning, code string) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}
