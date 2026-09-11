package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
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
			native, err := repositoryNativeCandidates(repository, discovery)
			if err != nil {
				t.Fatal(err)
			}
			for _, candidate := range native {
				for _, observation := range candidate.Row.Evidence {
					if observation.Kind == "launch_file_executable" || observation.Kind == "module_level_relative_import" {
						t.Fatalf("Python file-launch facts were invented for a %s package: %+v", language, observation)
					}
				}
			}
			if language == "jsts" {
				if len(native) != 4 {
					t.Fatalf("source-owning packages merged: %d", len(native))
				}
				var roots []string
				for _, row := range native {
					roots = append(roots, row.Row.Root)
					if len(row.Row.SeedOwners) != 0 {
						t.Fatal("invented JS/TS seed ownership")
					}
				}
				slices.Sort(roots)
				if !slices.Equal(roots, []string{".", "packages/documentation-tools", "packages/local-store", "packages/second-store"}) {
					t.Fatalf("native JS/TS package roots = %v", roots)
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

func TestCumulativePythonLaunchFactsReachPortfolioWithoutRemovingShebangCandidate(t *testing.T) {
	const source = "src/fixture_app/script_context.py"
	var firstKey repositoryTargetKey
	for _, executable := range []bool{false, true} {
		t.Run(strconv.FormatBool(executable), func(t *testing.T) {
			root, repository := cumulativeEvidenceRepository(t, "python")
			if executable {
				if err := os.Chmod(filepath.Join(root, source), 0755); err != nil {
					t.Fatal(err)
				}
				var paths []string
				for _, entry := range repository.Entries() {
					paths = append(paths, entry.Path)
				}
				var err error
				repository, err = corpus.New(t.Context(), root, gitfiles.Listing{Paths: paths, RegularPaths: paths, ExecutablePaths: []string{source}})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = repository.Close() })
			}
			discovery, err := discoverRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, NoModel: true, DiscoverPython: true})
			if err != nil {
				t.Fatal(err)
			}
			native, err := repositoryNativeCandidates(repository, discovery)
			if err != nil {
				t.Fatal(err)
			}
			var selected repositoryNativeCandidate
			var rows []targetportfolio.NativeCandidate
			for _, candidate := range native {
				rows = append(rows, candidate.Row)
				if candidate.Target.Selector == "python:.:script-file:src/fixture_app/script_context" {
					selected = candidate
				}
			}
			if selected.Row.Ref == "" || selected.Row.Kind != "executable" {
				t.Fatal("source facts removed the author-shebang candidate")
			}
			if !executable {
				firstKey = selected.Target.Key
			} else if selected.Target.Key != firstKey {
				t.Fatal("executable permission changed the native launch identity")
			}
			adapter := discovery.adapters[0]
			compiled, err := targetportfolio.CompileWithNativeAuthority(repository.Snapshot(), adapter.Candidates, adapter.RequiredFileRefs, rows)
			if err != nil {
				t.Fatal(err)
			}
			var refs []string
			for _, candidate := range compiled.Request.NativeTargets {
				if candidate.Ref == selected.Row.Ref {
					refs = candidate.EvidenceRefs
				}
			}
			shebang, mode, permission, imports := false, false, false, 0
			for _, observation := range compiled.Request.Observations {
				if !slices.Contains(refs, observation.Ref) || observation.Path != source {
					continue
				}
				switch observation.Kind {
				case "python_shebang":
					shebang = observation.Line == 1
				case "launch_root":
					mode = len(observation.Values) > 0 && observation.Values[0] == "script_file"
				case "launch_file_executable":
					permission = slices.Equal(observation.Values, []string{strconv.FormatBool(executable)})
				case "module_level_relative_import":
					imports++
					name := "GetLevelsInfoResponse"
					if observation.Line == 4 {
						name = "*"
					} else if observation.Line != 3 {
						t.Fatalf("nested import gained module-level authority: %+v", observation)
					}
					if !slices.Equal(observation.Values, []string{".models", name}) {
						t.Fatalf("relative import lost original names: %+v", observation)
					}
				}
			}
			if !shebang || !mode || !permission || imports != 2 {
				t.Fatalf("incomplete source facts: shebang=%t mode=%t permission=%t imports=%d", shebang, mode, permission, imports)
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
