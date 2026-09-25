package jstsproject

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
)

func TestCumulativeVitestFilesRequireAuthoredLiteralConfig(t *testing.T) {
	root := preparedCompilerProject(t)
	names := []string{"package.json", "tsconfig.json", "vitest.config.ts", "src/market.test.ts", "src/test-setup.ts", "src/testing/real-service.ts", "src/excluded/retained.test.ts"}
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, name, string(raw))
	}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: names, RegularPaths: names})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	read := func() map[string]bool {
		t.Helper()
		_, index, _, err := Build(t.Context(), repository, root)
		if err != nil {
			t.Fatal(err)
		}
		files := make(map[string]bool)
		for _, source := range index.Target.Sources {
			if filepath.Ext(source.Path) == ".ts" {
				files[source.Path] = slices.Contains(index.Target.TestSources, source.Path)
			}
		}
		return files
	}
	want := map[string]bool{"vitest.config.ts": true, "src/market.test.ts": true, "src/test-setup.ts": true, "src/testing/real-service.ts": false, "src/excluded/retained.test.ts": false}
	if got := read(); !reflect.DeepEqual(got, want) {
		t.Fatalf("configured test discovery lost source or invented a filename role: %+v", got)
	}
	// An arbitrary include variable is not an observed literal list. Keep all
	// declarations, and do not guess the previously configured test partition.
	// The declared runner's config file remains test code.
	writeTestFile(t, root, "vitest.config.ts", `import {defineConfig} from "vitest/config"; const include = ["src/**/*.test.ts"]; export default defineConfig({test:{include}});`)
	for file := range want {
		want[file] = file == "vitest.config.ts"
	}
	if got := read(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unsupported dynamic config hid source: %+v", got)
	}
}

// The canvas-ui package mirrors the report UI: Node runner unit tests beside
// production modules, and Playwright checks in their own test directory.
func TestCumulativeNodeRunnerAndPlaywrightDeclareTestSources(t *testing.T) {
	root := preparedCompilerProject(t)
	const project = "packages/canvas-ui/"
	names := []string{"build.mjs", "canvas.mjs", "drafts/wide-layout.test.mjs", "harness/api-stub.mjs", "layout.mjs", "layout.test.mjs", "package.json", "playwright.config.mjs", "reporters/journey.mjs", "serve.mjs", "visual/canvas.spec.mjs", "visual/fixture.mjs"}
	tracked := make([]string, 0, len(names))
	fixture := make(map[string]string)
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(project+name)))
		if err != nil {
			t.Fatal(err)
		}
		fixture[name] = string(raw)
		tracked = append(tracked, project+name)
	}
	want := map[string]bool{
		"build.mjs": false, "canvas.mjs": false, "layout.mjs": false,
		"serve.mjs":                   false, // webServer starts it, but so does the start script
		"drafts/wide-layout.test.mjs": false, // `*.test.mjs` does not cross directories
		"harness/api-stub.mjs":        true,  // only webServer starts it
		"layout.test.mjs":             true,  // `node --test *.test.mjs`
		"playwright.config.mjs":       true,
		"reporters/journey.mjs":       true, // named reporter module
		"visual/canvas.spec.mjs":      true, // testMatch inside testDir
		"visual/fixture.mjs":          true, // helper inside testDir
	}
	withoutRunner := strings.Replace(fixture["package.json"], `"@playwright/test": "1.63.0"`, `"playwright-core": "1.63.0"`, 1)
	withoutStart := strings.Replace(fixture["package.json"], `"start": "node serve.mjs",`, ``, 1)
	dynamicDirectory := `import { defineConfig } from "@playwright/test"
const directory = "./visual"
export default defineConfig({ testDir: directory, reporter: [["./reporters/journey.mjs"]] })
`
	packageDefaults := `import { defineConfig } from "@playwright/test"
export default defineConfig({ testIgnore: "drafts/**" })
`
	for _, test := range []struct {
		name, manifest, config string
		changed                map[string]bool
	}{
		{name: "fixture"},
		{name: "runner not declared", manifest: withoutRunner, changed: map[string]bool{
			"harness/api-stub.mjs": false, "playwright.config.mjs": false, "reporters/journey.mjs": false, "visual/canvas.spec.mjs": false, "visual/fixture.mjs": false,
		}},
		{name: "server not a package entry", manifest: withoutStart, changed: map[string]bool{
			"serve.mjs": true,
		}},
		{name: "non-literal test directory", config: dynamicDirectory, changed: map[string]bool{
			"harness/api-stub.mjs": false, "visual/canvas.spec.mjs": false, "visual/fixture.mjs": false,
		}},
		{name: "default directory and pattern", config: packageDefaults, changed: map[string]bool{
			"harness/api-stub.mjs": false, "reporters/journey.mjs": false, "visual/fixture.mjs": false,
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, name := range names {
				contents := fixture[name]
				if name == "package.json" && test.manifest != "" {
					contents = test.manifest
				}
				if name == "playwright.config.mjs" && test.config != "" {
					contents = test.config
				}
				writeTestFile(t, root, project+name, contents)
			}
			repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
			if err != nil {
				t.Fatal(err)
			}
			defer repository.Close()
			result, err := DiscoverSelected(t.Context(), repository, root, "jsts:"+project+"package.json")
			if err != nil {
				t.Fatal(err)
			}
			index, _, err := BuildFromResult(result)
			if err != nil {
				t.Fatal(err)
			}
			got := make(map[string]bool)
			for _, file := range result.Files {
				got[strings.TrimPrefix(file.Path, project)] = slices.Contains(index.Target.TestSources, file.Path)
			}
			expected := make(map[string]bool)
			for name, value := range want {
				expected[name] = value
			}
			for name, value := range test.changed {
				expected[name] = value
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("test sources = %v, want %v", got, expected)
			}
		})
	}
}

func TestNodeTestGlobsReadOnlyTheRunnerArguments(t *testing.T) {
	for _, test := range []struct {
		command string
		want    []string
	}{
		{`node --test *.test.mjs`, []string{"*.test.mjs"}},
		{`node --import tsx --test 'src/**/*.test.ts' test/*.spec.js`, []string{"src/**/*.test.ts", "test/*.spec.js"}},
		{`node --test --test-coverage-include 'src/**' --test-reporter spec test/*.mjs && node build.mjs`, []string{"test/*.mjs"}},
		{`cross-env NODE_ENV=test node --test-reporter=spec --test "test/**/*.js"`, []string{"test/**/*.js"}},
		{`node --test test/*.test.mjs; node build.mjs`, []string{"test/*.test.mjs"}},
		{`node --test *.test.mjs&& node build.mjs`, []string{"*.test.mjs"}},
		{`node build.mjs&&node --test *.test.mjs`, []string{"*.test.mjs"}},
		{`node --test "test/**/*.js" 2>&1 | tee test.log`, []string{"test/**/*.js"}},
		{`node --test *.test.mjs>test.log`, []string{"*.test.mjs"}},
		{`node --test`, []string{}},
		{`node scripts/run.mjs --test unit.mjs`, []string{}},
		{`cd e2e && node --test *.test.mjs`, []string{}},
		{`vitest run src/*.test.ts`, []string{}},
	} {
		if got := nodeTestGlobs(map[string]string{"test": test.command}); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s: globs = %q, want %q", test.command, got, test.want)
		}
	}
}
