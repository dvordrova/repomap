package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestRepositoryLanguagesActivatesJSTSProjects(t *testing.T) {
	for _, test := range []struct {
		name  string
		files map[string]string
		want  repositoryLanguageEvidence
	}{
		{
			name:  "root TypeScript package",
			files: map[string]string{"package.json": `{}`, "client/main.tsx": "export {}\n"},
			want:  repositoryLanguageEvidence{JavaScriptTypeScript: true},
		},
		{
			name:  "root JavaScript package",
			files: map[string]string{"package.json": `{}`, "client/main.jsx": "export {}\n"},
			want:  repositoryLanguageEvidence{JavaScriptTypeScript: true},
		},
		{
			name:  "root modern module package",
			files: map[string]string{"package.json": `{}`, "server/main.mts": "export {}\n"},
			want:  repositoryLanguageEvidence{JavaScriptTypeScript: true},
		},
		{
			name:  "single nested package is project authority",
			files: map[string]string{"web/package.json": `{}`, "web/main.ts": "export {}\n"},
			want:  repositoryLanguageEvidence{JavaScriptTypeScript: true},
		},
		{
			name:  "multiple nested packages activate multi-target discovery",
			files: map[string]string{"web/package.json": `{}`, "web/main.ts": "export {}\n", "admin/package.json": `{}`, "admin/main.ts": "export {}\n"},
			want:  repositoryLanguageEvidence{JavaScriptTypeScript: true},
		},
		{
			name:  "Go keeps priority evidence beside nested UI",
			files: map[string]string{"go.mod": "module example.com/app\n", "package.json": `{}`, "ui/main.ts": "export {}\n"},
			want:  repositoryLanguageEvidence{Go: true, JavaScriptTypeScript: true},
		},
		{
			name:  "Python keeps priority evidence beside root UI",
			files: map[string]string{"pyproject.toml": "[project]\nname='app'\n", "package.json": `{}`, "ui/main.js": "export {}\n"},
			want:  repositoryLanguageEvidence{Python: true, JavaScriptTypeScript: true},
		},
		{
			name:  "Python helper does not hide root TypeScript project evidence",
			files: map[string]string{"native/runtime.py": "print('helper')\n", "package.json": `{}`, "src/main.ts": "export {}\n"},
			want:  repositoryLanguageEvidence{Python: true, JavaScriptTypeScript: true},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := pythonTargetCorpus(t, test.files)
			if got := repositoryLanguages(repository); got != test.want {
				t.Fatalf("repositoryLanguages = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestSelectJSTSTargetRestoresCurrentRootProjectAfterFilePortfolio(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"package.json":    `{"name":"web"}`,
		"tsconfig.json":   `{}`,
		"client/main.tsx": "export {}\n",
	})
	project := jsTSTestProject(t, repository, "typescript")
	manifestRef, ok := repository.ID("package.json")
	if !ok {
		t.Fatal("package.json has no corpus ref")
	}
	response, err := json.Marshal(map[string]any{
		"default_file_ref": manifestRef,
		"target_file_refs": []corpus.FileID{manifestRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &targetPortfolioClientStub{response: response}
	discoveryCalls := 0
	selection, err := selectJSTSTargetForRun(
		context.Background(), "web", t.TempDir(), repository, "", nil,
		func() (llm.Provider, error) { return provider, nil },
		llm.Executor{Enabled: false},
		func(context.Context, *corpus.Corpus, string, string) (jstsproject.Result, error) {
			discoveryCalls++
			return project.Snapshot(), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Project.Project.Ref != project.Project.Ref ||
		selection.Project.ProgramTargetID != project.ProgramTargetID ||
		selection.Outcome.SelectedRef != project.Project.Ref ||
		selection.Outcome.SelectedTargets != 1 || selection.Outcome.SelectedFileRefs != 1 ||
		provider.calls != 1 || discoveryCalls != 1 {
		t.Fatalf("JavaScript/TypeScript selection = %#v; calls = %d/%d", selection, provider.calls, discoveryCalls)
	}
	if !strings.Contains(provider.prompt.User, `"path":"package.json"`) ||
		strings.Contains(strings.ToLower(provider.prompt.User), "application project") ||
		strings.Contains(provider.prompt.User, project.Project.Ref) ||
		strings.Contains(provider.prompt.User, project.ProgramTargetID) {
		t.Fatalf("TargetPortfolio request leaked adapter identity or omitted package candidate: %s", provider.prompt.User)
	}
	runDir := t.TempDir()
	_, index, catalog, err := buildJSTSProgramForRun(runDir, repository, selection, nil)
	if err != nil {
		t.Fatal(err)
	}
	if discoveryCalls != 1 || index.Target.ID != project.ProgramTargetID ||
		index.Target.Kind != "library" || len(index.Target.Seeds) != 0 ||
		catalog.Coverage.State != "complete" {
		t.Fatalf("one-pass projected authority = %#v / %#v; discovery calls = %d", index.Target, catalog.Coverage, discoveryCalls)
	}
	persisted, err := jstsproject.Load(runDir)
	if err != nil || persisted.SHA256 != project.SHA256 {
		t.Fatalf("persisted selected project = %#v, %v", persisted, err)
	}
}

func TestValidateJSTSProjectCorpusBindingAcceptsSelectedNestedManifest(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"front/package.json":  `{"name":"web"}`,
		"front/tsconfig.json": `{}`,
		"front/src/main.tsx":  "export {}\n",
	})
	project := jsTSTestProject(t, repository, "typescript")
	manifestRef, err := validateJSTSProjectCorpusBinding(repository, project)
	if err != nil {
		t.Fatal(err)
	}
	want, ok := repository.ID("front/package.json")
	if !ok || manifestRef != want || project.Project.Selector != "jsts:front/package.json" {
		t.Fatalf("nested binding = %q / %#v, want %q", manifestRef, project.Project, want)
	}
}

func TestValidateJSTSProjectCorpusBindingIncludesExactPackageBinary(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"package.json": `{"name":"tool","bin":{"tool":"./bin/tool"}}`,
		"bin/tool":     "#!/usr/bin/env node\n",
		"src/index.ts": "export const ready = true\n",
	})
	project := jsTSTestProject(t, repository, "typescript")
	binaryRef, ok := repository.ID("bin/tool")
	if !ok {
		t.Fatal("bin/tool has no corpus ref")
	}
	project.Project.Binaries = []jstsproject.PackageBinary{{
		Command: "tool", Path: "bin/tool", FileRef: string(binaryRef),
	}}
	if _, err := validateJSTSProjectCorpusBinding(repository, project); err != nil {
		t.Fatalf("exact package binary binding: %v", err)
	}
	project.Project.Binaries[0].FileRef = project.Project.ManifestFileRef
	if _, err := validateJSTSProjectCorpusBinding(repository, project); err == nil || !strings.Contains(err.Error(), "exact current FileRef") {
		t.Fatalf("stale package binary binding error = %v", err)
	}
}

func TestJSTSProductSurfaceCountIncludesCLI(t *testing.T) {
	result := jstsproject.Result{Surfaces: []jstsproject.Surface{
		{Kind: jstsproject.SurfaceCLI, Role: jstsproject.SurfaceProduct},
		{Kind: jstsproject.SurfaceTool, Role: jstsproject.SurfaceScript},
	}}
	if count := jsTSProductSurfaceCount(result); count != 1 {
		t.Fatalf("CLI product surface count = %d, want 1", count)
	}
}

func TestSelectJSTSNoProductProjectRemainsLibraryAuthority(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"package.json": `{"name":"library"}`,
		"src/index.ts": "export const answer = 42\n",
	})
	project := jsTSTestProject(t, repository, "typescript")
	if kind := jstsproject.TargetKind(project); kind != "library" {
		t.Fatalf("no-product target kind = %q, want library", kind)
	}
	if count := jsTSProductSurfaceCount(project); count != 0 {
		t.Fatalf("no-product surface count = %d, want zero", count)
	}
	manifestRef, _ := repository.ID("package.json")
	response, err := json.Marshal(map[string]any{
		"default_file_ref": manifestRef,
		"target_file_refs": []corpus.FileID{manifestRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &targetPortfolioClientStub{response: response}
	selection, err := selectJSTSTargetForRun(
		context.Background(), "library", t.TempDir(), repository, "", nil,
		func() (llm.Provider, error) { return provider, nil }, llm.Executor{},
		func(context.Context, *corpus.Corpus, string, string) (jstsproject.Result, error) {
			return project.Snapshot(), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Project.Project.Ref != project.Project.Ref ||
		strings.Contains(strings.ToLower(provider.prompt.User), "application") ||
		strings.Contains(strings.ToLower(provider.prompt.User), "runnable") {
		t.Fatalf("library project was promoted by target hypothesis: %s", provider.prompt.User)
	}
}

func TestSelectJSTSExplicitTargetRequiresExactProjectRefOrSelector(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"package.json": `{"name":"web"}`,
		"src/main.js":  "export {}\n",
	})
	project := jsTSTestProject(t, repository, "javascript")
	discoveredSelector := "not-called"
	discover := func(_ context.Context, _ *corpus.Corpus, _, selector string) (jstsproject.Result, error) {
		discoveredSelector = selector
		return project.Snapshot(), nil
	}
	selection, err := selectJSTSTargetForRun(
		context.Background(), "web", t.TempDir(), repository,
		project.Project.Selector, nil, nil, llm.Executor{}, discover,
	)
	if err != nil || selection.Project.Project.Ref != project.Project.Ref ||
		selection.Outcome.SelectedFileRefs != 0 || selection.Outcome.SelectedTargets != 1 ||
		discoveredSelector != project.Project.Selector {
		t.Fatalf("explicit JavaScript/TypeScript selection = %#v, %v", selection, err)
	}
	if _, err := selectJSTSTargetForRun(
		context.Background(), "web", t.TempDir(), repository,
		"package.json", nil, nil, llm.Executor{}, discover,
	); err == nil || !strings.Contains(err.Error(), project.Project.Selector) {
		t.Fatalf("inexact JavaScript/TypeScript target error = %v", err)
	}
	if discoveredSelector != "" {
		t.Fatalf("non-jsts override entered early package selection as %q", discoveredSelector)
	}
}

func TestSelectJSTSReportsOwnerPreparedNodeModulesRequirement(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"package.json": `{"name":"web"}`,
		"src/main.ts":  "export {}\n",
	})
	_, err := selectJSTSTargetForRun(
		context.Background(), "web", t.TempDir(), repository, "", nil, nil,
		llm.Executor{},
		func(context.Context, *corpus.Corpus, string, string) (jstsproject.Result, error) {
			return jstsproject.Result{}, fmt.Errorf("%w: cannot find manifest-declared package", jstsproject.ErrTypeScriptCompilerUnavailable)
		},
	)
	if err == nil || !strings.Contains(err.Error(), "owner must prepare") ||
		!strings.Contains(err.Error(), "repomap never installs packages") {
		t.Fatalf("owner-preparation error = %v", err)
	}
}

func TestJSTSOwnerPreparationGuidanceRequiresTypedCompilerFailure(t *testing.T) {
	if !jsTSOwnerPreparationError(fmt.Errorf("%w: typescript-api is not installed", jstsproject.ErrTypeScriptCompilerUnavailable)) {
		t.Fatal("typed compiler failure lost the owner-prepared node_modules requirement")
	}
	if jsTSOwnerPreparationError(errors.New("jsts project: TypeScript helper failed")) {
		t.Fatal("opaque helper failure was misreported as a missing owner-prepared dependency")
	}
}

func TestSelectJSTSRejectsProjectWithStaleManifestFileRef(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"package.json": `{"name":"web"}`,
		"src/main.ts":  "export {}\n",
	})
	project := jsTSTestProject(t, repository, "typescript")
	sourceRef, _ := repository.ID("src/main.ts")
	project.Project.ManifestFileRef = string(sourceRef)
	project, err := jstsproject.Seal(project)
	if err != nil {
		t.Fatal(err)
	}
	_, err = selectJSTSTargetForRun(
		context.Background(), "web", t.TempDir(), repository, project.Project.Selector,
		nil, nil, llm.Executor{},
		func(context.Context, *corpus.Corpus, string, string) (jstsproject.Result, error) {
			return project, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "exact current FileRef") {
		t.Fatalf("stale package.json binding error = %v", err)
	}
}

func TestMaterializeSelectedJSTSProjectsRetainsEveryExactPackageBinding(t *testing.T) {
	repository := pythonTargetCorpus(t, map[string]string{
		"admin/package.json": `{"name":"web"}`,
		"admin/src/main.ts":  "export const admin = true\n",
		"front/package.json": `{"name":"web"}`,
		"front/src/main.ts":  "export const front = true\n",
	})
	adminProject := jsTSTestProjectAt(
		t, repository, "typescript", "admin/package.json", "admin/src/main.ts",
	)
	frontProject := jsTSTestProjectAt(
		t, repository, "typescript", "front/package.json", "front/src/main.ts",
	)
	adminTarget, err := jstsproject.TargetFromResult(adminProject)
	if err != nil {
		t.Fatal(err)
	}
	frontTarget, err := jstsproject.TargetFromResult(frontProject)
	if err != nil {
		t.Fatal(err)
	}
	adminKey := repositoryTargetKey{Adapter: repositoryTargetAdapterJSTS, Ref: adminTarget.Ref}
	frontKey := repositoryTargetKey{Adapter: repositoryTargetAdapterJSTS, Ref: frontTarget.Ref}
	plan := repositoryTargetPlan{
		Targets: []repositoryTypedTarget{
			{
				Key: adminKey, Selector: adminTarget.Selector,
				FileRefs: []corpus.FileID{corpus.FileID(adminTarget.ManifestFileRef)}, JSTS: &adminTarget,
			},
			{
				Key: frontKey, Selector: frontTarget.Selector,
				FileRefs: []corpus.FileID{corpus.FileID(frontTarget.ManifestFileRef)}, JSTS: &frontTarget,
			},
		},
		Default: frontKey,
		Outcome: targetPortfolioRunOutcome{
			SelectedRef: frontKey.String(), SelectedTargets: 2,
			SelectedTargetRefs: []string{adminKey.String(), frontKey.String()},
		},
	}
	ordered, err := repositoryTargetExecutionOrder(plan)
	if err != nil {
		t.Fatal(err)
	}

	projects := map[string]jstsproject.Result{
		adminProject.Project.Selector: adminProject,
		frontProject.Project.Selector: frontProject,
	}
	calls := []string{}
	materialized, err := materializeSelectedJSTSProjects(
		context.Background(),
		repositoryTargetDispatchOptions{
			Repo: t.TempDir(), Corpus: repository, Plan: plan,
			DiscoverJSTSFn: func(
				_ context.Context, _ *corpus.Corpus, _ string, selector string,
			) (jstsproject.Result, error) {
				calls = append(calls, selector)
				project, ok := projects[selector]
				if !ok {
					return jstsproject.Result{}, fmt.Errorf("unexpected selector %q", selector)
				}
				return project.Snapshot(), nil
			},
		},
		ordered,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != frontTarget.Selector || calls[1] != adminTarget.Selector {
		t.Fatalf("materialization selectors = %v, want [%s %s]", calls, frontTarget.Selector, adminTarget.Selector)
	}
	if len(materialized) != 2 {
		t.Fatalf("materialized projects = %d, want 2", len(materialized))
	}
	for _, target := range []repositoryTypedTarget{plan.Targets[0], plan.Targets[1]} {
		project, ok := materialized[target.Key]
		if !ok {
			t.Fatalf("materialized project for %s is absent", target.Key.String())
		}
		if project.Project.Selector != target.Selector ||
			project.Project.ManifestFileRef != target.JSTS.ManifestFileRef {
			t.Fatalf("materialized project for %s = %#v", target.Key.String(), project.Project)
		}
		if err := validateJSTSTargetMaterialization(repository, *target.JSTS, project); err != nil {
			t.Fatalf("validate materialized project for %s: %v", target.Key.String(), err)
		}
	}
}

func jsTSTestProject(
	t *testing.T,
	repository *corpus.Corpus,
	language string,
) jstsproject.Result {
	t.Helper()
	manifestPath := ""
	for _, entry := range repository.Entries() {
		if path.Base(entry.Path) == "package.json" {
			if manifestPath != "" {
				t.Fatal("fixture has multiple package manifests")
			}
			manifestPath = entry.Path
		}
	}
	if manifestPath == "" {
		t.Fatal("fixture has no package.json")
	}
	var sourcePath string
	for _, entry := range repository.Entries() {
		isTypeScript := strings.HasSuffix(entry.Path, ".ts") || strings.HasSuffix(entry.Path, ".tsx")
		isJavaScript := strings.HasSuffix(entry.Path, ".js") || strings.HasSuffix(entry.Path, ".jsx")
		if (language == "typescript" && isTypeScript) || (language == "javascript" && isJavaScript) {
			sourcePath = entry.Path
			break
		}
	}
	if sourcePath == "" {
		t.Fatalf("fixture has no %s source", language)
	}
	return jsTSTestProjectAt(t, repository, language, manifestPath, sourcePath)
}

func jsTSTestProjectAt(
	t *testing.T,
	repository *corpus.Corpus,
	language string,
	manifestPath string,
	sourcePath string,
) jstsproject.Result {
	t.Helper()
	manifestRef, ok := repository.ID(manifestPath)
	if !ok || path.Base(manifestPath) != "package.json" {
		t.Fatalf("fixture manifest %q is unavailable", manifestPath)
	}
	sourceRef, ok := repository.ID(sourcePath)
	if !ok {
		t.Fatalf("fixture source %q is unavailable", sourcePath)
	}
	selector := "jsts:" + manifestPath
	projectRef := "project:root-package"
	if path.Dir(manifestPath) != "." {
		projectRef = "project:package:" + string(manifestRef)
	}
	moduleRef := "module:" + string(sourceRef)
	fileSHA256 := strings.Repeat("b", 64)
	sourceSHA256 := jsTSTestSourceDigest(sourcePath, string(sourceRef), fileSHA256)
	targetIdentity, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("c", 64), SourceSHA256: repository.SHA256(),
		Target: programindex.TargetInput{
			Language: language, Kind: "library", Name: "web", Selector: selector,
			Sources: []programindex.TargetSource{
				{FileRef: string(manifestRef), Path: manifestPath},
				{FileRef: string(sourceRef), Path: sourcePath},
			},
			AnchorFileRef: string(manifestRef), Seeds: []programindex.TargetSeedInput{},
		},
		Objects: []programindex.ObjectInput{}, Relations: []programindex.RelationInput{},
		Coverage: programindex.CoverageInput{Measured: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := jstsproject.Seal(jstsproject.Result{
		CorpusSHA256: repository.SHA256(), SourceSHA256: sourceSHA256,
		ProgramTargetID: targetIdentity.Target.ID,
		Project: jstsproject.Project{
			Ref: projectRef, Name: "web", PackagePath: "web",
			Language: language, Selector: selector, ManifestPath: manifestPath,
			ManifestFileRef:  string(manifestRef),
			ModuleResolution: "bundler", PathAliases: []jstsproject.PathAlias{},
			Scripts: []jstsproject.Script{}, SourceRoots: []string{path.Dir(sourcePath)},
			EntryFileRefs: []string{string(sourceRef)}, ToolConfigs: []jstsproject.ProjectFile{},
			Dependencies: []jstsproject.PackageDependency{},
		},
		Files: []jstsproject.File{{
			FileRef: string(sourceRef), Path: sourcePath, Language: language, Module: moduleRef,
			SHA256: fileSHA256,
		}},
		Declarations: []jstsproject.Declaration{{
			Ref: moduleRef, Kind: "module", Name: sourcePath, QualifiedName: sourcePath,
			Location: jstsproject.Location{Path: sourcePath, FileRef: string(sourceRef), Line: 1, Column: 1},
		}},
		Imports: []jstsproject.Import{}, Exports: []jstsproject.Export{},
		Calls: []jstsproject.Call{}, Surfaces: []jstsproject.Surface{},
		Routes: []jstsproject.Route{}, HTTPUses: []jstsproject.HTTPUse{},
		Contracts: []jstsproject.Contract{}, Resources: []jstsproject.Resource{},
		ProductPaths: []jstsproject.ProductPath{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func jsTSTestSourceDigest(fields ...string) string {
	digest := sha256.New()
	for _, field := range fields {
		_, _ = digest.Write([]byte(strconv.Itoa(len(field))))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write([]byte(field))
	}
	return hex.EncodeToString(digest.Sum(nil))
}
