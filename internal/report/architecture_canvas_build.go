package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

const architectureBuildContractVersion = "architecture-candidates-v1"

// ArchitectureSynthesisFile is the optional, replayable conceptual synthesis
// record stored beside other run artifacts.
const ArchitectureSynthesisFile = "architecture_synthesis.json"

// BuildArchitectureCanvasInput derives the exact local input for the v2
// architecture canvas from saved report facts. A repository landscape does
// not require a proven flow; saved FlowProof sessions add optional overlays.
// It intentionally chooses the deterministic landscape; conceptual synthesis
// may replace only that membership result later, using the returned candidate
// bundle.
func BuildArchitectureCanvasInput(data *ReportData) (ArchitectureCanvasInput, error) {
	if data == nil {
		return ArchitectureCanvasInput{}, fmt.Errorf("architecture canvas build: report data is nil")
	}

	builder := newArchitectureCandidateBuilder(data.RepositoryGraph, data.ArchitectureGrounding)
	builder.addRepositoryGraph(data.RepositoryGraph)
	builder.addArchitectureGrounding(data.ArchitectureGrounding)
	builder.addExecutableRoles(data.DiscoveredSurfaces)

	directions := append([]CandidateDirection(nil), data.CandidateDirections...)
	sort.SliceStable(directions, func(i, j int) bool {
		return directions[i].ID < directions[j].ID
	})
	seenFlows := make(map[componentmap.FlowID]struct{}, len(directions))
	for _, direction := range directions {
		if direction.Disposition == flowexplain.DirectionRejected {
			builder.diagnostics = append(builder.diagnostics, componentmap.Diagnostic{
				Code: "builder.rejected_direction_omitted", Severity: componentmap.FindingAdvisory,
				Message: fmt.Sprintf("direction %q remains available in analysis details but was not accepted as a flow", direction.Name),
			})
			continue
		}
		if direction.LocalProof == nil {
			builder.assessUngroundedDirection(direction)
			continue
		}
		flowID := componentmap.FlowID(direction.ID)
		if _, duplicate := seenFlows[flowID]; duplicate {
			return ArchitectureCanvasInput{}, fmt.Errorf(
				"architecture canvas build: duplicate saved flow id %q",
				direction.ID,
			)
		}
		seenFlows[flowID] = struct{}{}
		builder.addFlow(direction)
	}
	builder.addResearchFindings(data.ModelResearch)
	bundle := builder.bundle()
	if err := bundle.Validate(); err != nil {
		return ArchitectureCanvasInput{}, fmt.Errorf("architecture canvas build: candidate bundle: %w", err)
	}
	landscape, err := componentmap.Deterministic(bundle, componentmap.FallbackModelDisabled)
	if err != nil {
		return ArchitectureCanvasInput{}, fmt.Errorf("architecture canvas build: deterministic landscape: %w", err)
	}
	landscape.Diagnostics = append(landscape.Diagnostics, builder.diagnostics...)
	if err := landscape.Validate(bundle); err != nil {
		return ArchitectureCanvasInput{}, fmt.Errorf("architecture canvas build: deterministic landscape: %w", err)
	}

	return ArchitectureCanvasInput{
		CandidateBundle: bundle,
		Landscape:       landscape,
		Flows:           append([]ArchitectureFlowInput(nil), builder.flows...),
	}, nil
}

// ReplayArchitectureSynthesis replaces only conceptual naming and membership
// with a locally validated saved response. Candidate facts, relations,
// bindings, FlowProof, and layout remain outside model authority.
func ReplayArchitectureSynthesis(
	input ArchitectureCanvasInput,
	saved []byte,
) (ArchitectureCanvasInput, error) {
	var header struct {
		RepositoryRevision string `json:"repository_revision"`
	}
	if err := json.Unmarshal(saved, &header); err != nil {
		return input, fmt.Errorf("architecture canvas synthesis: decode header: %w", err)
	}
	landscape, err := componentmap.ReplaySynthesis(
		input.CandidateBundle,
		header.RepositoryRevision,
		saved,
	)
	if err != nil {
		landscape, err = componentmap.ReplayLegacyCapturedSynthesis(input.CandidateBundle, saved)
		if err != nil {
			return input, fmt.Errorf("architecture canvas synthesis: %w", err)
		}
	}
	input.Landscape = landscape
	return input, nil
}

type architectureCandidateBuilder struct {
	graph                 *RepositoryGraph
	knownPackages         map[string]componentmap.MemberID
	candidates            map[componentmap.MemberID]*architectureCandidateRecord
	relations             map[string]componentmap.LocalRelation
	bindings              map[architectureBindingKey]componentmap.FlowAnchorBinding
	flowFacts             []componentmap.Flow
	flows                 []ArchitectureFlowInput
	diagnostics           []componentmap.Diagnostic
	packageEdgeMembers    map[string]struct{}
	archetype             componentmap.RepositoryArchetype
	groundingMode         componentmap.GroundingMode
	behaviorAnchors       []componentmap.BehaviorAnchor
	behaviorMembers       map[string]componentmap.MemberID
	processEntryMembers   map[string]componentmap.MemberID
	declarationMembers    map[string]componentmap.MemberID
	groundedPaths         map[string]struct{}
	researchFindings      []componentmap.ResearchInterpretation
	researchPolicyVersion string
}

func (b *architectureCandidateBuilder) addResearchFindings(state *modelresearch.State) {
	if state == nil {
		return
	}
	b.researchPolicyVersion = state.Policy.Version
	facts := make(map[string]modelresearch.EvidenceItem, len(state.Theory.GroundedFacts))
	for _, fact := range state.Theory.GroundedFacts {
		facts[fact.ID] = fact
	}
	knownFlows := make(map[componentmap.FlowID]struct{}, len(b.flowFacts))
	for _, flow := range b.flowFacts {
		knownFlows[flow.ID] = struct{}{}
	}
	for _, round := range state.Rounds {
		for _, finding := range round.ValidatedFindings {
			paths := make(map[string]struct{})
			for _, evidenceID := range finding.EvidenceIDs {
				if fact, ok := facts[evidenceID]; ok && fact.Location != nil {
					paths[fact.Location.Path] = struct{}{}
				}
			}
			members := make([]componentmap.MemberID, 0)
			for memberID, record := range b.candidates {
				if memberID.Kind == componentmap.MemberPackage || memberID.Kind == componentmap.MemberFlow {
					continue
				}
				if candidateTouchesPaths(record.candidate, paths) {
					members = append(members, memberID)
				}
			}
			flowIDs := make([]componentmap.FlowID, 0)
			for _, flowID := range state.Theory.RelatedTraceIDs {
				id := componentmap.FlowID(flowID)
				if _, ok := knownFlows[id]; ok {
					flowIDs = append(flowIDs, id)
				}
			}
			anchorIDs := make([]string, 0)
			for _, anchor := range b.behaviorAnchors {
				if _, ok := paths[anchor.Location.Path]; ok {
					anchorIDs = append(anchorIDs, anchor.ID)
				}
			}
			if len(members) == 0 && len(flowIDs) == 0 && len(anchorIDs) == 0 {
				continue
			}
			sort.Slice(members, func(i, j int) bool {
				if members[i].Kind != members[j].Kind {
					return members[i].Kind < members[j].Kind
				}
				return members[i].Value < members[j].Value
			})
			sort.Slice(flowIDs, func(i, j int) bool { return flowIDs[i] < flowIDs[j] })
			sort.Strings(anchorIDs)
			b.researchFindings = append(b.researchFindings, componentmap.ResearchInterpretation{
				ID: round.ID + ":" + finding.ID, Question: round.Question,
				Interpretation: finding.Interpretation,
				EvidenceIDs:    append([]string(nil), finding.EvidenceIDs...),
				MemberIDs:      members, FlowIDs: flowIDs, AnchorIDs: anchorIDs,
			})
		}
	}
}

func candidateTouchesPaths(candidate componentmap.Candidate, paths map[string]struct{}) bool {
	for _, fact := range candidate.Facts {
		if fact.Location != nil {
			if _, ok := paths[fact.Location.Path]; ok {
				return true
			}
		}
		if fact.Kind == componentmap.FactRepositoryPath {
			if _, ok := paths[fact.Value]; ok {
				return true
			}
		}
	}
	return false
}

type architectureCandidateRecord struct {
	candidate      componentmap.Candidate
	participations map[componentmap.FlowID]componentmap.LocalFact
}

func newArchitectureCandidateBuilder(graph *RepositoryGraph, grounding *ArchitectureGrounding) *architectureCandidateBuilder {
	archetype := componentmap.ArchetypeApplication
	mode := componentmap.GroundingPackages
	if grounding != nil {
		archetype = grounding.RepositoryArchetype.Selected
		mode = grounding.GroundingMode
	} else if graph == nil || len(graph.PackageEdges) == 0 {
		archetype = componentmap.ArchetypeLibraryFramework
	}
	return &architectureCandidateBuilder{
		graph:               graph,
		knownPackages:       make(map[string]componentmap.MemberID),
		candidates:          make(map[componentmap.MemberID]*architectureCandidateRecord),
		relations:           make(map[string]componentmap.LocalRelation),
		bindings:            make(map[architectureBindingKey]componentmap.FlowAnchorBinding),
		packageEdgeMembers:  make(map[string]struct{}),
		archetype:           archetype,
		groundingMode:       mode,
		behaviorMembers:     make(map[string]componentmap.MemberID),
		processEntryMembers: make(map[string]componentmap.MemberID),
		declarationMembers:  make(map[string]componentmap.MemberID),
		groundedPaths:       make(map[string]struct{}),
	}
}

func (b *architectureCandidateBuilder) addArchitectureGrounding(grounding *ArchitectureGrounding) {
	if grounding == nil {
		return
	}
	for _, anchor := range sortedArchitectureGroundingAnchors(grounding.BehaviorAnchors) {
		b.groundedPaths[anchor.Location.Path] = struct{}{}
		memberIDs := make([]componentmap.MemberID, 0, len(anchor.AssociatedMembers))
		for _, member := range anchor.AssociatedMembers {
			location := member.Location
			b.groundedPaths[location.Path] = struct{}{}
			fileID := architectureBuildMemberID(componentmap.MemberFile, location.Path)
			var packageID *componentmap.MemberID
			if packagePath := b.packageForFile(location.Path); packagePath != "" {
				id := b.knownPackages[packagePath]
				packageID = &id
			}
			b.addCandidate(componentmap.Candidate{
				ID: fileID, Name: location.Path, ParentID: packageID,
				Facts: []componentmap.LocalFact{architectureBuildFact(
					componentmap.FactRepositoryPath, location.Path, evidence.CertaintyStatic,
					&evidence.Location{Path: location.Path}, "architecture_grounding",
					"behavior_anchor_file", "file containing a deterministic behavior-anchor member",
				)},
			})
			identity := strings.Join([]string{location.Path, strconv.Itoa(location.Line), strconv.Itoa(location.Column), member.ID}, "\x00")
			symbolID := architectureBuildMemberID(componentmap.MemberSymbol, identity)
			parentID := fileID
			b.addCandidate(componentmap.Candidate{
				ID: symbolID, Name: member.ID, ParentID: &parentID,
				Facts: []componentmap.LocalFact{architectureBuildFact(
					componentmap.FactDeclaration, member.ID, evidence.CertaintyStatic, &location,
					"architecture_grounding", "behavior_anchor_member",
					"exact declaration associated with a deterministic behavior anchor",
				)},
			})
			b.declarationMembers[architectureDeclarationKey(location, member.ID)] = symbolID
			if anchor.Kind == componentmap.AnchorProcessEntry {
				b.processEntryMembers[member.ID] = symbolID
			}
			memberIDs = append(memberIDs, symbolID)
		}
		if len(memberIDs) == 0 {
			continue
		}
		sort.Slice(memberIDs, func(i, j int) bool { return memberIDs[i].Value < memberIDs[j].Value })
		b.behaviorMembers[anchor.ID] = memberIDs[0]
		producer := anchor.Producer
		producer.Location = cloneArchitectureLocation(&anchor.Location)
		b.behaviorAnchors = append(b.behaviorAnchors, componentmap.BehaviorAnchor{
			ID: anchor.ID, Kind: anchor.Kind, Label: anchor.Label, Location: anchor.Location,
			Scenario: componentmap.ScenarioContext{
				ID: anchor.Scenario.ID, Name: "Recorded Go build scenario",
				Build: evidence.BuildContext{GOOS: anchor.Scenario.GOOS, GOARCH: anchor.Scenario.GOARCH, BuildTags: append([]string(nil), anchor.Scenario.Tags...)},
			},
			Producer: producer, Certainty: anchor.Certainty, MemberIDs: memberIDs,
			Limitations: append([]string(nil), anchor.Limitations...),
		})
	}
	for _, relationship := range compactArchitectureGroundingRelationships(grounding) {
		fromID, fromExists := b.behaviorMembers[relationship.From]
		toID, toExists := b.behaviorMembers[relationship.To]
		if !fromExists || !toExists || fromID == toID {
			continue
		}
		key := "behavior\x00" + string(fromID.Kind) + "\x00" + fromID.Value + "\x00" +
			string(toID.Kind) + "\x00" + toID.Value + "\x00" + relationship.Kind
		if _, duplicate := b.relations[key]; duplicate {
			continue
		}
		producer := relationship.Producer
		producer.Location = cloneArchitectureLocation(&relationship.Location)
		producer.Detail = fmt.Sprintf(
			"%d exact %s witness(es) across %d package(s)",
			max(relationship.WitnessCount, 1), relationship.EvidenceKind, max(relationship.PackageCount, 1),
		)
		b.relations[key] = componentmap.LocalRelation{
			ID: relationship.ID, From: fromID, To: toID,
			Kind:     componentmap.StructuralRelationBehaviorHandoff,
			Location: &relationship.Location, Certainty: relationship.Certainty,
			Provenance: []evidence.Provenance{producer},
			Scenarios: []componentmap.ScenarioContext{{
				ID: grounding.BehaviorAnchors[0].Scenario.ID, Name: "Recorded Go build scenario",
				Build: evidence.BuildContext{
					GOOS:      grounding.BehaviorAnchors[0].Scenario.GOOS,
					GOARCH:    grounding.BehaviorAnchors[0].Scenario.GOARCH,
					BuildTags: append([]string(nil), grounding.BehaviorAnchors[0].Scenario.Tags...),
				},
			}},
		}
	}
}

func (b *architectureCandidateBuilder) addExecutableRoles(surfaces *DiscoveredSurfaces) {
	if surfaces == nil {
		return
	}
	for _, trigger := range surfaces.Triggers {
		if trigger.Kind != "process_entry" || trigger.ProcessEntrypoint.ID == "" {
			continue
		}
		role := normalizeSurfaceExecutableRole(trigger.ExecutableRole)
		if role == ExecutableRoleUnknown {
			continue
		}
		memberID, exists := b.processEntryMembers[trigger.ProcessEntrypoint.ID]
		if !exists {
			continue
		}
		record := b.candidates[memberID]
		if record == nil {
			continue
		}
		var location *evidence.Location
		if trigger.ProcessEntrypoint.Location != nil {
			location = &evidence.Location{
				Path:   trigger.ProcessEntrypoint.Location.Path,
				Line:   trigger.ProcessEntrypoint.Location.Line,
				Column: trigger.ProcessEntrypoint.Location.Column,
			}
		}
		record.candidate.Facts = append(record.candidate.Facts, architectureBuildFact(
			componentmap.FactExecutableRole,
			role,
			evidence.CertaintyStatic,
			location,
			"surface_catalog",
			"exact_process_entry_role",
			"deterministic executable role joined by exact process-entry declaration ID",
		))
	}
}

func compactArchitectureGroundingRelationships(grounding *ArchitectureGrounding) []ArchitectureBehaviorHandoff {
	anchorKinds := make(map[string]componentmap.BehaviorAnchorKind, len(grounding.BehaviorAnchors))
	for _, anchor := range grounding.BehaviorAnchors {
		anchorKinds[anchor.ID] = anchor.Kind
	}
	byKey := make(map[string]ArchitectureBehaviorHandoff)
	packagesByKey := make(map[string]map[string]struct{})
	for _, relationship := range grounding.Relationships {
		kind := relationship.Kind
		if kind == "bounded_direct_call" || !validArchitectureRelationshipKind(kind) {
			kind = semanticArchitectureRelationshipKind(anchorKinds[relationship.From], anchorKinds[relationship.To])
		}
		key := relationship.From + "\x00" + relationship.To + "\x00" + kind
		aggregated, exists := byKey[key]
		if !exists {
			aggregated = relationship
			aggregated.ID = architectureBuildStableID("behavior-handoff", relationship.From, relationship.To, kind)
			aggregated.Kind = kind
			aggregated.EvidenceKind = "bounded_direct_call"
			aggregated.WitnessIDs = nil
			aggregated.RepresentativeLocations = nil
			aggregated.WitnessCount = 0
			aggregated.PackageCount = 0
			packagesByKey[key] = make(map[string]struct{})
		}
		witnessIDs := relationship.WitnessIDs
		if len(witnessIDs) == 0 {
			witnessIDs = []string{relationship.ID}
		}
		aggregated.WitnessIDs = append(aggregated.WitnessIDs, witnessIDs...)
		witnessCount := relationship.WitnessCount
		if witnessCount == 0 {
			witnessCount = len(witnessIDs)
		}
		aggregated.WitnessCount += witnessCount
		locations := relationship.RepresentativeLocations
		if len(locations) == 0 {
			locations = []evidence.Location{relationship.Location}
		}
		for _, location := range locations {
			if len(aggregated.RepresentativeLocations) < 8 {
				aggregated.RepresentativeLocations = append(aggregated.RepresentativeLocations, location)
			}
			packagesByKey[key][path.Dir(location.Path)] = struct{}{}
		}
		aggregated.PackageCount = max(relationship.PackageCount, len(packagesByKey[key]))
		byKey[key] = aggregated
	}
	result := make([]ArchitectureBehaviorHandoff, 0, len(byKey))
	for key, relationship := range byKey {
		sort.Strings(relationship.WitnessIDs)
		relationship.WitnessIDs = compactArchitectureStrings(relationship.WitnessIDs)
		if relationship.WitnessCount < len(relationship.WitnessIDs) {
			relationship.WitnessCount = len(relationship.WitnessIDs)
		}
		relationship.PackageCount = max(relationship.PackageCount, len(packagesByKey[key]))
		result = append(result, relationship)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func semanticArchitectureRelationshipKind(from, to componentmap.BehaviorAnchorKind) string {
	switch {
	case from == componentmap.AnchorProcessEntry && to == componentmap.AnchorCommandDispatch:
		return "dispatches_to"
	case from == componentmap.AnchorRegistryWrite && to == componentmap.AnchorExtensionFamily:
		return "registers_extension_family"
	case architectureConfigAnchorKind(from) && architectureConfigAnchorKind(to):
		return "loads_or_adapts_config"
	case from == componentmap.AnchorLifecycleInterface && to == componentmap.AnchorLifecycleStart:
		return "starts_lifecycle"
	case to == componentmap.AnchorAdminControlPlane:
		return "exposes_admin_control_plane"
	case to == componentmap.AnchorRequestDispatchRoot:
		return "dispatches_http_request"
	case to == componentmap.AnchorSecurityBoundary:
		return "configures_security_boundary"
	default:
		return "static_call_supporting_relation"
	}
}

func architectureConfigAnchorKind(kind componentmap.BehaviorAnchorKind) bool {
	return kind == componentmap.AnchorConfigIngress || kind == componentmap.AnchorConfigAdapter || kind == componentmap.AnchorConfigApply
}

func compactArchitectureStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func (b *architectureCandidateBuilder) assessUngroundedDirection(direction CandidateDirection) {
	paths := append([]string{direction.LikelyEntrypoint}, direction.LikelyFiles...)
	for _, candidatePath := range paths {
		if _, grounded := b.groundedPaths[candidatePath]; !grounded {
			continue
		}
		b.diagnostics = append(b.diagnostics, componentmap.Diagnostic{
			Code: "builder.direction_anchor_hypothesis", Severity: componentmap.FindingAdvisory,
			Message: fmt.Sprintf(
				"direction %q remains a hypothesis, but at least one supplied file contains an exact behavior anchor",
				direction.Name,
			),
		})
		return
	}
	b.diagnostics = append(b.diagnostics, componentmap.Diagnostic{
		Code: "builder.ungrounded_direction_omitted", Severity: componentmap.FindingAdvisory,
		Message: fmt.Sprintf(
			"direction %q was omitted from primary architecture because it has no selected-flow proof or matching behavior anchor",
			direction.Name,
		),
	})
}

func (b *architectureCandidateBuilder) addRepositoryGraph(graph *RepositoryGraph) {
	if graph == nil {
		return
	}
	for _, edge := range graph.PackageEdges {
		if edge.From == "" || edge.To == "" || edge.From == edge.To {
			b.diagnostics = append(b.diagnostics, componentmap.Diagnostic{
				Code: "builder.invalid_package_edge", Severity: componentmap.FindingAdvisory,
				Message: "a saved package edge was omitted because its exact endpoints were empty or identical",
			})
			continue
		}
		b.packageEdgeMembers[edge.From] = struct{}{}
		b.packageEdgeMembers[edge.To] = struct{}{}
	}

	packagePaths := make([]string, 0, len(b.packageEdgeMembers)+len(graph.Packages))
	for packagePath := range b.packageEdgeMembers {
		packagePaths = append(packagePaths, packagePath)
	}
	for _, pkg := range graph.Packages {
		if pkg.CanonicalPath != "" {
			packagePaths = append(packagePaths, pkg.CanonicalPath)
		}
	}
	sort.Strings(packagePaths)
	packagePaths = compactArchitectureStrings(packagePaths)
	for _, packagePath := range packagePaths {
		b.addPackageCandidate(packagePath)
	}

	for _, edge := range graph.PackageEdges {
		if edge.From == "" || edge.To == "" || edge.From == edge.To {
			continue
		}
		fromID := b.knownPackages[edge.From]
		toID := b.knownPackages[edge.To]
		key := fromID.Value + "\x00" + toID.Value
		if _, duplicate := b.relations[key]; duplicate {
			continue
		}
		b.relations[key] = componentmap.LocalRelation{
			ID:        architectureBuildStableID("package-import", edge.From, edge.To),
			From:      fromID,
			To:        toID,
			Kind:      componentmap.StructuralRelationPackageImport,
			Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{
				Provider:  "report_repository_graph",
				Version:   architectureBuildContractVersion,
				Operation: "saved_package_import",
				Detail:    "exact saved package endpoints; source import callsite unavailable",
			}},
			Scenarios: []componentmap.ScenarioContext{{
				ID:   "saved-package-graph",
				Name: "Saved package graph context; exact build values unavailable",
			}},
		}
	}
}

func (b *architectureCandidateBuilder) addPackageCandidate(packagePath string) componentmap.MemberID {
	if id, exists := b.knownPackages[packagePath]; exists {
		return id
	}
	id := architectureBuildMemberID(componentmap.MemberPackage, packagePath)
	b.knownPackages[packagePath] = id
	name := packagePath
	if b.graph != nil {
		for _, pkg := range b.graph.Packages {
			if pkg.CanonicalPath == packagePath && pkg.DisplayPath != "" {
				name = pkg.DisplayPath
				break
			}
		}
	}
	b.addCandidate(componentmap.Candidate{
		ID:   id,
		Name: name,
		Facts: []componentmap.LocalFact{architectureBuildFact(
			componentmap.FactDeclaration,
			packagePath,
			evidence.CertaintyStatic,
			nil,
			"report_repository_graph",
			"saved_package_member",
			"exact endpoint in the saved package-import graph",
		)},
	})
	return id
}

func (b *architectureCandidateBuilder) addFlow(direction CandidateDirection) {
	flowID := componentmap.FlowID(direction.ID)
	session := *direction.LocalProof
	proof := session.Proof
	name := architectureBuildFlowName(direction, proof)
	proofLocation := architectureBuildFirstLocation(proof.Anchors)
	b.flowFacts = append(b.flowFacts, componentmap.Flow{
		ID:   flowID,
		Name: name,
		Facts: []componentmap.LocalFact{architectureBuildFact(
			componentmap.FactDeclaration,
			"saved FlowProof v"+strconv.Itoa(proof.Version),
			evidence.CertaintyStatic,
			proofLocation,
			"flowproof",
			"saved_proof",
			"saved typed flow contract",
		)},
	})
	flowMemberID := architectureBuildMemberID(componentmap.MemberFlow, direction.ID)
	b.addCandidate(componentmap.Candidate{
		ID:   flowMemberID,
		Name: name,
		Facts: []componentmap.LocalFact{architectureBuildFact(
			componentmap.FactFlowParticipation,
			direction.ID,
			evidence.CertaintyStatic,
			proofLocation,
			"flowproof",
			"saved_flow_member",
			"exact identity of the saved typed flow contract",
		)},
	})
	b.addParticipation(flowMemberID, flowID, proofLocation)
	b.flows = append(b.flows, ArchitectureFlowInput{
		ID:      flowID,
		Name:    name,
		Trigger: direction.Trigger,
		Session: session,
	})

	validIdentity := session.Version == flowproof.SessionVersion &&
		proof.Version == flowproof.Version &&
		proof.ID == direction.ID &&
		(proof.Archetype == flowproof.ArchetypeCLI || proof.Archetype == flowproof.ArchetypeProcess)
	if !validIdentity {
		return
	}

	entrypointAnchors := architectureBuildEntrypointAnchors(proof)
	anchorCounts := make(map[string]int, len(proof.Anchors))
	for _, anchor := range proof.Anchors {
		anchorCounts[anchor.ID]++
	}
	for _, anchor := range proof.Anchors {
		if anchorCounts[anchor.ID] != 1 || validateArchitectureAnchor(anchor) != nil || anchor.Location == nil {
			continue
		}
		b.addAnchor(flowID, anchor, entrypointAnchors)
	}
}

func (b *architectureCandidateBuilder) addAnchor(
	flowID componentmap.FlowID,
	anchor flowproof.Anchor,
	entrypointAnchors map[string]struct{},
) {
	location := cloneArchitectureLocation(anchor.Location)
	fileID := architectureBuildMemberID(componentmap.MemberFile, location.Path)
	var packageID *componentmap.MemberID
	if packagePath := b.packageForFile(location.Path); packagePath != "" {
		id := b.knownPackages[packagePath]
		packageID = &id
	}
	b.addCandidate(componentmap.Candidate{
		ID:       fileID,
		Name:     location.Path,
		ParentID: packageID,
		Facts: []componentmap.LocalFact{architectureBuildFact(
			componentmap.FactRepositoryPath,
			location.Path,
			evidence.CertaintyStatic,
			&evidence.Location{Path: location.Path},
			"flowproof",
			"anchor_file",
			"file contains an exact saved flow anchor",
		)},
	})
	b.addParticipation(fileID, flowID, location)

	boundMemberID := fileID
	if anchor.Kind == flowproof.AnchorFunction || anchor.Kind == flowproof.AnchorMethod {
		declarationName := architectureBuildAnchorName(anchor)
		memberID, exists := b.declarationMembers[architectureDeclarationKey(*location, declarationName)]
		if !exists {
			kind := componentmap.MemberSymbol
			if _, isEntrypoint := entrypointAnchors[anchor.ID]; isEntrypoint {
				kind = componentmap.MemberEntrypoint
			}
			identity := strings.Join([]string{
				location.Path,
				strconv.Itoa(location.Line),
				strconv.Itoa(location.Column),
				string(anchor.Kind),
				anchor.QualifiedName,
				anchor.Label,
			}, "\x00")
			memberID = architectureBuildMemberID(kind, identity)
			parentID := fileID
			b.addCandidate(componentmap.Candidate{
				ID:       memberID,
				Name:     anchor.Label,
				ParentID: &parentID,
				Facts: []componentmap.LocalFact{architectureBuildFact(
					componentmap.FactDeclaration,
					declarationName,
					evidence.CertaintyStatic,
					location,
					"flowproof",
					"anchor_declaration",
					"exact declaration anchor from the saved typed flow contract",
				)},
			})
		}
		b.addParticipation(memberID, flowID, location)
		boundMemberID = memberID
	}

	key := architectureBindingKey{flowID: flowID, anchorID: anchor.ID}
	b.bindings[key] = componentmap.FlowAnchorBinding{
		FlowID:    flowID,
		AnchorID:  anchor.ID,
		MemberID:  boundMemberID,
		Location:  location,
		Certainty: evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{
			Provider:  "flowproof",
			Version:   strconv.Itoa(flowproof.Version),
			Operation: "bind_anchor_to_exact_member",
			Detail:    "binding produced directly from the saved typed anchor, not by presentation path matching",
			Location:  cloneArchitectureLocation(location),
		}},
	}
}

func architectureDeclarationKey(location evidence.Location, declaration string) string {
	return strings.Join([]string{
		location.Path,
		strconv.Itoa(location.Line),
		strings.TrimSpace(declaration),
	}, "\x00")
}

func (b *architectureCandidateBuilder) addCandidate(candidate componentmap.Candidate) {
	if _, exists := b.candidates[candidate.ID]; exists {
		return
	}
	b.candidates[candidate.ID] = &architectureCandidateRecord{
		candidate:      candidate,
		participations: make(map[componentmap.FlowID]componentmap.LocalFact),
	}
}

func (b *architectureCandidateBuilder) addParticipation(
	memberID componentmap.MemberID,
	flowID componentmap.FlowID,
	location *evidence.Location,
) {
	record := b.candidates[memberID]
	if record == nil {
		return
	}
	fact := architectureBuildFact(
		componentmap.FactFlowParticipation,
		string(flowID),
		evidence.CertaintyStatic,
		location,
		"flowproof",
		"anchor_flow_participation",
		"exact saved anchor witnesses participation in this flow",
	)
	previous, exists := record.participations[flowID]
	if !exists || architectureBuildLocationBefore(fact.Location, previous.Location) {
		record.participations[flowID] = fact
	}
}

func (b *architectureCandidateBuilder) packageForFile(filePath string) string {
	if b.graph == nil || !strings.HasSuffix(strings.ToLower(filePath), ".go") {
		return ""
	}
	dir := path.Dir(filePath)
	if dir == "." {
		dir = ""
	}
	if len(b.graph.Packages) > 0 {
		for _, pkg := range b.graph.Packages {
			if pkg.Dir == dir {
				return pkg.CanonicalPath
			}
		}
		return ""
	}
	var best *ModuleInfo
	for index := range b.graph.Modules {
		module := &b.graph.Modules[index]
		insideModule := module.Dir == "" || dir == module.Dir || strings.HasPrefix(dir, module.Dir+"/")
		if module.Path == "" || !insideModule {
			continue
		}
		if best == nil || len(module.Dir) > len(best.Dir) {
			best = module
		}
	}
	if best == nil {
		return ""
	}
	relative := dir
	if best.Dir != "" {
		relative = strings.TrimPrefix(strings.TrimPrefix(dir, best.Dir), "/")
	}
	packagePath := strings.TrimSuffix(best.Path, "/")
	if relative != "" {
		packagePath += "/" + relative
	}
	if _, exactSavedEndpoint := b.packageEdgeMembers[packagePath]; !exactSavedEndpoint {
		return ""
	}
	return packagePath
}

func (b *architectureCandidateBuilder) bundle() componentmap.CandidateBundle {
	candidates := make([]componentmap.Candidate, 0, len(b.candidates))
	for _, record := range b.candidates {
		flowIDs := make([]componentmap.FlowID, 0, len(record.participations))
		for flowID := range record.participations {
			flowIDs = append(flowIDs, flowID)
		}
		sort.Slice(flowIDs, func(i, j int) bool { return flowIDs[i] < flowIDs[j] })
		candidate := record.candidate
		for _, flowID := range flowIDs {
			candidate.Participations = append(candidate.Participations, componentmap.FlowParticipation{
				FlowID:   flowID,
				Evidence: record.participations[flowID],
			})
		}
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ID.Kind != candidates[j].ID.Kind {
			return candidates[i].ID.Kind < candidates[j].ID.Kind
		}
		return candidates[i].ID.Value < candidates[j].ID.Value
	})

	relations := make([]componentmap.LocalRelation, 0, len(b.relations))
	for _, relation := range b.relations {
		relations = append(relations, relation)
	}
	sort.Slice(relations, func(i, j int) bool { return relations[i].ID < relations[j].ID })

	bindings := make([]componentmap.FlowAnchorBinding, 0, len(b.bindings))
	for _, binding := range b.bindings {
		bindings = append(bindings, binding)
	}
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].FlowID != bindings[j].FlowID {
			return bindings[i].FlowID < bindings[j].FlowID
		}
		return bindings[i].AnchorID < bindings[j].AnchorID
	})
	sort.Slice(b.flowFacts, func(i, j int) bool { return b.flowFacts[i].ID < b.flowFacts[j].ID })
	sort.Slice(b.researchFindings, func(i, j int) bool { return b.researchFindings[i].ID < b.researchFindings[j].ID })

	return componentmap.CandidateBundle{
		Version:               componentmap.ContractVersion,
		RepositoryArchetype:   b.archetype,
		GroundingMode:         b.groundingMode,
		BehaviorAnchors:       append([]componentmap.BehaviorAnchor(nil), b.behaviorAnchors...),
		Candidates:            candidates,
		Flows:                 append([]componentmap.Flow(nil), b.flowFacts...),
		Relations:             relations,
		AnchorBindings:        bindings,
		ResearchFindings:      append([]componentmap.ResearchInterpretation(nil), b.researchFindings...),
		ResearchPolicyVersion: b.researchPolicyVersion,
	}
}

func architectureBuildEntrypointAnchors(proof flowproof.Proof) map[string]struct{} {
	result := make(map[string]struct{})
	for _, slot := range proof.Slots {
		if slot.Kind != flowproof.SlotEntrypoint || slot.Status != flowproof.SlotVerified {
			continue
		}
		for _, evidenceID := range slot.EvidenceIDs {
			result[evidenceID] = struct{}{}
		}
	}
	return result
}

func architectureBuildFlowName(direction CandidateDirection, proof flowproof.Proof) string {
	if direction.Name != "" {
		return direction.Name
	}
	if proof.Goal != "" {
		return proof.Goal
	}
	return direction.ID
}

func architectureBuildAnchorName(anchor flowproof.Anchor) string {
	if anchor.QualifiedName != "" {
		return anchor.QualifiedName
	}
	return anchor.Label
}

func architectureBuildFirstLocation(anchors []flowproof.Anchor) *evidence.Location {
	var first *evidence.Location
	for _, anchor := range anchors {
		if anchor.Location == nil || validateArchitectureLocation(*anchor.Location, false) != nil {
			continue
		}
		if first == nil || architectureBuildLocationBefore(anchor.Location, first) {
			first = cloneArchitectureLocation(anchor.Location)
		}
	}
	return first
}

func architectureBuildFact(
	kind componentmap.FactKind,
	value string,
	certainty evidence.Certainty,
	location *evidence.Location,
	provider, operation, detail string,
) componentmap.LocalFact {
	return componentmap.LocalFact{
		Kind:      kind,
		Value:     value,
		Location:  cloneArchitectureLocation(location),
		Certainty: certainty,
		Provenance: []evidence.Provenance{{
			Provider:  provider,
			Version:   architectureBuildContractVersion,
			Operation: operation,
			Detail:    detail,
			Location:  cloneArchitectureLocation(location),
		}},
	}
}

func architectureBuildMemberID(kind componentmap.MemberKind, identity string) componentmap.MemberID {
	return componentmap.MemberID{
		Kind:  kind,
		Value: architectureBuildStableID("member-"+string(kind), identity),
	}
}

func architectureBuildStableID(kind string, values ...string) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%s/%s\n", architectureBuildContractVersion, kind)
	for _, value := range values {
		fmt.Fprintf(hash, "%d:%s\n", len(value), value)
	}
	return kind + "-" + hex.EncodeToString(hash.Sum(nil)[:12])
}

func architectureBuildLocationBefore(left, right *evidence.Location) bool {
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.Line != right.Line {
		return left.Line < right.Line
	}
	return left.Column < right.Column
}
