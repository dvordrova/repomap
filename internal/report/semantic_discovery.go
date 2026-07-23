package report

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	maxSemanticDiscoveryFacts             = 192
	maxSemanticDiscoveryComponents        = 32
	maxSemanticDiscoveryFlows             = 16
	maxSemanticDiscoveryFlowFacts         = 72
	maxSemanticDiscoverySurfaces          = 32
	maxSemanticDiscoveryImportEdges       = 32
	maxSemanticDiscoverySourceSignals     = 48
	maxSemanticDiscoveryExternalImports   = 50
	maxSemanticDiscoveryResearchFacts     = 24
	maxSemanticDiscoveryWarnings          = 24
	maxSemanticDiscoveryPlannerContext    = 64
	maxSemanticDiscoverySupplementalFacts = 16
	maxSemanticDiscoveryEvidencePerFact   = 8
	maxSemanticDiscoveryStatementBytes    = 1800
	maxSemanticDiscoveryEvidenceLabelByte = 256
	maxSavedSemanticDiscoveryRecordBytes  = 512 << 10
)

const semanticDiscoveryRecordFile = semanticdiscovery.RecordFile

// BuildSemanticDiscoveryBundle projects only facts already present in a
// coherent report. It performs no repository I/O and does not infer new
// relations between report objects.
func BuildSemanticDiscoveryBundle(data *ReportData) (semanticdiscovery.Bundle, error) {
	if data == nil {
		return semanticdiscovery.Bundle{}, fmt.Errorf("semantic discovery: report data is required")
	}
	builder := newSemanticFactBuilder(data)
	builder.addPurpose()
	builder.addComponents()
	builder.addFlows()
	builder.addSurfaces()
	builder.addImportEdges()
	builder.addGuidedTour()
	builder.addDomainTerms()
	builder.addSourceSignals()
	builder.addResearchFacts()
	builder.addWarningsAndUnknowns()

	supplemental, err := semanticSupplementalFacts(data)
	if err != nil {
		return semanticdiscovery.Bundle{}, err
	}
	facts := make([]semanticdiscovery.Fact, 0, len(builder.facts)+len(supplemental))
	for _, fact := range builder.facts {
		facts = append(facts, fact)
	}
	baseLimit := maxSemanticDiscoveryFacts - len(supplemental)
	facts = selectSemanticDiscoveryFacts(facts, baseLimit)
	facts = append(facts, supplemental...)
	sort.Slice(facts, func(i, j int) bool {
		if facts[i].Kind != facts[j].Kind {
			return facts[i].Kind < facts[j].Kind
		}
		return facts[i].ID < facts[j].ID
	})
	plannerContext := append([]semanticdiscovery.PlannerContext(nil), builder.plannerContext...)
	sort.Slice(plannerContext, func(i, j int) bool {
		if plannerContext[i].Kind != plannerContext[j].Kind {
			return plannerContext[i].Kind < plannerContext[j].Kind
		}
		return plannerContext[i].Text < plannerContext[j].Text
	})
	bundle := semanticdiscovery.Bundle{
		Version:        semanticdiscovery.BundleVersion,
		RepoName:       semanticDiscoveryText(data.RepoName, 256),
		PlannerContext: plannerContext,
		Facts:          facts,
	}
	if _, _, err := semanticdiscovery.BundleHash(bundle); err != nil {
		return semanticdiscovery.Bundle{}, err
	}
	return bundle, nil
}

func selectSemanticDiscoveryFacts(
	facts []semanticdiscovery.Fact,
	limit int,
) []semanticdiscovery.Fact {
	selected := append([]semanticdiscovery.Fact(nil), facts...)
	sort.SliceStable(selected, func(i, j int) bool {
		left := semanticFactSelectionPriority(selected[i])
		right := semanticFactSelectionPriority(selected[j])
		if left != right {
			return left > right
		}
		if selected[i].Kind != selected[j].Kind {
			return selected[i].Kind < selected[j].Kind
		}
		return selected[i].ID < selected[j].ID
	})
	if limit < 0 {
		limit = 0
	}
	if len(selected) > limit {
		selected = selected[:limit]
	}
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].Kind != selected[j].Kind {
			return selected[i].Kind < selected[j].Kind
		}
		return selected[i].ID < selected[j].ID
	})
	return selected
}

func semanticFactSelectionPriority(fact semanticdiscovery.Fact) int {
	priority := 500
	if role, ok := semanticFactRole(fact); ok {
		priority = artifactrole.SelectionPriority(role) * 10
	}
	for _, capability := range fact.Capabilities {
		switch capability {
		case semanticdiscovery.CapabilityEntry:
			priority += 180
		case semanticdiscovery.CapabilityOutputEffect, semanticdiscovery.CapabilityDataWrite:
			priority += 170
		case semanticdiscovery.CapabilityDirectCall, semanticdiscovery.CapabilitySequence:
			priority += 140
		case semanticdiscovery.CapabilityBehavior, semanticdiscovery.CapabilityDataTransformation:
			priority += 100
		case semanticdiscovery.CapabilityDataRead, semanticdiscovery.CapabilityLifecycle:
			priority += 80
		case semanticdiscovery.CapabilityTestEvidence:
			priority -= 80
		}
	}
	switch fact.Kind {
	case semanticdiscovery.FactFlowStep:
		priority += 140
	case semanticdiscovery.FactRuntimeSurface:
		priority += 120
	case semanticdiscovery.FactSourceSignal:
		priority += 100
	case semanticdiscovery.FactPackageImport:
		priority += 70
	case semanticdiscovery.FactTestReference:
		priority -= 120
	}
	if len(fact.Evidence) > 0 {
		priority += 40
	}
	return priority
}

func semanticFactRole(fact semanticdiscovery.Fact) (artifactrole.Role, bool) {
	var best artifactrole.Role
	bestPriority := -1
	for _, reference := range fact.Evidence {
		if strings.TrimSpace(reference.Path) == "" {
			continue
		}
		role := artifactrole.Classify(reference.Path, artifactrole.Hints{
			Test: fact.Kind == semanticdiscovery.FactTestReference,
		})
		priority := artifactrole.SelectionPriority(role)
		if priority > bestPriority {
			best = role
			bestPriority = priority
		}
	}
	return best, bestPriority >= 0
}

// semanticSupplementalFacts attaches current report navigation to already
// validated local probe facts. A path is focused only when the existing
// architecture model gives it one unambiguous owner; ambiguous evidence stays
// evidence-only instead of inventing a canvas relationship.
func semanticSupplementalFacts(data *ReportData) ([]semanticdiscovery.Fact, error) {
	if data == nil || len(data.SemanticSupplementalFacts) == 0 {
		return nil, nil
	}
	if len(data.SemanticSupplementalFacts) > maxSemanticDiscoverySupplementalFacts {
		return nil, fmt.Errorf(
			"semantic discovery: supplemental fact count exceeds %d",
			maxSemanticDiscoverySupplementalFacts,
		)
	}
	owners := architectureOwnershipIndex{}
	if data.ArchitectureCanvas != nil {
		owners = buildArchitectureOwnershipIndex(*data.ArchitectureCanvas, data.RepositoryGraph)
	}
	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, path := range data.OpenablePaths {
		if path = strings.TrimSpace(path); path != "" {
			openable[path] = struct{}{}
		}
	}
	result := make([]semanticdiscovery.Fact, 0, len(data.SemanticSupplementalFacts))
	seen := make(map[string]struct{}, len(data.SemanticSupplementalFacts))
	for _, original := range data.SemanticSupplementalFacts {
		fact := original
		if _, duplicate := seen[fact.ID]; duplicate {
			return nil, fmt.Errorf("semantic discovery: duplicate supplemental fact id %q", fact.ID)
		}
		seen[fact.ID] = struct{}{}
		componentIDs := append([]string(nil), fact.Focus.ComponentIDs...)
		for _, reference := range fact.Evidence {
			if _, allowed := openable[reference.Path]; !allowed {
				return nil, fmt.Errorf(
					"semantic discovery: supplemental evidence path %q is not openable",
					reference.Path,
				)
			}
			pathOwners := ownersForPath(owners.pathOwners, reference.Path)
			if len(pathOwners) != 1 {
				continue
			}
			for componentID := range pathOwners {
				componentIDs = append(componentIDs, string(componentID))
			}
		}
		fact.Focus.ComponentIDs = semanticNonEmptyStrings(componentIDs)
		result = append(result, fact)
	}
	return result, nil
}

func replaySavedSemanticArtifacts(data *ReportData, path string) string {
	if data == nil {
		return "semantic discovery unavailable: report data is required"
	}
	data.SemanticArtifacts = nil
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		return "semantic discovery unavailable: cannot inspect saved record"
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSavedSemanticDiscoveryRecordBytes {
		return "semantic discovery unavailable: saved record is not a bounded regular file"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "semantic discovery unavailable: cannot read saved record"
	}
	bundle, err := BuildSemanticDiscoveryBundle(data)
	if err != nil {
		return fmt.Sprintf("semantic discovery unavailable: current saved facts are invalid: %v", err)
	}
	artifacts, err := semanticdiscovery.ReplayRecord(bundle, raw)
	if err != nil {
		return fmt.Sprintf("semantic discovery unavailable: saved record is stale or invalid: %v", err)
	}
	record, err := semanticdiscovery.DecodeRecord(raw)
	if err != nil {
		return fmt.Sprintf("semantic discovery unavailable: saved record is invalid: %v", err)
	}
	data.SemanticArtifacts = artifacts
	// Keep these funnel counts private until an independently replayed
	// canonical Mechanism exists. A base semantic record alone must not add a
	// publication summary to an otherwise unchanged report.
	data.semanticAttempted = len(record.Opportunity.Candidates)
	data.semanticInvestigated = len(record.SelectedCandidateIDs)
	data.SemanticCoverage = nil
	return ""
}

// replaySavedGoldenMechanism layers a bounded separately replayed, locally
// enriched candidate set onto the general semantic record. A missing, stale,
// or invalid golden pair never removes artifacts already replayed from the
// base bundle.
func replaySavedGoldenMechanism(
	data *ReportData,
	factsPath string,
	recordPath string,
) string {
	if data == nil {
		return "golden mechanism unavailable: report data is required"
	}
	baseArtifacts := append([]semanticdiscovery.Artifact(nil), data.SemanticArtifacts...)
	previousSupplement := data.SemanticSupplementalFacts
	record, warning := loadSemanticSupplementRecord(data, factsPath)
	if warning != "" {
		data.SemanticSupplementalFacts = previousSupplement
		data.SemanticArtifacts = baseArtifacts
		return warning
	}
	if record.Version == 0 {
		data.SemanticSupplementalFacts = previousSupplement
		return ""
	}
	boundCandidateIDs, err := semanticSupplementCandidateIDs(record)
	if err != nil {
		data.SemanticSupplementalFacts = previousSupplement
		data.SemanticArtifacts = baseArtifacts
		return "golden mechanism unavailable: saved candidate bindings are invalid"
	}
	info, err := os.Lstat(recordPath)
	if err != nil {
		data.SemanticSupplementalFacts = previousSupplement
		if os.IsNotExist(err) {
			return "golden mechanism unavailable: saved facts have no replay record"
		}
		return "golden mechanism unavailable: cannot inspect saved replay record"
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSavedSemanticDiscoveryRecordBytes {
		data.SemanticSupplementalFacts = previousSupplement
		return "golden mechanism unavailable: saved replay is not a bounded regular file"
	}
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		data.SemanticSupplementalFacts = previousSupplement
		return "golden mechanism unavailable: cannot read saved replay record"
	}
	bundle, err := BuildSemanticDiscoveryBundle(data)
	if err != nil {
		data.SemanticSupplementalFacts = previousSupplement
		return "golden mechanism unavailable: enriched facts cannot be bundled"
	}
	artifacts, err := semanticdiscovery.ReplayRecord(bundle, raw)
	if err != nil {
		data.SemanticSupplementalFacts = previousSupplement
		return fmt.Sprintf("golden mechanism unavailable: saved replay is stale or invalid: %v", err)
	}
	boundCandidates := make(map[string]struct{}, len(boundCandidateIDs))
	for _, candidateID := range boundCandidateIDs {
		boundCandidates[candidateID] = struct{}{}
	}
	replayedCandidates := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		if _, bound := boundCandidates[artifact.CandidateID]; !bound {
			data.SemanticSupplementalFacts = previousSupplement
			data.SemanticArtifacts = baseArtifacts
			return "golden mechanism unavailable: saved replay does not match the bound candidate set"
		}
		if _, duplicate := replayedCandidates[artifact.CandidateID]; duplicate {
			data.SemanticSupplementalFacts = previousSupplement
			data.SemanticArtifacts = baseArtifacts
			return "golden mechanism unavailable: saved replay does not match the bound candidate set"
		}
		replayedCandidates[artifact.CandidateID] = struct{}{}
	}
	if len(replayedCandidates) != len(boundCandidates) {
		data.SemanticSupplementalFacts = previousSupplement
		data.SemanticArtifacts = baseArtifacts
		return "golden mechanism unavailable: saved replay does not match the bound candidate set"
	}
	merged := make([]semanticdiscovery.Artifact, 0, len(baseArtifacts)+len(artifacts))
	for _, artifact := range baseArtifacts {
		if _, replaced := boundCandidates[artifact.CandidateID]; !replaced {
			merged = append(merged, artifact)
		}
	}
	merged = append(merged, artifacts...)
	sort.Slice(merged, func(i, j int) bool { return merged[i].ID < merged[j].ID })
	data.SemanticArtifacts = merged
	return ""
}

type semanticFactBuilder struct {
	data           *ReportData
	facts          map[string]semanticdiscovery.Fact
	plannerContext []semanticdiscovery.PlannerContext
	contextSeen    map[string]struct{}
	openablePaths  map[string]struct{}
	flowFactCount  int
}

func newSemanticFactBuilder(data *ReportData) *semanticFactBuilder {
	openablePaths := make(map[string]struct{}, len(data.OpenablePaths))
	for _, path := range data.OpenablePaths {
		path = strings.TrimSpace(path)
		if path != "" {
			openablePaths[path] = struct{}{}
		}
	}
	return &semanticFactBuilder{
		data:          data,
		facts:         make(map[string]semanticdiscovery.Fact),
		contextSeen:   make(map[string]struct{}),
		openablePaths: openablePaths,
	}
}

func (builder *semanticFactBuilder) addPurpose() {
	if guess := strings.TrimSpace(builder.data.ProjectGuess); guess != "" {
		builder.addContext(
			semanticdiscovery.PlannerContextOrientation,
			"Existing orientation describes the repository as: "+guess,
		)
	}
}

func (builder *semanticFactBuilder) addComponents() {
	canvas := builder.data.ArchitectureCanvas
	if canvas == nil {
		return
	}
	for index, component := range canvas.Components {
		if index >= maxSemanticDiscoveryComponents {
			break
		}
		text := fmt.Sprintf("Existing conceptual component %q", component.Name)
		if description := strings.TrimSpace(component.Description); description != "" {
			text += ": " + description
		}
		builder.addContext(semanticdiscovery.PlannerContextComponent, text)
	}
}

func (builder *semanticFactBuilder) addFlows() {
	canvas := builder.data.ArchitectureCanvas
	if canvas == nil {
		return
	}
	for flowIndex, flow := range canvas.Flows {
		if flowIndex >= maxSemanticDiscoveryFlows || builder.flowFactCount >= maxSemanticDiscoveryFlowFacts {
			break
		}
		focus := semanticdiscovery.Focus{
			FlowIDs:      []string{string(flow.ID)},
			ComponentIDs: componentIDStrings(flow.ParticipatingComponentIDs),
			SurfaceIDs:   semanticNonEmptyStrings([]string{flow.StartSurfaceID}),
		}
		detail := firstSemanticSearchText(flow.MentalModel, flow.Goal, flow.WhyInspect, flow.FrontierSummary)
		context := fmt.Sprintf("Existing saved flow %q", flow.Name)
		if detail != "" {
			context += " and is described as: " + detail
		}
		builder.addContext(semanticdiscovery.PlannerContextFlow, context)

		stepByID := make(map[string]ArchitectureFlowStep, len(flow.Steps))
		for stepIndex, step := range flow.Steps {
			if builder.flowFactCount >= maxSemanticDiscoveryFlowFacts {
				break
			}
			stepByID[step.ID] = step
			stepFocus := semanticdiscovery.Focus{FlowIDs: []string{string(flow.ID)}}
			if step.ComponentID != "" {
				stepFocus.ComponentIDs = []string{string(step.ComponentID)}
			}
			stepName := firstSemanticSearchText(step.QualifiedName, step.Label)
			statement := fmt.Sprintf("Saved exact flow-proof step %d identifies %q.", stepIndex+1, stepName)
			evidenceRefs := []semanticdiscovery.EvidenceRef{
				builder.evidence("flow_step", step.Label, step.Location),
			}
			builder.add(
				semanticdiscovery.FactFlowStep,
				"flow-step:"+string(flow.ID)+":"+step.ID,
				statement,
				[]semanticdiscovery.Capability{semanticdiscovery.CapabilityBehavior, semanticdiscovery.CapabilitySequence},
				semanticdiscovery.FactScopeFlow,
				stepFocus,
				evidenceRefs,
				[]string{"flow proof step", step.Label, step.QualifiedName},
			)
			builder.flowFactCount++
		}
		for _, edge := range canvas.FlowEdges {
			if edge.FlowID != flow.ID || builder.flowFactCount >= maxSemanticDiscoveryFlowFacts {
				continue
			}
			from, fromOK := stepByID[edge.From]
			to, toOK := stepByID[edge.To]
			if !fromOK || !toOK {
				continue
			}
			location := edge.Evidence
			builder.add(
				semanticdiscovery.FactFlowStep,
				"flow-edge:"+string(flow.ID)+":"+edge.ID,
				fmt.Sprintf(
					"A saved exact flow-proof %s transition links %q to %q.",
					edge.Relation,
					firstSemanticSearchText(from.QualifiedName, from.Label),
					firstSemanticSearchText(to.QualifiedName, to.Label),
				),
				[]semanticdiscovery.Capability{semanticdiscovery.CapabilityBehavior, semanticdiscovery.CapabilitySequence},
				semanticdiscovery.FactScopeFlow,
				focus,
				[]semanticdiscovery.EvidenceRef{builder.evidence("flow_transition", string(edge.Relation), &location)},
				[]string{"flow transition", string(edge.Relation)},
			)
			builder.flowFactCount++
		}
	}

	for _, flow := range builder.data.Flows {
		flowFocus := semanticdiscovery.Focus{FlowIDs: semanticNonEmptyStrings([]string{flow.ID})}
		if builder.flowFactCount < maxSemanticDiscoveryFlowFacts {
			statement := fmt.Sprintf(
				"A saved bounded neighborhood contains %d files, %d tests, %d packages, and %d import edges; it does not establish runtime order.",
				flow.BundleSummary.SelectedFilesCount,
				flow.BundleSummary.SelectedTestsCount,
				flow.BundleSummary.SelectedPkgsCount,
				flow.BundleSummary.RelatedEdgesCount,
			)
			builder.add(
				semanticdiscovery.FactFlow,
				"flow-neighborhood:"+flow.ID,
				statement,
				[]semanticdiscovery.Capability{
					semanticdiscovery.CapabilityStatic,
					semanticdiscovery.CapabilityLimitation,
				},
				semanticdiscovery.FactScopeFlow,
				semanticdiscovery.Focus{},
				nil,
				[]string{"bounded neighborhood"},
			)
			builder.flowFactCount++
		}
		for testIndex, item := range semanticFlowFileItems(flow.TestsToRead, flow.BundleTests) {
			if testIndex >= 12 || builder.flowFactCount >= maxSemanticDiscoveryFlowFacts {
				break
			}
			location := &evidence.Location{Path: item.Path}
			statement := "A saved bounded neighborhood includes a locally allowlisted test reference."
			builder.add(
				semanticdiscovery.FactTestReference,
				"flow-test:"+flow.ID+":"+item.Path,
				statement,
				[]semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic},
				semanticdiscovery.FactScopeFlow,
				flowFocus,
				[]semanticdiscovery.EvidenceRef{builder.evidence("test_reference", item.Path, location)},
				[]string{"test", "flow"},
			)
			builder.flowFactCount++
		}
		for docIndex, item := range semanticFlowFileItems(flow.BundleDocs) {
			if docIndex >= 4 || builder.flowFactCount >= maxSemanticDiscoveryFlowFacts {
				break
			}
			location := &evidence.Location{Path: item.Path}
			statement := "A saved bounded neighborhood includes a locally allowlisted documentation reference."
			builder.add(
				semanticdiscovery.FactSourceSignal,
				"flow-doc:"+flow.ID+":"+item.Path,
				statement,
				[]semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic},
				semanticdiscovery.FactScopeFlow,
				flowFocus,
				[]semanticdiscovery.EvidenceRef{builder.evidence("documentation_reference", item.Path, location)},
				[]string{"documentation", "flow"},
			)
			builder.flowFactCount++
		}
		for packageIndex, packagePath := range semanticNonEmptyStrings(flow.BundlePackages) {
			if packageIndex >= 8 || builder.flowFactCount >= maxSemanticDiscoveryFlowFacts {
				break
			}
			builder.add(
				semanticdiscovery.FactPackageImport,
				"flow-package:"+flow.ID+":"+packagePath,
				fmt.Sprintf("A saved bounded neighborhood includes package %q.", packagePath),
				[]semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic},
				semanticdiscovery.FactScopeFlow,
				flowFocus,
				nil,
				[]string{"flow package", packagePath},
			)
			builder.flowFactCount++
		}
		edges := append([]EdgeInfo(nil), flow.BundleEdges...)
		sort.Slice(edges, func(i, j int) bool {
			if edges[i].From != edges[j].From {
				return edges[i].From < edges[j].From
			}
			return edges[i].To < edges[j].To
		})
		for edgeIndex, edge := range edges {
			if edgeIndex >= 12 || builder.flowFactCount >= maxSemanticDiscoveryFlowFacts {
				break
			}
			builder.add(
				semanticdiscovery.FactPackageImport,
				"flow-edge:"+flow.ID+":"+edge.From+":"+edge.To,
				fmt.Sprintf(
					"A saved bounded neighborhood contains a static import edge from %q to %q; this does not establish runtime order.",
					edge.From,
					edge.To,
				),
				[]semanticdiscovery.Capability{
					semanticdiscovery.CapabilityStatic,
					semanticdiscovery.CapabilityLimitation,
				},
				semanticdiscovery.FactScopeFlow,
				flowFocus,
				nil,
				[]string{"flow import edge", edge.From, edge.To},
			)
			builder.flowFactCount++
		}
	}
}

func (builder *semanticFactBuilder) addSurfaces() {
	canvas := builder.data.ArchitectureCanvas
	if canvas == nil {
		return
	}
	for index, surface := range canvas.Surfaces {
		if index >= maxSemanticDiscoverySurfaces {
			break
		}
		if surface.Source != surfaceSourceCatalog {
			builder.addContext(
				semanticdiscovery.PlannerContextFlow,
				fmt.Sprintf("Existing saved-trace start surface is described as %q", surface.Name),
			)
			continue
		}
		statement := fmt.Sprintf("Saved runtime surface %q is a %s in category %s", surface.Name, surface.Kind, surface.Category)
		if surface.Status != "" {
			statement += " with status " + surface.Status
		}
		if surface.TraceUnavailableReason != "" {
			statement += "; trace limitation: " + surface.TraceUnavailableReason
		}
		evidenceRefs := make([]semanticdiscovery.EvidenceRef, 0, len(surface.Evidence))
		for _, item := range surface.Evidence {
			location := evidence.Location{Path: item.Path, Line: item.Line, Column: item.Column}
			evidenceRefs = append(evidenceRefs, builder.evidence("runtime_surface", surface.Name, &location))
		}
		componentIDs := componentIDStrings(surface.ParticipatingComponentIDs)
		if surface.OwningComponentID != "" {
			componentIDs = append(componentIDs, string(surface.OwningComponentID))
		}
		builder.add(
			semanticdiscovery.FactRuntimeSurface,
			"surface:"+surface.ID,
			statement,
			[]semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic},
			semanticdiscovery.FactScopeComponent,
			semanticdiscovery.Focus{
				ComponentIDs: componentIDs,
				FlowIDs:      semanticNonEmptyStrings([]string{string(surface.RelatedTraceID)}),
				SurfaceIDs:   []string{surface.ID},
			},
			evidenceRefs,
			[]string{"runtime surface", surface.Kind, surface.Category, surface.Name},
		)
	}
}

func (builder *semanticFactBuilder) addImportEdges() {
	if builder.data.RepositoryGraph != nil {
		for index, edge := range builder.data.RepositoryGraph.PackageEdges {
			if index >= maxSemanticDiscoveryImportEdges {
				break
			}
			builder.add(
				semanticdiscovery.FactPackageImport,
				"import:"+edge.From+":"+edge.To,
				fmt.Sprintf("Saved bounded package-import fact: %s imports %s.", edge.From, edge.To),
				[]semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic},
				semanticdiscovery.FactScopeRepository,
				semanticdiscovery.Focus{},
				nil,
				[]string{"package import", edge.From, edge.To},
			)
		}
	}
	externalImports := append([]externalImportUsage(nil), builder.data.externalImports...)
	sort.Slice(externalImports, func(i, j int) bool {
		if externalImports[i].UsedByCount != externalImports[j].UsedByCount {
			return externalImports[i].UsedByCount > externalImports[j].UsedByCount
		}
		return externalImports[i].ImportPath < externalImports[j].ImportPath
	})
	for index, item := range externalImports {
		if index >= maxSemanticDiscoveryExternalImports {
			break
		}
		builder.add(
			semanticdiscovery.FactDependency,
			"external-import:"+item.ImportPath,
			fmt.Sprintf(
				"Saved dependency aggregate lists %q as imported by %d package(s); package ownership, API usage, and runtime role are not preserved.",
				item.ImportPath,
				item.UsedByCount,
			),
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic,
				semanticdiscovery.CapabilityLimitation,
			},
			semanticdiscovery.FactScopeRepository,
			semanticdiscovery.Focus{},
			nil,
			[]string{"external dependency", item.ImportPath},
		)
	}
}

func (builder *semanticFactBuilder) addGuidedTour() {
	story := builder.data.GuidedTour
	if story == nil {
		return
	}
	for index, step := range story.Steps {
		if index >= 12 {
			break
		}
		statement := fmt.Sprintf("Existing editorial guided step %d in %q is %q", index+1, story.Title, step.Title)
		if step.Explanation != "" {
			statement += ": " + step.Explanation
		}
		builder.addContext(semanticdiscovery.PlannerContextGuidedTour, statement)
	}
	for _, summary := range story.GapSummary {
		for _, gap := range summary.Gaps {
			builder.addContext(
				semanticdiscovery.PlannerContextLimitation,
				"Existing guided-story gap: "+firstSemanticSearchText(gap.Label, gap.Detail, summary.Explanation),
			)
		}
	}
}

func (builder *semanticFactBuilder) addDomainTerms() {
	for index, term := range builder.data.ImportantDomainWords {
		if index >= 16 {
			break
		}
		builder.addContext(
			semanticdiscovery.PlannerContextVocabulary,
			fmt.Sprintf("Existing orientation vocabulary describes %q as %s", term.Word, term.Guess),
		)
	}
}

func (builder *semanticFactBuilder) addSourceSignals() {
	signals := append([]SourceSignal(nil), builder.data.sourceSignals...)
	for _, flow := range builder.data.Flows {
		signals = append(signals, flow.bundleSignals...)
	}
	sort.Slice(signals, func(i, j int) bool {
		if signals[i].Path != signals[j].Path {
			return signals[i].Path < signals[j].Path
		}
		if signals[i].Line != signals[j].Line {
			return signals[i].Line < signals[j].Line
		}
		return signals[i].Category < signals[j].Category
	})
	seen := make(map[string]struct{}, len(signals))
	kept := 0
	for _, signal := range signals {
		if _, allowed := builder.openablePaths[signal.Path]; !allowed {
			continue
		}
		key := fmt.Sprintf("%s:%d:%s", signal.Path, signal.Line, signal.Category)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		index := kept
		if index >= maxSemanticDiscoverySourceSignals {
			break
		}
		kept++
		location := &evidence.Location{Path: signal.Path, Line: signal.Line}
		statement := "Saved bounded local source signal"
		if signal.Category != "" {
			statement += " categorized as " + signal.Category
		}
		if signal.Reason != "" {
			statement += ": " + signal.Reason
		}
		if signal.Snippet != "" {
			statement += ". Bounded excerpt: " + semanticDiscoveryText(signal.Snippet, 360)
		}
		builder.add(
			semanticdiscovery.FactSourceSignal,
			fmt.Sprintf("source-signal:%s:%d:%s", signal.Path, signal.Line, signal.Category),
			statement,
			semanticSourceSignalCapabilities(signal),
			semanticdiscovery.FactScopeLocal,
			semanticdiscovery.Focus{},
			[]semanticdiscovery.EvidenceRef{builder.evidence("source_signal", signal.Category, location)},
			[]string{"source signal", signal.Category},
		)
	}
}

func semanticSourceSignalCapabilities(signal SourceSignal) []semanticdiscovery.Capability {
	capabilities := []semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic}
	reason := strings.ToLower(signal.Reason)
	snippet := strings.ToLower(signal.Snippet)
	switch signal.Category {
	case "background_loop":
		if strings.Contains(reason, "goroutine") || strings.Contains(reason, "cancellation") ||
			strings.Contains(snippet, "go func") || strings.Contains(snippet, "ctx.done") {
			capabilities = append(capabilities, semanticdiscovery.CapabilityBehavior)
		}
	case "request_handler":
		if strings.Contains(reason, "registration call") || strings.Contains(snippet, "handlefunc") {
			capabilities = append(capabilities, semanticdiscovery.CapabilityBehavior)
		}
	case "observability":
		if strings.Contains(snippet, "=") {
			capabilities = append(capabilities, semanticdiscovery.CapabilityBehavior)
		}
	case "threshold_limit":
		if strings.Contains(snippet, "return") {
			capabilities = append(
				capabilities,
				semanticdiscovery.CapabilityBehavior,
				semanticdiscovery.CapabilityLimitation,
			)
		}
	}
	return capabilities
}

func (builder *semanticFactBuilder) addResearchFacts() {
	if builder.data.ModelResearch == nil {
		return
	}
	for index, fact := range builder.data.ModelResearch.Theory.GroundedFacts {
		if index >= maxSemanticDiscoveryResearchFacts {
			break
		}
		builder.addContext(
			semanticdiscovery.PlannerContextResearch,
			"Existing model-research interpretation: "+fact.Statement,
		)
	}
	for _, frontier := range builder.data.ModelResearch.Theory.UnresolvedFrontiers {
		builder.addContext(
			semanticdiscovery.PlannerContextLimitation,
			"Existing unresolved research frontier: "+firstSemanticSearchText(frontier.Question, frontier.Reason),
		)
	}
}

func (builder *semanticFactBuilder) addWarningsAndUnknowns() {
	count := 0
	for _, warning := range builder.data.Warnings {
		if count >= maxSemanticDiscoveryWarnings {
			return
		}
		builder.addContext(
			semanticdiscovery.PlannerContextLimitation,
			"Existing report warning: "+warning,
		)
		count++
	}
	for _, flow := range builder.data.Flows {
		for _, unknown := range flow.Unknowns {
			if count >= maxSemanticDiscoveryWarnings {
				return
			}
			builder.addContext(
				semanticdiscovery.PlannerContextLimitation,
				fmt.Sprintf("Existing unknown for flow %q: %s", flow.Name, unknown),
			)
			count++
		}
	}
}

func (builder *semanticFactBuilder) add(
	kind semanticdiscovery.FactKind,
	sourceKey string,
	statement string,
	capabilities []semanticdiscovery.Capability,
	scope semanticdiscovery.FactScope,
	focus semanticdiscovery.Focus,
	evidenceRefs []semanticdiscovery.EvidenceRef,
	keywords []string,
) {
	statement = semanticDiscoveryText(statement, maxSemanticDiscoveryStatementBytes)
	if statement == "" || len(capabilities) == 0 {
		return
	}
	focus.ComponentIDs = semanticNonEmptyStrings(focus.ComponentIDs)
	focus.FlowIDs = semanticNonEmptyStrings(focus.FlowIDs)
	focus.SurfaceIDs = semanticNonEmptyStrings(focus.SurfaceIDs)
	evidenceRefs = semanticEvidenceRefs(evidenceRefs)
	keywords = semanticDiscoveryKeywords(keywords)
	// Source groups represent independently collected evidence families, not
	// individual facts. Sibling facts from one saved extractor must not make a
	// compositional claim look independently corroborated.
	sourceGroup := "sg-" + semanticHash(string(kind))
	id := "sf-" + semanticHash(string(kind)+"\x00"+sourceKey+"\x00"+statement)
	if _, duplicate := builder.facts[id]; duplicate {
		return
	}
	fact := semanticdiscovery.Fact{
		ID:           id,
		Kind:         kind,
		Statement:    statement,
		Keywords:     keywords,
		SourceGroup:  sourceGroup,
		Capabilities: append([]semanticdiscovery.Capability(nil), capabilities...),
		Scope:        scope,
		Focus:        focus,
		Evidence:     evidenceRefs,
	}
	if len(builder.facts) < maxSemanticDiscoveryFacts {
		builder.facts[id] = fact
		return
	}
	candidates := make([]semanticdiscovery.Fact, 0, len(builder.facts)+1)
	for _, existing := range builder.facts {
		candidates = append(candidates, existing)
	}
	candidates = append(candidates, fact)
	selected := selectSemanticDiscoveryFacts(candidates, maxSemanticDiscoveryFacts)
	if len(selected) == 0 || !semanticFactSliceContains(selected, id) {
		return
	}
	builder.facts = make(map[string]semanticdiscovery.Fact, len(selected))
	for _, selectedFact := range selected {
		builder.facts[selectedFact.ID] = selectedFact
	}
}

func semanticFactSliceContains(facts []semanticdiscovery.Fact, id string) bool {
	for _, fact := range facts {
		if fact.ID == id {
			return true
		}
	}
	return false
}

func (builder *semanticFactBuilder) addContext(kind semanticdiscovery.PlannerContextKind, text string) {
	if len(builder.plannerContext) >= maxSemanticDiscoveryPlannerContext {
		return
	}
	item := semanticdiscovery.PlannerContext{
		Kind: kind,
		Text: semanticDiscoveryText(text, maxSemanticDiscoveryStatementBytes),
	}
	// Old model-authored prose is useful only as planner context. If it carries
	// a repository reference or malformed text, omit it instead of promoting it
	// to evidence or making the optional semantic stage fail.
	if item.Validate() != nil {
		return
	}
	key := string(item.Kind) + "\x00" + item.Text
	if _, duplicate := builder.contextSeen[key]; duplicate {
		return
	}
	builder.contextSeen[key] = struct{}{}
	builder.plannerContext = append(builder.plannerContext, item)
}

func semanticFlowFileItems(groups ...[]FileItem) []FileItem {
	byPath := make(map[string]FileItem)
	for _, group := range groups {
		for _, item := range group {
			item.Path = strings.TrimSpace(item.Path)
			item.Reason = strings.TrimSpace(item.Reason)
			if item.Path == "" {
				continue
			}
			existing, found := byPath[item.Path]
			if !found || existing.Reason == "" && item.Reason != "" {
				byPath[item.Path] = item
			}
		}
	}
	result := make([]FileItem, 0, len(byPath))
	for _, item := range byPath {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func (builder *semanticFactBuilder) evidence(kind, label string, location *evidence.Location) semanticdiscovery.EvidenceRef {
	if location == nil || location.Path == "" {
		return semanticdiscovery.EvidenceRef{}
	}
	if _, ok := builder.openablePaths[location.Path]; !ok {
		return semanticdiscovery.EvidenceRef{}
	}
	kind = semanticEvidenceKind(kind)
	label = semanticDiscoveryText(label, maxSemanticDiscoveryEvidenceLabelByte)
	return semanticdiscovery.EvidenceRef{
		ID:     "se-" + semanticHash(fmt.Sprintf("%s\x00%s\x00%d\x00%d", kind, location.Path, location.Line, location.Column)),
		Kind:   semanticDiscoveryText(kind, 128),
		Label:  label,
		Path:   location.Path,
		Line:   location.Line,
		Column: location.Column,
	}
}

func semanticEvidenceRefs(values []semanticdiscovery.EvidenceRef) []semanticdiscovery.EvidenceRef {
	seen := make(map[string]struct{}, len(values))
	result := make([]semanticdiscovery.EvidenceRef, 0, min(len(values), maxSemanticDiscoveryEvidencePerFact))
	for _, value := range values {
		if value.ID == "" || value.Path == "" {
			continue
		}
		if _, duplicate := seen[value.ID]; duplicate {
			continue
		}
		seen[value.ID] = struct{}{}
		result = append(result, value)
		if len(result) == maxSemanticDiscoveryEvidencePerFact {
			break
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func componentIDStrings[T ~string](values []T) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return semanticNonEmptyStrings(result)
}

func semanticNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func semanticDiscoveryKeywords(values []string) []string {
	bounded := make([]string, 0, len(values))
	for _, value := range values {
		if value = semanticDiscoveryText(value, 256); value != "" {
			bounded = append(bounded, value)
		}
	}
	return semanticNonEmptyStrings(bounded)
}

func semanticEvidenceKind(value string) string {
	value = strings.TrimSpace(value)
	var result strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
			result.WriteRune(char)
		case char == '_', char == '-', char == '.', char == ':':
			result.WriteRune(char)
		default:
			result.WriteByte('_')
		}
		if result.Len() >= 128 {
			break
		}
	}
	if result.Len() == 0 {
		return "evidence"
	}
	return result.String()
}

func semanticDiscoveryText(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return strings.TrimSpace(value)
}

func semanticHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}
