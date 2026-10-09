package contracttest

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
)

func TestGoRuntimeTypesRetainOriginalMethodsWithStandardLibraryGenericMethods(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	writePublishedGoFixtureModule(t, root)
	authorities := sharedGoFixtureAuthorities(t, root, repository, goFixtureAppPackage, "runtime-types")
	index, err := goadapter.Build(repository, authorities.target, authorities.origins, authorities.direct, authorities.external, authorities.core, authorities.dynamic, authorities.tests)
	if err != nil {
		t.Fatal(err)
	}
	assertGoRuntimeTypeMethods(t, index)
	assertProgramIndexRoundTrip(t, index)
}

func assertGoRuntimeTypeMethods(t *testing.T, index programindex.Index) {
	t.Helper()
	const path = "cmd/app/random_runtime_types.go"
	sample := programIndexObjectNamed(t, index, programindex.ObjectMethod, "Sample", path)
	reset := programIndexObjectNamed(t, index, programindex.ObjectMethod, "Reset", path)
	helper := programIndexObjectNamed(t, index, programindex.ObjectFunction, "resetRuntimeSampler", path)
	for _, expected := range []struct {
		object programindex.Object
		line   int
	}{{sample, 12}, {reset, 18}, {helper, 22}} {
		if expected.object.Location.Line != expected.line || expected.object.Location.Column < 1 {
			t.Fatalf("original callable source = %#v, want %s:%d", expected.object.Location, path, expected.line)
		}
	}
	localCall, platformCall := false, false
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationCalls && relation.FromID == reset.ID &&
			relation.Resolution == programindex.ResolutionExact && slices.Equal(relation.ToIDs, []string{helper.ID}) {
			localCall = true
		}
		if relation.Kind != programindex.RelationInvokesExternal || relation.FromID != sample.ID || len(relation.ToIDs) != 1 {
			continue
		}
		external := programIndexObjectByID(index, relation.ToIDs[0]).External
		if external != nil && external.PackagePath == "math/rand/v2" && external.Receiver == "*Rand" && external.Name == "IntN" {
			if external.AuthorityKind != programindex.ExternalAuthorityPlatform {
				t.Fatalf("rand.Rand.IntN authority = %#v", external)
			}
			for _, pattern := range relation.Patterns {
				if pattern.Location != nil && pattern.Location.Path == path && pattern.Location.Line == 13 {
					platformCall = true
				}
			}
		}
	}
	if !localCall || !platformCall {
		t.Fatalf("original method body evidence: Reset -> helper %t, Sample -> platform Rand.IntN %t", localCall, platformCall)
	}
}
