package programpage

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestPortfolioCanonicalRoundTripRetainsEveryLanguageTarget(t *testing.T) {
	goTarget := testTarget(t, "go", "api", "cmd/api/main.go", "f-go")
	pythonTarget := testTarget(t, "python", "worker", "app/worker.py", "f-python")
	jstsTarget := testTarget(t, "typescript", "web", "server/routes.ts", "f-jsts")

	portfolio, err := Build(pythonTarget.ID, []Page{
		{Target: jstsTarget, RunID: "run-jsts-1"},
		{Target: goTarget, RunID: "run-go-1"},
		{Target: pythonTarget, RunID: "run-python-1"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if portfolio.Version != Version || portfolio.DefaultTargetID != pythonTarget.ID || len(portfolio.Pages) != 3 {
		t.Fatalf("portfolio identity = %#v", portfolio)
	}
	if ArtifactFilename != "program-page-portfolio.json" || len(portfolio.SHA256) != 64 {
		t.Fatalf("artifact identity = filename %q, sha256 %q", ArtifactFilename, portfolio.SHA256)
	}
	gotIDs := make([]string, 0, len(portfolio.Pages))
	gotLanguages := make([]string, 0, len(portfolio.Pages))
	defaultMatches := 0
	for _, page := range portfolio.Pages {
		gotIDs = append(gotIDs, page.Target.ID)
		gotLanguages = append(gotLanguages, page.Target.Language)
		if page.Target.ID == portfolio.DefaultTargetID {
			defaultMatches++
		}
	}
	if !slices.IsSorted(gotIDs) {
		t.Fatalf("page IDs are not canonical: %v", gotIDs)
	}
	slices.Sort(gotLanguages)
	if !slices.Equal(gotLanguages, []string{"go", "python", "typescript"}) || defaultMatches != 1 {
		t.Fatalf("languages = %v, default matches = %d", gotLanguages, defaultMatches)
	}

	reordered, err := Build(pythonTarget.ID, []Page{
		{Target: pythonTarget, RunID: "run-python-1"},
		{Target: jstsTarget, RunID: "run-jsts-1"},
		{Target: goTarget, RunID: "run-go-1"},
	})
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

	snapshot := portfolio.Snapshot()
	snapshot.Pages[0].Target.Sources[0].Path = "changed.go"
	if portfolio.Pages[0].Target.Sources[0].Path == "changed.go" {
		t.Fatal("Snapshot aliases nested target source storage")
	}
}

func TestPortfolioRejectsIncompleteAmbiguousOrUnsafeBindings(t *testing.T) {
	goTarget := testTarget(t, "go", "api", "cmd/api/main.go", "f-go")
	pythonTarget := testTarget(t, "python", "worker", "app/worker.py", "f-python")

	tests := []struct {
		name      string
		defaultID string
		pages     []Page
		want      string
	}{
		{name: "empty", defaultID: goTarget.ID, pages: nil, want: "page bound"},
		{
			name: "default absent", defaultID: "program-target-missing",
			pages: []Page{{Target: goTarget, RunID: "run-go-1"}}, want: "default target",
		},
		{
			name: "duplicate target", defaultID: goTarget.ID,
			pages: []Page{
				{Target: goTarget, RunID: "run-go-1"},
				{Target: goTarget, RunID: "run-go-2"},
			},
			want: "not canonical",
		},
		{
			name: "duplicate run", defaultID: goTarget.ID,
			pages: []Page{
				{Target: goTarget, RunID: "shared-run"},
				{Target: pythonTarget, RunID: "shared-run"},
			},
			want: "duplicate run id",
		},
		{
			name: "unsafe run", defaultID: goTarget.ID,
			pages: []Page{{Target: goTarget, RunID: "../run"}}, want: "invalid run id",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Build(test.defaultID, test.pages)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build error = %v, want %q", err, test.want)
			}
		})
	}

	invalidTarget := goTarget.Snapshot()
	invalidTarget.ID += "-tampered"
	if _, err := Build(invalidTarget.ID, []Page{{Target: invalidTarget, RunID: "run-go-1"}}); err == nil ||
		!strings.Contains(err.Error(), "target identity mismatch") {
		t.Fatalf("invalid target Build error = %v", err)
	}
}

func TestBuildRetainsPagesBeyondFormerLocalThreshold(t *testing.T) {
	pages := make([]Page, MaxPages+1)
	for position := range pages {
		name := fmt.Sprintf("app-%05d", position)
		target := testTarget(t, "go", name, "cmd/"+name+"/main.go", "f-"+name)
		pages[position] = Page{Target: target, RunID: fmt.Sprintf("run-%05d", position)}
	}
	portfolio, err := Build(pages[0].Target.ID, pages)
	if err != nil {
		t.Fatalf("Build rejected complete page inventory: %v", err)
	}
	if len(portfolio.Pages) != len(pages) {
		t.Fatalf("retained pages = %d, want %d", len(portfolio.Pages), len(pages))
	}
	warnings := ScaleWarnings(portfolio)
	if len(warnings) == 0 || warnings[0].Kind != ScaleWarningPages || warnings[0].Retained != len(pages) {
		t.Fatalf("scale warnings = %#v", warnings)
	}
}

func TestPortfolioCodecRejectsTamperAndNonCanonicalBytes(t *testing.T) {
	goTarget := testTarget(t, "go", "api", "cmd/api/main.go", "f-go")
	pythonTarget := testTarget(t, "python", "worker", "app/worker.py", "f-python")
	portfolio, err := Build(goTarget.ID, []Page{
		{Target: goTarget, RunID: "run-go-1"},
		{Target: pythonTarget, RunID: "run-python-1"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	encoded, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}

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

	tamperedRun := []byte(strings.Replace(string(encoded), "run-go-1", "run-go-2", 1))
	if _, err := Decode(tamperedRun); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("tampered run Decode error = %v", err)
	}
	tamperedDefault := []byte(strings.Replace(string(encoded), goTarget.ID, pythonTarget.ID, 1))
	if _, err := Decode(tamperedDefault); err == nil {
		t.Fatal("Decode accepted a tampered default target")
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

}

func TestValidateRunIDUsesPortableSafeSegments(t *testing.T) {
	for _, runID := range []string{"a", "A0", "20260825-010203-repomap-a1b2c3", "run.v1_2-x"} {
		if err := ValidateRunID(runID); err != nil {
			t.Errorf("valid run ID %q: %v", runID, err)
		}
	}
	for _, runID := range []string{
		"", ".", "..", ".hidden", "run-", "run_", "run.", "../run", "run/child",
		`run\child`, "/absolute", "file:run", "run%2fchild", " run", "run ", "rún",
		"run\nchild", "CON", "nul.txt", "LPT1", "com9.run", strings.Repeat("a", MaxRunIDBytes+1),
	} {
		if err := ValidateRunID(runID); err == nil {
			t.Errorf("accepted unsafe run ID %q", runID)
		}
	}
}

func TestExactTargetTextIsNotCutAtProgramIndexAdvisoryThreshold(t *testing.T) {
	text := strings.Repeat("x", programindex.MaxTextBytes+1)
	if !validText(text) {
		t.Fatal("exact target text was rejected at the ProgramIndex advisory threshold")
	}
}

func testTarget(t *testing.T, language, name, path, fileRef string) programindex.Target {
	t.Helper()
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64),
		SourceSHA256:   strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: language, Kind: "application", Name: name, Selector: language + ":" + name,
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
