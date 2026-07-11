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
	"github.com/dvordrova/repomap/internal/flowproof"
)

const architectureBuildContractVersion = "architecture-candidates-v1"

// ArchitectureSynthesisFile is the optional, replayable conceptual synthesis
// record stored beside other run artifacts.
const ArchitectureSynthesisFile = "architecture_synthesis.json"

// BuildArchitectureCanvasInput derives the exact local input for the v2
// architecture canvas from saved report facts. It intentionally chooses the
// deterministic landscape; conceptual synthesis may replace only that
// membership result later, using the returned candidate bundle.
func BuildArchitectureCanvasInput(data *ReportData) (ArchitectureCanvasInput, error) {
	if data == nil {
		return ArchitectureCanvasInput{}, fmt.Errorf("architecture canvas build: report data is nil")
	}

	builder := newArchitectureCandidateBuilder(data.RepositoryGraph)
	builder.addRepositoryGraph(data.RepositoryGraph)

	directions := append([]CandidateDirection(nil), data.CandidateDirections...)
	sort.SliceStable(directions, func(i, j int) bool {
		return directions[i].ID < directions[j].ID
	})
	seenFlows := make(map[componentmap.FlowID]struct{}, len(directions))
	for _, direction := range directions {
		if direction.LocalProof == nil {
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
	if len(builder.flows) == 0 {
		return ArchitectureCanvasInput{}, fmt.Errorf("architecture canvas build: no saved flowproof sessions")
	}

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
		return input, fmt.Errorf("architecture canvas synthesis: %w", err)
	}
	input.Landscape = landscape
	return input, nil
}

type architectureCandidateBuilder struct {
	graph              *RepositoryGraph
	knownPackages      map[string]componentmap.MemberID
	candidates         map[componentmap.MemberID]*architectureCandidateRecord
	relations          map[string]componentmap.LocalRelation
	bindings           map[architectureBindingKey]componentmap.FlowAnchorBinding
	flowFacts          []componentmap.Flow
	flows              []ArchitectureFlowInput
	diagnostics        []componentmap.Diagnostic
	packageEdgeMembers map[string]struct{}
}

type architectureCandidateRecord struct {
	candidate      componentmap.Candidate
	participations map[componentmap.FlowID]componentmap.LocalFact
}

func newArchitectureCandidateBuilder(graph *RepositoryGraph) *architectureCandidateBuilder {
	return &architectureCandidateBuilder{
		graph:              graph,
		knownPackages:      make(map[string]componentmap.MemberID),
		candidates:         make(map[componentmap.MemberID]*architectureCandidateRecord),
		relations:          make(map[string]componentmap.LocalRelation),
		bindings:           make(map[architectureBindingKey]componentmap.FlowAnchorBinding),
		packageEdgeMembers: make(map[string]struct{}),
	}
}

func (b *architectureCandidateBuilder) addRepositoryGraph(graph *RepositoryGraph) {
	if graph == nil {
		return
	}
	for _, edge := range graph.PackageEdges {
		if edge.From == "" || edge.To == "" || edge.From == edge.To {
			b.diagnostics = append(b.diagnostics, componentmap.Diagnostic{
				Code:    "builder.invalid_package_edge",
				Message: "a saved package edge was omitted because its exact endpoints were empty or identical",
			})
			continue
		}
		b.packageEdgeMembers[edge.From] = struct{}{}
		b.packageEdgeMembers[edge.To] = struct{}{}
	}

	packagePaths := make([]string, 0, len(b.packageEdgeMembers))
	for packagePath := range b.packageEdgeMembers {
		packagePaths = append(packagePaths, packagePath)
	}
	sort.Strings(packagePaths)
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
	b.addCandidate(componentmap.Candidate{
		ID:   id,
		Name: packagePath,
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
		proof.Archetype == flowproof.ArchetypeCLI
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
		memberID := architectureBuildMemberID(kind, identity)
		parentID := fileID
		b.addCandidate(componentmap.Candidate{
			ID:       memberID,
			Name:     anchor.Label,
			ParentID: &parentID,
			Facts: []componentmap.LocalFact{architectureBuildFact(
				componentmap.FactDeclaration,
				architectureBuildAnchorName(anchor),
				evidence.CertaintyStatic,
				location,
				"flowproof",
				"anchor_declaration",
				"exact declaration anchor from the saved typed flow contract",
			)},
		})
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

	return componentmap.CandidateBundle{
		Version:        componentmap.ContractVersion,
		Candidates:     candidates,
		Flows:          append([]componentmap.Flow(nil), b.flowFacts...),
		Relations:      relations,
		AnchorBindings: bindings,
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
