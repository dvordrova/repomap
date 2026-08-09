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
		absent []string
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
			name:  "ordinary UI shows exact partial Architecture truth without generic diagnostics",
			asset: js,
			tokens: []string{
				"architecturePartialTruth(data)",
				`validation_outcome) !== "accepted_partial"`,
				"data.local_remainder_component_id",
				"this.userMode && this.partialTruth",
				`"architecture.value.accepted_partial"`,
				`"architecture.copy.accepted_partial"`,
				`"architecture.count.local_remainder_members"`,
				"rm-arch__member-id",
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
				"mapStructuralEdges(this.data)",
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
			name:  "flow navigation and evidence remain keyboard reachable; edges are passive",
			asset: js,
			tokens: []string{
				`component: "", step: "", edge: ""`,
				`"architecture.nav.saved_traces"`,
				`"architecture.label.target_declaration"`,
				// Decision 229 D1: flow step nodes are real HTML buttons
				// (keyboard-native); edges are passive visual evidence.
				`button.type = "button";`,
				`button.appendChild(element("span", "rm-arch__step-dot"))`,
				"openFlowStep(flowID, stepID)",
				"this.flowStepsByKey.get(flowStepKey(flowID, stepID))",
				"openFlowStep: (flowID, stepID) => app.openFlowStep(flowID, stepID)",
				"rm-arch__edge-visible",
			},
		},
		{
			name:  "flow fit and redacted conditions remain honest",
			asset: js,
			tokens: []string{
				"selectedFlowBounds()",
				"landscapeBounds()",
				"this.fitBounds(bounds)",
				`"architecture.title.fit_readable"`,
				`"architecture.value.condition_expression_omitted"`,
				`"architecture.error.renderer_unavailable"`,
				`this.msg("architecture.label.starts_when")`,
				`this.msg("architecture.fallback.task_root")`,
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
				// Decision 234 (F1): the fit scale clamps only the upper
				// bound — a huge landscape fits entirely inside the
				// viewport (all principal node centers hit-testable).
				"FIT_MAX_SCALE",
				"rm-arch__surface.is-focusing",
				"rm-arch__viewport-hint",
				`const viewportHint = this.msg("architecture.hint.drag_groups_fit"`,
				"this.viewportHint.textContent = viewportHint",
				`this.viewportHint.setAttribute("aria-label", viewportHint)`,
			},
		},
		{
			name:  "entry handoffs draw only distinct-component arrows",
			asset: js + css,
			tokens: []string{
				`reason: "same_component"`,
				"projection.edges.forEach((item)",
			},
			absent: []string{
				"self_loops",
				"geometry.self_loop",
				".rm-arch__edge--entry-handoff.is-self-loop",
				".rm-arch__entry-handoff-source.is-self-loop",
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
			name:  "wheel zoom scales with normalized input (Decision 234: canvas owns wheel over the map)",
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
			name:  "wheel zoom no longer requires a modifier (Decision 234 supersedes D230 D3)",
			asset: js,
			tokens: []string{
				"Decision 234 (Archive 9, owner corrective 1)",
				"the canvas OWNS the",
				"event.preventDefault()",
				"const factor = Math.exp(-delta * WHEEL_ZOOM_SENSITIVITY);",
			},
		},
		{
			name:  "wheel handler must not gate on ctrlKey/metaKey (Decision 234)",
			asset: js,
			absent: []string{
				"if (!event.ctrlKey && !event.metaKey) return;",
			},
		},
		{
			name:  "landscape without proof stays explicit",
			asset: js,
			tokens: []string{
				`const hasFlows = this.flows.length > 0`,
				`"architecture.copy.no_compatible_flowproof"`,
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
			name:  "focused flow separates grounded sequence operations and lifecycle groups",
			asset: js,
			tokens: []string{
				"renderFocusedFlow(flow)",
				"primaryFlowSteps(flow)",
				"focusedOperationEdges(flow, primary)",
				"focusedLifecycleGroups(flow)",
				"groupLifecycleRelations(flow, edges)",
				`"architecture.label.grounded_sequence"`,
				`"architecture.section.key_operations"`,
				`"architecture.label.concurrent_activities"`,
				`"architecture.value.lifecycle_started_by"`,
				`"architecture.value.lifecycle_callback"`,
				`"architecture.value.lifecycle_cancellation"`,
				`"architecture.value.lifecycle_join"`,
				`"architecture.label.limitation"`,
				`"architecture.label.exact_source"`,
				`"architecture.label.this_task"`,
				`"architecture.copy.concurrent_activities_limit"`,
				"syncFocusedSelection()",
			},
		},
		{
			name:  "focused flow exposes static proof limits and full evidence",
			asset: js,
			tokens: []string{
				`"architecture.label.saved_trace"`,
				`"architecture.value.saved_cli_trace"`,
				`"architecture.value.saved_process_trace"`,
				`"architecture.count.evidenced_transitions"`,
				`"architecture.count.trace_lanes"`,
				`"architecture.count.proof_areas_grounded"`,
				`"architecture.copy.static_evidence_not_observed"`,
				`"architecture.action.inspect_full_evidence"`,
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
				".rm-arch__focus-lifecycle",
				".rm-arch__lifecycle-card",
				".rm-arch__lifecycle-row",
				".rm-arch__inspector",
			},
		},
		{
			name:  "coherent component surface trace navigation",
			asset: js + css,
			tokens: []string{
				`"architecture.label.surfaces"`,
				`"architecture.nav.saved_traces"`,
				`"architecture.section.suggested_investigations"`,
				`"architecture.section.purpose_grounding"`,
				`"architecture.section.exact_members"`,
				`"architecture.section.evidence"`,
				`"architecture.section.unknowns"`,
				"inspectSurface(surface)",
				"openTrace(flowID)",
				"backToArchitecture()",
				`"architecture.nav.back_to_architecture"`,
				`"architecture.label.trigger"`,
				`"architecture.label.what_system_does"`,
				`"architecture.label.participating_components"`,
				`"architecture.label.current_frontier"`,
				`"architecture.label.evidence_basis"`,
				"this.landscapeView",
				"is-return-highlighted",
				"rm-arch__drawer-backdrop",
				"rm-arch__component-surfaces",
				"rm-arch__surface-chip",
				"toggleTraceMenu(open)",
				"positionTraceMenu()",
				"position: fixed",
				`target.closest(".rm-explore")`,
				`target.closest(".rm-arch__component-card")`,
				"pointer-events: none",
				"rm-arch__inspector-close",
				"this.suggestionByID",
				"suggestion.investigation_available",
				"suggestion.trace_unavailable_reason",
				`"architecture.value.investigation_unavailable"`,
				`"architecture.count.exact_anchors"`,
				"savedTraceLabel(flow.archetype, this.message)",
				`"architecture.action.open_starting_source"`,
				"rm-arch__trace-purpose",
				"flow.why_inspect",
			},
		},
		{
			name:  "plural conceptual participants never become choose-first ownership",
			asset: js,
			tokens: []string{
				"participatingComponentIDs(record, componentByID)",
				"architectureStepComponentState(step, componentByID)",
				"participants.length === 1 ? participants[0] : UNASSIGNED_ID",
				"participants.length === 1 ? participants[0] : \"\"",
				"appendParticipantComponentLinks(record, excludedComponentID)",
				"this.flowStepSelectionComponent(flow.id, step.id)",
			},
		},
		{
			name:  "optional guided tour reuses canvas selection and exact evidence",
			asset: js + css,
			tokens: []string{
				"const guidedTourStory = this.options.guidedTour",
				"if (this.guidedTourStory) {",
				`"architecture.nav.start_here"`,
				"startGuidedTour()",
				"openGuidedTourStep(index)",
				"this.openGuidedTourStep(0)",
				"showGuidedTourStep(index, animate)",
				"renderGuidedTourInspector()",
				`"architecture.nav.guided_tour"`,
				`"architecture.count.step_progress"`,
				`"architecture.action.back"`,
				`"architecture.action.next"`,
				`"architecture.action.evidence"`,
				`"architecture.action.full_map"`,
				`"architecture.section.known_gaps"`,
				`"architecture.copy.editorial_order_limit"`,
				"guidedTourComponentIDs(step)",
				"is-guided-tour-highlight",
				"if (!preserveNarrative)",
				"appendGuidedTourReferences(references, step)",
				`this.msg("architecture.label.exact_source")`,
				"this.componentByID.get(componentID)",
				"this.surfaceByID.get(surfaceID)",
				"this.flowByID.get(flowID)",
				"this.flowStepsByKey.get(flowStepKey(flowID, stepID))",
				"finishGuidedTour(focusTrigger)",
				"openGuidedTourStep: (index) => app.openGuidedTourStep(index)",
				`this.listen(this.drawerBackdrop, "click", () => this.closeInspector())`,
				"target === this.drawerBackdrop",
				".rm-arch__tour-start",
				".rm-arch__tour-step",
				".rm-arch__tour-actions",
				".rm-arch__component.is-guided-tour-highlight",
			},
		},
		{
			name:  "default user mode keeps the map useful without pipeline internals",
			asset: js,
			tokens: []string{
				"this.userMode = this.options.userMode === true",
				"this.userMode ? [] : array(this.data.frontiers)",
				"this.userMode ? [] : array(this.data.diagnostics)",
				"const activeGuidedTourStory = this.userMode ? null : guidedTourStory",
				"if (!this.userMode) this.renderUnassignedRail()",
				"renderUserFocusedFlow(flow)",
				"array(flow && flow.steps).length >= 2",
				`this.pathList = element("div", "rm-arch__path-list")`,
				`"rm-arch__flow-button rm-arch__path-button"`,
				"inspectUserComponent(component)",
				"inspectUserSurface(surface)",
				"inspectUserFlowEdge(edge)",
				"inspectUserStructuralEdge(edge)",
				`"architecture.count.code_paths"`,
				`"architecture.section.source_linked_sequence"`,
				`"architecture.action.open_code"`,
				`"architecture.label.model_verdict"`,
				`if (!this.userMode) this.listen(global, "hashchange"`,
				`if (writeHash && !this.userMode) this.writeHash()`,
				`if (this.userMode) return { flow: "", component: "", surface: "", step: "", edge: "" }`,
				"if (this.userMode) return;",
			},
		},
		{
			name:  "semantic artifacts reuse the narrative inspector and exact map selectors",
			asset: js + css,
			tokens: []string{
				"this.options.semanticArtifacts",
				"openSemanticArtifact(artifactID, index)",
				"renderSemanticArtifactInspector()",
				`"architecture.section.claims_in_step"`,
				`"architecture.section.related_map_objects"`,
				`"architecture.section.known_gaps"`,
				"semanticArtifactReferenceStep(artifact, step)",
				"appendGuidedTourReferences(references, referenceStep)",
				"data-semantic-artifact-evidence",
				"is-semantic-artifact-highlight",
				"finishSemanticArtifact(focusTrigger)",
				"openSemanticArtifact: (artifactID, index) => app.openSemanticArtifact(artifactID, index)",
				".rm-arch__semantic-statement",
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
			for _, token := range test.absent {
				if strings.Contains(test.asset, token) {
					t.Errorf("asset must not contain %q (Decision 234 supersedes the D230 D3 modifier gate)", token)
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
	// Decision 229 D1: edges are passive visual evidence — no role, no
	// tabindex, no hit path, no click/keyboard handler on edge groups.
	for _, edgeInteractive := range []string{
		`class: className, role: "button", tabindex`,
		`rm-arch__edge-hit`,
		`this.listen(group, "click"`,
	} {
		if strings.Contains(js, edgeInteractive) {
			t.Errorf("architecture canvas edges must be passive (no role/tabindex/hitbox/click): found %q", edgeInteractive)
		}
	}
	if strings.Contains(js, `"Command path"`) {
		t.Error("focused saved traces must use the archetype-neutral Grounded sequence heading")
	}
	narrativeOrder := []string{
		`this.msg("architecture.label.trigger")`,
		`this.msg("architecture.label.what_system_does")`,
		`this.msg("architecture.label.participating_components")`,
		`this.msg("architecture.label.grounded_sequence")`,
		`this.msg("architecture.label.concurrent_activities")`,
		`this.msg("architecture.label.current_frontier")`,
		`this.msg("architecture.label.evidence_basis")`,
	}
	narrativeStart := strings.Index(js, "  renderFocusedFlow(flow) {")
	if narrativeStart < 0 {
		t.Fatal("focused trace narrative renderer is absent")
	}
	narrativeJS := js[narrativeStart:]
	lastNarrativeIndex := -1
	for _, token := range narrativeOrder {
		index := strings.Index(narrativeJS, token)
		if index <= lastNarrativeIndex {
			t.Fatalf("focused trace narrative token %q is absent or out of order", token)
		}
		lastNarrativeIndex = index
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

// TestArchitectureCanvasMobileInspectorSurvivesMediaQuery (Decision 229 D8):
// on mobile (<=560px) the architecture CANVAS CARD must not be display:none
// — the position:fixed inspector is mounted INSIDE it, so hiding the card
// would hide the inspector with it. The media query hides only the visual
// map surface (viewport/toolbar/flow-focus); the inspector opens as an
// independent bottom sheet above the component list.
func TestArchitectureCanvasMobileInspectorSurvivesMediaQuery(t *testing.T) {
	t.Parallel()

	styleCSS := readCanvasAsset(t, "style.css")
	if !strings.Contains(styleCSS, "@media (max-width: 560px)") {
		t.Fatal("mobile breakpoint @media (max-width: 560px) is missing from style.css")
	}
	mediaBlock := styleCSS[strings.Index(styleCSS, "@media (max-width: 560px)"):]
	// Find the matching closing brace accounting for nested blocks.
	depth := 0
	end := -1
	for index, char := range mediaBlock {
		switch char {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = index + 1
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		t.Fatal("mobile media query block is unterminated in style.css")
	}
	mediaBlock = mediaBlock[:end]
	if strings.Contains(mediaBlock, ".rm-architecture-canvas-card { display: none") {
		t.Fatal("Decision 229 D8: mobile media query hides the canvas card, which would hide the mounted inspector")
	}
	for _, token := range []string{
		".rm-architecture-canvas-card .rm-arch__viewport",
		".rm-architecture-canvas-card .rm-arch__toolbar",
		".rm-architecture-canvas-card .rm-arch__flow-focus",
		".rm-architecture-canvas-card .rm-arch__inspector { position: fixed",
	} {
		if !strings.Contains(mediaBlock, token) {
			t.Errorf("mobile media query is missing %q — the inspector must stay visible outside the hidden map surface", token)
		}
	}
	canvasJS := readCanvasAsset(t, "architecture_canvas.js")
	if !strings.Contains(canvasJS, "rm-arch__inspector-close") {
		t.Error("inspector close control (rm-arch__inspector-close) is missing — close/back must work on mobile")
	}
	if !strings.Contains(canvasJS, "closeInspector") {
		t.Error("inspector close handler (closeInspector) is missing — close/back must work on mobile")
	}
	reportJS := readCanvasAsset(t, "script.js")
	for _, token := range []string{
		"rm-architecture-list-disclosure",
		"bindArchitectureListDisclosureMobileDefault",
		"window.matchMedia('(max-width: 560px)')",
		"media.addEventListener('change', onChange)",
		"userChoseState",
	} {
		if !strings.Contains(reportJS, token) {
			t.Errorf("mobile Map list fallback is missing %q — hidden canvas must not leave controls-only content", token)
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
