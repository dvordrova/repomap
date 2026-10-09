package jstsproject

import (
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
)

func TestEmptyConfigFallbackKeepsOnlyExplicitNonOutputRoots(t *testing.T) {
	root := preparedCompilerProject(t)
	files := map[string]string{
		"package.json":                  `{"name":"template","scripts":{"build":"vite build","generated":"tsx dist/generated.ts"}}`,
		"tsconfig.json":                 `{"include":["src"],"compilerOptions":{"outDir":"dist","module":"ESNext","moduleResolution":"bundler"}}`,
		"vite.config.ts":                "export function configure() { return true }\nconfigure()\n",
		"index.tsx":                     "export function excludedTemplate() { return true }\n",
		"dist/generated.ts":             "export function generatedOutput() { return true }\n",
		"packages/sibling/package.json": `{"name":"sibling"}`,
		"packages/sibling/index.ts":     "export function sibling() { return true }\n",
	}
	var paths []string
	for name, source := range files {
		writeTestFile(t, root, name, source)
		paths = append(paths, name)
	}
	slices.Sort(paths)
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: paths, RegularPaths: paths})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, err := DiscoverSelected(t.Context(), repository, root, "jsts:package.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Path != "vite.config.ts" || result.Project.ModuleResolution != "nodenext" {
		t.Fatalf("empty config admitted excluded/output/sibling sources: %#v, %#v", result.Project, result.Files)
	}
}

func TestEmptyConfigDoesNotMaskOtherErrorsOrInventSources(t *testing.T) {
	for _, test := range []struct{ name, config, tool, want string }{
		{"unknown-option", `{"include":["src"],"compilerOptions":{"unknownCompilerOption":true}}`, "vite.config.ts", "Unknown compiler option"},
		{"missing-reference", `{"include":["src"],"references":[{"path":"missing"}]}`, "vite.config.ts", "unresolved project reference"},
		{"no-explicit-source", `{"include":["src"]}`, "index.ts", "configuration selects no tracked"},
		{"all-owned-output", `{"include":["src"],"compilerOptions":{"outDir":"."}}`, "vite.config.ts", "configuration selects no tracked"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := preparedCompilerProject(t)
			writeTestFile(t, root, "package.json", `{"name":"template"}`)
			writeTestFile(t, root, "tsconfig.json", test.config)
			writeTestFile(t, root, test.tool, "export const present = true\n")
			paths := []string{"package.json", "tsconfig.json", test.tool}
			repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: paths, RegularPaths: paths})
			if err != nil {
				t.Fatal(err)
			}
			defer repository.Close()
			_, err = DiscoverSelected(t.Context(), repository, root, "jsts:package.json")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestConfiguredAliasTargetsAreCanonicalMetadataNotCorpusFiles(t *testing.T) {
	for _, test := range []struct {
		target   string
		accepted bool
	}{
		{"node_modules/@fixture/sdk", true},
		{"node_modules/@fixture/sdk/*", true},
		{"/Users/example/sdk", false},
		{"../outside/sdk", false},
		{"node_modules/../sdk", false},
		{`node_modules\sdk`, false},
	} {
		t.Run(test.target, func(t *testing.T) {
			result := minimalResult(t, "typescript")
			result.Project.PathAliases = []PathAlias{{Pattern: "sdk", Targets: []string{test.target}}}
			_, err := Seal(result)
			if (err == nil) != test.accepted {
				t.Fatalf("target %q seal error %v, accepted %v", test.target, err, test.accepted)
			}
		})
	}
}
