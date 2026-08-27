package targetoutcome

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestPortfolioCanonicalRoundTripRetainsAnalyzedAndFailedTargets(t *testing.T) {
	goSelected := testSelectedTarget(t, LanguageGroupGo, ScopeExecutable, "cmd/api", "example.com/repo@.::example.com/repo/cmd/api")
	pythonSelected := testSelectedTarget(t, LanguageGroupPython, ScopeLibrary, "acme", "library:acme")
	jstsSelected := testSelectedTarget(t, LanguageGroupJavaScriptTypeScript, ScopePackage, "@acme/web", "jsts:packages/web/package.json")
	goTarget := testProgramTarget(t, "go", "executable", "example.com/repo/cmd/api", "go:api", "cmd/api/main.go", "f-go")
	jstsTarget := testProgramTarget(t, "typescript", "application", "@acme/web", "jsts:packages/web/package.json", "packages/web/src/main.ts", "f-jsts")

	goOutcome, err := NewAnalyzed(goSelected, goTarget, "run-go")
	if err != nil {
		t.Fatalf("NewAnalyzed Go: %v", err)
	}
	pythonOutcome, err := NewNotAnalyzed(pythonSelected, StageSemanticAnalysis, ReasonModelResultRejected)
	if err != nil {
		t.Fatalf("NewNotAnalyzed Python: %v", err)
	}
	jstsOutcome, err := NewAnalyzed(jstsSelected, jstsTarget, "run-jsts")
	if err != nil {
		t.Fatalf("NewAnalyzed JSTS: %v", err)
	}

	portfolio, err := Build(pythonSelected.ID, []Outcome{pythonOutcome, jstsOutcome, goOutcome})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if portfolio.Version != Version || portfolio.DefaultSelectedTargetID != pythonSelected.ID ||
		len(portfolio.Outcomes) != 3 || len(portfolio.SHA256) != 64 {
		t.Fatalf("portfolio identity = %#v", portfolio)
	}
	if ArtifactFilename != "target-outcome-portfolio.json" {
		t.Fatalf("ArtifactFilename = %q", ArtifactFilename)
	}
	selectedIDs := make([]string, 0, len(portfolio.Outcomes))
	for _, outcome := range portfolio.Outcomes {
		selectedIDs = append(selectedIDs, outcome.SelectedTarget.ID)
	}
	if !slices.IsSorted(selectedIDs) {
		t.Fatalf("selected target IDs are not canonical: %v", selectedIDs)
	}

	reordered, err := Build(pythonSelected.ID, []Outcome{jstsOutcome, goOutcome, pythonOutcome})
	if err != nil {
		t.Fatalf("Build reordered: %v", err)
	}
	if !reflect.DeepEqual(reordered, portfolio) {
		t.Fatalf("input order changed canonical portfolio:\nfirst=%#v\nsecond=%#v", portfolio, reordered)
	}

	encoded, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(decoded, portfolio) {
		t.Fatalf("round trip changed portfolio:\nencoded=%s\ndecoded=%#v", encoded, decoded)
	}
	for _, forbidden := range []string{`"error"`, `"detail"`, `"native_ref"`, `"adapter_ref"`} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("persisted schema contains forbidden free-form/internal field %s: %s", forbidden, encoded)
		}
	}

	snapshot := portfolio.Snapshot()
	for index := range snapshot.Outcomes {
		if snapshot.Outcomes[index].Analysis != nil {
			snapshot.Outcomes[index].Analysis.ProgramTarget.Sources[0].Path = "changed.go"
			if portfolio.Outcomes[index].Analysis.ProgramTarget.Sources[0].Path == "changed.go" {
				t.Fatal("Snapshot aliases nested ProgramTarget source storage")
			}
			break
		}
	}
}

func TestPortfolioAllowsFailedDefaultAndZeroAnalyzedTargets(t *testing.T) {
	first := testSelectedTarget(t, LanguageGroupGo, ScopeExecutable, "cmd/api", "go-api")
	second := testSelectedTarget(t, LanguageGroupJavaScriptTypeScript, ScopePackage, "web", "jsts:web")
	firstOutcome, err := NewNotAnalyzed(first, StageProgramAnalysis, ReasonSourceNotAnalyzable)
	if err != nil {
		t.Fatalf("first outcome: %v", err)
	}
	secondOutcome, err := NewNotAnalyzed(second, StageTargetPreparation, ReasonRequiredToolUnavailable)
	if err != nil {
		t.Fatalf("second outcome: %v", err)
	}
	portfolio, err := Build(first.ID, []Outcome{secondOutcome, firstOutcome})
	if err != nil {
		t.Fatalf("Build all-failed portfolio: %v", err)
	}
	for _, outcome := range portfolio.Outcomes {
		if outcome.State != StateNotAnalyzed || outcome.Analysis != nil || outcome.Failure == nil {
			t.Fatalf("all-failed outcome = %#v", outcome)
		}
	}
	if _, err := Decode(mustCanonicalJSON(t, portfolio)); err != nil {
		t.Fatalf("Decode all-failed portfolio: %v", err)
	}
}

func TestSelectedTargetIdentityBindsEveryPublicField(t *testing.T) {
	base := testSelectedTarget(t, LanguageGroupGo, ScopeExecutable, "cmd/api", "go-api")
	variants := []SelectedTarget{
		testSelectedTarget(t, LanguageGroupPython, ScopeExecutable, "cmd/api", "go-api"),
		testSelectedTarget(t, LanguageGroupGo, ScopeLibrary, "cmd/api", "go-api"),
		testSelectedTarget(t, LanguageGroupGo, ScopeExecutable, "cmd/worker", "go-api"),
		testSelectedTarget(t, LanguageGroupGo, ScopeExecutable, "cmd/api", "go-worker"),
	}
	for _, variant := range variants {
		if variant.ID == base.ID {
			t.Fatalf("selected target identity ignored changed field: base=%#v variant=%#v", base, variant)
		}
	}
	if !strings.HasPrefix(base.ID, "selected-target-") || len(base.ID) != len("selected-target-")+64 {
		t.Fatalf("selected target ID = %q", base.ID)
	}

	invalid := base
	invalid.ID += "tampered"
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("tampered SelectedTarget.Validate error = %v", err)
	}
	for _, input := range []struct {
		language LanguageGroup
		scope    ScopeKind
		display  string
		selector string
	}{
		{LanguageGroup("ruby"), ScopeExecutable, "app", "app"},
		{LanguageGroupGo, ScopeKind("tool"), "app", "app"},
		{LanguageGroupGo, ScopeExecutable, " app", "app"},
		{LanguageGroupGo, ScopeExecutable, "app", "app\nsecret"},
	} {
		if _, err := NewSelectedTarget(input.language, input.scope, input.display, input.selector); err == nil {
			t.Fatalf("NewSelectedTarget accepted invalid input %#v", input)
		}
	}
}

func TestOutcomeRejectsInvalidUnionFailureProgramTargetAndRun(t *testing.T) {
	selected := testSelectedTarget(t, LanguageGroupPython, ScopeExecutable, "worker", "python:worker")
	target := testProgramTarget(t, "python", "executable", "worker", "python:worker", "app/worker.py", "f-python")
	wrongLanguageTarget := testProgramTarget(t, "go", "executable", "worker", "python:worker", "app/worker.go", "f-go")

	invalidTarget := target.Snapshot()
	invalidTarget.ID += "tampered"
	if _, err := NewAnalyzed(selected, invalidTarget, "run-python"); err == nil ||
		!strings.Contains(err.Error(), "target identity mismatch") {
		t.Fatalf("invalid ProgramTarget error = %v", err)
	}
	if _, err := NewAnalyzed(selected, target, "../run"); err == nil ||
		!strings.Contains(err.Error(), "invalid run id") {
		t.Fatalf("unsafe RunID error = %v", err)
	}
	if _, err := NewAnalyzed(selected, wrongLanguageTarget, "run-go"); err == nil ||
		!strings.Contains(err.Error(), "language mismatch") {
		t.Fatalf("mismatched ProgramTarget language error = %v", err)
	}
	if _, err := NewNotAnalyzed(selected, Stage("provider_auth"), ReasonAnalysisFailed); err == nil {
		t.Fatal("NewNotAnalyzed accepted an open failure stage")
	}
	if _, err := NewNotAnalyzed(selected, StageSemanticAnalysis, Reason("raw_error")); err == nil {
		t.Fatal("NewNotAnalyzed accepted an open failure reason")
	}

	tests := []Outcome{
		{SelectedTarget: selected, State: StateAnalyzed},
		{
			SelectedTarget: selected, State: StateAnalyzed,
			Analysis: &Analysis{ProgramTarget: target, RunID: "run-python"},
			Failure:  &Failure{Stage: StageSemanticAnalysis, Reason: ReasonAnalysisFailed},
		},
		{SelectedTarget: selected, State: StateNotAnalyzed},
		{
			SelectedTarget: selected, State: StateNotAnalyzed,
			Analysis: &Analysis{ProgramTarget: target, RunID: "run-python"},
			Failure:  &Failure{Stage: StageSemanticAnalysis, Reason: ReasonAnalysisFailed},
		},
		{SelectedTarget: selected, State: State("partial")},
	}
	for index, outcome := range tests {
		if err := outcome.Validate(); err == nil {
			t.Fatalf("Outcome.Validate accepted invalid union %d: %#v", index, outcome)
		}
	}
}

func TestPortfolioRejectsIncompleteDuplicateOrUnsafeBindings(t *testing.T) {
	firstSelected := testSelectedTarget(t, LanguageGroupGo, ScopeExecutable, "api", "go-api")
	secondSelected := testSelectedTarget(t, LanguageGroupPython, ScopeExecutable, "worker", "python-worker")
	thirdSelected := testSelectedTarget(t, LanguageGroupGo, ScopeExecutable, "api alias", "go-api-alias")
	firstTarget := testProgramTarget(t, "go", "executable", "api", "api", "cmd/api/main.go", "f-go")
	secondTarget := testProgramTarget(t, "python", "executable", "worker", "worker", "app/worker.py", "f-python")

	first, err := NewAnalyzed(firstSelected, firstTarget, "run-first")
	if err != nil {
		t.Fatalf("first outcome: %v", err)
	}
	second, err := NewAnalyzed(secondSelected, secondTarget, "run-second")
	if err != nil {
		t.Fatalf("second outcome: %v", err)
	}
	third, err := NewAnalyzed(thirdSelected, firstTarget, "run-third")
	if err != nil {
		t.Fatalf("third outcome: %v", err)
	}

	tests := []struct {
		name      string
		defaultID string
		outcomes  []Outcome
		want      string
	}{
		{name: "empty", defaultID: firstSelected.ID, outcomes: nil, want: "outcome bound"},
		{name: "default absent", defaultID: thirdSelected.ID, outcomes: []Outcome{first, second}, want: "default selected target"},
		{name: "duplicate selected target", defaultID: firstSelected.ID, outcomes: []Outcome{first, first}, want: "not canonical"},
		{name: "duplicate program target", defaultID: firstSelected.ID, outcomes: []Outcome{first, third}, want: "duplicate analyzed program target"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Build(test.defaultID, test.outcomes)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build error = %v, want %q", err, test.want)
			}
		})
	}

	secondDuplicateRun := second.Snapshot()
	secondDuplicateRun.Analysis.RunID = first.Analysis.RunID
	if _, err := Build(firstSelected.ID, []Outcome{first, secondDuplicateRun}); err == nil ||
		!strings.Contains(err.Error(), "duplicate analyzed run id") {
		t.Fatalf("duplicate run Build error = %v", err)
	}
	tooMany := make([]Outcome, MaxOutcomes+1)
	if _, err := Build(firstSelected.ID, tooMany); err == nil || !strings.Contains(err.Error(), "outcome bound") {
		t.Fatalf("over-bound Build error = %v", err)
	}
}

func TestPortfolioCodecRejectsTamperAndNonCanonicalBytes(t *testing.T) {
	selected := testSelectedTarget(t, LanguageGroupGo, ScopeExecutable, "api", "go-api")
	target := testProgramTarget(t, "go", "executable", "api", "api", "cmd/api/main.go", "f-go")
	outcome, err := NewAnalyzed(selected, target, "run-go")
	if err != nil {
		t.Fatalf("NewAnalyzed: %v", err)
	}
	portfolio, err := Build(selected.ID, []Outcome{outcome})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	encoded := mustCanonicalJSON(t, portfolio)

	unknown := append([]byte(`{"unknown":true,`), encoded[1:]...)
	if _, err := Decode(unknown); err == nil {
		t.Fatal("Decode accepted an unknown field")
	}
	if _, err := Decode(append(append([]byte(nil), encoded...), []byte(` {}`)...)); err == nil {
		t.Fatal("Decode accepted a trailing JSON value")
	}
	if _, err := Decode(append(append([]byte(nil), encoded...), '\n')); err == nil ||
		!strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("non-canonical whitespace Decode error = %v", err)
	}
	tampered := []byte(strings.Replace(string(encoded), "run-go", "run-gx", 1))
	if _, err := Decode(tampered); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("tampered run Decode error = %v", err)
	}

	uppercaseSeal := portfolio.Snapshot()
	uppercaseSeal.SHA256 = strings.ToUpper(uppercaseSeal.SHA256)
	uppercaseBytes, err := json.Marshal(uppercaseSeal)
	if err != nil {
		t.Fatalf("marshal uppercase seal: %v", err)
	}
	if _, err := Decode(uppercaseBytes); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("uppercase seal Decode error = %v", err)
	}
	if _, err := Decode(bytes.Repeat([]byte{'x'}, MaxArtifactBytes+1)); err == nil ||
		!strings.Contains(err.Error(), "artifact size") {
		t.Fatalf("over-bound Decode error = %v", err)
	}
}

func testSelectedTarget(
	t *testing.T,
	language LanguageGroup,
	scope ScopeKind,
	displayName string,
	selector string,
) SelectedTarget {
	t.Helper()
	target, err := NewSelectedTarget(language, scope, displayName, selector)
	if err != nil {
		t.Fatalf("NewSelectedTarget(%q): %v", displayName, err)
	}
	return target
}

func testProgramTarget(
	t *testing.T,
	language string,
	kind string,
	name string,
	selector string,
	path string,
	fileRef string,
) programindex.Target {
	t.Helper()
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64),
		SourceSHA256:   strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: language, Kind: kind, Name: name, Selector: selector,
			Sources: []programindex.TargetSource{{FileRef: fileRef, Path: path}}, AnchorFileRef: fileRef,
			Seeds: []programindex.TargetSeedInput{},
		},
		Objects:   []programindex.ObjectInput{},
		Relations: []programindex.RelationInput{},
		Coverage:  programindex.CoverageInput{Measured: true},
	})
	if err != nil {
		t.Fatalf("programindex.New(%q): %v", language, err)
	}
	return index.Target
}

func mustCanonicalJSON(t *testing.T, portfolio Portfolio) []byte {
	t.Helper()
	encoded, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	return encoded
}
