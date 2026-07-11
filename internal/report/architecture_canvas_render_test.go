package report

import (
	"strings"
	"testing"
)

func TestRenderHTMLIncludesArchitectureCanvasAssetsOnlyForSavedCanvas(t *testing.T) {
	t.Parallel()

	withCanvas, err := RenderHTML(&ReportData{
		FormatVersion: 5,
		RepoName:      "fixture",
		ArchitectureCanvas: &ArchitectureCanvas{
			Version: ArchitectureCanvasVersion,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(withCanvas)
	for _, marker := range []string{
		`id="rm-architecture-canvas-css"`,
		`id="rm-elkjs"`,
		`id="rm-architecture-canvas-js"`,
		`"architecture_canvas"`,
		`window.RepomapArchitectureCanvas`,
	} {
		if !strings.Contains(rendered, marker) {
			t.Fatalf("rendered report is missing %q", marker)
		}
	}
	elkIndex := strings.Index(rendered, `id="rm-elkjs"`)
	canvasIndex := strings.Index(rendered, `id="rm-architecture-canvas-js"`)
	reportIndex := strings.LastIndex(rendered, "(function () {")
	if elkIndex < 0 || canvasIndex <= elkIndex || reportIndex <= canvasIndex {
		t.Fatalf("asset order must be ELK, canvas renderer, report script: elk=%d canvas=%d report=%d", elkIndex, canvasIndex, reportIndex)
	}

	withoutCanvas, err := RenderHTML(&ReportData{FormatVersion: 5, RepoName: "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	legacy := string(withoutCanvas)
	for _, marker := range []string{`id="rm-architecture-canvas-css"`, `id="rm-elkjs"`, `id="rm-architecture-canvas-js"`} {
		if strings.Contains(legacy, marker) {
			t.Fatalf("legacy report unexpectedly contains %q", marker)
		}
	}
}
