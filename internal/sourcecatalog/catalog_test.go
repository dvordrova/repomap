package sourcecatalog

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
)

func TestNewBuildsDeterministicRepositoryRootCatalog(t *testing.T) {
	t.Parallel()

	root := testRoot("repository")
	allowed := []string{"z.go", "a.go"}
	inputs := []freshness.CapturedInput{
		testCapturedInput("z.go", freshness.FileRegular, strings.Repeat("2", 64)),
		testCapturedInput("a.go", freshness.FileRegular, strings.Repeat("1", 64)),
	}
	catalog, err := New(Input{
		RepositoryRoot: root,
		AnalysisRoot:   root,
		AllowedPaths:   allowed,
		CapturedInputs: inputs,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if catalog.AnalysisRoot() != root {
		t.Fatalf("AnalysisRoot() = %q, want %q", catalog.AnalysisRoot(), root)
	}
	if catalog.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", catalog.Len())
	}
	if got := catalog.Paths(); !reflect.DeepEqual(got, []string{"a.go", "z.go"}) {
		t.Fatalf("Paths() = %#v", got)
	}
	source, ok := catalog.Lookup("a.go")
	if !ok || source.Path != "a.go" || source.RepositoryPath != "a.go" ||
		source.Kind != freshness.FileRegular || source.ContentSHA256 != strings.Repeat("1", 64) {
		t.Fatalf("Lookup(a.go) = %#v, %t", source, ok)
	}

	allowed[0] = "changed.go"
	inputs[1].ContentSHA256 = strings.Repeat("f", 64)
	first := catalog.Paths()
	first[0] = "mutated.go"
	if got := catalog.Paths(); !reflect.DeepEqual(got, []string{"a.go", "z.go"}) {
		t.Fatalf("catalog changed through caller-owned input/output: %#v", got)
	}
	source, ok = catalog.Lookup("a.go")
	if !ok || source.ContentSHA256 != strings.Repeat("1", 64) {
		t.Fatalf("catalog source changed through caller-owned input: %#v, %t", source, ok)
	}
}

func TestNewMapsSubdirectoryAnalysisPathsToRepositoryInputs(t *testing.T) {
	t.Parallel()

	repositoryRoot := testRoot("repository")
	analysisRoot := filepath.Join(repositoryRoot, "service")
	catalog, err := New(Input{
		RepositoryRoot: repositoryRoot,
		AnalysisRoot:   analysisRoot,
		AllowedPaths:   []string{"cmd/main.go"},
		CapturedInputs: []freshness.CapturedInput{
			testCapturedInput("service/cmd/main.go", freshness.FileRegular, strings.Repeat("a", 64)),
			testCapturedInput("cmd/main.go", freshness.FileRegular, strings.Repeat("b", 64)),
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	source, ok := catalog.Lookup("cmd/main.go")
	if !ok {
		t.Fatal("Lookup(cmd/main.go) was not authorized")
	}
	if source.Path != "cmd/main.go" || source.RepositoryPath != "service/cmd/main.go" ||
		source.ContentSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("source = %#v", source)
	}
	for _, invalid := range []string{"./cmd/main.go", "../main.go", "/cmd/main.go", `cmd\main.go`} {
		if source, ok := catalog.Lookup(invalid); ok {
			t.Fatalf("Lookup(%q) = %#v, true", invalid, source)
		}
	}
}

func TestNewPrefersExactSubdirectoryInputOverUnrelatedRootPath(t *testing.T) {
	t.Parallel()

	repositoryRoot := testRoot("repository")
	catalog, err := New(Input{
		RepositoryRoot: repositoryRoot,
		AnalysisRoot:   filepath.Join(repositoryRoot, "service"),
		AllowedPaths:   []string{"main.go"},
		CapturedInputs: []freshness.CapturedInput{
			testCapturedInput("main.go", freshness.FileRegular, strings.Repeat("b", 64)),
			testCapturedInput("service/main.go", freshness.FileRegular, strings.Repeat("a", 64)),
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	source, ok := catalog.Lookup("main.go")
	if !ok || source.RepositoryPath != "service/main.go" ||
		source.ContentSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("Lookup(main.go) = %#v, %t", source, ok)
	}
}

func TestNewRejectsUnsafeOrAmbiguousSourceScopes(t *testing.T) {
	t.Parallel()

	repositoryRoot := testRoot("repository")
	analysisRoot := filepath.Join(repositoryRoot, "service")
	valid := Input{
		RepositoryRoot: repositoryRoot,
		AnalysisRoot:   analysisRoot,
		AllowedPaths:   []string{"main.go"},
		CapturedInputs: []freshness.CapturedInput{
			testCapturedInput("service/main.go", freshness.FileRegular, strings.Repeat("a", 64)),
		},
	}
	tests := []struct {
		name   string
		mutate func(*Input)
		want   string
	}{
		{
			name: "relative repository root",
			mutate: func(input *Input) {
				input.RepositoryRoot = "repository"
			},
			want: "repository root must be a canonical absolute path",
		},
		{
			name: "non-canonical analysis root",
			mutate: func(input *Input) {
				input.AnalysisRoot = filepath.Join(repositoryRoot, "service", "..", "service") + string(filepath.Separator)
			},
			want: "analysis root must be a canonical absolute path",
		},
		{
			name: "analysis root outside repository",
			mutate: func(input *Input) {
				input.AnalysisRoot = testRoot("outside")
			},
			want: "analysis root must be inside repository root",
		},
		{
			name: "absolute allowed path",
			mutate: func(input *Input) {
				input.AllowedPaths[0] = filepath.Join(repositoryRoot, "main.go")
			},
			want: "canonical relative slash path",
		},
		{
			name: "traversal",
			mutate: func(input *Input) {
				input.AllowedPaths[0] = "../main.go"
			},
			want: "canonical relative slash path",
		},
		{
			name: "dot path",
			mutate: func(input *Input) {
				input.AllowedPaths[0] = "./main.go"
			},
			want: "canonical relative slash path",
		},
		{
			name: "backslash separator",
			mutate: func(input *Input) {
				input.AllowedPaths[0] = `service\main.go`
			},
			want: "canonical relative slash path",
		},
		{
			name: "two allowed paths map to one exact captured input",
			mutate: func(input *Input) {
				input.AllowedPaths = append(input.AllowedPaths, "main.go")
			},
			want: "duplicate allowed path",
		},
		{
			name: "missing captured input",
			mutate: func(input *Input) {
				input.CapturedInputs = nil
			},
			want: "has no captured input",
		},
		{
			name: "analysis-relative captured alias",
			mutate: func(input *Input) {
				input.CapturedInputs = []freshness.CapturedInput{
					testCapturedInput("main.go", freshness.FileRegular, strings.Repeat("a", 64)),
				}
			},
			want: "analysis-relative alias",
		},
		{
			name: "duplicate exact captured input path",
			mutate: func(input *Input) {
				input.CapturedInputs = append(input.CapturedInputs, input.CapturedInputs[0])
			},
			want: "duplicate captured input path",
		},
		{
			name: "captured symlink",
			mutate: func(input *Input) {
				input.CapturedInputs[0] = testCapturedInput(
					"service/main.go", freshness.FileSymlink, strings.Repeat("a", 64),
				)
			},
			want: "is not a regular file",
		},
		{
			name: "captured missing file",
			mutate: func(input *Input) {
				input.CapturedInputs[0] = testCapturedInput(
					"service/main.go", freshness.FileMissing, "",
				)
			},
			want: "is not a regular file",
		},
		{
			name: "missing content hash",
			mutate: func(input *Input) {
				input.CapturedInputs[0].ContentSHA256 = ""
			},
			want: "content SHA-256 is required",
		},
		{
			name: "invalid content hash",
			mutate: func(input *Input) {
				input.CapturedInputs[0].ContentSHA256 = strings.Repeat("A", 64)
			},
			want: "content SHA-256 is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := cloneInput(valid)
			test.mutate(&input)
			if _, err := New(input); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestNewDoesNotRequireRootsOrSourcesToExistOnDisk(t *testing.T) {
	t.Parallel()

	repositoryRoot := filepath.Join(t.TempDir(), "not-created-repository")
	analysisRoot := filepath.Join(repositoryRoot, "not-created-analysis")
	catalog, err := New(Input{
		RepositoryRoot: repositoryRoot,
		AnalysisRoot:   analysisRoot,
		AllowedPaths:   []string{"main.go"},
		CapturedInputs: []freshness.CapturedInput{
			testCapturedInput(
				"not-created-analysis/main.go",
				freshness.FileRegular,
				strings.Repeat("a", 64),
			),
		},
	})
	if err != nil {
		t.Fatalf("New read the filesystem or rejected an offline scope: %v", err)
	}
	if _, ok := catalog.Lookup("main.go"); !ok {
		t.Fatal("offline catalog did not retain its authorized source")
	}
}

func TestCatalogRejectsTrackedSymlink(t *testing.T) {
	t.Parallel()

	root := testRoot("repository")
	input := testCapturedInput(
		"client/v3/example_lease_test.go",
		freshness.FileSymlink,
		strings.Repeat("a", 64),
	)
	input.Mode = "120000"
	_, err := New(Input{
		RepositoryRoot: root,
		AnalysisRoot:   root,
		AllowedPaths:   []string{"client/v3/example_lease_test.go"},
		CapturedInputs: []freshness.CapturedInput{input},
	})
	if err == nil || !strings.Contains(err.Error(), "is not a regular file") {
		t.Fatalf("New tracked-symlink error = %v", err)
	}
}

func TestNeutralCatalogContractHasNoReportBinding(t *testing.T) {
	t.Parallel()

	for _, value := range []any{Input{}, Source{}, Catalog{}} {
		valueType := reflect.TypeOf(value)
		for index := 0; index < valueType.NumField(); index++ {
			field := valueType.Field(index)
			normalized := strings.ToLower(field.Name)
			if normalized == "reportsha256" || normalized == "bindingsha256" {
				t.Fatalf("%s unexpectedly contains %s", valueType.Name(), field.Name)
			}
		}
	}
}

func cloneInput(input Input) Input {
	input.AllowedPaths = append([]string(nil), input.AllowedPaths...)
	input.CapturedInputs = append([]freshness.CapturedInput(nil), input.CapturedInputs...)
	for index := range input.CapturedInputs {
		input.CapturedInputs[index].Stages = append([]string(nil), input.CapturedInputs[index].Stages...)
	}
	return input
}

func testCapturedInput(path string, kind freshness.FileKind, contentSHA256 string) freshness.CapturedInput {
	id := sha256.Sum256([]byte("sourcecatalog-test\x00" + path))
	return freshness.CapturedInput{
		Version:       freshness.CapturedInputVersion,
		ID:            fmt.Sprintf("%x", id[:]),
		Path:          path,
		Kind:          kind,
		Mode:          "100644",
		ContentSHA256: contentSHA256,
		Stages:        []string{"report_evidence"},
	}
}

func testRoot(name string) string {
	return filepath.Join(string(filepath.Separator), "repomap-sourcecatalog-test", name)
}
