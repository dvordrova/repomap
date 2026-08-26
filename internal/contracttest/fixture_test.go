package contracttest

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/programindex"
)

const fileInventoryContractVersion = 1

type fileInventoryContract struct {
	Version int                          `json:"version"`
	Entries []fileInventoryContractEntry `json:"entries"`
}

type fileInventoryContractEntry struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Executable bool   `json:"executable,omitempty"`
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve contract-test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func materializeFixtureRepository(t *testing.T, language string) (string, *corpus.Corpus) {
	t.Helper()
	isolateFixtureGitEnvironment(t)
	if language != "go" && language != "python" && language != "jsts" {
		t.Fatalf("unsupported fixture language %q", language)
	}
	root := repositoryRoot(t)
	source := filepath.Join(root, "testdata", "repositories", language)
	destination := filepath.Join(t.TempDir(), "repository")
	copyFixtureTree(t, source, destination)
	runFixtureGit(t, destination, "init", "--quiet")
	runFixtureGit(t, destination, "add", "--all", "--")

	repository, err := corpus.Open(t.Context(), destination)
	if err != nil {
		t.Fatalf("open %s fixture corpus: %v", language, err)
	}
	t.Cleanup(func() {
		if closeErr := repository.Close(); closeErr != nil {
			t.Errorf("close %s fixture corpus: %v", language, closeErr)
		}
	})
	assertFileInventory(t, language, repository)
	return destination, repository
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

func copyFixtureTree(t *testing.T, source, destination string) {
	t.Helper()
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		t.Fatalf("fixture repository %q is unavailable or not a directory: %v", source, err)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatalf("create materialized fixture: %v", err)
	}
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
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
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		mode := fileInfo.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		return os.WriteFile(target, contents, mode)
	})
	if err != nil {
		t.Fatalf("copy fixture repository: %v", err)
	}
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
		t.Fatalf("materialize fixture git index (%s): %v: %s", strings.Join(arguments, " "), err, output)
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

func assertFileInventory(t *testing.T, language string, repository *corpus.Corpus) {
	t.Helper()
	contractPath := filepath.Join(repositoryRoot(t), "testdata", "contracts", language+".files.json")
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read %s file-inventory contract: %v", language, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var contract fileInventoryContract
	if err := decoder.Decode(&contract); err != nil {
		t.Fatalf("decode %s file-inventory contract: %v", language, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("decode %s file-inventory contract trailing data: %v", language, err)
	}
	if contract.Version != fileInventoryContractVersion || contract.Entries == nil {
		t.Fatalf("invalid %s file-inventory contract version or entries", language)
	}
	seen := make(map[string]struct{}, len(contract.Entries))
	for position, entry := range contract.Entries {
		wantID := fmt.Sprintf("f%d", position+1)
		if entry.ID != wantID || entry.Path == "" || entry.Path != filepath.ToSlash(filepath.Clean(entry.Path)) {
			t.Fatalf("invalid %s file-inventory entry %d: %#v", language, position, entry)
		}
		if _, duplicate := seen[entry.Path]; duplicate {
			t.Fatalf("duplicate %s file-inventory path %q", language, entry.Path)
		}
		seen[entry.Path] = struct{}{}
	}
	if !sort.SliceIsSorted(contract.Entries, func(i, j int) bool {
		return contract.Entries[i].Path < contract.Entries[j].Path
	}) {
		t.Fatalf("%s file-inventory contract is not path-sorted", language)
	}

	actual := repository.Entries()
	if len(actual) != len(contract.Entries) {
		t.Fatalf("%s tracked-file inventory has %d entries, want %d; add every new fixture file to %s",
			language, len(actual), len(contract.Entries), contractPath)
	}
	for position, want := range contract.Entries {
		got := actual[position]
		if string(got.ID) != want.ID || got.Path != want.Path || got.Executable != want.Executable {
			t.Fatalf("%s tracked-file entry %d = %#v, want %#v; update %s",
				language, position, got, want, contractPath)
		}
	}
}

func assertProgramIndexRoundTrip(t *testing.T, index programindex.Index) {
	t.Helper()
	if err := index.Validate(); err != nil {
		t.Fatalf("validate ProgramIndex: %v", err)
	}
	encoded, err := programindex.Encode(index)
	if err != nil {
		t.Fatalf("encode ProgramIndex: %v", err)
	}
	decoded, err := programindex.Decode(encoded)
	if err != nil {
		t.Fatalf("decode ProgramIndex: %v", err)
	}
	if !reflect.DeepEqual(decoded, index) {
		t.Fatal("ProgramIndex canonical encode/decode changed the authority")
	}
}
