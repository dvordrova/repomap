package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

func TestCumulativeNativeEvidenceSeparatesGoConsumersAndJSTSPackages(t *testing.T) {
	for _, language := range []string{"go", "jsts"} {
		t.Run(language, func(t *testing.T) {
			root, repository := cumulativeEvidenceRepository(t, language)
			opts := repositoryTargetRuntimeOptions{Repository: repository, NoModel: true, DiscoverJSTS: language == "jsts"}
			if language == "go" {
				source, err := snapshot.BuildContext(t.Context(), snapshot.Options{RepoPath: root, GoTarget: runtime.GOOS + "/" + runtime.GOARCH, RepositoryCorpus: repository})
				if err != nil {
					t.Fatal(err)
				}
				opts.GoSnapshot = &source
			}
			discovery, err := discoverRepositoryTargets(t.Context(), opts)
			if err != nil {
				t.Fatal(err)
			}
			guidance, err := readmetargetscout.Compile(language, repository)
			if err != nil {
				t.Fatal(err)
			}
			discovery.guidance, err = guidance.GuidanceSnapshot()
			if err != nil {
				t.Fatal(err)
			}
			native, err := repositoryNativeCandidates(discovery)
			if err != nil {
				t.Fatal(err)
			}
			if language == "jsts" {
				if len(native) != 3 {
					t.Fatalf("source-owning packages merged: %d", len(native))
				}
				for _, row := range native {
					if len(row.Row.SeedOwners) != 0 {
						t.Fatal("invented JS/TS seed ownership")
					}
				}
				return
			}
			found := false
			for _, candidate := range native {
				if candidate.Row.Kind != "library" || candidate.Row.Root != "." {
					continue
				}
				found = true
				if len(candidate.Consumers) != 2 {
					t.Fatalf("shared library consumers = %v", candidate.Consumers)
				}
				observations := make(map[string]targetportfolio.Observation)
				commands := make(map[string]bool)
				for _, row := range candidate.Row.Evidence {
					observations[row.Kind] = row
					if row.Kind == "documented_command" && row.Path == "README.md" && row.Line > 0 {
						commands[row.Values[1]] = true
					}
				}
				if observations["own_main_packages"].Values[0] != "count=3" || observations["own_main_consumers"].Values[0] != "count=2" || observations["other_module_imports"].Values[0] != "count=0" {
					t.Fatalf("incorrect Go import evidence: %#v", observations)
				}
				if !commands["go run ./cmd/api"] || !commands["go run ./cmd/worker"] {
					t.Fatalf("documented service launches missing: %v", commands)
				}
			}
			if !found {
				t.Fatal("common library not analyzed")
			}
		})
	}
}

func cumulativeEvidenceRepository(t *testing.T, language string) (string, *corpus.Corpus) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	source := filepath.Join(filepath.Dir(file), "../../testdata/repositories", language)
	var inventory struct {
		Entries []struct {
			Path string `json:"path"`
		} `json:"entries"`
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../testdata/contracts", language+".files.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &inventory); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "repository")
	var paths []string
	for _, entry := range inventory.Entries {
		contents, err := os.ReadFile(filepath.Join(source, entry.Path))
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(root, entry.Path)
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, contents, 0644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, entry.Path)
	}
	if language == "go" {
		published := filepath.Join(parent, "published-root")
		if err := os.MkdirAll(published, 0755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"go.mod", "root.go"} {
			contents, err := os.ReadFile(filepath.Join(source, name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(published, name), contents, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	ordinaryGraphGit(t, root, "init", "--quiet")
	ordinaryGraphGit(t, root, "add", ".")
	ordinaryGraphGit(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "cumulative fixture")
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: paths, RegularPaths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	return root, repository
}

func TestGuidanceCommandsKeepContinuationAndIgnoreProse(t *testing.T) {
	rows := guidanceCommandObservations(readmetargetscout.GuidanceSnapshot{Documents: []readmetargetscout.GuidanceDocument{{Path: "README.md", Content: "# Run\nDo not execute this prose.\n~~~~sh\npython -m acme.api \\\n  --port 8000\n~~~~\n```python\nimport acme\nvalue = 2\n```\n"}}})
	if len(rows) != 2 || rows[0].Line != 4 || rows[0].Kind != "documented_command" || !strings.Contains(rows[0].Values[1], "\n  --port 8000") || rows[1].Line != 8 || rows[1].Kind != "documented_import" {
		t.Fatalf("guidance source boundaries: %#v", rows)
	}
}

func TestGuidanceImportsKeepTheirMultilineModuleNames(t *testing.T) {
	for _, statement := range []string{"import(\n  \"example.com/acme/service\" // comment with (\n)", "from acme import (\n  api,\n  worker,\n)", "import {\n  start,\n  stop\n} from 'acme'"} {
		rows := guidanceCommandObservations(readmetargetscout.GuidanceSnapshot{Documents: []readmetargetscout.GuidanceDocument{{Path: "README.md", Content: "# Use\n```\n" + statement + "\n```\n"}}})
		if len(rows) != 1 || rows[0].Kind != "documented_import" || rows[0].Line != 3 || rows[0].Values[1] != statement {
			t.Fatalf("import lost its original module or source extent: %#v", rows)
		}
	}
}
