package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSurfaceCatalogAssetContract(t *testing.T) {
	t.Parallel()

	js := readSurfaceCatalogAsset(t, "surface_catalog.js")
	css := readSurfaceCatalogAsset(t, "surface_catalog.css")

	tests := []struct {
		name   string
		asset  string
		tokens []string
	}{
		{
			name:  "replaceable runtime API",
			asset: js,
			tokens: []string{
				"global.RepomapSurfaceCatalog",
				"Object.freeze({ mount: mount })",
				"RepomapSurfaceCatalog.mount requires a host Element",
			},
		},
		{
			name:  "bounded local filters",
			asset: js,
			tokens: []string{
				"const PAGE_SIZE = 6",
				"Surface kind",
				"All evidence",
				"Through wrapper",
				"Show less",
			},
		},
		{
			name:  "honest semantics stay distinct",
			asset: js,
			tokens: []string{
				"execution was not observed.",
				"Static · not observed",
				`this.semantic("Status"`,
				`this.semantic("Certainty"`,
				`this.semantic("Resolution"`,
				"does not prove callback execution",
				"Complete repository-wide catalog",
				"Application, tooling, tests/helpers, unassigned, and dynamic evidence stay distinct",
				"non-worker async tasks",
				"Executable · ",
				"Dynamic call-target bound reached",
				`task[1] + " · task " + task[2]`,
			},
		},
		{
			name:  "zero and filtered states do not claim absence",
			asset: js,
			tokens: []string{
				"No surfaces matched the configured terminal catalog under this build scenario.",
				"No surfaces match these filters.",
				"absence here does not prove runtime absence.",
			},
		},
		{
			name:  "catalog groups and trace progression",
			asset: js + css,
			tokens: []string{
				`"All surfaces"`,
				`label: "Application"`,
				`label: "Tooling"`,
				`label: "Tests/helpers"`,
				`label: "Unassigned"`,
				`label: "Dynamic/unresolved"`,
				`"Open saved trace"`,
				`"Trace unavailable: "`,
				`"View in Architecture"`,
			},
		},
		{
			name:  "empty analysis avoids inert catalog controls",
			asset: js,
			tokens: []string{
				`if (this.triggers.length === 0)`,
				`this.root.classList.add("is-empty")`,
				`this.summary.remove()`,
			},
		},
		{
			name:  "expandable evidence remains separated",
			asset: js,
			tokens: []string{
				`this.detailSection("Middleware"`,
				`this.detailSection("Wrapper chain"`,
				`this.detailSection("Evidence"`,
				`this.detailSection("Dynamic frontiers"`,
				"Coverage and limits",
				"Loop signals",
			},
		},
		{
			name:  "editor links and hash are supplied integration points",
			asset: js,
			tokens: []string{
				`typeof this.options.openLocation === "function"`,
				"this.options.openLocation(object(location))",
				"new URLSearchParams",
				`params.set("surface", id)`,
				`params.delete("surface")`,
			},
		},
		{
			name:  "keyboard controls and scoped styles",
			asset: css,
			tokens: []string{
				".rm-surface button:focus-visible",
				".rm-surface summary:focus-visible",
				".rm-surface__filter[aria-pressed=\"true\"]",
				".rm-surface__semantics",
				".rm-surface__coverage",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			for _, token := range test.tokens {
				if !strings.Contains(test.asset, token) {
					t.Errorf("asset is missing integration token %q", token)
				}
			}
		})
	}

	for _, forbidden := range []string{"fetch(", "XMLHttpRequest", "WebSocket", "EventSource"} {
		if strings.Contains(js, forbidden) {
			t.Errorf("surface catalog must not initiate external work: found %q", forbidden)
		}
	}
}

func readSurfaceCatalogAsset(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("templates", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}
