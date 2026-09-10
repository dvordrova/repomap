package places

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestExternalCallsUseJSTSWorkspaceOriginInsteadOfSharedPackageName(t *testing.T) {
	for _, language := range []string{"javascript", "typescript", "go", "python"} {
		t.Run(language, func(t *testing.T) {
			b := builder{
				files:  map[string]*fileState{"app/main": {path: "app/main"}},
				fileOf: map[string]string{"caller": "app/main"},
				byID:   map[string]programindex.Object{"caller": {Name: "Read"}},
				bounds: map[boundaryKey]*boundaryState{}, workspace: map[string]struct{}{"redis": {}},
			}
			index := programindex.Index{Target: programindex.Target{ID: "app", Language: language}}
			for position, origin := range []struct{ id, packagePath, directory string }{
				{"local-a", "redis", "shared/a"},
				{"local-b", "redis", "shared/b"},
				{"installed", "redis", ""},
				{"other-installed", "pymongo", ""},
			} {
				b.byID[origin.id] = programindex.Object{Kind: programindex.ObjectExternalSymbol, External: &programindex.ExternalSymbol{
					AuthorityKind: programindex.ExternalAuthorityPackage, PackagePath: origin.packagePath,
					RepositoryPath: origin.directory, Name: "get",
				}}
				index.Relations = append(index.Relations, programindex.Relation{
					Kind: programindex.RelationInvokesExternal, FromID: "caller", ToIDs: []string{origin.id},
					Patterns: []programindex.RelationPattern{{
						Location:  &programindex.Location{Path: "app/main", Line: 10 + position, Column: 1},
						Arguments: []programindex.PatternArgument{{Kind: programindex.PatternLiteralString, Value: "records"}},
					}},
				})
			}
			b.collectExternalCallCandidates(TargetInput{Index: index})
			b.releaseTargetObjects()
			b.collectExternalCalls()
			wantCount := 1
			if language == "javascript" || language == "typescript" {
				wantCount = 2
				redis := b.bounds[boundaryKey{path: "app/main", line: 12, kind: "sdk"}]
				if redis == nil || redis.place.Boundary.External != "redis.get" {
					t.Fatal("a same-name workspace package suppressed the installed Redis call")
				}
			}
			if len(b.bounds) != wantCount {
				t.Fatalf("boundary count = %d, want %d: %+v", len(b.bounds), wantCount, b.bounds)
			}
			for _, state := range b.bounds {
				boundary := state.place.Boundary
				if state.place.LineNo < 12 || boundary.Caller != "Read" || boundary.Direction != atlas.DirectionOut ||
					len(boundary.Values) != 1 || boundary.Values[0] != "records" {
					t.Fatalf("workspace call promoted or original evidence lost: %+v", state.place)
				}
			}
		})
	}
}
