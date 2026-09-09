package jstsproject

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestCumulativeJSTSWorkspaceOriginsDoNotInventHTTP(t *testing.T) {
	for _, source := range []string{"src/workspace-origins.ts", "src/workspace-origins-js.js"} {
		t.Run(source, func(t *testing.T) {
			lineOffset := 0
			if strings.HasSuffix(source, ".js") {
				lineOffset = 1 // The JS source keeps its author shebang.
			}
			root := preparedCompilerProject(t)
			_, filename, _, _ := runtime.Caller(0)
			fixture := filepath.Join(filepath.Dir(filename), "../../testdata/repositories/jsts")
			var tracked []string
			err := filepath.WalkDir(fixture, func(name string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				relative, err := filepath.Rel(fixture, name)
				if err != nil {
					return err
				}
				contents, err := os.ReadFile(name)
				if err != nil {
					return err
				}
				relative = filepath.ToSlash(relative)
				writeTestFile(t, root, relative, string(contents))
				tracked = append(tracked, relative)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(tracked)
			writeTestFile(t, root, "tsconfig.json", `{"include":["`+source+`"],"compilerOptions":{"allowJs":true,"module":"ESNext","moduleResolution":"bundler","strict":true}}`)
			writeTestFile(t, root, "node_modules/got/package.json", `{"name":"got","version":"1.0.0","types":"index.d.ts"}`)
			writeTestFile(t, root, "node_modules/got/index.d.ts", "export function get(url: string): Promise<string>\n")
			repository, err := corpus.New(context.Background(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
			if err != nil {
				t.Fatal(err)
			}
			defer repository.Close()
			result, err := DiscoverSelected(context.Background(), repository, root, "jsts:package.json")
			if err != nil {
				t.Fatal(err)
			}
			index, catalog, err := BuildFromResult(result)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Files) != 1 || result.Files[0].Path != source {
				t.Fatalf("sibling files entered the caller: %#v", result.Files)
			}
			assertRepeatedAliasedImports(t, source, result, index, catalog)
			if strings.HasSuffix(source, ".js") {
				assertCumulativeJSTSCallbackAliases(t, index, source, programindex.ResolutionAlternatives)
				assertCumulativeJSTSChainedCallbacks(t, index, source, programindex.ResolutionAlternatives)
			}
			objects := make(map[string]programindex.Object)
			for _, object := range index.Objects {
				objects[object.ID] = object
			}
			calls := make(map[string]programindex.Relation)
			for _, relation := range index.Relations {
				if relation.Location != nil && relation.Location.Path == source && len(relation.Patterns) == 1 {
					calls[relation.SourceRef] = relation
				}
			}
			wantResolution := programindex.ResolutionExact
			if strings.HasSuffix(source, ".js") {
				wantResolution = programindex.ResolutionAlternatives
			}
			identities := map[string]string{}
			for expression, directory := range map[string]string{"local.get": "packages/local-store", "other.get": "packages/second-store", "remote.get": ""} {
				var relation programindex.Relation
				for _, call := range result.Calls {
					if call.Expression != expression {
						continue
					}
					if call.ExternalPackage != "got" || call.RepositoryPath != directory {
						t.Fatalf("origin %s: %#v", expression, call)
					}
					relation = calls["program:"+call.Ref]
				}
				if relation.Kind != programindex.RelationInvokesExternal || relation.Resolution != wantResolution || len(relation.ToIDs) != 1 {
					t.Fatalf("call %s: %#v", expression, relation)
				}
				object := objects[relation.ToIDs[0]]
				if object.External == nil || object.External.AuthorityKind != programindex.ExternalAuthorityPackage || object.External.RepositoryPath != directory || object.External.PackagePath != "got" {
					t.Fatalf("projected %s: %#v", expression, object)
				}
				if directory == "" {
					if len(object.SymbolLinkIdentities) != 0 {
						t.Fatalf("npm linked to repository: %#v", object)
					}
				} else {
					identity := identityForObjectID(t, index, object.ID, "got#get")
					identities[directory] = identity.Key
					// The actual sibling export meets this exact boundary even though
					// both packages have the same name and identical source contents.
					sibling, err := DiscoverSelected(context.Background(), repository, root, "jsts:"+directory+"/package.json")
					if err != nil {
						t.Fatal(err)
					}
					siblingIndex, _, err := BuildFromResult(sibling)
					if err != nil {
						t.Fatal(err)
					}
					found := false
					for _, declaration := range sibling.Declarations {
						if declaration.Name == "get" {
							local := identityForSourceRef(t, siblingIndex, declaration.Ref, "got#get")
							if local.Key != identity.Key {
								t.Fatalf("sibling identity mismatch: %#v / %#v", identity, local)
							}
							found = true
						}
					}
					if !found {
						t.Fatal("sibling export missing")
					}
				}
			}
			if identities["packages/local-store"] == identities["packages/second-store"] {
				t.Fatal("same-name sibling identities collapsed")
			}
			var origins []string
			for _, dependency := range catalog.Dependencies {
				if dependency.PackagePath == "got" {
					origins = append(origins, string(dependency.Kind)+":"+dependency.RepositoryPath)
				}
			}
			sort.Strings(origins)
			if !reflect.DeepEqual(origins, []string{string(dependencies.KindExternal) + ":", string(dependencies.KindWorkspace) + ":packages/local-store", string(dependencies.KindWorkspace) + ":packages/second-store"}) {
				t.Fatalf("dependency origins = %v", origins)
			}
			// The factory's import remains receiver provenance, never an HTTP client.
			var factory programindex.Relation
			for _, call := range result.Calls {
				if call.Expression == "store.get" {
					factory = calls["program:"+call.Ref]
				}
			}
			if len(factory.Patterns) != 1 {
				t.Fatalf("factory pattern missing: %#v", factory)
			}
			if strings.HasSuffix(source, ".ts") && (len(factory.Patterns[0].ReceiverOriginIDs) != 1 || objects[factory.Patterns[0].ReceiverOriginIDs[0]].External.RepositoryPath != "packages/local-store") {
				t.Fatalf("factory origin lost: %#v", factory)
			}
			factResult, err := facts.Build(facts.Input{Revision: strings.Repeat("a", 40), Repository: repository, Targets: []facts.TargetInput{{Index: index, Dependencies: &catalog}}})
			if err != nil {
				t.Fatal(err)
			}
			httpCount, dependencyCount := 0, 0
			for _, fact := range factResult.Facts {
				if fact.Kind == facts.KindHTTPCall || fact.Kind == facts.KindHTTPRoute {
					httpCount++
					if fact.Path != "/remote-key" || fact.Method != "GET" || fact.Anchor == nil || fact.Anchor.Path != source || fact.Anchor.Line != 8+lineOffset {
						t.Fatalf("invented or misplaced HTTP: %#v", fact)
					}
				}
				if fact.Kind == facts.KindDependency && fact.Key == "got" {
					dependencyCount++
					if fact.Anchor == nil || fact.Anchor.Line != 3+lineOffset {
						t.Fatalf("npm dependency anchored to sibling: %#v", fact)
					}
				}
			}
			if httpCount != 1 || dependencyCount != 1 {
				t.Fatalf("HTTP=%d dependency=%d", httpCount, dependencyCount)
			}
		})
	}
}

func assertRepeatedAliasedImports(t *testing.T, source string, result Result, index programindex.Index, catalog dependencies.Catalog) {
	t.Helper()
	lineOffset := 0
	if strings.HasSuffix(source, ".js") {
		lineOffset = 1
	}
	if err := index.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	if catalog.Coverage.State != dependencies.CoverageComplete || len(catalog.Coverage.Omissions) != 0 {
		t.Fatalf("repeated named imports invented missing dependency evidence: %+v", catalog.Coverage)
	}
	importRef := ""
	for _, imported := range result.Imports {
		if imported.Location.Path == source && imported.Location.Line == 13+lineOffset {
			if importRef != "" || imported.Specifier != "../packages/local-store/src/index" || imported.RepositoryPath != "packages/local-store" {
				t.Fatalf("one repeated named import acquired duplicate or wrong authority: %+v", imported)
			}
			importRef = "program:" + imported.Ref
		}
	}
	if importRef == "" {
		t.Fatal("repeated named import disappeared from compiler result")
	}
	imports := 0
	for _, relation := range index.Relations {
		if relation.SourceRef != importRef {
			continue
		}
		imports++
		if relation.Kind != programindex.RelationImports || relation.TargetsObserved != 1 || relation.TargetsOmitted != 0 ||
			relation.WitnessesObserved != 1 || relation.WitnessesOmitted != 0 || len(relation.Witnesses) != 1 ||
			relation.Witnesses[0].Location == nil || relation.Witnesses[0].Location.Path != source || relation.Witnesses[0].Location.Line != 13+lineOffset {
			t.Fatalf("repeated named import inflated witness count: %+v", relation)
		}
	}
	if imports != 1 {
		t.Fatalf("repeated named import relations = %d, want one", imports)
	}
	want := map[string]bool{"get": false, "datasetGet": false, "runGet": false}
	columns := make(map[int]bool)
	target := ""
	for _, call := range result.Calls {
		if _, known := want[call.Expression]; !known {
			continue
		}
		if want[call.Expression] || call.ExternalPackage != "got" || call.RepositoryPath != "packages/local-store" ||
			call.Location.Path != source || call.Location.Line != 16+lineOffset || call.Location.Column <= 0 {
			t.Fatalf("aliased import call acquired wrong origin or source: %+v", call)
		}
		want[call.Expression] = true
		columns[call.Location.Column] = true
		found := false
		for _, relation := range index.Relations {
			if relation.SourceRef != "program:"+call.Ref {
				continue
			}
			found = true
			if relation.Kind != programindex.RelationInvokesExternal || len(relation.ToIDs) != 1 ||
				relation.TargetsObserved != 1 || relation.TargetsOmitted != 0 || relation.WitnessesObserved != 1 || relation.WitnessesOmitted != 0 || len(relation.Witnesses) != 1 {
				t.Fatalf("aliased import call lost exact witness coverage: %+v", relation)
			}
			if target != "" && target != relation.ToIDs[0] {
				t.Fatalf("aliases of one export acquired different declaration identities: %s / %s", target, relation.ToIDs[0])
			}
			target = relation.ToIDs[0]
		}
		if !found {
			t.Fatalf("aliased import call %s disappeared at adapter boundary", call.Expression)
		}
	}
	for expression, found := range want {
		if !found {
			t.Fatalf("aliased import call %s missing", expression)
		}
	}
	if len(columns) != 3 {
		t.Fatalf("three same-line aliased calls collapsed: %v", columns)
	}
}
