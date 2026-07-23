package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

var chiDispatchAliases = []string{
	"how chi routes a request",
	"how request reaches handler",
	"Mux ServeHTTP",
	"chi 404 405 routing",
	"как запрос chi доходит до handler",
}

func prepareChiDispatch(
	ctx context.Context,
	runDir string,
) (chiDispatchPrepared, goldenmechanism.BudgetStats, error) {
	loaded, err := loadChiDispatchRun(ctx, runDir)
	if err != nil {
		return chiDispatchPrepared{}, goldenmechanism.BudgetStats{}, err
	}
	windows, err := sourcewindowfacts.LoadRun(runDir, loaded.analysisRoot)
	if err != nil {
		return chiDispatchPrepared{}, goldenmechanism.BudgetStats{}, err
	}
	windowByPath, err := requiredChiSavedWindows(windows)
	if err != nil {
		return chiDispatchPrepared{}, goldenmechanism.BudgetStats{}, err
	}
	if err := requireChiFindRouteFrontier(runDir, windowByPath["tree.go"].EvidenceID); err != nil {
		return chiDispatchPrepared{}, goldenmechanism.BudgetStats{}, err
	}

	serveHTTP, err := sourcewindowfacts.ExtractGoFunction(windowByPath["mux.go"], "Mux.ServeHTTP")
	if err != nil {
		return chiDispatchPrepared{}, goldenmechanism.BudgetStats{}, err
	}
	reset, err := sourcewindowfacts.ExtractGoFunction(windowByPath["context.go"], "Context.Reset")
	if err != nil {
		return chiDispatchPrepared{}, goldenmechanism.BudgetStats{}, err
	}
	chainServe, err := sourcewindowfacts.ExtractGoFunction(
		windowByPath["chain.go"],
		"ChainHandler.ServeHTTP",
	)
	if err != nil {
		return chiDispatchPrepared{}, goldenmechanism.BudgetStats{}, err
	}
	muxAnchor, err := chiSavedMuxAnchor(windowByPath["mux.go"], serveHTTP)
	if err != nil {
		return chiDispatchPrepared{}, goldenmechanism.BudgetStats{}, err
	}
	treeAnchor, err := chiSavedTreeAnchor(windowByPath["tree.go"])
	if err != nil {
		return chiDispatchPrepared{}, goldenmechanism.BudgetStats{}, err
	}

	plan := chiDispatchProbePlan(muxAnchor, treeAnchor, windowByPath)
	probe, err := goldenmechanism.Probe(ctx, loaded.analysisRoot, plan)
	if err != nil {
		return chiDispatchPrepared{}, goldenmechanism.BudgetStats{}, err
	}
	actualBudget := probe.Budget
	if probe.Partial || probe.Budget.FilesParsed != 2 || probe.Budget.FunctionsIncluded != 3 {
		return chiDispatchPrepared{}, actualBudget, fmt.Errorf(
			"chi request dispatch: bounded probe escaped its exact 2-file and 3-function scope",
		)
	}
	routeWindow, err := chiProbeFunctionWindow(probe, "Mux.routeHTTP")
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	findWindow, err := chiProbeFunctionWindow(probe, "node.FindRoute")
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	routeHTTP, err := sourcewindowfacts.ExtractGoFunction(routeWindow, "Mux.routeHTTP")
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	findRoute, err := sourcewindowfacts.ExtractGoFunction(findWindow, "node.FindRoute")
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	stableProbe := probe
	stableProbe.Budget.ElapsedMillis = 0
	probeBytes, err := marshalGoldenJSON(stableProbe)
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	probeSHA := digestSHA256(probeBytes)
	if err := writeFrozenGoldenFile(
		filepath.Join(runDir, report.GoldenMechanismProbeFile),
		probeBytes,
	); err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}

	projection, err := projectChiDispatchFacts(
		loaded.baseBundle,
		windowByPath,
		serveHTTP,
		reset,
		chainServe,
		routeHTTP,
		findRoute,
		muxAnchor,
		treeAnchor,
	)
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	supplement, bundle, err := report.PrepareSemanticSupplement(
		loaded.data,
		projection.Candidate.ID,
		probeSHA,
		projection.Facts,
	)
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	proposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{projection.Candidate},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(bundle, proposal); err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	leaf, err := buildChiDispatchLeaf(bundle, projection.Candidate)
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	projection.Leaf = leaf
	projectionBytes, err := marshalGoldenJSON(projection)
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	if err := writeFrozenGoldenFile(
		filepath.Join(runDir, chiDispatchProjectionFile),
		projectionBytes,
	); err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	supplementBytes, err := marshalGoldenJSON(supplement)
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	if err := writeFrozenGoldenFile(
		filepath.Join(runDir, chiDispatchSupplementFile),
		supplementBytes,
	); err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	prompt, err := semanticdiscovery.BuildGoldenMechanismPrompt(bundle, leaf)
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	promptBytes, err := marshalGoldenJSON(prompt)
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	bundleSHA, _, err := semanticdiscovery.BundleHash(bundle)
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	fixture := chiDispatchFixture{
		Version:              1,
		State:                "frozen_before_model_response",
		RepositoryRevision:   loaded.manifest.RepositoryState.Head,
		Question:             chiDispatchQuestion,
		IntentKey:            chiDispatchIntentKey,
		CandidateID:          projection.Candidate.ID,
		ProbePlan:            plan,
		ProbeSHA256:          probeSHA,
		ProjectionSHA256:     digestSHA256(projectionBytes),
		SupplementSHA256:     digestSHA256(supplementBytes),
		EnrichedBundleSHA256: bundleSHA,
		PromptVersion:        prompt.Version,
		PromptSHA256:         digestSHA256(promptBytes),
		MaxModelCalls:        chiDispatchMaxModelCalls,
	}
	fixtureBytes, err := marshalGoldenJSON(fixture)
	if err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	if err := writeFrozenGoldenFile(filepath.Join(runDir, chiDispatchFixtureFile), fixtureBytes); err != nil {
		return chiDispatchPrepared{}, actualBudget, err
	}
	return chiDispatchPrepared{
		Loaded: loaded, Probe: stableProbe, Projection: projection, Supplement: supplement,
		Bundle: bundle, Proposal: proposal, Leaf: leaf, Prompt: prompt,
		Fixture: fixture, FixtureHash: digestSHA256(fixtureBytes),
	}, actualBudget, nil
}

func requiredChiSavedWindows(
	windows []sourcewindowfacts.Window,
) (map[string]sourcewindowfacts.Window, error) {
	required := map[string]string{
		"chain.go":   "evidence-612da3ffe3636943",
		"context.go": "evidence-f7357ef4a26ef84d",
		"mux.go":     "evidence-d329282ab3d000c6",
		"tree.go":    "evidence-8bbdeb1409138cbc",
	}
	result := make(map[string]sourcewindowfacts.Window, len(required))
	for _, window := range windows {
		wantID, wanted := required[window.Path]
		if !wanted || window.EvidenceID != wantID {
			continue
		}
		if _, duplicate := result[window.Path]; duplicate {
			return nil, fmt.Errorf("chi request dispatch: duplicate saved window for %s", window.Path)
		}
		result[window.Path] = window
	}
	if len(result) != len(required) {
		return nil, fmt.Errorf("chi request dispatch: fixed run does not contain all four exact source windows")
	}
	return result, nil
}

func requireChiFindRouteFrontier(runDir, treeWindowID string) error {
	state, err := modelresearch.ReadState(runDir)
	if err != nil {
		return fmt.Errorf("chi request dispatch: read saved research frontier: %w", err)
	}
	for _, frontier := range state.Theory.UnresolvedFrontiers {
		if !slicesContains(frontier.EvidenceIDs, treeWindowID) {
			continue
		}
		text := frontier.Question + " " + frontier.Reason
		if strings.Contains(text, "FindRoute") {
			return nil
		}
	}
	return fmt.Errorf("chi request dispatch: saved tree frontier no longer requests FindRoute")
}

func chiProbeFunctionWindow(
	probe goldenmechanism.Result,
	symbol string,
) (sourcewindowfacts.Window, error) {
	var matched *goldenmechanism.Function
	for index := range probe.Functions {
		function := &probe.Functions[index]
		if function.Symbol != symbol {
			continue
		}
		if matched != nil {
			return sourcewindowfacts.Window{}, fmt.Errorf(
				"chi request dispatch: probe returned duplicate %s functions",
				symbol,
			)
		}
		matched = function
	}
	if matched == nil || matched.SourceTruncated || len(matched.Source) == 0 {
		return sourcewindowfacts.Window{}, fmt.Errorf(
			"chi request dispatch: probe did not retain complete %s source",
			symbol,
		)
	}
	lines := make([]string, 0, len(matched.Source))
	for index, line := range matched.Source {
		wantLine := matched.Location.Line + index
		if line.Truncated || line.Location.Path != matched.Path || line.Location.Line != wantLine {
			return sourcewindowfacts.Window{}, fmt.Errorf(
				"chi request dispatch: probe source for %s is not contiguous",
				symbol,
			)
		}
		lines = append(lines, line.Text)
	}
	return sourcewindowfacts.NewWindow(
		matched.ID,
		matched.Path,
		matched.Location.Line,
		lines,
	)
}

func chiDispatchProbePlan(
	muxAnchor semanticdiscovery.Fact,
	treeAnchor semanticdiscovery.Fact,
	windows map[string]sourcewindowfacts.Window,
) goldenmechanism.Plan {
	return goldenmechanism.Plan{
		MechanismID: chiDispatchIntentKey,
		Seeds: []goldenmechanism.Seed{
			{
				OriginFactID:     muxAnchor.ID,
				OriginEvidenceID: windows["mux.go"].EvidenceID,
				Path:             "mux.go", Symbol: "Mux.ServeHTTP",
			},
			{
				OriginFactID:     muxAnchor.ID,
				OriginEvidenceID: windows["mux.go"].EvidenceID,
				Path:             "mux.go", Symbol: "Mux.routeHTTP",
			},
			{
				OriginFactID:     treeAnchor.ID,
				OriginEvidenceID: windows["tree.go"].EvidenceID,
				Path:             "tree.go", Symbol: "node.FindRoute",
			},
		},
		ExpansionAllowlist: []string{"Mux.ServeHTTP", "Mux.routeHTTP", "node.FindRoute"},
		Limits: goldenmechanism.Limits{
			MaxDepth: 1, MaxFiles: 2, MaxFunctions: 3,
			MaxParsedSourceBytes: 128 << 10,
			MaxSourceBytes:       32 << 10,
			MaxFunctionLines:     80,
			MaxFunctionBytes:     16 << 10,
			Timeout:              2 * time.Second,
		},
	}
}

func chiSavedMuxAnchor(
	window sourcewindowfacts.Window,
	serveHTTP sourcewindowfacts.Function,
) (semanticdiscovery.Fact, error) {
	declaration, err := chiObservations(
		serveHTTP,
		chiObservationSelector{kind: sourcewindowfacts.ObservationDeclaration},
	)
	if err != nil {
		return semanticdiscovery.Fact{}, err
	}
	comment, err := chiWindowEvidence(
		window,
		"mx.handler that is comprised of mx.middlewares + mx.routeHTTP.",
	)
	if err != nil {
		return semanticdiscovery.Fact{}, err
	}
	evidenceRefs := []semanticdiscovery.EvidenceRef{chiSavedWindowEvidence(window)}
	evidenceRefs = append(evidenceRefs, chiObservationEvidence(serveHTTP, declaration...)...)
	evidenceRefs = append(evidenceRefs, comment...)
	return newChiDispatchFact(
		"saved-mux-window",
		"A saved bounded source window contains the Mux request-handler declaration and a source comment naming routeHTTP as part of the computed handler. The comment authorizes bounded retrieval but does not prove dynamic wiring.",
		nil,
		[]string{"saved mux window", "route retrieval", "computed handler gap"},
		[]semanticdiscovery.Capability{
			semanticdiscovery.CapabilityStatic,
			semanticdiscovery.CapabilityLimitation,
		},
		chiFunctionSource(serveHTTP),
		chiGroup("saved-window", window.EvidenceID),
		evidenceRefs,
	), nil
}

func chiSavedTreeAnchor(window sourcewindowfacts.Window) (semanticdiscovery.Fact, error) {
	evidenceRefs, err := chiWindowEvidence(
		window,
		"type node struct {",
		"endpoints endpoints",
	)
	if err != nil {
		return semanticdiscovery.Fact{}, err
	}
	evidenceRefs = append([]semanticdiscovery.EvidenceRef{chiSavedWindowEvidence(window)}, evidenceRefs...)
	return newChiDispatchFact(
		"saved-tree-window",
		"A saved bounded tree window declares a node with endpoint storage. The window does not contain the route lookup method and does not prove traversal behavior.",
		nil,
		[]string{"saved tree window", "node endpoints", "scope limitation"},
		[]semanticdiscovery.Capability{
			semanticdiscovery.CapabilityStatic,
			semanticdiscovery.CapabilityLimitation,
		},
		&semanticdiscovery.FactSource{
			Path:            window.Path,
			StartLine:       window.StartLine,
			EndLine:         window.EndLine,
			EnclosingSymbol: "node",
			ContentSHA256:   window.ContentSHA256,
		},
		chiGroup("saved-window", window.EvidenceID),
		evidenceRefs,
	), nil
}

func chiSavedWindowEvidence(window sourcewindowfacts.Window) semanticdiscovery.EvidenceRef {
	return semanticdiscovery.EvidenceRef{
		ID: window.EvidenceID, Kind: "saved_source_window",
		Label: "saved bounded source window", Path: window.Path, Line: window.StartLine,
	}
}

func projectChiDispatchFacts(
	base semanticdiscovery.Bundle,
	windows map[string]sourcewindowfacts.Window,
	serveHTTP sourcewindowfacts.Function,
	reset sourcewindowfacts.Function,
	chainServe sourcewindowfacts.Function,
	routeHTTP sourcewindowfacts.Function,
	findRoute sourcewindowfacts.Function,
	muxAnchor semanticdiscovery.Fact,
	treeAnchor semanticdiscovery.Fact,
) (goldenProjection, error) {
	entryEvidence, err := chiObservations(
		serveHTTP,
		chiObservationSelector{kind: sourcewindowfacts.ObservationDeclaration},
		chiObservationSelector{kind: sourcewindowfacts.ObservationBranch, object: "mx.handler == nil"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "mx.NotFoundHandler().ServeHTTP"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationReturn, lineAfter: 65},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	contextEvidence, err := chiObservations(
		serveHTTP,
		chiObservationSelector{kind: sourcewindowfacts.ObservationAssignment, object: "rctx", contains: "mx.pool.Get()"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "rctx.Reset"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationAssignment, object: "r", contains: "context.WithValue"},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	resetEvidence, err := chiObservations(
		reset,
		chiObservationSelector{kind: sourcewindowfacts.ObservationAssignment, object: "x.Routes", contains: "nil"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationAssignment, object: "x.URLParams.Keys", contains: "[:0]"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationAssignment, object: "x.methodNotAllowed", contains: "false"},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	handlerEvidence, err := chiObservations(
		serveHTTP,
		chiObservationSelector{kind: sourcewindowfacts.ObservationBranch, object: "rctx != nil"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "mx.handler.ServeHTTP"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "mx.handler.ServeHTTP", lineAfter: 80},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	routeLookupEvidence, err := chiObservations(
		routeHTTP,
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "mx.tree.FindRoute"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationBranch, object: "h != nil"},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	findDeclaration, err := chiObservations(
		findRoute,
		chiObservationSelector{kind: sourcewindowfacts.ObservationDeclaration},
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "n.findRoute"},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	parameterEvidence, err := chiObservations(
		findRoute,
		chiObservationSelector{kind: sourcewindowfacts.ObservationAssignment, object: "rctx.URLParams.Keys", contains: "append"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationAssignment, object: "rctx.URLParams.Values", contains: "append"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationAssignment, object: "rctx.RoutePatterns", contains: "append"},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	requestParameterEvidence, err := chiObservations(
		routeHTTP,
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "r.SetPathValue"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationAssignment, object: "r.Pattern", contains: "rctx.RoutePattern()"},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	endpointEvidence, err := chiObservations(
		routeHTTP,
		chiObservationSelector{kind: sourcewindowfacts.ObservationBranch, object: "h != nil"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "r.SetPathValue"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationAssignment, object: "r.Pattern", contains: "rctx.RoutePattern()"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "h.ServeHTTP"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationReturn, lineAfter: 480},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	fallbackEvidence, err := chiObservations(
		routeHTTP,
		chiObservationSelector{kind: sourcewindowfacts.ObservationBranch, object: "!ok"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationBranch, object: "rctx.methodNotAllowed"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "mx.MethodNotAllowedHandler().ServeHTTP"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "mx.MethodNotAllowedHandler(rctx.methodsAllowed...).ServeHTTP"},
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "mx.NotFoundHandler().ServeHTTP"},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	chainEvidence, err := chiObservations(
		chainServe,
		chiObservationSelector{kind: sourcewindowfacts.ObservationDirectCall, target: "c.chain.ServeHTTP"},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	facts := []semanticdiscovery.Fact{
		newChiDispatchFact(
			"request-entry",
			"The Mux HTTP handler is the bounded request entry. A local nil-handler branch selects the not-found handler and returns instead of entering computed dispatch.",
			[]string{"request_entry"},
			[]string{"Mux ServeHTTP", "request entry", "nil handler"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityEntry,
				semanticdiscovery.CapabilityBranch,
				semanticdiscovery.CapabilityDirectCall,
			},
			chiFunctionSource(serveHTTP),
			chiGroup("saved-window", windows["mux.go"].EvidenceID),
			chiObservationEvidence(serveHTTP, entryEvidence...),
		),
		newChiDispatchFact(
			"route-context",
			"The request entry obtains a route context from its pool, directly calls reset, and attaches that context to the request. Selected reset observations clear the routes field, URL parameter keys, and the method-not-allowed flag.",
			[]string{"route_context_acquisition"},
			[]string{"route context", "pool acquire", "context reset", "request attach"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilityDataWrite,
				semanticdiscovery.CapabilitySequence,
			},
			chiFunctionSource(reset),
			chiGroup("saved-window", windows["context.go"].EvidenceID),
			append(
				chiObservationEvidence(serveHTTP, contextEvidence...),
				chiObservationEvidence(reset, resetEvidence...)...,
			),
		),
		newChiDispatchFact(
			"computed-handler",
			"The request entry invokes the computed handler both when a parent route context already exists and after attaching a newly obtained context. These local calls do not establish how that handler was assembled.",
			[]string{"computed_handler_invocation"},
			[]string{"computed handler", "parent route context", "handler invocation"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityBranch,
				semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilityLimitation,
				semanticdiscovery.CapabilitySequence,
			},
			chiFunctionSource(serveHTTP),
			chiGroup("saved-window", windows["mux.go"].EvidenceID),
			chiObservationEvidence(serveHTTP, handlerEvidence...),
		),
		newChiDispatchFact(
			"route-lookup",
			"The bounded routing method contains a selector call to FindRoute on the Mux tree field with the route context, method, and route path. The retained node lookup declaration calls its local tree search routine.",
			[]string{"route_lookup"},
			[]string{"route lookup", "tree FindRoute", "route context", "route path"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilityDataRead,
			},
			chiFunctionSource(routeHTTP),
			chiGroup("frontier", routeHTTP.ContentSHA256),
			append(
				chiObservationEvidence(routeHTTP, routeLookupEvidence...),
				chiObservationEvidence(findRoute, findDeclaration...)...,
			),
		),
		newChiDispatchFact(
			"parameter-context-update",
			"On the successful local lookup path, the lookup method appends route parameters and the matched pattern to the route context; the routing method copies parameter values and the route pattern onto the request.",
			[]string{"parameter_context_update"},
			[]string{"route parameters", "matched pattern", "request path values"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityDataWrite,
				semanticdiscovery.CapabilityDataTransformation,
				semanticdiscovery.CapabilitySequence,
			},
			chiFunctionSource(findRoute),
			chiGroup("frontier", findRoute.ContentSHA256),
			append(
				chiObservationEvidence(findRoute, parameterEvidence...),
				chiObservationEvidence(routeHTTP, requestParameterEvidence...)...,
			),
		),
		newChiDispatchFact(
			"endpoint-invocation",
			"Within the non-nil handler branch, the bounded routing method updates request path values and pattern, invokes the selected endpoint handler, and returns from that branch.",
			[]string{"endpoint_invocation"},
			[]string{"selected endpoint handler", "handler branch", "request pattern"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityBranch,
				semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilityOutputEffect,
				semanticdiscovery.CapabilitySequence,
			},
			chiFunctionSource(routeHTTP),
			chiGroup("frontier", routeHTTP.ContentSHA256),
			chiObservationEvidence(routeHTTP, endpointEvidence...),
		),
		newChiDispatchFact(
			"fallback-selection",
			"The routing method selects a method-not-allowed handler for an unsupported method, and after an unmatched lookup it branches between the method-not-allowed handler and the not-found handler.",
			[]string{"not_found_or_method_not_allowed"},
			[]string{"method not allowed", "not found", "fallback selection"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityBranch,
				semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilityOutputEffect,
				semanticdiscovery.CapabilitySequence,
			},
			chiFunctionSource(routeHTTP),
			chiGroup("frontier", routeHTTP.ContentSHA256),
			chiObservationEvidence(routeHTTP, fallbackEvidence...),
		),
		newChiDispatchFact(
			"known-boundary",
			"The bounded facts do not establish the dynamic wiring from the computed handler call to the routing method, actual runtime branch selection, numeric response emission, registration, or complete middleware composition.",
			[]string{"known_unknowns"},
			[]string{"evidence gap", "computed handler wiring", "runtime branch", "numeric response"},
			[]semanticdiscovery.Capability{semanticdiscovery.CapabilityLimitation},
			chiFunctionSource(serveHTTP),
			chiGroup("boundary", serveHTTP.ContentSHA256),
			append(
				chiObservationEvidence(serveHTTP, handlerEvidence...),
				chiObservationEvidence(routeHTTP, routeLookupEvidence...)...,
			),
		),
		newChiDispatchFact(
			"saved-chain-window",
			"A separately saved bounded handler-chain window declares a handler wrapper whose local ServeHTTP method delegates to its stored chain handler; this does not connect a particular computed handler instance to the routing method.",
			nil,
			[]string{"saved chain window", "handler wrapper", "scope limitation"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilityLimitation,
			},
			chiFunctionSource(chainServe),
			chiGroup("saved-window", windows["chain.go"].EvidenceID),
			chiObservationEvidence(chainServe, chainEvidence...),
		),
		muxAnchor,
		treeAnchor,
	}

	answerFacts := facts[:8]
	supportIDs := make([]string, 0, len(answerFacts))
	for _, fact := range answerFacts {
		supportIDs = append(supportIDs, fact.ID)
	}
	enrichmentIDs := []string{facts[8].ID, facts[9].ID, facts[10].ID}
	candidate := semanticdiscovery.OpportunityCandidate{
		Kind:                 semanticdiscovery.ArtifactMechanism,
		Title:                "Chi Request Dispatch",
		QuestionAnswered:     chiDispatchCandidateQuestion,
		SupportIDs:           supportIDs,
		EnrichmentSupportIDs: enrichmentIDs,
		MissingInformation: []string{
			"The dynamic computed-handler wiring is outside the bounded proof.",
			"Numeric response emission and actual runtime branch selection remain uninspected.",
		},
		ExpectedValue: semanticdiscovery.ExpectedValueHigh,
		Confidence:    semanticdiscovery.ConfidenceHigh,
		CapabilityContract: &semanticdiscovery.CapabilityContract{
			RequiredCapabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityEntry,
				semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilityBranch,
				semanticdiscovery.CapabilityDataWrite,
				semanticdiscovery.CapabilitySequence,
				semanticdiscovery.CapabilityLimitation,
				semanticdiscovery.CapabilityLifecycle,
			},
			AvailableCapabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityEntry,
				semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilityBranch,
				semanticdiscovery.CapabilityDataWrite,
				semanticdiscovery.CapabilitySequence,
				semanticdiscovery.CapabilityLimitation,
			},
			MissingCapabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityLifecycle,
			},
			Resolution: semanticdiscovery.CapabilityResolutionPartial,
		},
		IntentContract: &semanticdiscovery.IntentContract{
			RequiredAnswerAspects: chiDispatchAspects(),
			MinCovered:            7,
			MinKeyCovered:         7,
			LocalSearchAliases:    append([]string(nil), chiDispatchAliases...),
		},
	}
	projected := base
	projected.Facts = append(append([]semanticdiscovery.Fact(nil), base.Facts...), facts...)
	normalized, normalization := semanticdiscovery.NormalizeOpportunityProposal(
		projected,
		semanticdiscovery.OpportunityProposal{
			Version:    semanticdiscovery.OpportunityProposalVersion,
			Candidates: []semanticdiscovery.OpportunityCandidate{candidate},
		},
	)
	if len(normalization.Issues) != 0 || len(normalized.Candidates) != 1 {
		return goldenProjection{}, fmt.Errorf(
			"chi request dispatch: locally normalized candidate changed: %#v",
			normalization.Issues,
		)
	}
	candidate = normalized.Candidates[0]
	return goldenProjection{Candidate: candidate, Facts: facts}, nil
}

func chiDispatchAspects() []semanticdiscovery.AnswerAspect {
	return []semanticdiscovery.AnswerAspect{
		{ID: "request_entry", Label: "Where request dispatch enters", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityEntry}, Key: true},
		{ID: "route_context_acquisition", Label: "How the route context is obtained, reset, and attached", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityDirectCall, semanticdiscovery.CapabilityDataWrite, semanticdiscovery.CapabilitySequence}, Key: true},
		{ID: "computed_handler_invocation", Label: "Where the computed handler is invoked", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityDirectCall}, Key: true},
		{ID: "route_lookup", Label: "How the route tree lookup is requested", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityDirectCall}, Key: true},
		{ID: "parameter_context_update", Label: "How matched parameters and pattern update context and request", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityDataWrite, semanticdiscovery.CapabilitySequence}, Key: true},
		{ID: "endpoint_invocation", Label: "How a selected endpoint handler is invoked", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityDirectCall, semanticdiscovery.CapabilitySequence}, Key: true},
		{ID: "not_found_or_method_not_allowed", Label: "How not-found or method-not-allowed fallback is selected", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityBranch, semanticdiscovery.CapabilityDirectCall}, Key: true},
		{ID: "known_unknowns", Label: "Where the bounded dispatch proof stops", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityLimitation}},
	}
}

func buildChiDispatchLeaf(
	bundle semanticdiscovery.Bundle,
	candidate semanticdiscovery.OpportunityCandidate,
) (semanticdiscovery.LeafResult, error) {
	tasks, err := semanticdiscovery.PlanLeafTasks(
		bundle,
		[]semanticdiscovery.OpportunityCandidate{candidate},
	)
	if err != nil {
		return semanticdiscovery.LeafResult{}, err
	}
	if len(tasks) != 1 {
		return semanticdiscovery.LeafResult{}, fmt.Errorf("chi request dispatch: expected one leaf task")
	}
	facts := make(map[string]semanticdiscovery.Fact, len(tasks[0].Facts))
	for _, fact := range tasks[0].Facts {
		facts[fact.ID] = fact
	}
	artifact := semanticdiscovery.LeafArtifact{
		Version:     semanticdiscovery.LeafArtifactVersion,
		TaskID:      tasks[0].ID,
		CandidateID: candidate.ID,
		Status:      semanticdiscovery.LeafStatusUsable,
	}
	used := make([]string, 0, len(candidate.SupportIDs))
	limitationID := ""
	for _, id := range candidate.SupportIDs {
		fact := facts[id]
		if slicesContains(fact.Keywords, "answer_aspect:known_unknowns") {
			limitationID = id
			continue
		}
		artifact.Observations = append(
			artifact.Observations,
			semanticdiscovery.LeafObservation{Text: fact.Statement, SupportIDs: []string{id}},
		)
		used = append(used, id)
	}
	if limitationID == "" {
		return semanticdiscovery.LeafResult{}, fmt.Errorf("chi request dispatch: boundary fact is unavailable")
	}
	artifact.MissingEvidence = []semanticdiscovery.LeafMissingEvidence{{
		Explanation:         "The bounded facts do not establish dynamic computed-handler wiring, actual runtime branch selection, numeric response emission, registration, or complete middleware lifecycle.",
		SupportIDs:          []string{limitationID},
		MissingCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityLifecycle},
	}}
	used = append(used, limitationID)
	artifact.CandidateConnection = semanticdiscovery.LeafCandidateConnection{
		CandidateID: candidate.ID,
		Relation:    "needs_combination",
		Explanation: "The independently verified request-entry, context, lookup, endpoint, and fallback observations require one bounded editorial synthesis.",
		SupportIDs:  sortedGoldenStrings(used),
	}
	artifact = semanticdiscovery.NormalizeLeafArtifact(artifact)
	if err := semanticdiscovery.ValidateLeafArtifact(tasks[0], artifact); err != nil {
		return semanticdiscovery.LeafResult{}, err
	}
	return semanticdiscovery.LeafResult{Task: tasks[0], Artifact: artifact}, nil
}

type chiObservationSelector struct {
	kind      sourcewindowfacts.ObservationKind
	object    string
	target    string
	contains  string
	lineAfter int
}

func chiObservations(
	function sourcewindowfacts.Function,
	selectors ...chiObservationSelector,
) ([]sourcewindowfacts.Observation, error) {
	result := make([]sourcewindowfacts.Observation, 0, len(selectors))
	for _, selector := range selectors {
		found := false
		for _, observation := range function.Observations {
			content := strings.Join([]string{observation.Object, observation.Target, observation.Value}, " ")
			if observation.Kind != selector.kind ||
				(selector.object != "" && observation.Object != selector.object) ||
				(selector.target != "" && observation.Target != selector.target) ||
				(selector.contains != "" && !strings.Contains(content, selector.contains)) ||
				(selector.lineAfter != 0 && observation.Line <= selector.lineAfter) {
				continue
			}
			result = append(result, observation)
			found = true
			break
		}
		if !found {
			return nil, fmt.Errorf(
				"chi request dispatch: %s lacks required %s observation (%s %s)",
				function.Symbol, selector.kind, selector.object, selector.target,
			)
		}
	}
	return result, nil
}

func chiWindowEvidence(
	window sourcewindowfacts.Window,
	needles ...string,
) ([]semanticdiscovery.EvidenceRef, error) {
	result := make([]semanticdiscovery.EvidenceRef, 0, len(needles))
	for _, needle := range needles {
		line := 0
		for index, text := range window.Lines {
			if strings.Contains(text, needle) {
				if line != 0 {
					return nil, fmt.Errorf("chi request dispatch: saved evidence %q is ambiguous", needle)
				}
				line = window.StartLine + index
			}
		}
		if line == 0 {
			return nil, fmt.Errorf("chi request dispatch: saved evidence %q is unavailable", needle)
		}
		result = append(result, semanticdiscovery.EvidenceRef{
			ID:   goldenStableID("swfe", window.EvidenceID, needle, fmt.Sprint(line)),
			Kind: "saved_source_window", Label: needle, Path: window.Path, Line: line,
		})
	}
	return result, nil
}

func chiObservationEvidence(
	function sourcewindowfacts.Function,
	observations ...sourcewindowfacts.Observation,
) []semanticdiscovery.EvidenceRef {
	result := make([]semanticdiscovery.EvidenceRef, 0, len(observations))
	for _, observation := range observations {
		result = append(result, semanticdiscovery.EvidenceRef{
			ID:     observation.ID,
			Kind:   "bounded_go_window",
			Label:  string(observation.Kind),
			Path:   function.Path,
			Line:   observation.Line,
			Column: observation.Column,
		})
	}
	return result
}

func chiFunctionSource(function sourcewindowfacts.Function) *semanticdiscovery.FactSource {
	return &semanticdiscovery.FactSource{
		Path:            function.Path,
		StartLine:       function.StartLine,
		EndLine:         function.EndLine,
		EnclosingSymbol: function.Symbol,
		ContentSHA256:   function.ContentSHA256,
	}
}

func newChiDispatchFact(
	key string,
	statement string,
	aspects []string,
	keywords []string,
	capabilities []semanticdiscovery.Capability,
	source *semanticdiscovery.FactSource,
	sourceGroup string,
	evidenceRefs []semanticdiscovery.EvidenceRef,
) semanticdiscovery.Fact {
	keywords = append([]string(nil), keywords...)
	for _, aspect := range aspects {
		keywords = append(keywords, "answer_aspect:"+aspect)
	}
	keywords = sortedGoldenStrings(keywords)
	capabilities = append([]semanticdiscovery.Capability(nil), capabilities...)
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i] < capabilities[j] })
	sort.Slice(evidenceRefs, func(i, j int) bool {
		if evidenceRefs[i].Path != evidenceRefs[j].Path {
			return evidenceRefs[i].Path < evidenceRefs[j].Path
		}
		if evidenceRefs[i].Line != evidenceRefs[j].Line {
			return evidenceRefs[i].Line < evidenceRefs[j].Line
		}
		return evidenceRefs[i].ID < evidenceRefs[j].ID
	})
	identity := []string{chiDispatchIntentKey, key, statement, source.ContentSHA256}
	for _, reference := range evidenceRefs {
		identity = append(identity, reference.ID)
	}
	return semanticdiscovery.Fact{
		ID:           goldenStableID("swf", identity...),
		Kind:         semanticdiscovery.FactSourceSignal,
		Statement:    statement,
		Keywords:     keywords,
		SourceGroup:  sourceGroup,
		Capabilities: capabilities,
		Scope:        semanticdiscovery.FactScopeLocal,
		Source:       source,
		Evidence:     evidenceRefs,
	}
}

func chiGroup(kind, identity string) string {
	return goldenStableID("swg", chiDispatchIntentKey, kind, identity)
}

func slicesContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
