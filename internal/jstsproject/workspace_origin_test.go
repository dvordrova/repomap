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
					if fact.Path != "/remote-key" || fact.Method != "GET" || fact.Anchor == nil || fact.Anchor.Path != source || fact.Anchor.Line != 8 {
						t.Fatalf("invented or misplaced HTTP: %#v", fact)
					}
				}
				if fact.Kind == facts.KindDependency && fact.Key == "got" {
					dependencyCount++
					if fact.Anchor == nil || fact.Anchor.Line != 3 {
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
