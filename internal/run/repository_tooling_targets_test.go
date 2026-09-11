package run

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
)

func TestRepositoryToolingPathMatchesDirectoriesAtAnyDepth(t *testing.T) {
	for filePath, want := range map[string]bool{
		".claude/hooks/obsidian-session-context.py": true,
		".github/workflows/ci.yml":                  true,
		".github/actions/notify/main.go":            true,
		"packages/web/.vscode/launch.json":          true,
		".githubx/tool.py":                          false,
		"src/claude/hooks.py":                       false,
		"main.py":                                   false,
		".vscode":                                   false,
	} {
		if repositoryToolingPath(filePath) != want {
			t.Errorf("repositoryToolingPath(%q) = %v, want %v", filePath, !want, want)
		}
	}
}

// Morfeu's .claude/hooks/obsidian-session-context.py carries a main guard.
// It reached the portfolio as a native Python target with root "." and a
// launch_root observation, and every run ended with WARN "Target not
// analyzed" for it. Hooks, workflows and editor settings are not targets.
func TestToolingDirectoryScriptsAreNoTargetCandidates(t *testing.T) {
	guard := "def main():\n\treturn 0\n\nif __name__ == \"__main__\":\n\tmain()\n"
	repository := pythonTargetCorpus(t, map[string]string{
		"native/runtime.py":                guard,
		".claude/hooks/session-context.py": guard,
		".github/scripts/release.py":       guard,
		".vscode/tasks.py":                 guard,
	})
	discovery, err := discoverRepositoryTargets(t.Context(), repositoryTargetRuntimeOptions{Repository: repository, NoModel: true, DiscoverPython: true})
	if err != nil {
		t.Fatal(err)
	}
	restored := 0
	for _, adapter := range discovery.adapters {
		rows, err := adapter.RestoreFiles(adapter.RequiredFileRefs)
		if err != nil {
			t.Fatal(err)
		}
		restored += len(rows)
	}
	if restored != 4 {
		t.Fatalf("exact Python targets = %d, want the four guarded scripts before the tooling rule", restored)
	}
	native, err := repositoryNativeCandidates(repository, discovery)
	if err != nil {
		t.Fatal(err)
	}
	var offered []string
	for _, candidate := range native {
		info, ok := repository.Info(candidate.Row.FileRef)
		if !ok {
			t.Fatalf("candidate file %q is outside the corpus", candidate.Row.FileRef)
		}
		offered = append(offered, info.Entry.Path)
	}
	if !slices.Equal(offered, []string{"native/runtime.py"}) {
		t.Fatalf("native candidates = %v, want only the script outside tooling directories", offered)
	}

	runtimeRef := repositoryTargetRuntimeFileRef(t, repository, "native/runtime.py")
	response, err := json.Marshal(map[string]any{"default_file_ref": runtimeRef, "target_file_refs": []corpus.FileID{runtimeRef}})
	if err != nil {
		t.Fatal(err)
	}
	portfolio := &targetPortfolioClientStub{response: response}
	plan, err := selectRepositoryTargetPlanForRun(t.Context(), repositoryTargetRuntimeOptions{
		RepoName: "tooling", Repository: repository, DiscoverPython: true, Executor: llm.Executor{Enabled: false},
		Providers: func() (llm.Provider, error) { return portfolio, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if portfolio.calls != 1 || !strings.Contains(portfolio.prompt.User, "native/runtime.py") {
		t.Fatalf("portfolio request lost the ordinary script: calls=%d user=%s", portfolio.calls, portfolio.prompt.User)
	}
	for _, dir := range repositoryToolingDirectories {
		if strings.Contains(portfolio.prompt.User, dir+"/") {
			t.Fatalf("portfolio request offers a %s file: %s", dir, portfolio.prompt.User)
		}
	}
	if len(plan.Targets) != 1 || len(plan.Targets[0].FileRefs) != 1 || plan.Targets[0].FileRefs[0] != runtimeRef {
		t.Fatalf("plan targets = %+v, want the one ordinary script", plan.Targets)
	}
}
