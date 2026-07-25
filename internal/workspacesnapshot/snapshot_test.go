package workspacesnapshot

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
)

func TestNewConstructsDeterministicRootAndSubdirectorySnapshots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		analysisRoot   string
		repositoryPath string
	}{
		{name: "repository root", analysisRoot: "/repo", repositoryPath: "main.go"},
		{name: "subdirectory", analysisRoot: "/repo/service", repositoryPath: "service/main.go"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := validSnapshotInput("/repo", test.analysisRoot, test.repositoryPath)
			first, err := New(input)
			if err != nil {
				t.Fatalf("New(first): %v", err)
			}
			second, err := New(input)
			if err != nil {
				t.Fatalf("New(second): %v", err)
			}
			repositoryDigest, err := input.Repository.Digest()
			if err != nil {
				t.Fatal(err)
			}
			inputsDigest, err := freshness.CapturedInputsDigest(input.CapturedInputs)
			if err != nil {
				t.Fatal(err)
			}
			if first.RepositoryRoot() != "/repo" ||
				first.AnalysisRoot() != test.analysisRoot ||
				first.Revision() != input.Repository.Head ||
				first.RepositoryDigest() != repositoryDigest ||
				first.CapturedInputsDigest() != inputsDigest {
				t.Fatalf("snapshot identity = %#v", first)
			}
			if first.RepositoryDigest() != second.RepositoryDigest() ||
				first.CapturedInputsDigest() != second.CapturedInputsDigest() ||
				!reflect.DeepEqual(first.Catalog().Paths(), second.Catalog().Paths()) {
				t.Fatalf("construction is not deterministic: first=%#v second=%#v", first, second)
			}
			source, ok := first.Catalog().Lookup("main.go")
			if !ok || source.Path != "main.go" || source.RepositoryPath != test.repositoryPath ||
				source.ContentSHA256 != input.CapturedInputs[0].ContentSHA256 {
				t.Fatalf("catalog source = %#v, %t", source, ok)
			}
		})
	}
}

func TestNewRejectsInvalidOrAmbiguousAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Input)
	}{
		{
			name: "analysis root outside repository",
			mutate: func(input *Input) {
				input.AnalysisRoot = "/other"
			},
		},
		{
			name: "noncanonical analysis root",
			mutate: func(input *Input) {
				input.AnalysisRoot = "/repo/service/.."
			},
		},
		{
			name: "invalid repository",
			mutate: func(input *Input) {
				input.Repository.Head = "not-a-revision"
			},
		},
		{
			name: "duplicate captured input",
			mutate: func(input *Input) {
				input.CapturedInputs = append(input.CapturedInputs, input.CapturedInputs[0])
			},
		},
		{
			name: "duplicate allowed path",
			mutate: func(input *Input) {
				input.AllowedPaths = append(input.AllowedPaths, input.AllowedPaths[0])
			},
		},
		{
			name: "missing captured source",
			mutate: func(input *Input) {
				input.CapturedInputs = nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := validSnapshotInput("/repo", "/repo", "main.go")
			test.mutate(&input)
			if _, err := New(input); err == nil {
				t.Fatal("New unexpectedly succeeded")
			}
		})
	}
}

func TestNewChecksCollectionAndPathBoundsBeforeConstruction(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Input)
	}{
		{
			name: "allowed paths",
			mutate: func(input *Input) {
				input.AllowedPaths = make([]string, maxAllowedPaths+1)
			},
		},
		{
			name: "captured inputs",
			mutate: func(input *Input) {
				input.CapturedInputs = make([]freshness.CapturedInput, maxCapturedInputs+1)
			},
		},
		{
			name: "dirty files",
			mutate: func(input *Input) {
				input.Repository.Dirty = make([]freshness.DirtyFile, maxRepositoryEntries+1)
			},
		},
		{
			name: "submodules",
			mutate: func(input *Input) {
				input.Repository.Submodules = make([]freshness.SubmoduleState, maxRepositoryEntries+1)
			},
		},
		{
			name: "captured input stages",
			mutate: func(input *Input) {
				input.CapturedInputs[0].Stages = make([]string, maxStagesPerInput+1)
			},
		},
		{
			name: "repository root",
			mutate: func(input *Input) {
				input.Repository.Identity = "/" + strings.Repeat("r", maxPathBytes)
			},
		},
		{
			name: "analysis root",
			mutate: func(input *Input) {
				input.AnalysisRoot = "/" + strings.Repeat("a", maxPathBytes)
			},
		},
		{
			name: "captured path",
			mutate: func(input *Input) {
				input.CapturedInputs[0].Path = strings.Repeat("p", maxPathBytes+1)
			},
		},
		{
			name: "allowed path",
			mutate: func(input *Input) {
				input.AllowedPaths[0] = strings.Repeat("p", maxPathBytes+1)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validSnapshotInput("/repo", "/repo", "main.go")
			test.mutate(&input)
			if _, err := New(input); err == nil || !strings.Contains(err.Error(), "bounds") {
				t.Fatalf("New error = %v, want bounded rejection", err)
			}
		})
	}
}

func TestNewRejectsAggregateScalarBytesBeforeDigest(t *testing.T) {
	oversized := strings.Repeat("x", maxAuthorityScalarBytes+1)
	halfBudget := strings.Repeat("x", maxAuthorityScalarBytes/2+1)
	tests := []struct {
		name   string
		mutate func(*Input)
	}{
		{
			name: "captured ownership",
			mutate: func(input *Input) {
				input.CapturedInputs[0].OwningPackage = oversized
			},
		},
		{
			name: "captured stage",
			mutate: func(input *Input) {
				input.CapturedInputs[0].Stages = []string{oversized}
			},
		},
		{
			name: "dirty mode",
			mutate: func(input *Input) {
				input.Repository.Dirty = []freshness.DirtyFile{
					dirtyFile("notes.txt", strings.Repeat("d", 64)),
				}
				input.Repository.Dirty[0].Mode = oversized
			},
		},
		{
			name: "aggregate captured metadata",
			mutate: func(input *Input) {
				input.CapturedInputs[0].OwningModuleID = halfBudget
				input.CapturedInputs[0].OwningPackage = halfBudget
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validSnapshotInput("/repo", "/repo", "main.go")
			test.mutate(&input)
			if _, err := New(input); err == nil ||
				!strings.Contains(err.Error(), "scalar authority exceeds bounds") {
				t.Fatalf("New error = %v, want scalar authority bounds", err)
			}
		})
	}
}

func TestAssessRejectsOversizedCurrentScalarsBeforeFreshnessProcessing(t *testing.T) {
	input := validSnapshotInput("/repo", "/repo", "main.go")
	snapshot, err := New(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	current := repositoryWithDirty(
		input.Repository,
		dirtyFile("notes.txt", strings.Repeat("d", 64)),
	)
	current.Dirty[0].Mode = strings.Repeat("x", maxAuthorityScalarBytes+1)
	result := snapshot.Assess(current)
	if result.State != freshness.FreshnessUnavailable ||
		!reflect.DeepEqual(result.Diagnostics, []string{unavailableDiagnostic}) {
		t.Fatalf("Assess oversized current state = %#v", result)
	}
	if err := snapshot.Verify(current); err == nil ||
		!strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Verify oversized current state error = %v", err)
	}
}

func TestSnapshotCopiesPreserveNilAndEmptyCollections(t *testing.T) {
	tests := []struct {
		name    string
		empty   bool
		wantNil bool
	}{
		{name: "nil", wantNil: true},
		{name: "empty", empty: true, wantNil: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := Input{
				AnalysisRoot: "/repo",
				Repository:   validRepository("/repo"),
			}
			input.Repository.Dirty = nil
			input.Repository.Submodules = nil
			if test.empty {
				input.Repository.Dirty = []freshness.DirtyFile{}
				input.Repository.Submodules = []freshness.SubmoduleState{}
				input.CapturedInputs = []freshness.CapturedInput{}
				input.AllowedPaths = []string{}
			}
			snapshot, err := New(input)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if (snapshot.repository.Dirty == nil) != test.wantNil ||
				(snapshot.repository.Submodules == nil) != test.wantNil ||
				(snapshot.capturedInputs == nil) != test.wantNil {
				t.Fatalf(
					"copied nil state: dirty=%t submodules=%t inputs=%t, want %t",
					snapshot.repository.Dirty == nil,
					snapshot.repository.Submodules == nil,
					snapshot.capturedInputs == nil,
					test.wantNil,
				)
			}
		})
	}
}

func TestSnapshotDefensivelyCopiesAuthorityAndResults(t *testing.T) {
	t.Parallel()

	input := validSnapshotInput("/repo", "/repo", "main.go")
	input.Repository.Dirty = []freshness.DirtyFile{dirtyFile("notes.txt", strings.Repeat("d", 64))}
	input.Repository.Submodules = []freshness.SubmoduleState{testSubmodule("deps/library")}
	current := cloneRepositoryState(input.Repository)
	wantRepositoryDigest, err := input.Repository.Digest()
	if err != nil {
		t.Fatal(err)
	}
	wantInputsDigest, err := freshness.CapturedInputsDigest(input.CapturedInputs)
	if err != nil {
		t.Fatal(err)
	}
	wantContentDigest := input.CapturedInputs[0].ContentSHA256

	snapshot, err := New(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	input.Repository.Identity = "/mutated"
	input.Repository.Dirty[0].Path = "mutated.txt"
	input.Repository.Submodules[0].Path = "mutated"
	input.CapturedInputs[0].Path = "mutated.go"
	input.CapturedInputs[0].ContentSHA256 = strings.Repeat("f", 64)
	input.CapturedInputs[0].Stages[0] = "mutated"
	input.AllowedPaths[0] = "mutated.go"

	if snapshot.RepositoryRoot() != "/repo" ||
		snapshot.RepositoryDigest() != wantRepositoryDigest ||
		snapshot.CapturedInputsDigest() != wantInputsDigest {
		t.Fatalf("snapshot mutated through caller input: %#v", snapshot)
	}
	source, ok := snapshot.Catalog().Lookup("main.go")
	if !ok || source.ContentSHA256 != wantContentDigest {
		t.Fatalf("catalog mutated through caller input: %#v, %t", source, ok)
	}
	paths := snapshot.Catalog().Paths()
	paths[0] = "changed.go"
	if got := snapshot.Catalog().Paths(); !reflect.DeepEqual(got, []string{"main.go"}) {
		t.Fatalf("catalog paths mutated through return value: %#v", got)
	}

	first := snapshot.Assess(current)
	if first.State != freshness.FreshnessFresh {
		t.Fatalf("Assess state = %s, want fresh", first.State)
	}
	first.Diagnostics = append(first.Diagnostics, "mutated")
	first.AffectedPaths = append(first.AffectedPaths, "mutated.go")
	second := snapshot.Assess(current)
	if second.State != freshness.FreshnessFresh || len(second.Diagnostics) != 0 || len(second.AffectedPaths) != 0 {
		t.Fatalf("Assess result retained caller mutation: %#v", second)
	}
}

func TestSnapshotFreshnessMatchesCapturedInputPolicy(t *testing.T) {
	t.Parallel()

	input := validSnapshotInput("/repo", "/repo", "main.go")
	snapshot, err := New(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tests := []struct {
		name          string
		current       freshness.RepositoryState
		wantState     freshness.FreshnessState
		wantPath      string
		verifyAllowed bool
	}{
		{
			name:          "fresh",
			current:       input.Repository,
			wantState:     freshness.FreshnessFresh,
			verifyAllowed: true,
		},
		{
			name: "unrelated changes",
			current: repositoryWithDirty(
				input.Repository,
				dirtyFile("notes.txt", strings.Repeat("d", 64)),
			),
			wantState:     freshness.FreshnessUnrelatedChanges,
			verifyAllowed: true,
		},
		{
			name: "partially stale",
			current: repositoryWithDirty(
				input.Repository,
				dirtyFile("main.go", strings.Repeat("e", 64)),
			),
			wantState:     freshness.FreshnessPartiallyStale,
			wantPath:      "main.go",
			verifyAllowed: false,
		},
		{
			name: "unavailable",
			current: func() freshness.RepositoryState {
				state := input.Repository
				state.Identity = "/other"
				return state
			}(),
			wantState:     freshness.FreshnessUnavailable,
			verifyAllowed: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result := snapshot.Assess(test.current)
			if result.State != test.wantState {
				t.Fatalf("Assess state = %s, want %s; result=%#v", result.State, test.wantState, result)
			}
			if test.wantPath != "" {
				if !reflect.DeepEqual(result.AffectedPaths, []string{test.wantPath}) ||
					!reflect.DeepEqual(result.AffectedInputIDs, []string{input.CapturedInputs[0].ID}) {
					t.Fatalf("affected authority = paths %#v ids %#v", result.AffectedPaths, result.AffectedInputIDs)
				}
			}
			verifyErr := snapshot.Verify(test.current)
			if (verifyErr == nil) != test.verifyAllowed {
				t.Fatalf("Verify error = %v, allowed=%t", verifyErr, test.verifyAllowed)
			}
		})
	}
}

func TestSnapshotPreservesAffectedSubmoduleSemantics(t *testing.T) {
	t.Parallel()

	input := validSnapshotInput("/repo", "/repo", "main.go")
	input.Repository.Submodules = []freshness.SubmoduleState{testSubmodule("deps/library")}
	snapshot, err := New(input)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	current := cloneRepositoryState(input.Repository)
	current.Submodules[0].WorktreeModified = true
	result := snapshot.Assess(current)
	if result.State != freshness.FreshnessUnrelatedChanges ||
		!reflect.DeepEqual(result.AffectedSubmodules, []string{"deps/library"}) {
		t.Fatalf("Assess submodule result = %#v", result)
	}
	if err := snapshot.Verify(current); err != nil {
		t.Fatalf("Verify unrelated submodule change: %v", err)
	}
}

func TestVerifyRejectsMixedSnapshotAndZeroValue(t *testing.T) {
	t.Parallel()

	mixed := freshness.NewFreshnessResult(freshness.FreshnessMixedSnapshot)
	if err := verifyFreshnessResult(mixed); err == nil || !strings.Contains(err.Error(), "mixed_snapshot") {
		t.Fatalf("mixed Verify error = %v", err)
	}
	if err := (Snapshot{}).Verify(validRepository("/repo")); err == nil ||
		!strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("zero Snapshot Verify error = %v", err)
	}
}

func TestSnapshotHasNoExportedStateOrJSONContract(t *testing.T) {
	t.Parallel()

	snapshotType := reflect.TypeOf(Snapshot{})
	for index := 0; index < snapshotType.NumField(); index++ {
		field := snapshotType.Field(index)
		if field.IsExported() || field.Tag.Get("json") != "" {
			t.Fatalf("Snapshot field %q exposes state or JSON metadata", field.Name)
		}
	}
	if _, ok := any(Snapshot{}).(interface{ MarshalJSON() ([]byte, error) }); ok {
		t.Fatal("Snapshot implements JSON persistence")
	}
}

func validSnapshotInput(repositoryRoot, analysisRoot, capturedPath string) Input {
	return Input{
		AnalysisRoot: analysisRoot,
		Repository:   validRepository(repositoryRoot),
		CapturedInputs: []freshness.CapturedInput{{
			Version:       freshness.CapturedInputVersion,
			ID:            strings.Repeat("b", 64),
			Path:          capturedPath,
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: strings.Repeat("c", 64),
			Stages:        []string{"report_evidence"},
		}},
		AllowedPaths: []string{"main.go"},
	}
}

func validRepository(root string) freshness.RepositoryState {
	return freshness.RepositoryState{
		Version:  freshness.RepositoryStateVersion,
		Identity: root,
		Head:     strings.Repeat("a", 40),
		Dirty:    []freshness.DirtyFile{},
	}
}

func repositoryWithDirty(
	repository freshness.RepositoryState,
	files ...freshness.DirtyFile,
) freshness.RepositoryState {
	repository = cloneRepositoryState(repository)
	repository.Dirty = append([]freshness.DirtyFile(nil), files...)
	return repository
}

func dirtyFile(path, digest string) freshness.DirtyFile {
	return freshness.DirtyFile{
		Status:        "modified",
		Path:          path,
		Kind:          freshness.FileRegular,
		Mode:          "100644",
		ContentSHA256: digest,
	}
}

func testSubmodule(path string) freshness.SubmoduleState {
	return freshness.SubmoduleState{
		Path:               path,
		IncludedInAnalysis: false,
		RecordedGitlink:    strings.Repeat("d", 40),
		CurrentHead:        strings.Repeat("d", 40),
		Availability:       freshness.SubmoduleClean,
	}
}
