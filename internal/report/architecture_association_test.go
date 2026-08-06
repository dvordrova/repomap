package report

// Decision 225 provider-free acceptance: the component↔boundary/resource
// association join over exact local data (canvas member package scopes +
// Atlas observations). No model call, no new stage.
import (
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func associationFixture() (*ArchitectureCanvas, *repositoryatlas.Atlas) {
	canvas := &ArchitectureCanvas{
		Version: ArchitectureCanvasVersion,
		Components: []ArchitectureComponent{
			{
				ID:   componentmap.ComponentID("component-service"),
				Name: "Service core",
				Members: []componentmap.Candidate{
					{
						ID:   componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "member-service"},
						Role: "conceptual_member",
						Name: "service",
						Facts: []componentmap.LocalFact{{
							Kind: componentmap.FactDeclaration, Value: "github.com/example/repo/service",
							Certainty: evidence.CertaintyStatic,
						}},
					},
					{
						ID:   componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "member-util"},
						Role: "conceptual_member",
						Name: "util",
						Facts: []componentmap.LocalFact{{
							Kind: componentmap.FactDeclaration, Value: "github.com/example/repo/util",
							Certainty: evidence.CertaintyStatic,
						}},
					},
				},
			},
			{
				ID:   componentmap.ComponentID("component-other"),
				Name: "Other area",
				Members: []componentmap.Candidate{
					{
						ID:   componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "member-other"},
						Role: "conceptual_member",
						Name: "other",
						Facts: []componentmap.LocalFact{{
							Kind: componentmap.FactDeclaration, Value: "github.com/example/repo/other",
							Certainty: evidence.CertaintyStatic,
						}},
					},
				},
			},
		},
		StructuralEdges: []ArchitectureStructuralEdge{
			{ID: "edge-1", FromComponentID: "component-other", ToComponentID: "component-service"},
		},
	}
	atlas := &repositoryatlas.Atlas{
		Units: []repositoryatlas.Unit{
			{ID: "unit-service", Kind: repositoryatlas.UnitPackage, Name: "github.com/example/repo/service"},
			{ID: "unit-util", Kind: repositoryatlas.UnitPackage, Name: "github.com/example/repo/util"},
			{ID: "unit-other", Kind: repositoryatlas.UnitPackage, Name: "github.com/example/repo/other"},
			{ID: "unit-orphan", Kind: repositoryatlas.UnitPackage, Name: "github.com/example/repo/standalone"},
		},
		Entities: []repositoryatlas.Entity{
			{ID: "boundary-db", Kind: repositoryatlas.EntityBoundary, UnitID: "unit-service"},
			{ID: "resource-http", Kind: repositoryatlas.EntityResource, UnitID: "unit-service"},
			{ID: "resource-db2", Kind: repositoryatlas.EntityResource, UnitID: "unit-other"},
		},
		Observations: []repositoryatlas.Observation{
			{ID: "obs-1", UnitID: "unit-service", Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityBoundary, ID: "boundary-db"}, EvidenceRefs: []string{"ev-1"}},
			{ID: "obs-2", UnitID: "unit-service", Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityResource, ID: "resource-http"}, EvidenceRefs: []string{"ev-2"}},
			{ID: "obs-3", UnitID: "unit-other", Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityResource, ID: "resource-db2"}, EvidenceRefs: []string{"ev-3"}},
			{ID: "obs-4", UnitID: "unit-orphan", Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityBoundary, ID: "boundary-db"}, EvidenceRefs: []string{"ev-4"}},
		},
		Evidence: []repositoryatlas.Evidence{
			{ID: "ev-1", UnitID: "unit-service", Location: evidence.Location{Path: "service/db.go", Line: 10, Column: 5}, Symbol: "openDB",
				Provenance: evidence.Provenance{Provider: "boundary", Version: "v1", Operation: "observe", Detail: "database/sql"}},
			{ID: "ev-2", UnitID: "unit-service", Location: evidence.Location{Path: "service/http.go", Line: 20, Column: 5}, Symbol: "handle",
				Provenance: evidence.Provenance{Provider: "boundary", Version: "v1", Operation: "observe", Detail: "net/http"}},
			{ID: "ev-3", UnitID: "unit-other", Location: evidence.Location{Path: "other/store.go", Line: 30, Column: 5}, Symbol: "store",
				Provenance: evidence.Provenance{Provider: "boundary", Version: "v1", Operation: "observe", Detail: "github.com/example/driver"}},
			{ID: "ev-4", UnitID: "unit-orphan", Location: evidence.Location{Path: "standalone/x.go", Line: 5, Column: 5}, Symbol: "x",
				Provenance: evidence.Provenance{Provider: "boundary", Version: "v1", Operation: "observe", Detail: "os"}},
		},
	}
	return canvas, atlas
}

func TestProjectArchitectureAssociationsJoinsExactObservations(t *testing.T) {
	canvas, atlas := associationFixture()
	projection, err := ProjectArchitectureAssociations(canvas, atlas)
	if err != nil {
		t.Fatalf("ProjectArchitectureAssociations: %v", err)
	}
	if projection.Version != ArchitectureAssociationVersion {
		t.Fatalf("version = %d, want %d", projection.Version, ArchitectureAssociationVersion)
	}
	// 3 of 4 observations are inside component scopes; the orphan unit is
	// listed as an omission, never silently dropped.
	if projection.Total != 3 {
		t.Fatalf("total = %d, want 3", projection.Total)
	}
	if len(projection.Omissions) != 1 || projection.Omissions[0].Unit != "github.com/example/repo/standalone" {
		t.Fatalf("omissions = %#v, want the orphan unit", projection.Omissions)
	}
	if len(projection.Components) != 2 {
		t.Fatalf("components = %d, want 2", len(projection.Components))
	}

	// Service core: boundary(database/sql) + resource(net/http), witnesses exact.
	var service *ArchitectureComponentAssociations
	for index := range projection.Components {
		if projection.Components[index].ComponentID == "component-service" {
			service = &projection.Components[index]
		}
	}
	if service == nil {
		t.Fatalf("service component missing: %#v", projection.Components)
	}
	if len(service.Associations) != 2 {
		t.Fatalf("service associations = %d, want 2: %#v", len(service.Associations), service.Associations)
	}
	if service.Associations[0].Kind != "boundary" || service.Associations[0].ImportedFamily != "database" ||
		len(service.Associations[0].Witnesses) != 1 ||
		service.Associations[0].Witnesses[0].Path != "service/db.go" ||
		service.Associations[0].Witnesses[0].Line != 10 ||
		service.Associations[0].Witnesses[0].Symbol != "openDB" {
		t.Fatalf("boundary row = %#v", service.Associations[0])
	}
	if service.Associations[1].Kind != "resource" || service.Associations[1].ImportedFamily != "net" {
		t.Fatalf("resource row = %#v", service.Associations[1])
	}
	// Structural neighbors: other -> service is incoming for service.
	if len(service.Incoming) != 1 || service.Incoming[0].ComponentID != "component-other" ||
		service.Incoming[0].Kind != "incoming" || service.Incoming[0].Name != "Other area" {
		t.Fatalf("service incoming = %#v", service.Incoming)
	}
}

func TestProjectArchitectureAssociationsScopeBoundaryIsExact(t *testing.T) {
	canvas, atlas := associationFixture()
	// A sibling package with a shared prefix must NOT match: repo/utilx is
	// not under repo/util (the '/' boundary prevents over-matching).
	atlas.Units = append(atlas.Units, repositoryatlas.Unit{
		ID: "unit-utilx", Kind: repositoryatlas.UnitPackage, Name: "github.com/example/repo/utilx",
	})
	atlas.Observations = append(atlas.Observations, repositoryatlas.Observation{
		ID: "obs-5", UnitID: "unit-utilx",
		Subject:      repositoryatlas.EntityRef{Kind: repositoryatlas.EntityResource, ID: "resource-http"},
		EvidenceRefs: []string{"ev-1"},
	})
	projection, err := ProjectArchitectureAssociations(canvas, atlas)
	if err != nil {
		t.Fatalf("ProjectArchitectureAssociations: %v", err)
	}
	// obs-5 must be an omission, not silently matched to Service core.
	if projection.Total != 3 {
		t.Fatalf("total = %d, want 3 (sibling prefix must not match)", projection.Total)
	}
	found := false
	for _, omission := range projection.Omissions {
		if omission.Unit == "github.com/example/repo/utilx" {
			found = true
		}
	}
	if !found {
		t.Fatalf("sibling-prefix unit not listed as omission: %#v", projection.Omissions)
	}
}

func TestClassifySourceRole(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want string
	}{
		{"object/ormer.go", "production"},
		{"object/ormer_test.go", "test"},
		{"test/helpers.go", "test"},
		{"tests/integration/setup.go", "test"},
		{"contrib/analyzers/main.go", "tooling"},
		{"examples/server/main.go", "tooling"},
		{"docs/examples/main.go", "tooling"},
		{"tools/release/main.go", "tooling"},
		{"hack/verify.sh", "tooling"},
		{"", "production"},
	}
	for _, tc := range cases {
		if got := classifySourceRole(tc.path); got != tc.want {
			t.Errorf("classifySourceRole(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestArchitectureAssociationsSourceRolesReconcile proves goal 05 source
// roles: every witness is classified deterministically and the row's
// production+test+tooling counts sum exactly to the observation count.
func TestArchitectureAssociationsSourceRolesReconcile(t *testing.T) {
	t.Parallel()
	canvas, atlas := associationFixture()
	fragment, err := ProjectArchitectureAssociations(canvas, atlas)
	if err != nil {
		t.Fatal(err)
	}
	var rows, witnessed, reconciled int
	for _, component := range fragment.Components {
		for _, row := range component.Associations {
			rows++
			for _, witness := range row.Witnesses {
				witnessed++
				if witness.Role != "production" && witness.Role != "test" && witness.Role != "tooling" {
					t.Fatalf("witness role out of closed set: %q", witness.Role)
				}
				if classifySourceRole(witness.Path) != witness.Role {
					t.Fatalf("witness role %q does not match deterministic classification of %q", witness.Role, witness.Path)
				}
			}
			sum := row.SourceRoles.Production + row.SourceRoles.Test + row.SourceRoles.Tooling
			if sum != len(row.Witnesses) || sum != row.ObservationCount {
				t.Fatalf("row %s/%s roles %d+%d+%d=%d != witnesses %d != observations %d",
					row.Kind, row.OwningUnit,
					row.SourceRoles.Production, row.SourceRoles.Test, row.SourceRoles.Tooling,
					sum, len(row.Witnesses), row.ObservationCount)
			}
			reconciled++
		}
	}
	if rows == 0 || witnessed == 0 || reconciled != rows {
		t.Fatalf("no association rows exercised: rows=%d witnessed=%d", rows, witnessed)
	}
}

func TestArchitectureAssociationsRoundTripValidate(t *testing.T) {
	canvas, atlas := associationFixture()
	projection, err := ProjectArchitectureAssociations(canvas, atlas)
	if err != nil {
		t.Fatalf("ProjectArchitectureAssociations: %v", err)
	}
	if err := ValidateArchitectureAssociations(canvas, atlas, projection); err != nil {
		t.Fatalf("ValidateArchitectureAssociations: %v", err)
	}
	// Drift detection: mutate a witness path and expect validation failure.
	projection.Components[0].Associations[0].Witnesses[0].Path = "mutated.go"
	if err := ValidateArchitectureAssociations(canvas, atlas, projection); err == nil {
		t.Fatal("ValidateArchitectureAssociations accepted drifted projection")
	}
}
