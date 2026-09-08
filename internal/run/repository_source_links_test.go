package run

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/freshness"
)

func TestStandaloneSourceGuardRejectsOnlyUncommittedCorpusPaths(t *testing.T) {
	for _, test := range []struct {
		name, path string
		outside    bool
		wantCorpus bool
	}{
		{name: "new source", path: "new.py", wantCorpus: true},
		{name: "ignored source", path: "ignored.py", wantCorpus: true},
		{name: "source with spaces", path: "new code/δelta.py", wantCorpus: true},
		{name: "explicit analysis exclusion", path: "excluded/new.py"},
		{name: "forbidden environment file", path: ".env.local"},
		{name: "dependency subtree", path: "node_modules/new.js"},
		{name: "unread local data", path: "scratch.bin"},
		{name: "outside analysis root", path: "outside.py", outside: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			gitRoot := t.TempDir()
			analysisRoot := filepath.Join(gitRoot, "nested")
			write := func(path, text string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(analysisRoot, "main.py"), "def main(): pass\n")
			write(filepath.Join(analysisRoot, ".gitignore"), "ignored.py\n")
			write(filepath.Join(analysisRoot, ".repomapignore"), "excluded/\n")
			// A committed basename at the Git root cannot authorize the same
			// name under the selected nested analysis directory.
			write(filepath.Join(gitRoot, "new.py"), "def unrelated(): pass\n")
			ordinaryGraphGit(t, gitRoot, "init")
			ordinaryGraphGit(t, gitRoot, "add", ".")
			ordinaryGraphGit(t, gitRoot, "-c", "user.name=repomap test", "-c", "user.email=repomap@example.test", "commit", "-m", "initial")
			pathRoot := analysisRoot
			if test.outside {
				pathRoot = gitRoot
			}
			write(filepath.Join(pathRoot, filepath.FromSlash(test.path)), "def new(): pass\n")
			repository, err := corpus.Open(t.Context(), analysisRoot)
			if err != nil {
				t.Fatal(err)
			}
			defer repository.Close()
			_, inCorpus := repository.ID(test.path)
			if inCorpus != test.wantCorpus {
				t.Fatalf("test source in corpus = %t, want %t", inCorpus, test.wantCorpus)
			}
			state, err := freshness.CaptureRepository(t.Context(), analysisRoot, repository)
			if err != nil {
				t.Fatal(err)
			}
			analysisRoot, err = resolveAnalysisRoot(analysisRoot)
			if err != nil {
				t.Fatal(err)
			}
			if repositoryCorpusHasWorkingTreeChanges(repository, analysisRoot, state) {
				t.Fatal("test must expose a path missed by the tracked-change guard")
			}
			for _, host := range []string{"GitHub", "GitLab", ""} {
				err := validateRepositorySourceLinks(t.Context(), host, repository, analysisRoot, state)
				wantError := host != "" && test.wantCorpus
				if !wantError {
					if err != nil {
						t.Fatalf("%q publication rejected unrelated file or local serving: %v", host, err)
					}
					continue
				}
				if err == nil || !strings.Contains(err.Error(), test.path) || !strings.Contains(err.Error(), state.Head) || !strings.Contains(err.Error(), "local serving") {
					t.Fatalf("%s missing-source error = %v", host, err)
				}
				if test.name == "new source" {
					// Exercise the ordinary flag path before target work. The
					// no-model flag guarantees this regression never calls a provider.
					args := []string{"--no-model", "--target", "python:.:guard:main", "--no-open", "--debug-dir", t.TempDir(),
						"--" + strings.ToLower(host) + "-url", "https://" + strings.ToLower(host) + ".com/team/project"}
					err := runDefaultWithDeps(analysisRoot, args, defaultRunDeps{stdout: io.Discard, stderr: io.Discard})
					if err == nil || !strings.Contains(err.Error(), "absent from captured revision") || !strings.Contains(err.Error(), test.path) {
						t.Fatalf("ordinary %s path bypassed the missing-source guard: %v", host, err)
					}
				}
			}
		})
	}
}
