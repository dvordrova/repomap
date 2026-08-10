package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func TestAllTargetsOfflinePublishesEveryExactTargetWithOneDefaultAndMetadata(t *testing.T) {
	repository := t.TempDir()
	for _, directory := range []string{"cmd/app", "cmd/helper", "core"} {
		if err := os.MkdirAll(filepath.Join(repository, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(repository, "go.mod"), "module example.com/all-targets\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repository, "cmd/app/main.go"), "package main\nimport \"example.com/all-targets/core\"\nfunc main() { core.Public() }\n")
	writeFile(t, filepath.Join(repository, "cmd/helper/main.go"), "package main\nfunc main() {}\n")
	writeFile(t, filepath.Join(repository, "core/core.go"), "package core\nfunc Public() {}\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod", "cmd/app/main.go", "cmd/helper/main.go", "core/core.go")
	commitTestRepository(t, repository)

	debugDir := t.TempDir()
	selectorFactoryCalls := 0
	var stderr bytes.Buffer
	err := runDefaultWithDeps(repository, []string{
		"--all-targets", "--target", "cmd/app", "--offline",
		"--no-open", "--no-serve", "--debug-dir", debugDir, "--depth", "1",
	}, defaultRunDeps{
		ctx: context.Background(), stdout: io.Discard, stderr: &stderr,
		newTargetPortfolioClient: func() (targetPortfolioClient, error) {
			selectorFactoryCalls++
			return nil, errors.New("explicit all-target default must bypass selector")
		},
	})
	if err != nil {
		t.Fatalf("run all targets: %v\n%s", err, stderr.String())
	}
	if selectorFactoryCalls != 0 {
		t.Fatalf("selector factory calls = %d", selectorFactoryCalls)
	}

	entries, err := os.ReadDir(debugDir)
	if err != nil {
		t.Fatal(err)
	}
	runDirs := make([]string, 0, 3)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(debugDir, entry.Name())
		if _, err := os.Stat(filepath.Join(runDir, "metadata.json")); err == nil {
			runDirs = append(runDirs, runDir)
		}
	}
	sort.Strings(runDirs)
	if len(runDirs) != 3 {
		t.Fatalf("published run dirs = %#v\n%s", runDirs, stderr.String())
	}

	var portfolioBytes []byte
	defaultRunID := ""
	moduleLibraryFound := false
	seenPackages := make(map[string]struct{})
	for _, runDir := range runDirs {
		metadataRaw, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
		if err != nil {
			t.Fatal(err)
		}
		var metadata debugdump.RunMeta
		if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
			t.Fatal(err)
		}
		if !metadata.EffectiveOptions.AllTargets ||
			metadata.EffectiveOptions.AnalysisTargetOverride != "cmd/app" {
			t.Fatalf("all-target metadata = %#v", metadata.EffectiveOptions)
		}
		seenPackages[metadata.AnalysisTargetPackage] = struct{}{}
		if metadata.AnalysisTargetKind == string(analysistarget.KindModuleLibrary) {
			moduleLibraryFound = true
			if metadata.AnalysisTargetPackage != "" ||
				metadata.AnalysisTargetModule != "example.com/all-targets" ||
				metadata.AnalysisTargetDisplayPath != "." {
				t.Fatalf("module-library metadata = %#v", metadata)
			}
		}
		if metadata.AnalysisTargetPackage == "example.com/all-targets/cmd/app" {
			defaultRunID = metadata.RunID
		}
		manifest, err := report.ReadRunManifest(runDir)
		if err != nil {
			t.Fatalf("read target manifest %s: %v", metadata.AnalysisTargetPackage, err)
		}
		if manifest.MaterialInputs.AnalysisTargetRef != metadata.AnalysisTargetRef ||
			manifest.MaterialInputs.TargetRunContainerSHA256 == "" ||
			manifest.MaterialInputs.TargetPagePortfolioSHA256 == "" {
			t.Fatalf("target manifest authority = %#v / %#v", metadata, manifest.MaterialInputs)
		}
		raw, err := os.ReadFile(filepath.Join(runDir, snapshot.TargetPagePortfolioArtifactFilename))
		if err != nil {
			t.Fatal(err)
		}
		if portfolioBytes == nil {
			portfolioBytes = raw
		} else if !bytes.Equal(portfolioBytes, raw) {
			t.Fatal("successful target runs have different portfolio bytes")
		}
	}
	if len(seenPackages) != 3 || defaultRunID == "" || !moduleLibraryFound {
		t.Fatalf("published packages/default = %#v / %q", seenPackages, defaultRunID)
	}
	finalizedBytes := make(map[string][]byte)
	for _, runDir := range runDirs {
		for _, name := range []string{
			"report.html",
			"report.json",
			report.RunManifestFilename,
			snapshot.TargetRunContainerArtifactFilename,
			snapshot.TargetPagePortfolioArtifactFilename,
		} {
			path := filepath.Join(runDir, name)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			finalizedBytes[path] = raw
		}
	}
	var recoveryOutput bytes.Buffer
	if err := runFinalizeTargetPagesCLI(
		[]string{"--run-dir", filepath.Join(debugDir, defaultRunID)},
		&recoveryOutput,
		io.Discard,
	); err != nil {
		t.Fatalf("idempotent target-page recovery: %v", err)
	}
	for path, want := range finalizedBytes {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("idempotent recovery changed %s", path)
		}
	}
	latest, err := os.Readlink(filepath.Join(debugDir, "latest"))
	if err != nil {
		t.Fatal(err)
	}
	if latest != defaultRunID {
		t.Fatalf("latest = %q, want default %q", latest, defaultRunID)
	}
}

func TestFinalizeTargetPagesRecoveryUsesExistingSiblingRunsWithoutReanalysis(t *testing.T) {
	repository := t.TempDir()
	for _, directory := range []string{"cmd/app", "cmd/helper"} {
		if err := os.MkdirAll(filepath.Join(repository, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(repository, "go.mod"), "module example.com/recover-pages\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repository, "cmd/app/main.go"), "package main\nfunc main() {}\n")
	writeFile(t, filepath.Join(repository, "cmd/helper/main.go"), "package main\nfunc main() {}\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod", "cmd/app/main.go", "cmd/helper/main.go")
	commitTestRepository(t, repository)

	debugDir := t.TempDir()
	interrupted := errors.New("test interruption before portfolio binding")
	var readyRuns []targetPublishedRun
	err := runDefaultWithDeps(repository, []string{
		"--all-targets", "--target", "cmd/app", "--offline",
		"--no-open", "--no-serve", "--debug-dir", debugDir, "--depth", "1",
	}, defaultRunDeps{
		ctx: context.Background(), stdout: io.Discard, stderr: io.Discard,
		newTargetPortfolioClient: func() (targetPortfolioClient, error) {
			return nil, errors.New("explicit all-target recovery setup must bypass selector")
		},
		finalizeTargetPages: func(
			_ snapshot.TargetRunContainer,
			_ snapshot.TargetPagePortfolio,
			runs []targetPublishedRun,
		) error {
			readyRuns = append([]targetPublishedRun(nil), runs...)
			for _, run := range runs {
				for _, name := range []string{"report.html", "report.json", report.RunManifestFilename} {
					info, statErr := os.Lstat(filepath.Join(run.RunDir, name))
					if statErr != nil || !info.Mode().IsRegular() {
						t.Fatalf("existing run %s is missing %s: %v", run.RunID, name, statErr)
					}
				}
				if _, statErr := os.Lstat(filepath.Join(run.RunDir, snapshot.TargetPagePortfolioArtifactFilename)); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("portfolio exists before finalization in %s: %v", run.RunID, statErr)
				}
			}
			return interrupted
		},
	})
	if !errors.Is(err, interrupted) {
		t.Fatalf("interrupted setup error = %v", err)
	}
	if len(readyRuns) != 2 {
		t.Fatalf("ready runs = %d", len(readyRuns))
	}
	defaultRunDir := ""
	args := []string{"--run-dir", ""}
	for _, run := range readyRuns {
		if run.Target.PackageDir == "cmd/app" {
			defaultRunDir = run.RunDir
			continue
		}
		args = append(args, "--sibling-run", run.RunDir)
	}
	if defaultRunDir == "" {
		t.Fatal("default run was not captured")
	}
	args[1] = defaultRunDir
	var stdout bytes.Buffer
	if err := runFinalizeTargetPagesCLI(args, &stdout, io.Discard); err != nil {
		t.Fatalf("recover existing target pages: %v", err)
	}
	if !strings.Contains(stdout.String(), "Finalized target pages: 2") {
		t.Fatalf("recovery output = %q", stdout.String())
	}
	for _, run := range readyRuns {
		manifest, err := report.ReadRunManifest(run.RunDir)
		if err != nil {
			t.Fatalf("recovered manifest %s: %v", run.RunID, err)
		}
		if manifest.MaterialInputs.TargetRunContainerSHA256 == "" ||
			manifest.MaterialInputs.TargetPagePortfolioSHA256 == "" {
			t.Fatalf("recovered manifest is unbound: %#v", manifest.MaterialInputs)
		}
	}
	finalizedBytes := make(map[string][]byte)
	for _, run := range readyRuns {
		for _, name := range []string{
			"report.html", "report.json", report.RunManifestFilename,
			snapshot.TargetRunContainerArtifactFilename,
			snapshot.TargetPagePortfolioArtifactFilename,
		} {
			path := filepath.Join(run.RunDir, name)
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			finalizedBytes[path] = raw
		}
	}
	if err := runFinalizeTargetPagesCLI(args, io.Discard, io.Discard); err != nil {
		t.Fatalf("repeat exact recovery command: %v", err)
	}
	for path, want := range finalizedBytes {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("repeat recovery changed %s", path)
		}
	}
}

func TestCollectTargetPageRunsInvokesEveryAdditionalTargetOnceAndIsolatesFailure(t *testing.T) {
	container := targetPageRuntimeContainer(t)
	defaultProjection := targetPageProjection(t, container, container.DefaultTargetRef)
	defaultRun := targetPublishedRun{
		RunID:  "run-default-1",
		RunDir: filepath.Join(t.TempDir(), "run-default-1"),
		Target: defaultProjection.Target,
	}

	calls := make(map[string]int)
	result, err := collectTargetPageRuns(
		container,
		defaultRun,
		func(projection snapshot.TargetRunProjection) string {
			return "run-" + projection.Target.Ref
		},
		func(scoped snapshot.Snapshot, projection snapshot.TargetRunProjection, runID string) (targetPublishedRun, error) {
			calls[projection.Target.Ref]++
			if scoped.AnalysisTarget == nil || scoped.AnalysisTarget.Ref != projection.Target.Ref ||
				scoped.TargetCatalog != nil {
				t.Fatalf("scoped target = %#v", scoped.AnalysisTarget)
			}
			if projection.DisplayPath == "cmd/helper" {
				return targetPublishedRun{}, errors.New("local typed failure detail")
			}
			return targetPublishedRun{
				RunID: runID, RunDir: filepath.Join(filepath.Dir(defaultRun.RunDir), runID), Target: projection.Target,
			}, nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != len(container.Targets)-1 {
		t.Fatalf("runner targets = %#v, want %d", calls, len(container.Targets)-1)
	}
	for ref, count := range calls {
		if ref == container.DefaultTargetRef || count != 1 {
			t.Fatalf("runner calls = %#v", calls)
		}
	}
	if len(result.Outcomes) != len(container.Targets) || len(result.Ready) != len(container.Targets)-1 {
		t.Fatalf("run set = %#v", result)
	}
	for index, outcome := range result.Outcomes {
		if outcome.TargetRef != container.Targets[index].Target.Ref {
			t.Fatalf("outcome order = %#v", result.Outcomes)
		}
		if container.Targets[index].DisplayPath == "cmd/helper" {
			if outcome.State != snapshot.TargetPageUnavailable || outcome.RunID != "" ||
				outcome.UnavailableCode != snapshot.TargetPageUnavailableTargetRunFailed {
				t.Fatalf("failure leaked into portfolio state: %#v", outcome)
			}
		}
	}
}

func TestCollectTargetPageRunsRejectsAutomaticGoTargetProvenanceDrift(t *testing.T) {
	container := targetPageRuntimeTwoTargetContainer(t)
	defaultProjection := targetPageProjection(t, container, container.DefaultTargetRef)
	runsDir := t.TempDir()
	defaultRun := targetPublishedRun{
		RunID: "run-default-auto", RunDir: filepath.Join(runsDir, "run-default-auto"),
		Target: defaultProjection.Target, RepositoryStateSHA256: strings.Repeat("a", 64),
		SelectedRevision: "revision", GoTarget: "linux/amd64",
		GoTargetSource: snapshot.GoTargetSelectionAuto, GoTargetBaseline: "darwin/amd64",
	}

	result, err := collectTargetPageRuns(
		container,
		defaultRun,
		func(snapshot.TargetRunProjection) string { return "run-sibling-auto" },
		func(_ snapshot.Snapshot, projection snapshot.TargetRunProjection, runID string) (targetPublishedRun, error) {
			return targetPublishedRun{
				RunID: runID, RunDir: filepath.Join(runsDir, runID), Target: projection.Target,
				RepositoryStateSHA256: defaultRun.RepositoryStateSHA256,
				SelectedRevision:      defaultRun.SelectedRevision,
				GoTarget:              defaultRun.GoTarget,
				GoTargetSource:        defaultRun.GoTargetSource,
				GoTargetBaseline:      "windows/amd64",
			}, nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Ready) != 1 || len(result.Outcomes) != 2 {
		t.Fatalf("provenance drift was not isolated: %#v", result)
	}
	for _, outcome := range result.Outcomes {
		if outcome.TargetRef == container.DefaultTargetRef {
			continue
		}
		if outcome.State != snapshot.TargetPageUnavailable ||
			outcome.UnavailableCode != snapshot.TargetPageUnavailableTargetRunFailed {
			t.Fatalf("provenance drift was not isolated: %#v", result)
		}
	}
}

func TestTwoTargetPipelinesEncloseRepeatedArchitectureOutputWithExactTargetContext(t *testing.T) {
	container := targetPageRuntimeTwoTargetContainer(t)
	defaultProjection := targetPageProjection(t, container, container.DefaultTargetRef)
	runsDir := t.TempDir()
	defaultRun := targetPublishedRun{
		RunID:  "run-default-1",
		RunDir: filepath.Join(runsDir, "run-default-1"),
		Target: defaultProjection.Target,
	}

	var console bytes.Buffer
	output := newRunOutput(&console)
	defaultContext := targetPageConsoleContext{
		DisplayPath: defaultProjection.DisplayPath,
		Scope:       analysisTargetSubject(defaultProjection.Target),
		RunID:       defaultRun.RunID,
		Role:        "default",
	}
	output.TargetPage("started", defaultContext)
	output.State("Architecture", "generated")
	output.TargetPage("complete", defaultContext)

	_, err := collectTargetPageRuns(
		container,
		defaultRun,
		func(snapshot.TargetRunProjection) string { return "run-sibling-1" },
		func(_ snapshot.Snapshot, projection snapshot.TargetRunProjection, runID string) (targetPublishedRun, error) {
			output.State("Architecture", "generated")
			return targetPublishedRun{
				RunID:  runID,
				RunDir: filepath.Join(runsDir, runID),
				Target: projection.Target,
			}, nil
		},
		output,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	got := console.String()
	if strings.Count(got, "Architecture:") != 2 {
		t.Fatalf("Architecture blocks = %d, want 2\n%s", strings.Count(got, "Architecture:"), got)
	}
	siblingProjection := container.Targets[0]
	if siblingProjection.Target.Ref == container.DefaultTargetRef {
		siblingProjection = container.Targets[1]
	}
	wantsInOrder := []string{
		"Target page:\n  state: started\n  target: " + defaultProjection.DisplayPath,
		"scope: " + analysisTargetSubject(defaultProjection.Target),
		"run: run-default-1",
		"role: default",
		"Architecture:\n  state: generated",
		"Target page:\n  state: complete\n  target: " + defaultProjection.DisplayPath,
		"Target page:\n  state: started\n  target: " + siblingProjection.DisplayPath,
		"scope: " + analysisTargetSubject(siblingProjection.Target),
		"run: run-sibling-1",
		"role: sibling",
		"Architecture:\n  state: generated",
		"Target page:\n  state: complete\n  target: " + siblingProjection.DisplayPath,
	}
	position := 0
	for _, want := range wantsInOrder {
		next := strings.Index(got[position:], want)
		if next < 0 {
			t.Fatalf("console does not contain %q after byte %d:\n%s", want, position, got)
		}
		position += next + len(want)
	}
}

func TestTargetNavigationUsesBackendRelativeSiblingLinksAndDisablesUnavailable(t *testing.T) {
	container := targetPageRuntimeContainer(t)
	outcomes := make([]snapshot.TargetPageOutcome, 0, len(container.Targets))
	currentRef := ""
	for _, projection := range container.Targets {
		outcome := snapshot.TargetPageOutcome{TargetRef: projection.Target.Ref}
		switch projection.DisplayPath {
		case "cmd/helper":
			outcome.State = snapshot.TargetPageUnavailable
			outcome.UnavailableCode = snapshot.TargetPageUnavailableTargetRunFailed
		default:
			outcome.State = snapshot.TargetPageReady
			outcome.RunID = "run-" + projection.Target.Ref
			if projection.Target.Ref != container.DefaultTargetRef {
				currentRef = projection.Target.Ref
			}
		}
		outcomes = append(outcomes, outcome)
	}
	portfolio, err := snapshot.BuildTargetPagePortfolio(container, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	navigation, err := targetNavigationForRun(container, portfolio, currentRef)
	if err != nil {
		t.Fatal(err)
	}
	for index, item := range navigation.Targets {
		page := portfolio.Targets[index]
		switch {
		case item.TargetRef == currentRef:
			if item.Href != "#/map" || !item.Available {
				t.Fatalf("current navigation = %#v", item)
			}
		case page.State == snapshot.TargetPageReady:
			want := "../" + page.RunID + "/report.html#/map"
			if item.Href != want || !item.Available {
				t.Fatalf("sibling navigation = %#v, want %q", item, want)
			}
		default:
			if item.Available || item.Href != "" {
				t.Fatalf("unavailable navigation = %#v", item)
			}
		}
	}
}

func TestTargetPageRecoveryMetadataBindsExecutableAndModuleLibraryIdentity(t *testing.T) {
	container := targetPageRuntimeContainer(t)
	seenPackage := false
	seenModuleLibrary := false
	for _, projection := range container.Targets {
		metadata := debugdump.RunMeta{
			RunID:                     "run-identity",
			AnalysisTargetRef:         projection.Target.Ref,
			AnalysisTargetKind:        string(projection.Target.Kind),
			AnalysisTargetModule:      projection.Target.ModulePath,
			AnalysisTargetDisplayPath: projection.Target.DisplayPath(),
			AnalysisTargetPackage:     projection.Target.PackagePath,
		}
		wire, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		var restored debugdump.RunMeta
		if err := json.Unmarshal(wire, &restored); err != nil {
			t.Fatal(err)
		}
		if !targetPageRecoveryMetadataMatches(restored, metadata.RunID, projection) {
			t.Fatalf("round-trip target metadata = %#v / %#v", restored, projection)
		}
		if projection.Target.PackagePath == "" {
			seenModuleLibrary = true
			if restored.AnalysisTargetPackage != "" {
				t.Fatalf("module library invented package metadata: %#v", restored)
			}
		} else {
			seenPackage = true
		}

		mutations := []func(*debugdump.RunMeta){
			func(value *debugdump.RunMeta) { value.RunID = "other-run" },
			func(value *debugdump.RunMeta) { value.AnalysisTargetRef = "other-target" },
			func(value *debugdump.RunMeta) { value.AnalysisTargetKind += "-drift" },
			func(value *debugdump.RunMeta) { value.AnalysisTargetModule += "/drift" },
			func(value *debugdump.RunMeta) { value.AnalysisTargetDisplayPath += "/drift" },
			func(value *debugdump.RunMeta) { value.AnalysisTargetPackage += "/drift" },
		}
		for index, mutate := range mutations {
			tampered := restored
			mutate(&tampered)
			if targetPageRecoveryMetadataMatches(tampered, metadata.RunID, projection) {
				t.Fatalf("accepted metadata tamper %d for %#v", index, projection.Target)
			}
		}
	}
	if !seenPackage || !seenModuleLibrary {
		t.Fatalf("metadata fixtures missed executable/module library: %#v", container.Targets)
	}
}

func TestTargetPageRecoveryRequiresExactGoTargetProvenance(t *testing.T) {
	tests := []struct {
		name    string
		options debugdump.EffectiveOptions
		want    bool
	}{
		{
			name:    "ordinary exact target",
			options: debugdump.EffectiveOptions{GoTarget: "darwin/amd64"},
			want:    true,
		},
		{
			name: "automatic exact target",
			options: debugdump.EffectiveOptions{
				GoTarget: "linux/amd64", GoTargetSource: snapshot.GoTargetSelectionAuto,
				GoTargetBaseline: "darwin/amd64",
			},
			want: true,
		},
		{
			name: "source without baseline",
			options: debugdump.EffectiveOptions{
				GoTarget: "linux/amd64", GoTargetSource: snapshot.GoTargetSelectionAuto,
			},
		},
		{
			name: "baseline without source",
			options: debugdump.EffectiveOptions{
				GoTarget: "linux/amd64", GoTargetBaseline: "darwin/amd64",
			},
		},
		{
			name: "same platform",
			options: debugdump.EffectiveOptions{
				GoTarget: "linux/amd64", GoTargetSource: snapshot.GoTargetSelectionAuto,
				GoTargetBaseline: "linux/amd64",
			},
		},
		{
			name: "architecture drift",
			options: debugdump.EffectiveOptions{
				GoTarget: "linux/arm64", GoTargetSource: snapshot.GoTargetSelectionAuto,
				GoTargetBaseline: "darwin/amd64",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validRecoveredGoTargetProvenance(test.options); got != test.want {
				t.Fatalf("validRecoveredGoTargetProvenance() = %t, want %t", got, test.want)
			}
		})
	}
}

func targetPageRuntimeContainer(t *testing.T) snapshot.TargetRunContainer {
	t.Helper()
	repository := t.TempDir()
	for _, directory := range []string{"cmd/app", "cmd/helper", "core"} {
		if err := os.MkdirAll(filepath.Join(repository, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(repository, "go.mod"), "module example.com/pages\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repository, "cmd/app/main.go"), "package main\nimport _ \"example.com/pages/core\"\nfunc main() {}\n")
	writeFile(t, filepath.Join(repository, "cmd/helper/main.go"), "package main\nfunc main() {}\n")
	writeFile(t, filepath.Join(repository, "core/core.go"), "package core\nfunc Public() {}\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod", "cmd/app/main.go", "cmd/helper/main.go", "core/core.go")

	deferred, err := snapshot.Build(snapshot.Options{
		RepoPath: repository, DeferAnalysisTargetResolution: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	refs := make(map[string]string)
	for _, entry := range deferred.TargetCatalog.Entries {
		refs[entry.DisplayPath] = entry.Candidate.Target.Ref
	}
	container, err := snapshot.BuildTargetRunContainer(deferred, snapshot.TargetRunSelection{
		DefaultTargetRef: refs["cmd/app"],
		TargetRefs:       []string{refs["cmd/app"], refs["cmd/helper"], refs["."]},
	})
	if err != nil {
		t.Fatal(err)
	}
	return container
}

func targetPageRuntimeTwoTargetContainer(t *testing.T) snapshot.TargetRunContainer {
	t.Helper()
	repository := t.TempDir()
	for _, directory := range []string{"cmd/app", "cmd/helper"} {
		if err := os.MkdirAll(filepath.Join(repository, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(repository, "go.mod"), "module example.com/two-pages\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repository, "cmd/app/main.go"), "package main\nfunc main() {}\n")
	writeFile(t, filepath.Join(repository, "cmd/helper/main.go"), "package main\nfunc main() {}\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod", "cmd/app/main.go", "cmd/helper/main.go")

	deferred, err := snapshot.Build(snapshot.Options{
		RepoPath: repository, DeferAnalysisTargetResolution: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	refs := make(map[string]string)
	for _, entry := range deferred.TargetCatalog.Entries {
		refs[entry.DisplayPath] = entry.Candidate.Target.Ref
	}
	container, err := snapshot.BuildTargetRunContainer(deferred, snapshot.TargetRunSelection{
		DefaultTargetRef: refs["cmd/app"],
		TargetRefs:       []string{refs["cmd/app"], refs["cmd/helper"]},
	})
	if err != nil {
		t.Fatal(err)
	}
	return container
}

func targetPageProjection(
	t *testing.T,
	container snapshot.TargetRunContainer,
	targetRef string,
) snapshot.TargetRunProjection {
	t.Helper()
	for _, projection := range container.Targets {
		if projection.Target.Ref == targetRef {
			return projection
		}
	}
	t.Fatalf("target ref %q is unavailable", targetRef)
	return snapshot.TargetRunProjection{}
}
