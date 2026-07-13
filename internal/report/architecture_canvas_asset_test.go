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
			},
		},
		{
			name:  "Landscape classifies graph board and hybrid projections",
			asset: js,
			tokens: []string{
				"projectLandscapeGraph()",
				"chooseLandscapeLayoutMode(projection)",
				`return "board"`,
				`return "graph"`,
				`return "hybrid"`,
				"primaryRegion",
			},
		},
		{
			name:  "balanced Landscape packing stays deterministic",
			asset: js,
			tokens: []string{
				"layoutArchitectureBoard(projection)",
				"orderedLandscapeGroups(projection)",
				"orderedGroupComponents(group, projection)",
				"packLandscapeGroups(groups, projection, profile, startY)",
				"centeredColumnOrder(columns)",
				"graphLayoutIsReadable(layout)",
				"buildLandscapeEdgeRoutes()",
			},
		},
		{
			name:  "Landscape groups flatten and exclude diagnostic remainder",
			asset: js + css,
			tokens: []string{
				"childGridShape(childCount, boardColumns)",
				"shortestCompatiblePlacement(heights, span)",
				"diagnosticSubsystemIDs(subsystems, diagnostics)",
				"diagnosticGroups: diagnosticGroups",
				"SINGLETON_GROUP_HEIGHT",
				"rm-arch__diagnostic-control",
				".rm-arch__group.is-singleton",
				"rm-arch__component-group",
			},
		},
		{
			name:  "separate evidence layers and branch semantics",
			asset: js,
			tokens: []string{
				"setSVGVisible(group",
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
				`"Saved traces"`,
				`"Target declaration"`,
				`role: "button", tabindex: "0"`,
				`event.key !== "Enter" && event.key !== " "`,
			},
		},
		{
			name:  "flow fit and redacted conditions remain honest",
			asset: js,
			tokens: []string{
				"selectedFlowBounds()",
				"landscapeBounds()",
				"this.fitBounds(bounds)",
				`"Fit architecture at readable scale"`,
				"condition (expression omitted)",
				"Architecture renderer is unavailable.",
				`this.appendKeyValue(parent, "Starts when"`,
				`root && (root.label || root.qualified_name) || "task root"`,
			},
		},
		{
			name:  "Landscape viewport actions remain distinct",
			asset: js + css,
			tokens: []string{
				"focusInitialLandscape()",
				"readableFitScale(bounds, viewport, padding)",
				"componentFocusScale(bounds, viewport, padding)",
				"focusComponent(componentID, animate)",
				"componentContextBounds(componentID)",
				"FOCUS_MIN_SCALE",
				"FIT_MIN_SCALE",
				"rm-arch__surface.is-focusing",
				"rm-arch__viewport-hint",
			},
		},
		{
			name:  "flow tabs scroll independently from fixed controls",
			asset: css,
			tokens: []string{
				".rm-arch__flows",
				"flex: 1 1 auto",
				"overflow-x: auto",
				".rm-arch__controls",
				"flex: 0 0 auto",
			},
		},
		{
			name:  "initial Landscape view stays readable and survives flow visits",
			asset: js,
			tokens: []string{
				"INITIAL_MIN_SCALE",
				"INITIAL_MAX_SCALE",
				"focusInitialLandscape()",
				"primaryLandscapeBounds()",
				"centeredTransform(bounds",
			},
		},
		{
			name:  "wheel zoom scales with normalized input",
			asset: js,
			tokens: []string{
				"WHEEL_ZOOM_SENSITIVITY",
				"MAX_WHEEL_DELTA",
				"WheelEvent.DOM_DELTA_LINE",
				"WheelEvent.DOM_DELTA_PAGE",
				"Math.exp(-delta * WHEEL_ZOOM_SENSITIVITY)",
			},
		},
		{
			name:  "landscape without proof stays explicit",
			asset: js,
			tokens: []string{
				`const hasFlows = this.flows.length > 0`,
				`"No compatible saved FlowProof is available for this run.`,
				`The landscape remains useful, but no runtime sequence is implied.`,
			},
		},
		{
			name:  "same component transitions bypass ELK self loops",
			asset: js,
			tokens: []string{
				"representedPairs",
				"localFlowLanes()",
				"localFlowRoute(edge, lane)",
				"crossFlowRoute(edge)",
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
			name:  "focused flow separates path operations and handoffs",
			asset: js,
			tokens: []string{
				"renderFocusedFlow(flow)",
				"primaryFlowSteps(flow)",
				"focusedOperationEdges(flow, primary)",
				"focusedHandoffEdges(flow, primary)",
				`"Command path"`,
				`"Key operations"`,
				`"Concurrency and lifecycle"`,
				"syncFocusedSelection()",
			},
		},
		{
			name:  "focused flow exposes static proof limits and full evidence",
			asset: js,
			tokens: []string{
				`"Saved trace"`,
				"evidenced transitions",
				"trace lanes",
				"proof areas grounded",
				`"Static evidence; execution was not observed"`,
				`"Inspect full evidence"`,
				"focusedProofSummary(flow)",
				"focusedEvidenceDisclosure(flow)",
				"appendFlowEvidence(parent, flow)",
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
				".rm-arch__flow-focus",
				".rm-arch__focus-path",
				".rm-arch__focus-operations",
				".rm-arch__focus-handoffs",
				".rm-arch__inspector",
			},
		},
		{
			name:  "coherent component surface trace navigation",
			asset: js + css,
			tokens: []string{
				`"Surfaces"`,
				`"Saved traces"`,
				`"Suggested investigations"`,
				`"Purpose and grounding"`,
				`"Exact members"`,
				`"Evidence"`,
				`"Unknowns"`,
				"inspectSurface(surface)",
				"openTrace(flowID)",
				"backToArchitecture()",
				`"← Back to architecture"`,
				`"Participating components"`,
				`"Evidence basis"`,
				"this.landscapeView",
				"is-return-highlighted",
				"rm-arch__drawer-backdrop",
				"rm-arch__component-surfaces",
				"rm-arch__surface-chip",
				"toggleTraceMenu(open)",
				"positionTraceMenu()",
				"position: fixed",
				`target.closest(".rm-arch__component-card")`,
				"pointer-events: none",
				"rm-arch__inspector-close",
				"this.suggestionByID",
				"suggestion.investigation_available",
				`"Investigation unavailable"`,
				`" exact anchor"`,
				`"CLI command · "`,
				`"Open starting source"`,
				"rm-arch__trace-purpose",
				"flow.why_inspect",
			},
		},
		{
			name:  "restrained Landscape hierarchy uses data-backed categories",
			asset: js + css,
			tokens: []string{
				"semanticCategory(record, fallback)",
				"rm-arch__group-header",
				"rm-arch__group-count",
				"rm-arch__component-description",
				"rm-arch__component-meta",
				".rm-arch__group.is-primary",
				".rm-arch__group.is-external",
				".rm-arch__group.is-support",
				".rm-arch__group.is-diagnostic",
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
	surfaceIndex := strings.Index(js, "this.surfaces.forEach((surface) => {")
	flowIndex := strings.Index(js, "this.flows.forEach((flow) => {")
	if surfaceIndex < 0 || flowIndex < 0 || surfaceIndex > flowIndex {
		t.Error("surface records must be indexed independently before saved traces")
	}

	for _, forbidden := range []string{"fetch(", "XMLHttpRequest", "WebSocket"} {
		if strings.Contains(js, forbidden) {
			t.Errorf("architecture canvas must not initiate network or analysis work: found %q", forbidden)
		}
	}
	for _, unsupportedClaim := range []string{
		"Verified execution trace",
		"verified transitions",
		"execution lanes",
		"in saved order",
	} {
		if strings.Contains(js, unsupportedClaim) {
			t.Errorf("architecture canvas must not imply observed or ordered execution: found %q", unsupportedClaim)
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
