package jstsproject

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestCumulativeJSTSNativeCompilerTypeMembers(t *testing.T) {
	tsc, err := exec.LookPath("tsc")
	if err != nil {
		t.Skip("a prepared native TypeScript tsc is required on PATH")
	}
	tsc, err = filepath.EvalSymlinks(tsc)
	if err != nil {
		t.Fatal(err)
	}
	compiler := filepath.Dir(filepath.Dir(tsc))
	if _, err := os.Stat(filepath.Join(compiler, "dist", "api", "sync", "api.js")); err != nil {
		t.Skip("active tsc does not belong to a prepared native TypeScript compiler")
	}
	prefix := isolatedNodeEnvironment(t)
	if err := os.Symlink(tsc, filepath.Join(prefix, "bin", "tsc")); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	_, file, _, _ := runtime.Caller(0)
	fixture := filepath.Join(filepath.Dir(file), "../../testdata/repositories/jsts")
	tracked := []string{"package.json", "shared/contracts.ts", "src/ambiguity.tsx", "src/platform.ts", "src/server.ts", "src/type-members.ts", "tsconfig.json"}
	for _, name := range tracked {
		data, err := os.ReadFile(filepath.Join(fixture, name))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, name, string(data))
	}
	materializeCumulativeJSTSDependencyTypes(t, root)
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, index, catalog, err := Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository,
		Targets: []places.TargetInput{{Index: index, Dependencies: &catalog}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCumulativeJSTSTypeMembers(t, result, index, lines.QuestionRows(graph))
}

// A real Node executable in an empty prefix makes missing-compiler checks
// independent of the machine's global installation. The hard link avoids a
// large copied runtime and, unlike a symlink, keeps process.execPath local.
func isolatedNodeEnvironment(t *testing.T) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required to execute the embedded helper")
	}
	node, err = filepath.EvalSymlinks(node)
	if err != nil {
		t.Fatal(err)
	}
	prefix := t.TempDir()
	bin := filepath.Join(prefix, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(node, filepath.Join(bin, filepath.Base(node))); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return prefix
}

func TestCumulativeJSTSExplicitScriptsSurviveSiblingOnlyConfig(t *testing.T) {
	compiler, err := preparedCompilerPackage()
	if err != nil {
		t.Skip(err)
	}
	prefix := isolatedNodeEnvironment(t)
	if err := materializeCompilerTree(compiler, filepath.Join(prefix, "lib", "node_modules", "typescript")); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	_, filename, _, _ := runtime.Caller(0)
	fixture := filepath.Join(filepath.Dir(filename), "../../testdata/repositories/jsts")
	const project = "packages/documentation-tools/"
	tracked := []string{
		"package.json", "tsconfig.json", "packages/local-store/package.json", "packages/local-store/src/index.ts",
		project + "package.json", project + "tsconfig.json", project + "scripts/check-links.ts",
		project + "scripts/prepare-readme.mjs", project + "scripts/unused.mjs",
	}
	for _, name := range tracked {
		contents, err := os.ReadFile(filepath.Join(fixture, name))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, name, string(contents))
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
	index, catalog, err := BuildFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, file := range result.Files {
		files = append(files, file.Path)
	}
	if !reflect.DeepEqual(files, []string{project + "scripts/check-links.ts", project + "scripts/prepare-readme.mjs"}) {
		t.Fatalf("explicit scripts lost or unselected/sibling files entered: %v", files)
	}
	if result.Project.ConfigPath != project+"tsconfig.json" || result.Project.ModuleResolution != "nodenext" ||
		len(result.Project.Scripts) != 1 || len(result.Project.Scripts[0].EntryFileRefs) != 2 ||
		catalog.Coverage.State != dependencies.CoverageComplete {
		t.Fatalf("script/config facts or complete dependency authority lost: project=%#v coverage=%#v", result.Project, catalog.Coverage)
	}
	controlFound := false
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.Location != nil && pattern.Location.Path == project+"scripts/check-links.ts" && pattern.Location.Line == 8 {
				if len(pattern.Context) != 1 || pattern.Context[0].Location == nil ||
					pattern.Context[0].Location.Path != pattern.Location.Path || pattern.Context[0].Location.Line != 7 ||
					pattern.Context[0].Detail != "for-of body" {
					t.Fatalf("nested package call lost its original loop anchor: %#v", pattern.Context)
				}
				controlFound = true
			}
		}
	}
	if !controlFound {
		t.Fatal("nested package loop call was omitted")
	}
	for name, source := range map[string]string{"checkLinks": "scripts/check-links.ts", "prepareReadme": "scripts/prepare-readme.mjs"} {
		var objectID string
		for _, object := range index.Objects {
			if object.Name == name && object.Location != nil && object.Location.Path == project+source {
				objectID = object.ID
			}
		}
		if objectID == "" {
			t.Fatalf("explicit %s script lost its anchored callable", source)
		}
		found := false
		for _, relation := range index.Relations {
			if relation.Kind == programindex.RelationCalls && relation.Location != nil && relation.Location.Path == project+source &&
				len(relation.ToIDs) == 1 && relation.ToIDs[0] == objectID {
				found = true
			}
		}
		if !found {
			t.Fatalf("explicit %s script lost its observed local invocation", source)
		}
	}
}

func TestCumulativeJSTSUsesActiveEnvironmentCompilerWithoutDeclaration(t *testing.T) {
	compiler, err := preparedCompilerPackage()
	if err != nil {
		t.Skip(err)
	}
	for _, via := range []string{"node-prefix", "path-tsc"} {
		t.Run(via, func(t *testing.T) {
			prefix := isolatedNodeEnvironment(t)
			if via == "node-prefix" {
				if err := materializeCompilerTree(compiler, filepath.Join(prefix, "lib", "node_modules", "typescript")); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Symlink(filepath.Join(compiler, "bin", "tsc"), filepath.Join(prefix, "bin", "tsc")); err != nil {
					t.Fatal(err)
				}
			}
			root := t.TempDir()
			_, file, _, _ := runtime.Caller(0)
			fixture := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "repositories", "jsts")
			tracked := []string{"package.json", "shared/contracts.ts", "src/ambiguity.tsx", "src/platform.ts", "src/server.ts", "src/type-members.ts", "tsconfig.json"}
			for _, name := range tracked {
				data, err := os.ReadFile(filepath.Join(fixture, name))
				if err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, root, name, string(data))
			}
			materializeCumulativeJSTSDependencyTypes(t, root)
			repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
			if err != nil {
				t.Fatal(err)
			}
			defer repository.Close()
			manifest, err := readPackageManifest(repository, "package.json")
			if err != nil || len(typeScriptCompilerPackageNames(manifest)) != 0 {
				t.Fatalf("fixture must exercise undeclared TypeScript: %v", err)
			}
			global, globalIndex, globalDependencies, err := Build(t.Context(), repository, root)
			if err != nil {
				t.Fatalf("environment compiler via %s: %v", via, err)
			}
			assertCumulativeJSTSCallbackAliases(t, globalIndex, "src/server.ts", programindex.ResolutionExact)
			assertCumulativeJSTSChainedCallbacks(t, globalIndex, "src/server.ts", programindex.ResolutionExact)
			assertCumulativeJSTSActualToFormalValueProvenance(t, global, globalIndex)
			var runtimeCaller string
			for _, object := range globalIndex.Objects {
				if object.Name == "registerRuntimeOrderHandler" && object.Location != nil && object.Location.Path == "src/server.ts" {
					runtimeCaller = object.ID
				}
			}
			if runtimeCaller == "" {
				t.Fatal("cumulative runtime callback contrast is missing")
			}
			for _, relation := range globalIndex.Relations {
				if relation.FromID == runtimeCaller && relation.Kind == programindex.RelationPassesCallback {
					t.Fatalf("environment compiler invented a runtime callback: %#v", relation)
				}
			}
			// An unusable installed package does not mask the working environment.
			writeTestFile(t, root, "node_modules/typescript/package.json", `{"name":"typescript"}`)
			writeTestFile(t, root, "node_modules/typescript/lib/typescript.js", `module.exports = {}`)
			fallback, _, _, err := Build(t.Context(), repository, root)
			if err != nil || !reflect.DeepEqual(fallback, global) {
				t.Fatalf("unusable local compiler changed the environment result: %v", err)
			}
			if err := os.RemoveAll(filepath.Join(root, "node_modules", "typescript")); err != nil {
				t.Fatal(err)
			}
			if err := materializeCompilerTree(compiler, filepath.Join(root, "node_modules", "typescript")); err != nil {
				t.Fatal(err)
			}
			// If the environment compiler is touched now, loading it fails. The
			// usable local package must win without consulting that fallback.
			if via == "node-prefix" {
				writeTestFile(t, prefix, "lib/node_modules/typescript/lib/typescript.js", `throw new Error("environment compiler must not load")`)
			} else {
				if err := os.Remove(filepath.Join(prefix, "bin", "tsc")); err != nil {
					t.Fatal(err)
				}
			}
			local, localIndex, localDependencies, err := Build(t.Context(), repository, root)
			if err != nil || !reflect.DeepEqual(local, global) || !reflect.DeepEqual(localIndex, globalIndex) || !reflect.DeepEqual(localDependencies, globalDependencies) {
				t.Fatalf("local/environment choice changed native facts or local priority: %v", err)
			}
		})
	}
}
