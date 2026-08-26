package jstsproject

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestHelperUsesPreparedLocalCompilerAndBindsSourceBytes(t *testing.T) {
	root := preparedTempProject(t)
	tracked := []string{
		"package.json", "postcss.config.js", "src/excluded.ts", "src/main.ts",
		"src/collisions.ts", "src/lookalike.js", "src/one.ts", "src/two.ts", "src/view.tsx", "src/widget.jsx", "tsconfig.json",
	}
	listing := gitfiles.Listing{Paths: tracked, RegularPaths: tracked}
	firstCorpus, err := corpus.New(context.Background(), root, listing)
	if err != nil {
		t.Fatal(err)
	}
	first, firstIndex, firstCatalog, err := Build(context.Background(), firstCorpus, root)
	firstCorpus.Close()
	if err != nil {
		t.Fatal(err)
	}
	if first.Project.ModuleResolution != "bundler" || first.Project.Language != "typescript" {
		t.Fatalf("project = %#v", first.Project)
	}
	wantFiles := []string{"postcss.config.js", "src/collisions.ts", "src/lookalike.js", "src/main.ts", "src/one.ts", "src/two.ts", "src/view.tsx", "src/widget.jsx"}
	gotFiles := make([]string, len(first.Files))
	for index, file := range first.Files {
		gotFiles[index] = file.Path
	}
	if strings.Join(gotFiles, "\n") != strings.Join(wantFiles, "\n") {
		t.Fatalf("project-selected deterministic files = %#v, want %#v", gotFiles, wantFiles)
	}
	if !hasExactImport(first, "@/one", "src/one.ts") || !hasExactImport(first, "@/two", "src/two.ts") {
		t.Fatalf("path-alias imports were not resolved exactly: %#v", first.Imports)
	}
	qualifiedSame := map[string]bool{}
	for _, declaration := range first.Declarations {
		if declaration.Name == "same" {
			qualifiedSame[declaration.QualifiedName] = true
		}
	}
	if !qualifiedSame["src/one#same"] || !qualifiedSame["src/two#same"] || len(qualifiedSame) != 2 {
		t.Fatalf("same-name qualified declarations were merged: %#v", qualifiedSame)
	}
	if len(first.Routes) != 0 || len(first.Surfaces) != 0 {
		t.Fatalf("framework-name lookalikes gained route/surface authority: %#v / %#v", first.Routes, first.Surfaces)
	}
	foundExactTS := false
	foundBoundedExpression := false
	for _, call := range first.Calls {
		if call.Location.Path == "src/main.ts" && call.Resolution == "exact" {
			foundExactTS = true
		}
		if strings.HasPrefix(call.Expression, `["abc",`) {
			foundBoundedExpression = true
			if len(call.Expression) > 512 || call.Expression != strings.TrimSpace(call.Expression) {
				t.Fatalf("bounded call expression is not valid ProgramIndex witness text: %q", call.Expression)
			}
		}
	}
	if !foundExactTS {
		t.Fatalf("TypeScript calls = %#v", first.Calls)
	}
	if !foundBoundedExpression {
		t.Fatalf("long call expression was not retained: %#v", first.Calls)
	}
	assertReceiverlessObjectMethodProjection(t, first, firstIndex)
	assertEveryMethodHasExactTypeReceiver(t, firstIndex)
	assertCompilerResolvedInvocationProjection(t, first, firstIndex)
	for _, dependency := range firstCatalog.Dependencies {
		if dependency.PackagePath == javascriptPlatform {
			t.Fatalf("JavaScript platform authority leaked into dependency catalog: %#v", dependency)
		}
	}
	foundUnresolvedDynamic := false
	foundUnresolvedPropertyNameCollision := false
	for _, call := range first.Calls {
		if call.Location.Path == "src/lookalike.js" && call.Resolution == "unresolved" {
			foundUnresolvedDynamic = true
		}
		if call.Location.Path == "src/lookalike.js" && strings.Contains(call.Expression, ".test") {
			if call.Resolution != "unresolved" || len(call.CalleeRefs) != 0 {
				t.Fatalf("property-name collision invented call targets: %#v", call)
			}
			foundUnresolvedPropertyNameCollision = true
		}
		if strings.HasSuffix(call.Location.Path, ".js") || strings.HasSuffix(call.Location.Path, ".jsx") {
			if call.Resolution == "exact" {
				t.Fatalf("JavaScript-origin call retained exact authority: %#v", call)
			}
		}
	}
	if !foundUnresolvedDynamic {
		t.Fatalf("dynamic JavaScript call frontier was not retained: %#v", first.Calls)
	}
	if !foundUnresolvedPropertyNameCollision {
		t.Fatalf("property-name collision frontier was not retained: %#v", first.Calls)
	}
	for _, relation := range firstIndex.Relations {
		if relation.Location != nil && relation.Location.Path == "postcss.config.js" && relation.Kind == "calls" {
			if relation.Resolution != "alternatives" || len(relation.Witnesses) != 1 || relation.Witnesses[0].Kind != "javascript_call_candidate" {
				t.Fatalf("JavaScript ProgramIndex authority = %#v", relation)
			}
		}
	}

	writeTestFile(t, root, "src/main.ts", "export function hello(): string { return \"changed\" }\nhello()\n")
	secondCorpus, err := corpus.New(context.Background(), root, listing)
	if err != nil {
		t.Fatal(err)
	}
	second, secondIndex, _, err := Build(context.Background(), secondCorpus, root)
	secondCorpus.Close()
	if err != nil {
		t.Fatal(err)
	}
	if first.CorpusSHA256 != second.CorpusSHA256 {
		t.Fatal("path-only corpus identity unexpectedly changed")
	}
	if first.SourceSHA256 == second.SourceSHA256 || first.SHA256 == second.SHA256 || firstIndex.SourceSHA256 == secondIndex.SourceSHA256 {
		t.Fatal("source-byte change did not alter adapter/index identity")
	}

	writeTestFile(t, root, "tsconfig.json", `{"include":["src/**/*"],"exclude":["src/excluded.ts"],"compilerOptions":{"allowJs":true,"module":"ESNext","moduleResolution":"bundler","jsx":"preserve","baseUrl":".","paths":{"@/*":["./src/*"],"#/*":["./src/*"]},"strict":true}}`)
	thirdCorpus, err := corpus.New(context.Background(), root, listing)
	if err != nil {
		t.Fatal(err)
	}
	third, thirdIndex, _, err := Build(context.Background(), thirdCorpus, root)
	thirdCorpus.Close()
	if err != nil {
		t.Fatal(err)
	}
	if second.SourceSHA256 != third.SourceSHA256 {
		t.Fatal("configuration-only change unexpectedly altered selected source-byte identity")
	}
	if second.SHA256 == third.SHA256 || secondIndex.ScenarioSHA256 == thirdIndex.ScenarioSHA256 {
		t.Fatal("material project-configuration change did not alter semantic scenario identity")
	}
}

func TestCumulativeJSTSRepositoryCompilerAndProgramIndexContract(t *testing.T) {
	root := preparedCompilerProject(t)
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve JSTS contract-test source path")
	}
	fixtureRoot := filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "repositories", "jsts")
	tracked := []string{"package.json", "src/platform.ts", "tsconfig.json"}
	for _, relative := range tracked {
		contents, err := os.ReadFile(filepath.Join(fixtureRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read cumulative JSTS fixture %s: %v", relative, err)
		}
		writeTestFile(t, root, relative, string(contents))
	}
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, index, catalog, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != Version || result.HelperVersion != HelperVersion ||
		result.Project.Language != "typescript" || result.Project.Name != "repomap-cumulative-jsts-fixture" {
		t.Fatalf("cumulative JSTS producer identity = %#v", result.Project)
	}
	if err := ValidateProgramIndex(result, index); err != nil {
		t.Fatalf("validate cumulative JSTS ProgramIndex: %v", err)
	}
	encoded, err := programindex.Encode(index)
	if err != nil {
		t.Fatalf("encode cumulative JSTS ProgramIndex: %v", err)
	}
	if _, err := programindex.Decode(encoded); err != nil {
		t.Fatalf("round-trip cumulative JSTS ProgramIndex: %v", err)
	}

	calls := make(map[string]Call, len(result.Calls))
	for _, call := range result.Calls {
		calls[call.Expression] = call
	}
	for expression, externalName := range map[string]string{
		"new Date": "Date", "new Image": "Image", "new Promise": "Promise",
	} {
		call, exists := calls[expression]
		if !exists || call.Invocation != "construct" || call.Resolution != "exact" ||
			call.ExternalPackage != javascriptPlatform || call.ExternalName != externalName ||
			len(call.CalleeRefs) != 0 {
			t.Fatalf("cumulative platform constructor %q = %#v", expression, call)
		}
	}
	for expression, target := range map[string]struct {
		receiver string
		name     string
	}{
		"this.context.beginPath": {receiver: "CanvasRenderingContext2D", name: "beginPath"},
		"this.context.moveTo":    {receiver: "CanvasRenderingContext2D", name: "moveTo"},
		"this.context.lineTo":    {receiver: "CanvasRenderingContext2D", name: "lineTo"},
		"this.context.stroke":    {receiver: "CanvasRenderingContext2D", name: "stroke"},
		"canvas.getContext":      {receiver: "HTMLCanvasElement", name: "getContext"},
		"Math.min":               {receiver: "Math", name: "min"},
		"console.log":            {receiver: "Console", name: "log"},
	} {
		call, exists := calls[expression]
		if !exists || call.Invocation != "call" || call.Resolution != "exact" ||
			call.ExternalPackage != javascriptPlatform || call.ExternalReceiver != target.receiver ||
			call.ExternalName != target.name || len(call.CalleeRefs) != 0 {
			t.Fatalf("cumulative platform call %q = %#v, want %#v", expression, call, target)
		}
	}
	localConstructor := calls["new LevelDrawer"]
	if localConstructor.Invocation != "construct" || localConstructor.Resolution != "exact" ||
		localConstructor.ExternalPackage != "" || len(localConstructor.CalleeRefs) != 1 {
		t.Fatalf("cumulative local constructor = %#v", localConstructor)
	}
	localTyped := calls["new localDateConstructor"]
	if localTyped.Invocation != "construct" || localTyped.Resolution != "unresolved" ||
		localTyped.ExternalPackage != "" || len(localTyped.CalleeRefs) != 0 {
		t.Fatalf("cumulative local typed-constructor frontier = %#v", localTyped)
	}
	for _, dependency := range catalog.Dependencies {
		if dependency.PackagePath == javascriptPlatform {
			t.Fatalf("JavaScript platform authority leaked into cumulative dependency catalog: %#v", dependency)
		}
	}
}

func TestPreparedJavaScriptJSXProjectUsesWeakerAuthority(t *testing.T) {
	root := preparedTempProject(t)
	writeTestFile(t, root, "jsconfig.json", `{"include":["src/**/*.js","src/**/*.jsx"],"compilerOptions":{"allowJs":true,"checkJs":false,"module":"ESNext","moduleResolution":"bundler","jsx":"preserve"}}`)
	tracked := []string{"jsconfig.json", "package.json", "src/lookalike.js", "src/widget.jsx"}
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, index, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.Language != "javascript" || index.Target.Language != "javascript" || TargetKind(result) != "library" {
		t.Fatalf("pure JavaScript/JSX project authority = %#v / %#v", result.Project, index.Target)
	}
	for _, call := range result.Calls {
		if call.Resolution == "exact" {
			t.Fatalf("pure JavaScript/JSX call gained exact authority: %#v", call)
		}
	}
}

func TestPreparedPackageBinaryAndDevEntryRemainSeparateCLIAuthorities(t *testing.T) {
	root := preparedTempProject(t)
	writeTestFile(t, root, "package.json", `{
  "name":"opencode",
  "bin":{"opencode":"./bin/opencode"},
  "scripts":{
    "dev":"bun run --conditions=browser ./src/index.ts",
    "dev:temporary":"bun run ./src/temporary.ts"
  },
  "devDependencies":{"typescript":"5.9.3"}
}`)
	writeTestFile(t, root, "tsconfig.json", `{
  "include":["src/index.ts","src/temporary.ts"],
  "compilerOptions":{"module":"ESNext","moduleResolution":"bundler","strict":true}
}`)
	writeTestFile(t, root, "bin/opencode", "#!/usr/bin/env node\n")
	writeTestFile(t, root, "src/index.ts", "export const command = 'opencode'\n")
	writeTestFile(t, root, "src/temporary.ts", "export const temporary = true\n")
	tracked := []string{"bin/opencode", "package.json", "src/index.ts", "src/temporary.ts", "tsconfig.json"}
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked, ExecutablePaths: []string{"bin/opencode"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, index, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Project.Binaries) != 1 ||
		result.Project.Binaries[0].Command != "opencode" ||
		result.Project.Binaries[0].Path != "bin/opencode" {
		t.Fatalf("package binaries = %#v", result.Project.Binaries)
	}
	assertSurface(t, result, SurfaceCLI, SurfaceProduct)
	for _, surface := range result.Surfaces {
		if surface.Kind == SurfaceCLI && len(surface.EntryRefs) != 0 {
			t.Fatalf("CLI surface invented bin-to-source entry refs: %#v", surface)
		}
	}
	if index.Target.Kind != "application" || len(index.Target.Seeds) != 1 || index.Target.Seeds[0].Kind != programindex.SeedScript {
		t.Fatalf("CLI ProgramTarget = %#v", index.Target)
	}
	seededPath := ""
	for _, object := range index.Objects {
		if object.ID == index.Target.Seeds[0].ObjectID && object.Location != nil {
			seededPath = object.Location.Path
		}
	}
	if seededPath != "src/index.ts" {
		t.Fatalf("CLI runtime seed path = %q, want src/index.ts", seededPath)
	}
}

func TestPreparedNamelessPackageUsesLockfileIdentityWithoutInventingImplicitBin(t *testing.T) {
	root := preparedTempProject(t)
	writeTestFile(t, root, "package.json", `{
  "bin":"./bin/meetup",
  "devDependencies":{"typescript":"5.9.3"}
}`)
	writeTestFile(t, root, "package-lock.json", `{
  "name":"meetup",
  "lockfileVersion":3,
  "packages":{"":{"devDependencies":{"typescript":"5.9.3"}}}
}`)
	writeTestFile(t, root, "bun.lock", "lockfileVersion = 1\n")
	writeTestFile(t, root, "bin/meetup", "#!/usr/bin/env node\n")
	tracked := []string{"bin/meetup", "bun.lock", "package-lock.json", "package.json", "src/main.ts", "tsconfig.json"}
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked, ExecutablePaths: []string{"bin/meetup"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, index, catalog, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.Name != "meetup" || result.Project.PackagePath != "meetup" ||
		index.Target.Name != "meetup" || len(catalog.Importers) != 1 || catalog.Importers[0].Name != "meetup" {
		t.Fatalf("lockfile-restored project identity = %#v / %#v / %#v", result.Project, index.Target, catalog.Importers)
	}
	if result.Project.PackageManager != "bun" || result.Project.LockfilePath != "bun.lock" {
		t.Fatalf("package-manager lockfile facts = %#v, want bun/bun.lock", result.Project)
	}
	if len(result.Project.Binaries) != 0 || hasCLIProductSurface(result) {
		t.Fatalf("lockfile name invented package.json string-bin command authority: %#v / %#v", result.Project.Binaries, result.Surfaces)
	}

	fallbackTracked := []string{"bin/meetup", "package.json", "src/main.ts", "tsconfig.json"}
	fallbackRepository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: fallbackTracked, RegularPaths: fallbackTracked, ExecutablePaths: []string{"bin/meetup"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer fallbackRepository.Close()
	fallback, fallbackIndex, fallbackCatalog, err := Build(context.Background(), fallbackRepository, root)
	if err != nil {
		t.Fatal(err)
	}
	if fallback.Project.Name != "root-package" || fallback.Project.PackagePath != "root-package" ||
		fallbackIndex.Target.Name != "root-package" || len(fallbackCatalog.Importers) != 1 ||
		fallbackCatalog.Importers[0].Name != "root-package" {
		t.Fatalf("repository-relative root fallback = %#v / %#v / %#v", fallback.Project, fallbackIndex.Target, fallbackCatalog.Importers)
	}
	if len(fallback.Project.Binaries) != 0 || hasCLIProductSurface(fallback) {
		t.Fatalf("root fallback invented package.json string-bin command authority: %#v / %#v", fallback.Project.Binaries, fallback.Surfaces)
	}
}

func TestNamelessPackageIdentityFallbackIsRepositoryRelative(t *testing.T) {
	if got := selectedPackageIdentityName(nil, ".", ""); got != "root-package" {
		t.Fatalf("root package fallback = %q, want root-package", got)
	}
	if got := selectedPackageIdentityName(nil, "packages/web", ""); got != "packages/web" {
		t.Fatalf("nested package fallback = %q, want packages/web", got)
	}
	if got := selectedPackageIdentityName(nil, ".", " declared "); got != "declared" {
		t.Fatalf("manifest package name = %q, want declared", got)
	}
}

func TestReadPackageManifestRejectsInvalidEnvelope(t *testing.T) {
	for _, content := range []string{
		`null`,
		`{"name":"active"}{"name":"shadow"}`,
	} {
		root := t.TempDir()
		writeTestFile(t, root, "package.json", content)
		repository, err := corpus.New(
			context.Background(), root,
			gitfiles.Listing{Paths: []string{"package.json"}, RegularPaths: []string{"package.json"}},
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := readPackageManifest(repository, "package.json"); err == nil ||
			!strings.Contains(err.Error(), "invalid package manifest") {
			t.Fatalf("package manifest %q error = %v", content, err)
		}
		if err := repository.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestManifestTypeScriptCompilerPackagesKeepDirectAndNpmAliasAuthority(t *testing.T) {
	manifest := packageManifest{
		Dependencies: map[string]string{
			"runtime":        "1.0.0",
			"typescript-api": "npm:typescript@6.0.3",
		},
		DevDependencies: map[string]string{
			"typescript": "^7.0.2",
			"lookalike":  "npm:not-typescript@1.0.0",
		},
	}
	got := typeScriptCompilerPackageNames(manifest)
	want := []string{"typescript", "typescript-api"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("TypeScript compiler package candidates = %#v, want %#v", got, want)
	}
}

func TestSelectedManifestCompilerDeclarationsDoNotMixWithRootFallback(t *testing.T) {
	selected := packageManifest{DevDependencies: map[string]string{"typescript": "7.0.2"}}
	root := packageManifest{DevDependencies: map[string]string{"typescript-api": "npm:typescript@6.0.3"}}
	got := typeScriptCompilerPackagesForProject(selected, &root)
	if len(got) != 1 || got[0].Name != "typescript" || got[0].ResolutionBase != helperCompilerResolutionProject {
		t.Fatalf("selected compiler candidates = %#v", got)
	}
	if fallback := typeScriptCompilerPackagesForProject(packageManifest{}, &root); len(fallback) != 1 ||
		fallback[0].Name != "typescript-api" || fallback[0].ResolutionBase != helperCompilerResolutionRepositoryRoot {
		t.Fatalf("root compiler fallback candidates = %#v", fallback)
	}
}

func TestRepositoryRootCompilerFallbackCannotBeShadowedBySelectedChild(t *testing.T) {
	repositoryRoot := preparedCompilerProject(t)
	rootNodeModules := filepath.Join(repositoryRoot, "node_modules")
	if err := os.Rename(filepath.Join(rootNodeModules, "typescript"), filepath.Join(rootNodeModules, "typescript-api")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, repositoryRoot, "package.json", `{
  "name":"workspace",
  "workspaces":["front"],
  "scripts":{"dev":"bun run --cwd front src/main.ts"},
  "devDependencies":{"typescript-api":"npm:typescript@5.9.3"}
}`)
	writeTestFile(t, repositoryRoot, "front/package.json", `{"name":"front"}`)
	writeTestFile(t, repositoryRoot, "front/tsconfig.json", `{
  "include":["src/**/*.ts"],
  "compilerOptions":{"module":"ESNext","moduleResolution":"bundler","strict":true}
}`)
	writeTestFile(t, repositoryRoot, "front/src/main.ts", "export const ready: boolean = true\n")

	// This deliberately has the same install name as the root alias. It is a
	// supported-looking legacy package shape, but not a usable compiler. A
	// child-based require would select it and fail; root-owned fallback
	// authority must resolve from the repository-root manifest instead.
	writeTestFile(t, repositoryRoot, "front/node_modules/typescript-api/package.json", `{"name":"typescript","version":"0.0.0"}`)
	writeTestFile(t, repositoryRoot, "front/node_modules/typescript-api/lib/typescript.js", `module.exports = {}`)

	tracked := []string{
		"front/package.json",
		"front/src/main.ts",
		"front/tsconfig.json",
		"package.json",
	}
	repository, err := corpus.New(
		context.Background(), repositoryRoot,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, err := Discover(context.Background(), repository, repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.ManifestPath != "front/package.json" || len(result.Files) != 1 || result.Files[0].Path != "front/src/main.ts" {
		t.Fatalf("root-fallback compiler project = %#v; files = %#v", result.Project, result.Files)
	}
}

func TestPreparedCompilerPrefersOneLegacyAPICandidateOverNativeAPI(t *testing.T) {
	root := preparedCompilerProject(t)
	nodeModules := filepath.Join(root, "node_modules")
	if err := os.Rename(filepath.Join(nodeModules, "typescript"), filepath.Join(nodeModules, "typescript-api")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "node_modules/typescript/package.json", `{
  "name":"typescript",
  "version":"7.0.2",
  "type":"module",
  "exports":{
    "./package.json":"./package.json",
    "./unstable/sync":"./dist/api/sync.js",
    "./unstable/ast":"./dist/ast.js"
  }
}`)
	// These files establish a supported native package shape. They deliberately
	// cannot execute: success therefore proves the one legacy API candidate was
	// selected before native Compiler API startup.
	writeTestFile(t, root, "node_modules/typescript/dist/api/sync.js", `throw new Error("native compiler must not load")`)
	writeTestFile(t, root, "node_modules/typescript/dist/ast.js", `throw new Error("native compiler must not load")`)
	writeTestFile(t, root, "package.json", `{
  "name":"dual-compiler",
  "devDependencies":{
    "typescript":"7.0.2",
    "typescript-api":"npm:typescript@6.0.3"
  }
}`)
	writeTestFile(t, root, "tsconfig.json", `{"include":["src/**/*.ts"],"compilerOptions":{"strict":true}}`)
	writeTestFile(t, root, "src/main.ts", "export const ready: boolean = true\n")
	tracked := []string{"package.json", "src/main.ts", "tsconfig.json"}
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, _, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Path != "src/main.ts" {
		t.Fatalf("legacy alias compiler files = %#v", result.Files)
	}
}

func TestPreparedCompilerRejectsDistinctLegacyCandidateAmbiguity(t *testing.T) {
	root := preparedTempProject(t)
	writeTestFile(t, root, "node_modules/typescript-api/package.json", `{"name":"typescript","version":"6.0.3"}`)
	writeTestFile(t, root, "node_modules/typescript-api/lib/typescript.js", `module.exports = {}`)
	writeTestFile(t, root, "package.json", `{
  "name":"ambiguous-compiler",
  "devDependencies":{
    "typescript":"5.9.3",
    "typescript-api":"npm:typescript@6.0.3"
  }
}`)
	tracked := []string{"package.json", "src/main.ts", "tsconfig.json"}
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, err = Discover(context.Background(), repository, root)
	if !errors.Is(err, ErrTypeScriptCompilerUnavailable) ||
		!strings.Contains(err.Error(), "ambiguous supported TypeScript compiler packages: typescript, typescript-api") {
		t.Fatalf("ambiguous compiler error = %v", err)
	}
}

func TestPreparedCompilerLoadsThroughPackageExportsBoundary(t *testing.T) {
	root := preparedTempProject(t)
	manifestPath := filepath.Join(root, "node_modules", "typescript", "package.json")
	encoded, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		t.Fatal(err)
	}
	// Modern packages may explicitly export package.json while keeping their
	// compatible Compiler API implementation as a package-owned private file.
	// The helper must establish the package root without asking Node to resolve
	// that private subpath through exports.
	manifest["exports"] = map[string]any{
		".":              "./lib/typescript.js",
		"./package.json": "./package.json",
	}
	encoded, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	tracked := []string{"package.json", "src/main.ts", "tsconfig.json"}
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, _, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Path != "src/main.ts" {
		t.Fatalf("exports-bound compiler files = %#v", result.Files)
	}
}

func TestSolutionConfigKeepsReferencedProjectOptionsSeparate(t *testing.T) {
	root := preparedTempProject(t)
	writeTestFile(t, root, "tsconfig.json", `{
  "files": [],
  "references": [
    {"path": "./config/tsconfig.node.json"},
    {"path": "./config/tsconfig.web.json"}
  ]
}`)
	writeTestFile(t, root, "config/tsconfig.node.json", `{
  "include": ["../server/**/*.ts"],
  "compilerOptions": {"module": "Node16", "moduleResolution": "Node16", "strict": true}
}`)
	writeTestFile(t, root, "config/tsconfig.web.json", `{
  "include": ["../client/**/*.ts"],
  "compilerOptions": {
    "module": "ESNext",
    "moduleResolution": "bundler",
    "paths": {"@/*": ["../client/*"]},
    "strict": true
  }
}`)
	writeTestFile(t, root, "server/main.ts", `import { worker } from "./worker"
export function serve(): number { return worker() }
`)
	writeTestFile(t, root, "server/worker.ts", `export function worker(): number { return 1 }
`)
	writeTestFile(t, root, "client/main.ts", `import { view } from "@/view"
export function render(): string { return view() }
`)
	writeTestFile(t, root, "client/view.ts", `export function view(): string { return "ready" }
`)
	tracked := []string{
		"client/main.ts", "client/view.ts", "config/tsconfig.node.json", "config/tsconfig.web.json",
		"package.json", "server/main.ts", "server/worker.ts", "tsconfig.json",
	}
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, _, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	wantFiles := []string{"client/main.ts", "client/view.ts", "server/main.ts", "server/worker.ts"}
	gotFiles := make([]string, len(result.Files))
	for index, file := range result.Files {
		gotFiles[index] = file.Path
	}
	if strings.Join(gotFiles, "\n") != strings.Join(wantFiles, "\n") {
		t.Fatalf("solution files = %#v, want %#v", gotFiles, wantFiles)
	}
	if result.Project.ModuleResolution != "mixed" {
		t.Fatalf("solution module resolution = %q, want mixed", result.Project.ModuleResolution)
	}
	if !hasExactImport(result, "@/view", "client/view.ts") || !hasExactImport(result, "./worker", "server/worker.ts") {
		t.Fatalf("referenced project imports lost leaf options: %#v", result.Imports)
	}
	if len(result.Project.PathAliases) != 1 || result.Project.PathAliases[0].Pattern != "@/*" ||
		strings.Join(result.Project.PathAliases[0].Targets, ",") != "client/*" {
		t.Fatalf("solution path aliases = %#v", result.Project.PathAliases)
	}
}

func TestNestedPackageKeepsSiblingProjectReferenceAsExternalBoundary(t *testing.T) {
	root := preparedTempProject(t)
	var sharedSource strings.Builder
	sharedSource.WriteString("export class Shared { value = true }\nexport function makeShared() { return { shared: new Shared()")
	for index := 0; index < 400; index++ {
		_, _ = fmt.Fprintf(&sharedSource, ", field%d: %d", index, index)
	}
	sharedSource.WriteString(" } }\n")
	files := map[string]string{
		"packages/app/package.json": `{"name":"app"}`,
		"packages/app/src/main.ts":  "import { makeShared } from \"../../shared/src/index\"\nexport const app = makeShared()\n",
		"packages/app/tsconfig.json": `{
  "include": ["src/**/*.ts"],
  "references": [{"path": "../shared"}],
  "compilerOptions": {"module": "ESNext", "moduleResolution": "bundler", "strict": true}
}`,
		"packages/shared/package.json": `{"name":"shared"}`,
		"packages/shared/src/index.ts": sharedSource.String(),
		"packages/shared/tsconfig.json": `{
  "include": ["src/**/*.ts"],
  "references": [{"path": "./missing-child"}],
  "compilerOptions": {"composite": true, "module": "ESNext", "moduleResolution": "bundler", "strict": true}
}`,
	}
	tracked := []string{"package.json"}
	for filePath, content := range files {
		writeTestFile(t, root, filePath, content)
		tracked = append(tracked, filePath)
	}
	sort.Strings(tracked)
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, err := DiscoverSelected(
		context.Background(), repository, root, "jsts:packages/app/package.json",
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.Selector != "jsts:packages/app/package.json" ||
		result.Project.ConfigPath != "packages/app/tsconfig.json" || len(result.Files) != 1 ||
		result.Files[0].Path != "packages/app/src/main.ts" {
		t.Fatalf("package with sibling project reference = project %#v; files %#v", result.Project, result.Files)
	}
	for _, file := range result.Files {
		if strings.HasPrefix(file.Path, "packages/shared/") {
			t.Fatalf("sibling package source leaked into selected package page: %#v", file)
		}
	}
	foundApp := false
	for _, declaration := range result.Declarations {
		if declaration.Name != "app" {
			continue
		}
		foundApp = true
		if declaration.Signature != "" {
			t.Fatalf("unsafe inferred sibling signature was retained: %q", declaration.Signature)
		}
	}
	if !foundApp {
		t.Fatalf("app declaration was not retained: %#v", result.Declarations)
	}

	writeTestFile(t, filepath.Dir(root), "outside-project/tsconfig.json", `{"include":["src/**/*.ts"]}`)
	writeTestFile(t, root, "packages/app/tsconfig.json", `{
  "include": ["src/**/*.ts"],
  "references": [{"path": "../../../outside-project"}],
  "compilerOptions": {"module": "ESNext", "moduleResolution": "bundler", "strict": true}
}`)
	_, err = DiscoverSelected(
		context.Background(), repository, root, "jsts:packages/app/package.json",
	)
	if err == nil || !strings.Contains(err.Error(), "unresolved project reference ../../../outside-project") {
		t.Fatalf("outside-repository project reference error = %v", err)
	}
}

func TestRootPackageKeepsNestedProjectReferenceAsExternalBoundary(t *testing.T) {
	root := preparedTempProject(t)
	files := map[string]string{
		"package.json": `{"name":"root","devDependencies":{"typescript":"5.9.3"}}`,
		"src/root.ts":  "export const root = true\n",
		"tsconfig.json": `{
  "include": ["src/**/*.ts"],
  "references": [{"path": "./packages/shared"}],
  "compilerOptions": {"module": "ESNext", "moduleResolution": "bundler", "strict": true}
}`,
		"packages/shared/package.json": `{"name":"shared"}`,
		"packages/shared/src/index.ts": "export const shared = true\n",
		"packages/shared/tsconfig.json": `{
  "include": ["src/**/*.ts"],
  "references": [{"path": "./missing-child"}],
  "compilerOptions": {"composite": true, "module": "ESNext", "moduleResolution": "bundler", "strict": true}
}`,
	}
	tracked := make([]string, 0, len(files))
	for filePath, content := range files {
		writeTestFile(t, root, filePath, content)
		tracked = append(tracked, filePath)
	}
	sort.Strings(tracked)
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, err := DiscoverSelected(context.Background(), repository, root, "jsts:package.json")
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.Selector != "jsts:package.json" || result.Project.ConfigPath != "tsconfig.json" ||
		len(result.Files) != 1 || result.Files[0].Path != "src/root.ts" {
		t.Fatalf("root package with nested project reference = project %#v; files %#v", result.Project, result.Files)
	}
	for _, file := range result.Files {
		if strings.HasPrefix(file.Path, "packages/shared/") {
			t.Fatalf("nested package source leaked into root package page: %#v", file)
		}
	}
}

func TestCredentialBearingManifestValuesNeverEnterHelperOrArtifact(t *testing.T) {
	root := preparedTempProject(t)
	secret := "token-value-123456789"
	writeTestFile(t, root, "package.json", `{"name":"safe","scripts":{"build":"NPM_TOKEN=`+secret+` tsx src/main.ts"},"dependencies":{"safe-package":"git+https://user:`+secret+`@example.invalid/repo.git"},"devDependencies":{"typescript":"5.9.3"}}`)
	listing := gitfiles.Listing{Paths: []string{"package.json", "src/main.ts", "tsconfig.json"}, RegularPaths: []string{"package.json", "src/main.ts", "tsconfig.json"}}
	repository, err := corpus.New(context.Background(), root, listing)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, err := Discover(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	request := newHelperRequest(
		[]helperCompilerPackage{{Name: "typescript", ResolutionBase: helperCompilerResolutionProject}},
		nil,
	)
	request.Files = append(request.Files, helperFile{Path: "src/main.ts", FileRef: "f2"})
	requestBytes, err := jsonMarshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(secret)) || bytes.Contains(requestBytes, []byte(secret)) {
		t.Fatal("credential-bearing manifest value entered helper input or artifact")
	}
}

func TestHelperRequestEncodesEmptyPackageBoundariesAsArray(t *testing.T) {
	request := newHelperRequest(
		[]helperCompilerPackage{{Name: "typescript", ResolutionBase: helperCompilerResolutionProject}},
		nil,
	)
	encoded, err := jsonMarshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{
		[]byte(`"package_boundary_dirs":[]`),
		[]byte(`"files":[]`),
		[]byte(`"additional_files":[]`),
	} {
		if !bytes.Contains(encoded, want) {
			t.Fatalf("helper request omitted canonical empty array %s: %s", want, encoded)
		}
	}
}

func TestDiscoverSelectsOneNestedPreparedPackageAndKeepsRepositoryPaths(t *testing.T) {
	preparedRoot := preparedTempProject(t)
	repositoryRoot := t.TempDir()
	projectRoot := filepath.Join(repositoryRoot, "front")
	if err := os.Rename(preparedRoot, projectRoot); err != nil {
		t.Fatal(err)
	}
	projectFiles := []string{
		"package.json", "postcss.config.js", "src/excluded.ts", "src/main.ts",
		"src/lookalike.js", "src/one.ts", "src/two.ts", "src/view.tsx", "src/widget.jsx", "tsconfig.json",
	}
	tracked := make([]string, len(projectFiles))
	for index, filePath := range projectFiles {
		tracked[index] = path.Join("front", filePath)
	}
	repository, err := corpus.New(context.Background(), repositoryRoot, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, index, catalog, err := Build(context.Background(), repository, repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.ManifestPath != "front/package.json" || result.Project.Selector != "jsts:front/package.json" ||
		result.Project.ConfigPath != "front/tsconfig.json" || result.Project.BaseURL != "front" {
		t.Fatalf("nested project identity = %#v", result.Project)
	}
	if len(result.Project.PathAliases) != 1 || strings.Join(result.Project.PathAliases[0].Targets, ",") != "front/src/*" {
		t.Fatalf("nested path aliases = %#v", result.Project.PathAliases)
	}
	for _, file := range result.Files {
		if !strings.HasPrefix(file.Path, "front/") {
			t.Fatalf("nested source escaped repository-relative package path: %#v", file)
		}
	}
	if !hasExactImport(result, "@/one", "front/src/one.ts") || !hasExactImport(result, "@/two", "front/src/two.ts") {
		t.Fatalf("nested project imports lost exact authority: %#v", result.Imports)
	}
	if index.Target.Selector != result.Project.Selector || index.Target.AnchorFileRef != result.Project.ManifestFileRef {
		t.Fatalf("nested ProgramTarget binding = %#v", index.Target)
	}
	if len(catalog.Importers) != 1 || catalog.Importers[0].RepositoryPath != "front" {
		t.Fatalf("nested dependency importer = %#v", catalog.Importers)
	}
}

func TestNestedProjectUsesPreparedCompilerOnlyFromAnalyzedRepository(t *testing.T) {
	repositoryRoot := preparedTempProject(t)
	projectRoot := filepath.Join(repositoryRoot, "front")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, filePath := range []string{"package.json", "postcss.config.js", "src", "tsconfig.json"} {
		if err := os.Rename(filepath.Join(repositoryRoot, filePath), filepath.Join(projectRoot, filePath)); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, repositoryRoot, "package.json", `{
  "name":"workspace",
  "workspaces":["front"],
  "scripts":{"dev":"bun run --cwd front src/main.ts"}
}`)
	projectFiles := []string{
		"package.json", "postcss.config.js", "src/excluded.ts", "src/main.ts",
		"src/lookalike.js", "src/one.ts", "src/two.ts", "src/view.tsx", "src/widget.jsx", "tsconfig.json",
	}
	tracked := []string{"package.json"}
	for _, filePath := range projectFiles {
		tracked = append(tracked, path.Join("front", filePath))
	}
	repository, err := corpus.New(context.Background(), repositoryRoot, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	result, _, _, err := Build(context.Background(), repository, repositoryRoot)
	if closeErr := repository.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.ManifestPath != "front/package.json" || len(result.Files) == 0 {
		t.Fatalf("workspace-root compiler project = %#v; files = %d", result.Project, len(result.Files))
	}

	// The same prepared compiler is deliberately outside this second analyzed
	// repository. Node package lookup can see it through the parent directory,
	// but the helper must reject that authority.
	nestedRepositoryRoot := filepath.Join(repositoryRoot, "outside-check")
	files := map[string]string{
		"package.json":        `{"name":"outside-check","workspaces":["front"],"scripts":{"dev":"bun run --cwd front src/main.ts"}}`,
		"front/package.json":  `{"name":"front","devDependencies":{"typescript":"5.9.3"}}`,
		"front/src/main.ts":   "export const ready = true\n",
		"front/tsconfig.json": `{"include":["src/**/*.ts"],"compilerOptions":{"module":"ESNext","moduleResolution":"bundler"}}`,
	}
	nestedTracked := make([]string, 0, len(files))
	for filePath, content := range files {
		writeTestFile(t, nestedRepositoryRoot, filePath, content)
		nestedTracked = append(nestedTracked, filePath)
	}
	sort.Strings(nestedTracked)
	nestedRepository, err := corpus.New(
		context.Background(),
		nestedRepositoryRoot,
		gitfiles.Listing{Paths: nestedTracked, RegularPaths: nestedTracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer nestedRepository.Close()
	_, err = Discover(context.Background(), nestedRepository, nestedRepositoryRoot)
	if !errors.Is(err, ErrTypeScriptCompilerUnavailable) {
		t.Fatalf("outside-repository compiler authority error = %v", err)
	}
}

func TestDiscoverRejectsAmbiguousNestedPackagesBeforeCompilerExecution(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"admin/package.json": `{"name":"admin"}`,
		"admin/main.ts":      "export {}\n",
		"web/package.json":   `{"name":"web"}`,
		"web/main.ts":        "export {}\n",
	}
	tracked := make([]string, 0, len(files))
	for filePath, content := range files {
		writeTestFile(t, root, filePath, content)
		tracked = append(tracked, filePath)
	}
	sort.Strings(tracked)
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, err = Discover(context.Background(), repository, root)
	if err == nil || !strings.Contains(err.Error(), "multiple nested package projects are ambiguous") {
		t.Fatalf("ambiguous nested discovery error = %v", err)
	}
}

func TestProjectManifestSelectionKeepsWorkspaceRootWithExactOwnedEntry(t *testing.T) {
	manifestPath, projectDir, err := selectProjectManifest([]corpus.Entry{
		{Path: "package.json"},
		{Path: "src/main.ts"},
		{Path: "front/package.json"},
		{Path: "front/src/main.ts"},
	}, "", &packageManifest{
		Workspaces: json.RawMessage(`["front"]`),
		Exports:    json.RawMessage(`"./src/main.ts"`),
		Scripts:    map[string]string{"dev": "bun run --cwd front src/main.ts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifestPath != "package.json" || projectDir != "." {
		t.Fatalf("selected project = %q at %q, want repository root", manifestPath, projectDir)
	}
}

func TestPackageBinaryFactsKeepOnlyCanonicalTrackedOwnedPairs(t *testing.T) {
	owned := map[string]string{
		"bin/opencode": "f2",
		"bin/other":    "f3",
	}
	binaries := packageBinaryFacts(
		"@scope/opencode",
		json.RawMessage(`{
  "opencode": "./bin/opencode",
  "alias": "bin/opencode",
  "escape": "../outside",
  "missing": "bin/missing",
  "generated": "dist/opencode",
  "invalid": {"path":"bin/other"}
}`),
		"packages/opencode",
		owned,
	)
	if len(binaries) != 2 ||
		binaries[0] != (PackageBinary{Command: "alias", Path: "packages/opencode/bin/opencode", FileRef: "f2"}) ||
		binaries[1] != (PackageBinary{Command: "opencode", Path: "packages/opencode/bin/opencode", FileRef: "f2"}) {
		t.Fatalf("package binary facts = %#v", binaries)
	}

	stringForm := packageBinaryFacts(
		"@scope/opencode", json.RawMessage(`"./bin/opencode"`), ".", owned,
	)
	if len(stringForm) != 1 || stringForm[0] != (PackageBinary{Command: "opencode", Path: "bin/opencode", FileRef: "f2"}) {
		t.Fatalf("string package binary facts = %#v", stringForm)
	}
}

func TestPackageBinaryCandidatesDiscardConflictingCommandWithoutDiscardingValidSibling(t *testing.T) {
	candidates := packageBinaryCandidates(
		"sample",
		json.RawMessage(`{"tool":"bin/one","stable":"bin/other","tool":"bin/two"}`),
	)
	if len(candidates) != 1 || candidates[0] != (packageBinaryCandidate{Command: "stable", Path: "bin/other"}) {
		t.Fatalf("conflicting package binary candidates = %#v", candidates)
	}
}

func TestProjectManifestSelectionKeepsWorkspaceRootWithTrackedExtensionlessBinary(t *testing.T) {
	manifestPath, projectDir, err := selectProjectManifest([]corpus.Entry{
		{Path: "package.json"},
		{Path: "bin/tool"},
		{Path: "src/main.ts"},
		{Path: "front/package.json"},
		{Path: "front/src/main.ts"},
	}, "", &packageManifest{
		Name:       "tool",
		Workspaces: json.RawMessage(`["front"]`),
		Bin:        json.RawMessage(`"./bin/tool"`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifestPath != "package.json" || projectDir != "." {
		t.Fatalf("selected extensionless-bin project = %q at %q, want repository root", manifestPath, projectDir)
	}
}

func TestProjectManifestSelectionUsesOneExactWorkspaceDelegate(t *testing.T) {
	entries := []corpus.Entry{
		{Path: "package.json"},
		{Path: "src/index.ts"},
		{Path: "front/package.json"},
		{Path: "front/src/main.ts"},
	}
	manifestPath, projectDir, err := selectProjectManifest(entries, "", &packageManifest{
		Workspaces: json.RawMessage(`["front"]`),
		Scripts: map[string]string{
			"dev": "bun run --cwd front --conditions=browser src/index.ts",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifestPath != "front/package.json" || projectDir != "front" {
		t.Fatalf("selected delegated project = %q at %q, want front", manifestPath, projectDir)
	}
}

func TestProjectManifestSelectionFailsClosedWithoutOneWorkspaceDelegate(t *testing.T) {
	entries := []corpus.Entry{
		{Path: "package.json"},
		{Path: "admin/package.json"},
		{Path: "admin/src/main.ts"},
		{Path: "front/package.json"},
		{Path: "front/src/main.ts"},
	}
	for _, test := range []struct {
		name    string
		scripts map[string]string
		count   string
	}{
		{name: "zero", scripts: map[string]string{"dev": "bun run build"}, count: "has 0 exact"},
		{name: "multiple", scripts: map[string]string{
			"dev":   "bun run --cwd front dev",
			"start": "bun run --cwd=admin start",
		}, count: "has 2 exact"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := selectProjectManifest(entries, "", &packageManifest{
				Workspaces: json.RawMessage(`["admin","front"]`),
				Scripts:    test.scripts,
			})
			if err == nil || !strings.Contains(err.Error(), test.count) ||
				!strings.Contains(err.Error(), "jsts:admin/package.json") ||
				!strings.Contains(err.Error(), "jsts:front/package.json") {
				t.Fatalf("workspace delegation error = %v", err)
			}
		})
	}
}

func TestProjectManifestExactSelectorBypassesWorkspaceRootPriority(t *testing.T) {
	manifestPath, projectDir, err := selectProjectManifest([]corpus.Entry{
		{Path: "package.json"},
		{Path: "src/main.ts"},
		{Path: "front/package.json"},
		{Path: "front/src/main.ts"},
	}, "jsts:front/package.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if manifestPath != "front/package.json" || projectDir != "front" {
		t.Fatalf("exact selected project = %q at %q, want front", manifestPath, projectDir)
	}
}

func TestPackageProjectOwnershipUsesPathDepthNotDirectoryNameLength(t *testing.T) {
	candidates := packageProjectCandidates([]corpus.Entry{
		{Path: "package.json"},
		{Path: "z/package.json"},
		{Path: "z/main.ts"},
	})
	if len(candidates) != 2 {
		t.Fatalf("package candidates = %#v", candidates)
	}
	for _, candidate := range candidates {
		switch candidate.manifestPath {
		case "package.json":
			if len(candidate.ownSources) != 0 {
				t.Fatalf("root stole nested source ownership: %#v", candidate.ownSources)
			}
		case "z/package.json":
			if _, ok := candidate.ownSources["z/main.ts"]; !ok {
				t.Fatalf("nested package lost source ownership: %#v", candidate.ownSources)
			}
		}
	}
}

var (
	preparedCompilerOnce   sync.Once
	preparedCompilerSource string
	preparedCompilerErr    error
)

func preparedCompilerPackage() (string, error) {
	preparedCompilerOnce.Do(func() {
		npm, err := exec.LookPath("npm")
		if err != nil {
			preparedCompilerErr = fmt.Errorf("npm is unavailable: %w", err)
			return
		}
		output, err := exec.Command(npm, "root", "-g").Output()
		if err != nil {
			preparedCompilerErr = fmt.Errorf("global npm root is unavailable: %w", err)
			return
		}
		preparedCompilerSource = filepath.Join(strings.TrimSpace(string(output)), "typescript")
		if _, err := os.Stat(filepath.Join(preparedCompilerSource, "lib", "typescript.js")); err != nil {
			preparedCompilerErr = fmt.Errorf("global TypeScript compiler is unavailable: %w", err)
		}
	})
	return preparedCompilerSource, preparedCompilerErr
}

func preparedCompilerProject(t *testing.T) string {
	t.Helper()
	compilerSource, err := preparedCompilerPackage()
	if err != nil {
		t.Skip(err)
	}
	root := t.TempDir()
	compilerTarget := filepath.Join(root, "node_modules", "typescript")
	if err := materializeCompilerTree(compilerSource, compilerTarget); err != nil {
		t.Fatal(err)
	}
	return root
}

func materializeCompilerTree(source, target string) error {
	return filepath.WalkDir(source, func(sourcePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, sourcePath)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("prepared TypeScript compiler contains unsupported file %q", relative)
		}
		if err := os.Link(sourcePath, targetPath); err == nil {
			return nil
		}
		contents, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, contents, info.Mode().Perm())
	})
}

func preparedTempProject(t *testing.T) string {
	t.Helper()
	root := preparedCompilerProject(t)
	writeTestFile(t, root, "package.json", `{"name":"sample","devDependencies":{"typescript":"5.9.3"}}`)
	writeTestFile(t, root, "tsconfig.json", `{"include":["src/**/*"],"exclude":["src/excluded.ts"],"compilerOptions":{"allowJs":true,"module":"ESNext","moduleResolution":"bundler","jsx":"preserve","baseUrl":".","paths":{"@/*":["./src/*"]},"strict":true}}`)
	var collisions strings.Builder
	for index := 0; index <= programindex.MaxTargetsPerRelation; index++ {
		_, _ = fmt.Fprintf(&collisions, "export namespace Candidate%d { export function test() {} }\n", index)
	}
	writeTestFile(t, root, "src/collisions.ts", collisions.String())
	writeTestFile(t, root, "src/main.ts", "import { same as one } from \"@/one\"\nimport { same as two } from \"@/two\"\nexport class ExactReceiver { statusCode(): number { return 201 } }\nexport class Constructed { constructor() {} }\nexport function createResponse() { return { statusCode(): number { return 200 } }\nexport const response = { get statusCode(): number { return 202 } }\nexport function hello(): string { return \"hello\" }\nexport function platformCalls(canvas: HTMLCanvasElement) {\n  new Constructed()\n  new Date()\n  new Promise<void>((resolve) => resolve())\n  const localDateConstructor: DateConstructor = Date\n  new localDateConstructor()\n  canvas.getContext(\"2d\")\n  Math.min(1, 2)\n  console.log(\"ready\")\n  return new Image()\n}\nhello()\none()\ntwo()\nexport const prompt = ["+strings.Repeat("\"abc\", ", 100)+"\"end\"].join(\"\\n\")\n")
	writeTestFile(t, root, "node_modules/@types/node/index.d.ts", "interface Console { log(message?: any, ...optionalParams: any[]): void }\n")
	writeTestFile(t, root, "src/one.ts", "export function same(): number { return 1 }\n")
	writeTestFile(t, root, "src/two.ts", "export function same(): number { return 2 }\n")
	writeTestFile(t, root, "src/view.tsx", "export function View() { return <main /> }\n")
	writeTestFile(t, root, "src/widget.jsx", "export function Widget() { return <aside /> }\nWidget()\n")
	writeTestFile(t, root, "src/lookalike.js", "const app = { get() {}, listen() {} }\nfunction createRoot() {}\nfunction test() {}\napp.get(\"/not-express\", () => {})\napp.listen()\ncreateRoot()\n/robot/.test(\"robot\")\nconst dynamicName = \"./missing.js\"\nimport(dynamicName)\n")
	writeTestFile(t, root, "src/excluded.ts", "export function excluded(): never { throw new Error(\"excluded\") }\n")
	writeTestFile(t, root, "postcss.config.js", "export function plugin() {}\nplugin()\n")
	return root
}

func hasExactImport(result Result, specifier, resolvedPath string) bool {
	fileRef := ""
	for _, file := range result.Files {
		if file.Path == resolvedPath {
			fileRef = file.FileRef
			break
		}
	}
	for _, value := range result.Imports {
		if value.Specifier == specifier && value.Resolution == "exact" && value.ResolvedFileRef == fileRef && fileRef != "" {
			return true
		}
	}
	return false
}

func assertCompilerResolvedInvocationProjection(t *testing.T, result Result, index programindex.Index) {
	t.Helper()
	type expectedPlatformTarget struct {
		invocation string
		receiver   string
		name       string
	}
	wantPlatform := map[string]expectedPlatformTarget{
		"canvas.getContext": {invocation: "call", receiver: "HTMLCanvasElement", name: "getContext"},
		"Math.min":          {invocation: "call", receiver: "Math", name: "min"},
		"console.log":       {invocation: "call", receiver: "Console", name: "log"},
		"new Date":          {invocation: "construct", name: "Date"},
		"new Image":         {invocation: "construct", name: "Image"},
		"new Promise":       {invocation: "construct", name: "Promise"},
	}
	callsByExpression := make(map[string]Call, len(result.Calls))
	for _, call := range result.Calls {
		if !validInvocation(call.Invocation) {
			t.Fatalf("call has no closed invocation kind: %#v", call)
		}
		callsByExpression[call.Expression] = call
	}
	objectsByID := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objectsByID[object.ID] = object
	}
	relationsBySourceRef := make(map[string]programindex.Relation, len(index.Relations))
	for _, relation := range index.Relations {
		relationsBySourceRef[relation.SourceRef] = relation
	}
	for expression, want := range wantPlatform {
		call, ok := callsByExpression[expression]
		if !ok || call.Resolution != "exact" || call.Invocation != want.invocation || call.ExternalPackage != javascriptPlatform ||
			call.ExternalReceiver != want.receiver || call.ExternalName != want.name || len(call.CalleeRefs) != 0 {
			t.Fatalf("compiler-resolved platform invocation %q = %#v, want %#v", expression, call, want)
		}
		relation, ok := relationsBySourceRef["program:"+call.Ref]
		if !ok || relation.Kind != programindex.RelationInvokesExternal || relation.Resolution != programindex.ResolutionExact ||
			relation.Invocation != want.invocation || len(relation.ToIDs) != 1 {
			t.Fatalf("platform invocation relation %q = %#v", expression, relation)
		}
		target := objectsByID[relation.ToIDs[0]]
		if target.External == nil || target.External.PackagePath != javascriptPlatform || target.External.Receiver != want.receiver || target.External.Name != want.name {
			t.Fatalf("platform invocation target %q = %#v", expression, target)
		}
	}
	if call, ok := callsByExpression["new localDateConstructor"]; !ok || call.Resolution != "unresolved" ||
		call.ExternalPackage != "" || len(call.CalleeRefs) != 0 {
		t.Fatalf("repository-local value typed as a platform constructor gained platform authority: %#v", call)
	}

	constructorCall, ok := callsByExpression["new Constructed"]
	if !ok || constructorCall.Invocation != "construct" || constructorCall.Resolution != "exact" || constructorCall.ExternalPackage != "" || len(constructorCall.CalleeRefs) != 1 {
		t.Fatalf("local constructor invocation = %#v", constructorCall)
	}
	declarationsByRef := make(map[string]Declaration, len(result.Declarations))
	for _, declaration := range result.Declarations {
		declarationsByRef[declaration.Ref] = declaration
	}
	constructor := declarationsByRef[constructorCall.CalleeRefs[0]]
	owner := declarationsByRef[constructor.OwnerRef]
	if constructor.Name != "constructor" || owner.Name != "Constructed" {
		t.Fatalf("local constructor target = %#v, owner %#v", constructor, owner)
	}
	relation := relationsBySourceRef["program:"+constructorCall.Ref]
	if relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionExact || relation.Invocation != "construct" || len(relation.ToIDs) != 1 {
		t.Fatalf("local constructor relation = %#v", relation)
	}
}

func assertReceiverlessObjectMethodProjection(t *testing.T, result Result, index programindex.Index) {
	t.Helper()
	objectsBySourceRef := make(map[string]programindex.Object, len(index.Objects))
	objectsByID := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objectsBySourceRef[object.SourceRef] = object
		objectsByID[object.ID] = object
	}
	classMethodFound := false
	receiverlessOwners := map[programindex.ObjectKind]bool{}
	for _, declaration := range result.Declarations {
		if declaration.Name != "statusCode" {
			continue
		}
		object, ok := objectsBySourceRef[declaration.Ref]
		if !ok {
			t.Fatalf("statusCode declaration %q is absent from ProgramIndex", declaration.Ref)
		}
		owner := objectsByID[object.OwnerID]
		switch owner.Kind {
		case programindex.ObjectType:
			classMethodFound = true
			if object.Kind != programindex.ObjectMethod {
				t.Fatalf("class method lost exact receiver authority: %#v / owner=%#v", object, owner)
			}
		case programindex.ObjectFunction:
			receiverlessOwners[owner.Kind] = true
			if object.Kind != programindex.ObjectFunction {
				t.Fatalf("object-literal callable invented method receiver authority: %#v / owner=%#v", object, owner)
			}
		case programindex.ObjectVariable:
			receiverlessOwners[owner.Kind] = true
			if object.Kind != programindex.ObjectFunction {
				t.Fatalf("object-literal callable invented method receiver authority: %#v / owner=%#v", object, owner)
			}
		}
	}
	if !classMethodFound || !receiverlessOwners[programindex.ObjectFunction] || !receiverlessOwners[programindex.ObjectVariable] {
		t.Fatalf("receiver projection coverage: class=%t receiverless owners=%#v", classMethodFound, receiverlessOwners)
	}
}

func writeTestFile(t *testing.T, root, filePath, content string) {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(filePath))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func jsonMarshal(value any) ([]byte, error) { return json.Marshal(value) }
