package repositoryatlas

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
)

func TestValidateAcceptsScopedDescendantRelationAndObservation(t *testing.T) {
	atlas := validAtlasFixture()
	atlas.Entities[1].UnitID = "package"
	atlas.Observations[0].UnitID = "module"
	atlas.Observations[1].UnitID = "module"
	atlas.Relations[0].UnitID = "module"

	if err := atlas.Validate(); err != nil {
		t.Fatalf("Validate() descendant-scoped contract error = %v", err)
	}
}

func TestValidateAcceptsRepositoryOwnedAppWithoutInventingModule(t *testing.T) {
	atlas := validAtlasFixture()
	atlas.Units[2].ParentID = "repository"

	if err := atlas.Validate(); err != nil {
		t.Fatalf("Validate() repository-owned app error = %v", err)
	}
}

func TestValidateRejectsWrongKindDanglingAndOutsideScopeRefs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Atlas)
		want   string
	}{
		{
			name: "wrong typed kind",
			mutate: func(atlas *Atlas) {
				atlas.Relations[0].Source.Kind = EntityOperation
			},
			want: "has kind",
		},
		{
			name: "dangling target",
			mutate: func(atlas *Atlas) {
				atlas.Relations[0].Target.ID = "missing"
			},
			want: "unknown entity",
		},
		{
			name: "relation endpoint outside app scope",
			mutate: func(atlas *Atlas) {
				atlas.Entities[1].UnitID = "package"
			},
			want: "outside scope",
		},
		{
			name: "relation evidence outside app scope",
			mutate: func(atlas *Atlas) {
				atlas.Evidence[0].UnitID = "package"
			},
			want: "outside scope",
		},
		{
			name: "observation subject outside app scope",
			mutate: func(atlas *Atlas) {
				atlas.Entities[0].UnitID = "package"
			},
			want: "outside scope",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			atlas := validAtlasFixture()
			test.mutate(&atlas)
			if err := atlas.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateKeepsPhaseAndAuthorityClosed(t *testing.T) {
	phases := []Phase{
		PhaseRuntime, PhaseStartup, PhaseShutdown, PhaseScheduled, PhaseBuild,
		PhaseGeneration, PhaseMigration, PhaseDeploy, PhaseTest, PhaseDevelopment,
	}
	for _, phase := range phases {
		atlas := validAtlasFixture()
		atlas.Relations[0].Phase = phase
		if err := atlas.Validate(); err != nil {
			t.Fatalf("Validate() phase %q error = %v", phase, err)
		}
	}
	authorities := []Authority{
		AuthorityObserved, AuthorityResolved, AuthorityInferred, AuthorityPartial,
		AuthorityConflicted, AuthorityUnknown,
	}
	for _, authority := range authorities {
		atlas := validAtlasFixture()
		atlas.Relations[0].Authority = authority
		if err := atlas.Validate(); err != nil {
			t.Fatalf("Validate() authority %q error = %v", authority, err)
		}
	}

	for _, mutate := range []func(*Relation){
		func(relation *Relation) { relation.Phase = "warmup" },
		func(relation *Relation) { relation.Authority = "guessed" },
	} {
		atlas := validAtlasFixture()
		mutate(&atlas.Relations[0])
		if err := atlas.Validate(); err == nil {
			t.Fatal("Validate() accepted an open phase or authority value")
		}
	}
}

func TestValidateRejectsInvalidUnitTopologyAndLocation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Atlas)
	}{
		{name: "app owns module", mutate: func(atlas *Atlas) { atlas.Units[1].ParentID = "app" }},
		{name: "package path escapes repository", mutate: func(atlas *Atlas) { atlas.Evidence[0].Location.Path = "../main.go" }},
		{name: "source path is not clean", mutate: func(atlas *Atlas) { atlas.Evidence[0].Location.Path = "cmd/../main.go" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			atlas := validAtlasFixture()
			test.mutate(&atlas)
			if err := atlas.Validate(); err == nil {
				t.Fatal("Validate() accepted invalid topology or location")
			}
		})
	}
}

func validAtlasFixture() Atlas {
	return Atlas{
		Version: Version,
		Units: []Unit{
			{ID: "repository", Kind: UnitRepository, Name: "fixture"},
			{ID: "module", Kind: UnitModule, ParentID: "repository", Name: "example.com/fixture"},
			{ID: "app", Kind: UnitApp, ParentID: "module", Name: "example.com/fixture/cmd/app"},
			{ID: "package", Kind: UnitPackage, ParentID: "module", Name: "example.com/fixture/cmd/app"},
		},
		Entities: []Entity{
			{ID: "surface", Kind: EntitySurface, UnitID: "app"},
			{ID: "operation", Kind: EntityOperation, UnitID: "app"},
		},
		Observations: []Observation{
			{ID: "surface-observation", UnitID: "app", Subject: EntityRef{Kind: EntitySurface, ID: "surface"}, EvidenceRefs: []string{"source"}},
			{ID: "operation-observation", UnitID: "app", Subject: EntityRef{Kind: EntityOperation, ID: "operation"}, EvidenceRefs: []string{"source"}},
		},
		Evidence: []Evidence{{
			ID: "source", UnitID: "app", Location: evidence.Location{Path: "cmd/app/main.go", Line: 7},
			Symbol:     "example.com/fixture/cmd/app.main",
			Provenance: evidence.Provenance{Provider: "gofacts", Operation: "build_selected_main_declaration"},
		}},
		Relations: []Relation{{
			ID: "surface-operation", UnitID: "app", Kind: RelationExposes,
			Source: EntityRef{Kind: EntitySurface, ID: "surface"},
			Target: EntityRef{Kind: EntityOperation, ID: "operation"},
			Phase:  PhaseStartup, Authority: AuthorityResolved, EvidenceRefs: []string{"source"},
		}},
	}
}
