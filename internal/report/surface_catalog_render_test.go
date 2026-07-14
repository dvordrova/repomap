package report

import (
	"slices"
	"strings"
	"testing"
)

func TestReadRunDirProjectsDiscoveredSurfaces(t *testing.T) {
	runDir := copySurfaceFixture(t)
	writeSurfaceTestFile(t, runDir, "snapshot.json", []byte(`{"repo_name":"surface-fixture"}`))
	writeSurfaceTestFile(t, runDir, "metadata.json", []byte(`{"warnings":["surface discovery unavailable: fixture warning"]}`))
	writeSurfaceTestFile(t, runDir, "orientation_report.json", []byte(`{"project_guess":"fixture"}`))
	writeSurfaceTestFile(t, runDir, "llm_bundle.json", []byte(`{"allowed_paths":[]}`))

	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.DiscoveredSurfaces == nil || len(data.DiscoveredSurfaces.Triggers) != 2 {
		t.Fatalf("discovered surfaces = %#v", data.DiscoveredSurfaces)
	}
	for _, path := range []string{
		"cmd/server/main.go",
		"internal/http/registry.go",
		"internal/http/routes.go",
		"internal/worker/queue.go",
	} {
		if !slices.Contains(data.OpenablePaths, path) {
			t.Errorf("surface evidence path %q is not openable: %#v", path, data.OpenablePaths)
		}
	}
	if len(data.Components) != 0 || data.ArchitectureCanvas != nil || len(data.Flows) != 0 {
		t.Fatalf(
			"surface facts leaked into architecture/flows: components=%d canvas=%v flows=%d",
			len(data.Components),
			data.ArchitectureCanvas != nil,
			len(data.Flows),
		)
	}
	if !slices.Contains(data.Warnings, "surface discovery unavailable: fixture warning") {
		t.Fatalf("run warning was not projected: %#v", data.Warnings)
	}
}

func TestRenderHTMLIncludesSurfaceCatalogAssetsOnlyForSavedProjection(t *testing.T) {
	t.Parallel()

	withSurfaces, err := RenderHTML(&ReportData{
		FormatVersion: CurrentFormatVersion,
		RepoName:      "fixture",
		DiscoveredSurfaces: &DiscoveredSurfaces{
			Version:         surfaceArtifactVersion,
			AnalyzerVersion: "surface-ssa-v2",
			ScenarioID:      "go:darwin/amd64:tags=",
			Triggers:        []DiscoveredTrigger{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(withSurfaces)
	for _, marker := range []string{
		`id="rm-surface-catalog-css"`,
		`id="rm-surface-catalog-js"`,
		`"discovered_surfaces"`,
		`window.RepomapSurfaceCatalog.mount`,
		`DATA.discovered_surfaces`,
	} {
		if !strings.Contains(rendered, marker) {
			t.Errorf("rendered surface report is missing %q", marker)
		}
	}
	surfaceAssetIndex := strings.Index(rendered, `id="rm-surface-catalog-js"`)
	reportScriptIndex := strings.LastIndex(rendered, "(function () {")
	if surfaceAssetIndex < 0 || reportScriptIndex <= surfaceAssetIndex {
		t.Fatalf("surface renderer must load before the report script: surface=%d report=%d", surfaceAssetIndex, reportScriptIndex)
	}

	withoutSurfaces, err := RenderHTML(&ReportData{FormatVersion: CurrentFormatVersion, RepoName: "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	legacy := string(withoutSurfaces)
	for _, marker := range []string{`id="rm-surface-catalog-css"`, `id="rm-surface-catalog-js"`} {
		if strings.Contains(legacy, marker) {
			t.Errorf("legacy report unexpectedly contains %q", marker)
		}
	}
}
