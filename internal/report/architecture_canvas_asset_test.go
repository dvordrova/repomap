package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchitectureCanvasAssetContract(t *testing.T) {
	t.Parallel()

	js := readCanvasAsset(t, "architecture_canvas.js")
	css := readCanvasAsset(t, "architecture_canvas.css")

	// Replaceable presentation contract: these checks keep the standalone
	// renderer integrable while its visual details remain intentionally cheap
	// to rewrite during canvas iteration.
	tests := []struct {
		name   string
		asset  string
		tokens []string
	}{
		{
			name:  "runtime API and one-shot ELK layout",
			asset: js,
			tokens: []string{
				"global.RepomapArchitectureCanvas",
				"mount: mount",
				"layoutOnce()",
				`"elk.algorithm": "layered"`,
				`"elk.edgeRouting": "ORTHOGONAL"`,
				`"elk.hierarchyHandling": "INCLUDE_CHILDREN"`,
			},
		},
		{
			name:  "separate evidence layers and branch semantics",
			asset: js,
			tokens: []string{
				"rm-arch__edges--structural",
				"rm-arch__edges--flow",
				"from_branch_id",
				"to_branch_id",
				"cross_branch",
				"edge.witness",
				"step.binding",
			},
		},
		{
			name:  "stable selection hash and editor callback",
			asset: js,
			tokens: []string{
				"new URLSearchParams",
				`params.set("flow"`,
				`params.set("component"`,
				`params.set("step"`,
				`params.set("edge"`,
				"this.options.openLocation",
			},
		},
		{
			name:  "flow navigation and evidence remain keyboard reachable",
			asset: js,
			tokens: []string{
				`component: "", step: "", edge: ""`,
				`"Participating flows"`,
				`"Target declaration"`,
				`role: "button", tabindex: "0"`,
				`event.key !== "Enter" && event.key !== " "`,
			},
		},
		{
			name:  "same component transitions bypass ELK self loops",
			asset: js,
			tokens: []string{
				"if (!fromOwner || !toOwner || fromOwner === toOwner) return;",
				"localFlowLanes()",
				"localFlowRoute(edge, lane)",
				`isLocal ? "is-local"`,
			},
		},
		{
			name:  "bounded chips preserve branch and lifecycle semantics",
			asset: js,
			tokens: []string{
				`const BRANCH_PRIORITY = ["main", "task", "shared"]`,
				`const SEMANTIC_PRIORITY = ["is-join", "is-start", "is-callback", "is-cancel", "is-frontier", "is-call"]`,
				"selectVisibleItems(flow, items)",
				"flowItemStableKey(item)",
				"overflowGeometry(owner)",
			},
		},
		{
			name:  "flow and inspector styles",
			asset: css,
			tokens: []string{
				".rm-arch__edges--structural",
				".rm-arch__edges--flow",
				".rm-arch__step.is-cancel",
				".rm-arch__step.is-join",
				".rm-arch__step.is-shared",
				".rm-arch__step.is-frontier",
				".rm-arch__step.is-overflow",
				".rm-arch__edge--flow.is-local",
				".rm-arch__inspector",
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

	for _, forbidden := range []string{"fetch(", "XMLHttpRequest", "WebSocket"} {
		if strings.Contains(js, forbidden) {
			t.Errorf("architecture canvas must not initiate network or analysis work: found %q", forbidden)
		}
	}
}

func readCanvasAsset(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("templates", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}
