package run

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestCumulativePythonDispatchSharesParserAndPreservesEveryTarget(t *testing.T) {
	_, repository := cumulativeEvidenceRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	var targets []repositoryTypedTarget
	want := make(map[string]string)
	for _, native := range catalog.Entries {
		if native.ProjectDir != "." {
			continue // This test compares all views of the same source project.
		}
		target, err := newPythonRepositoryTypedTarget(native)
		if err != nil {
			t.Fatal(err)
		}
		targets = append(targets, target)
		input, err := pythonprogramindex.BuildInput(t.Context(), repository, native)
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.New(input)
		if err != nil {
			t.Fatal(err)
		}
		want[native.Ref] = index.SHA256
	}
	if len(targets) < 3 {
		t.Fatal("fixture must keep its library, callable launch and guards")
	}
	state, err := preparePythonRepositoryDispatchPlan(repositoryTargetPlan{
		Authorities: map[repositoryTargetAdapter]any{repositoryTargetAdapterPython: catalog},
	}, targets)
	if err != nil {
		t.Fatal(err)
	}
	// Count real interpreter launches, forwarding every argument to the actual
	// Python executable. The fixture still runs the complete production parser.
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	counter := filepath.Join(dir, "parser-calls")
	astCounter := filepath.Join(dir, "ast-calls")
	probe := fmt.Sprintf("import ast\n_real_ast_parse = ast.parse\ndef _record_ast_parse(*args, **kwargs):\n with open(%q, 'a') as log: log.write(kwargs.get('filename', '') + '\\n')\n return _real_ast_parse(*args, **kwargs)\nast.parse = _record_ast_parse\n", astCounter)
	wrapper := fmt.Sprintf("#!%s\nimport os, sys\nwith open(%q, 'a') as log: log.write('parse\\n')\nargs = sys.argv[1:]\nargs[args.index('-c') + 1] = %q + args[args.index('-c') + 1]\nos.execv(%q, [%q] + args)\n", python, counter, probe, python, python)
	if err := os.WriteFile(filepath.Join(dir, "python3"), []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		t.Fatal(err)
	}
	store := programindex.NewArtifactStore()
	var artifactPaths []string
	for _, target := range targets {
		binding, err := preparePythonRepositoryDispatchTarget(t.Context(), repositoryTargetDispatchOptions{}, target, state)
		if err != nil {
			t.Fatal(err)
		}
		page, err := buildRepositoryProgramPageAuthority(registry, repositoryProgramBuildRequest{
			Context: t.Context(), Corpus: repository, Target: target, Facts: binding.ProgramFacts,
		})
		if err != nil {
			t.Fatalf("%s: %v", target.Selector, err)
		}
		if page.ProgramIndex.SHA256 != want[target.Key.Ref] {
			t.Fatalf("shared parse changed exact target %s", target.Selector)
		}
		runDir := filepath.Join(dir, fmt.Sprintf("target-%d", len(artifactPaths)))
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := persistProgramPageIndex(runDir, page, store); err != nil {
			t.Fatal(err)
		}
		artifact := filepath.Join(runDir, programindex.ArtifactFilename)
		artifactPaths = append(artifactPaths, artifact)
		restored, err := programindex.ReadFile(artifact)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := programindex.Encode(page.ProgramIndex)
		after, _ := programindex.Encode(restored)
		if !bytes.Equal(before, after) {
			t.Fatalf("shared artifact changed %s", target.Selector)
		}
		if bytes.Contains(before, []byte(`"source_ref"`)) || bytes.Contains(before, []byte(`"storage_version"`)) {
			t.Fatal("sealed artifact retained builder storage fields")
		}
	}
	data, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if calls := strings.Count(string(data), "parse\n"); calls != 1 {
		t.Fatalf("one fixture project with %d targets started %d parsers", len(targets), calls)
	}
	astLog, err := os.ReadFile(astCounter)
	if err != nil {
		t.Fatal(err)
	}
	parsedPaths := make(map[string]bool)
	for _, file := range strings.Fields(string(astLog)) {
		if parsedPaths[file] {
			t.Fatalf("AST parsed repeatedly for %s", file)
		}
		parsedPaths[file] = true
	}
	if len(parsedPaths) != len(catalog.Entries[0].Modules) {
		t.Fatalf("parsed %d ASTs for %d source files", len(parsedPaths), len(catalog.Entries[0].Modules))
	}
	if _, err := os.Stat(filepath.Join(dir, "owner", "program-facts")); !os.IsNotExist(err) {
		t.Fatalf("builder facts were persisted: %v", err)
	}
	// Reading a moved complete cohort uses only its saved facts. A missing or
	// mismatched shared input cannot quietly yield a partial target.
	moved := dir + "-moved"
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(moved) })
	first := filepath.Join(moved, "target-0", programindex.ArtifactFilename)
	if _, err := programindex.ReadFile(first); err != nil {
		t.Fatalf("moved cohort: %v", err)
	}
	viewWire, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	var view map[string]any
	if err := json.Unmarshal(viewWire, &view); err != nil {
		t.Fatal(err)
	}
	view["sha256"] = strings.Repeat("0", 64)
	badView, _ := json.Marshal(view)
	if err := os.WriteFile(first, badView, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := programindex.ReadFile(first); err == nil {
		t.Fatal("mismatched target binding accepted")
	}
	t.Logf("%d exact indexes unchanged; %d source ASTs parsed once; no builder facts persisted", len(targets), len(parsedPaths))
}

func TestPythonSharedPreparationFailureKeepsHealthySibling(t *testing.T) {
	_, repository := cumulativeEvidenceRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	var good pythontarget.Target
	for _, target := range catalog.Entries {
		if target.Selector == "python:.:script:repomap-fixture" {
			good = target
		}
	}
	bad := good.Snapshot()
	bad.Selector += "-missing"
	bad.DisplayName += " missing"
	bad.Roots[0].Qualname = "missing_declaration"
	brokenCatalog, err := pythontarget.NewCatalog([]pythontarget.Target{bad}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bad = brokenCatalog.Entries[0]
	group := pythonRepositoryParserGroup{targets: []pythontarget.Target{bad, good}}
	if _, err := group.take(t.Context(), repository, bad); err == nil {
		t.Fatal("missing launch declaration was accepted")
	}
	if len(group.inputs) != 1 || group.inputs[good.Ref].Err != nil || len(group.inputs[good.Ref].Input.Objects) == 0 {
		t.Fatal("failed launch discarded its healthy sibling's already parsed facts")
	}
	input, err := group.take(t.Context(), repository, good)
	if err != nil {
		t.Fatalf("failed sibling refused a valid target: %v", err)
	}
	index, err := programindex.New(input)
	if err != nil || index.Target.Selector != good.Selector {
		t.Fatalf("healthy sibling lost its own program identity: %v", err)
	}
}
