package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func TestValidateRepositoryAtlasForReportBindsExactSurfaceEvidence(t *testing.T) {
	data := repositoryAtlasReportFixture()
	if err := validateRepositoryAtlasForReport(data); err != nil {
		t.Fatalf("validateRepositoryAtlasForReport: %v", err)
	}
	data.RepoName = ""
	if err := validateRepositoryAtlasForReport(data); err != nil {
		t.Fatalf("validateRepositoryAtlasForReport with unavailable presentation name: %v", err)
	}
	unitOnly := repositoryAtlasReportFixture()
	unitOnly.RepoName = ""
	unitOnly.OpenablePaths = nil
	unitOnly.DiscoveredSurfaces = nil
	unitOnly.RepositoryAtlas.Entities = nil
	unitOnly.RepositoryAtlas.Observations = nil
	unitOnly.RepositoryAtlas.Evidence = nil
	unitOnly.RepositoryAtlas.Relations = nil
	if err := validateRepositoryAtlasForReport(unitOnly); err != nil {
		t.Fatalf("validateRepositoryAtlasForReport with unit-only Atlas: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ReportData)
		want   string
	}{
		{
			name: "repository mismatch",
			mutate: func(data *ReportData) {
				data.RepoName = "other"
			},
			want: "does not match report repository",
		},
		{
			name: "evidence is not openable",
			mutate: func(data *ReportData) {
				data.OpenablePaths = nil
			},
			want: "path is not report-openable",
		},
		{
			name: "surface trigger is missing",
			mutate: func(data *ReportData) {
				data.DiscoveredSurfaces.Triggers = nil
			},
			want: "no matching exact process trigger",
		},
		{
			name: "surface source changed",
			mutate: func(data *ReportData) {
				changed := *data.DiscoveredSurfaces.Triggers[0].ProcessEntrypoint.Location
				changed.Line++
				data.DiscoveredSurfaces.Triggers[0].ProcessEntrypoint.Location = &changed
			},
			want: "evidence does not match",
		},
		{
			name: "relation authority changed",
			mutate: func(data *ReportData) {
				data.RepositoryAtlas.Relations[0].Authority = repositoryatlas.AuthorityInferred
			},
			want: "outside the exact process-entry contract",
		},
		{
			name: "operation observation removed",
			mutate: func(data *ReportData) {
				data.RepositoryAtlas.Observations = data.RepositoryAtlas.Observations[:1]
			},
			want: "evidence does not match its exact observations",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := repositoryAtlasReportFixture()
			test.mutate(data)
			if err := validateRepositoryAtlasForReport(data); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateRepositoryAtlasForReport error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestReadRepositoryAtlasArtifactRequiresCanonicalRegularFile(t *testing.T) {
	atlas := repositoryAtlasFixture()
	encoded, err := repositoryatlas.CanonicalJSON(atlas)
	if err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runDir, repositoryatlas.ArtifactFilename), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readRepositoryAtlasArtifact(runDir)
	if err != nil || got == nil || len(got.Relations) != 1 {
		t.Fatalf("readRepositoryAtlasArtifact = %#v, %v", got, err)
	}

	if err := os.WriteFile(filepath.Join(runDir, repositoryatlas.ArtifactFilename), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRepositoryAtlasArtifact(runDir); err == nil {
		t.Fatal("readRepositoryAtlasArtifact accepted a noncanonical artifact")
	}

	if err := os.Remove(filepath.Join(runDir, repositoryatlas.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(runDir, repositoryatlas.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if _, err := readRepositoryAtlasArtifact(runDir); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("symlink artifact error = %v", err)
	}
}

func repositoryAtlasReportFixture() *ReportData {
	location := &SurfaceLocation{Path: "cmd/app/main.go", Line: 7}
	return &ReportData{
		RepoName:        "fixture",
		OpenablePaths:   []string{location.Path},
		RepositoryAtlas: repositoryAtlasFixturePtr(),
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{
			ID: "surface", Kind: "process_entry", Resolution: "exact", Certainty: "static",
			ProcessEntrypoint: SurfaceSymbol{
				ID: "example.com/fixture/cmd/app.main", Package: "example.com/fixture/cmd/app",
				Name: "main", Location: location,
			},
			Evidence: []SurfaceEvidence{{
				ID: "entrypoint-source", Kind: "process_entry_declaration", Location: location,
			}},
			Provenance: []SurfaceProvenance{{
				Provider: "gofacts", Version: "entrypoint-anchor-v1",
				Operation: "build_selected_main_declaration",
			}},
		}}},
	}
}

func repositoryAtlasFixturePtr() *repositoryatlas.Atlas {
	atlas := repositoryAtlasFixture()
	return &atlas
}

func repositoryAtlasFixture() repositoryatlas.Atlas {
	return repositoryatlas.Atlas{
		Version: repositoryatlas.Version,
		Units: []repositoryatlas.Unit{
			{ID: "repository", Kind: repositoryatlas.UnitRepository, Name: "fixture"},
			{ID: "module", Kind: repositoryatlas.UnitModule, ParentID: "repository", Name: "example.com/fixture"},
			{ID: "app", Kind: repositoryatlas.UnitApp, ParentID: "module", Name: "example.com/fixture/cmd/app"},
		},
		Entities: []repositoryatlas.Entity{
			{ID: "surface", Kind: repositoryatlas.EntitySurface, UnitID: "app"},
			{ID: "operation", Kind: repositoryatlas.EntityOperation, UnitID: "app"},
		},
		Observations: []repositoryatlas.Observation{
			{ID: "surface-observation", UnitID: "app", Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: "surface"}, EvidenceRefs: []string{"source"}},
			{ID: "operation-observation", UnitID: "app", Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: "operation"}, EvidenceRefs: []string{"source"}},
		},
		Evidence: []repositoryatlas.Evidence{{
			ID: "source", UnitID: "app", Location: evidence.Location{Path: "cmd/app/main.go", Line: 7},
			Symbol: "example.com/fixture/cmd/app.main",
			Provenance: evidence.Provenance{
				Provider: "gofacts", Version: "entrypoint-anchor-v1",
				Operation: "build_selected_main_declaration",
			},
		}},
		Relations: []repositoryatlas.Relation{{
			ID: "surface-operation", UnitID: "app", Kind: repositoryatlas.RelationExposes,
			Source: repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: "surface"},
			Target: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: "operation"},
			Phase:  repositoryatlas.PhaseStartup, Authority: repositoryatlas.AuthorityResolved,
			EvidenceRefs: []string{"source"},
		}},
	}
}
