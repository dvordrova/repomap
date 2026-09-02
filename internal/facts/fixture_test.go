package facts

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
)

// expectedFacts is fixtures/<name>/expected.json: the rows a reader must be
// able to find, each as kind + distinguishing fields + "path:line" anchor.
type expectedFacts struct {
	Version    int                 `json:"version"`
	Repository string              `json:"repository"`
	Revision   string              `json:"revision"`
	Facts      []map[string]string `json:"facts"`
	Questions  json.RawMessage     `json:"questions"`
}

func TestFixturePythonTutorialGame(t *testing.T) {
	fixture := filepath.Join(repositoryRoot(t), "fixtures", "python-tutorial-game")
	expected := readExpected(t, filepath.Join(fixture, "expected.json"))
	revision, err := os.ReadFile(filepath.Join(fixture, "REVISION"))
	if err != nil {
		t.Fatalf("read REVISION: %v", err)
	}
	if strings.TrimSpace(string(revision)) != expected.Revision {
		t.Fatalf("REVISION %q disagrees with expected.json revision %q", strings.TrimSpace(string(revision)), expected.Revision)
	}
	repository := materializeFixture(t, fixture)
	input := Input{
		Revision:     expected.Revision,
		Repository:   repository,
		TrackedPaths: repository.VisiblePaths(),
		Targets: []TargetInput{
			{Index: decodeIndex(t, fixture, "backend-program-index.json"), Dependencies: decodeCatalog(t, fixture, "backend-dependency-catalog.json"), Root: "backend", Manifest: "backend/Pipfile", RunID: "backend"},
			{Index: decodeIndex(t, fixture, "front-program-index.json"), Dependencies: decodeCatalog(t, fixture, "front-dependency-catalog.json"), Root: "front", Manifest: "front/package.json", RunID: "front"},
		},
	}
	first := mustBuild(t, input)
	second := mustBuild(t, input)
	if first.SHA256 != second.SHA256 {
		t.Fatalf("Build is not deterministic: %s vs %s", first.SHA256, second.SHA256)
	}
	var missing []string
	for _, row := range expected.Facts {
		if !expectedRowPresent(t, first, row) {
			missing = append(missing, fmt.Sprintf("%v", row))
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d expected rows are missing:\n  %s", len(missing), strings.Join(missing, "\n  "))
		for _, kind := range []Kind{KindHTTPRoute, KindHTTPCall, KindPortal, KindConfigRead, KindDynamicExecution, KindTODO, KindDeadModule, KindNegative, KindManifest, KindDependency, KindImport} {
			for _, fact := range first.OfKind(kind) {
				t.Logf("have %s %s method=%s path=%s key=%s value=%s symbol=%s", fact.Kind, fact.Anchor, fact.Method, fact.Path, fact.Key, fact.Value, fact.Symbol)
			}
		}
		t.Logf("diagnostics: %+v", first.Diagnostics)
	}
}

func readExpected(t *testing.T, path string) expectedFacts {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read expected.json: %v", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var expected expectedFacts
	if err := decoder.Decode(&expected); err != nil {
		t.Fatalf("decode expected.json: %v", err)
	}
	if expected.Version != 1 || len(expected.Facts) == 0 {
		t.Fatalf("expected.json has version %d and %d facts", expected.Version, len(expected.Facts))
	}
	return expected
}

// expectedRowPresent matches one expected row against the built result. The
// field vocabulary is closed so a typo in expected.json fails loudly.
func expectedRowPresent(t *testing.T, result Result, row map[string]string) bool {
	t.Helper()
	kind := row["kind"]
	if kind == "target" {
		for _, target := range result.Targets {
			if target.Language == row["language"] && target.Root == row["root"] && (row["anchor"] == "" || target.Anchor.String() == row["anchor"]) {
				return true
			}
		}
		return false
	}
	for _, fact := range result.OfKind(Kind(kind)) {
		if factMatchesRow(t, fact, row) {
			return true
		}
	}
	return false
}

func factMatchesRow(t *testing.T, fact Fact, row map[string]string) bool {
	t.Helper()
	for field, want := range row {
		var have string
		switch field {
		case "kind", "note":
			continue
		case "anchor", "from":
			if fact.Anchor != nil {
				have = fact.Anchor.String()
			}
		case "to":
			if len(fact.Evidence) > 0 {
				have = fact.Evidence[0].String()
			}
		case "file":
			if fact.Anchor != nil {
				have = fact.Anchor.Path
			}
		case "method":
			have = fact.Method
		case "path":
			have = fact.Path
		case "key", "name", "pattern":
			have = fact.Key
		case "value":
			have = fact.Value
		case "symbol":
			have = fact.Symbol
		case "text":
			have = fact.Text
		case "resolution":
			have = string(fact.Resolution)
		default:
			t.Fatalf("expected.json row %v uses unknown field %q", row, field)
		}
		if have != want {
			return false
		}
	}
	return true
}

func decodeIndex(t *testing.T, fixture, name string) programindex.Index {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixture, "artifacts", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	index, err := programindex.Decode(raw)
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return index
}

func decodeCatalog(t *testing.T, fixture, name string) *dependencies.Catalog {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixture, "artifacts", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	catalog, err := dependencies.Decode(raw)
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return &catalog
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

// materializeFixture copies the tracked fixture tree (without the expectation
// files) into a throwaway git repository and opens it as a corpus.
func materializeFixture(t *testing.T, fixture string) *corpus.Corpus {
	t.Helper()
	isolateFixtureGitEnvironment(t)
	destination := filepath.Join(t.TempDir(), "repository")
	copyFixtureTree(t, fixture, destination, map[string]struct{}{"expected.json": {}, "artifacts": {}, "REVISION": {}})
	runFixtureGit(t, destination, "init", "--quiet")
	runFixtureGit(t, destination, "add", "--all", "--")
	runFixtureGit(t, destination, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "--message", "fixture")
	repository, err := corpus.Open(t.Context(), destination)
	if err != nil {
		t.Fatalf("open fixture corpus: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := repository.Close(); closeErr != nil {
			t.Errorf("close fixture corpus: %v", closeErr)
		}
	})
	return repository
}

func copyFixtureTree(t *testing.T, source, destination string, skipTopLevel map[string]struct{}) {
	t.Helper()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatalf("create materialized fixture: %v", err)
	}
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if _, skip := skipTopLevel[relative]; skip {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture contains unsupported symbolic link %q", filepath.ToSlash(relative))
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("fixture contains unsupported non-regular file %q", filepath.ToSlash(relative))
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		return os.WriteFile(target, contents, mode)
	})
	if err != nil {
		t.Fatalf("copy fixture repository: %v", err)
	}
}

func isolateFixtureGitEnvironment(t *testing.T) {
	t.Helper()
	original := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")
		if !isFixtureGitOverride(name) {
			continue
		}
		original[name] = value
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset ambient %s: %v", name, err)
		}
	}
	for name, value := range map[string]string{
		"GIT_CONFIG_COUNT":    "0",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CONFIG_GLOBAL":   os.DevNull,
		"GIT_CONFIG_SYSTEM":   os.DevNull,
	} {
		if err := os.Setenv(name, value); err != nil {
			t.Fatalf("set isolated %s: %v", name, err)
		}
	}
	t.Cleanup(func() {
		for _, entry := range os.Environ() {
			name, _, _ := strings.Cut(entry, "=")
			if isFixtureGitOverride(name) {
				if err := os.Unsetenv(name); err != nil {
					t.Errorf("clear isolated %s: %v", name, err)
				}
			}
		}
		for name, value := range original {
			if err := os.Setenv(name, value); err != nil {
				t.Errorf("restore ambient %s: %v", name, err)
			}
		}
	})
}

func runFixtureGit(t *testing.T, repository string, arguments ...string) {
	t.Helper()
	commandArguments := append([]string{"-C", repository}, arguments...)
	command := exec.CommandContext(t.Context(), "git", commandArguments...)
	command.Env = append(fixtureGitEnvironment(os.Environ()),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("materialize fixture git repository (%s): %v: %s", strings.Join(arguments, " "), err, output)
	}
}

func fixtureGitEnvironment(base []string) []string {
	result := make([]string, 0, len(base)+3)
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if isFixtureGitOverride(name) {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func isFixtureGitOverride(name string) bool {
	switch name {
	case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
		"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
		"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS",
		"GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_CONFIG_NOSYSTEM":
		return true
	default:
		return strings.HasPrefix(name, "GIT_CONFIG_KEY_") ||
			strings.HasPrefix(name, "GIT_CONFIG_VALUE_")
	}
}
