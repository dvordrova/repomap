package report

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowproof"
)

// ArchitectureCanvasVersion changes when the saved projection semantics or
// identity rules change. It is independent of the landscape and proof versions.
const ArchitectureCanvasVersion = 8

type ArchitectureCanvasInput struct {
	CandidateBundle componentmap.CandidateBundle
	Landscape       componentmap.Landscape
	Flows           []ArchitectureFlowInput
}

type ArchitectureFlowInput struct {
	ID          componentmap.FlowID `json:"id"`
	Name        string              `json:"name,omitempty"`
	Trigger     string              `json:"trigger,omitempty"`
	Scope       string              `json:"scope,omitempty"`
	MentalModel string              `json:"mental_model,omitempty"`
	Session     flowproof.Session   `json:"session"`
}

type ArchitectureCanvas struct {
	Version                   int                                   `json:"version"`
	LandscapeVersion          int                                   `json:"landscape_version"`
	FlowProofVersion          int                                   `json:"flowproof_version"`
	Fallback                  bool                                  `json:"fallback"`
	FallbackReason            componentmap.FallbackReason           `json:"fallback_reason,omitempty"`
	ValidationOutcome         componentmap.ValidationOutcome        `json:"validation_outcome"`
	ArchitectureSource        componentmap.ArchitectureSource       `json:"architecture_source"`
	ArchitectureLevel         int                                   `json:"architecture_level"`
	Normalizations            []componentmap.NormalizationOperation `json:"normalization_operations,omitempty"`
	OriginalProposalSHA256    string                                `json:"original_proposal_sha256,omitempty"`
	RepositoryArchetype       componentmap.RepositoryArchetype      `json:"repository_archetype"`
	GroundingMode             componentmap.GroundingMode            `json:"grounding_mode"`
	Title                     string                                `json:"title"`
	Subtitle                  string                                `json:"subtitle"`
	BehaviorAnchors           []componentmap.BehaviorAnchor         `json:"behavior_anchors,omitempty"`
	Subsystems                []ArchitectureSubsystem               `json:"subsystems"`
	Components                []ArchitectureComponent               `json:"components"`
	LocalRemainderComponentID componentmap.ComponentID              `json:"local_remainder_component_id,omitempty"`
	StructuralLocators        []ArchitectureStructuralLocator       `json:"structural_locators,omitempty"`
	Surfaces                  []ArchitectureSurface                 `json:"surfaces,omitempty"`
	Suggestions               []ArchitectureSuggestion              `json:"suggested_investigations,omitempty"`
	StructuralFacts           []componentmap.LocalRelation          `json:"structural_facts,omitempty"`
	StructuralEdges           []ArchitectureStructuralEdge          `json:"structural_edges,omitempty"`
	Flows                     []ArchitectureFlow                    `json:"flows,omitempty"`
	FlowEdges                 []ArchitectureFlowEdge                `json:"flow_edges,omitempty"`
	Frontiers                 []ArchitectureFrontier                `json:"frontiers,omitempty"`
	Diagnostics               []ArchitectureDiagnostic              `json:"diagnostics,omitempty"`
}

type ArchitectureSubsystem struct {
	ID           componentmap.SubsystemID       `json:"id"`
	Name         string                         `json:"name"`
	Description  string                         `json:"description,omitempty"`
	Category     componentmap.SubsystemCategory `json:"category,omitempty"`
	ComponentIDs []componentmap.ComponentID     `json:"component_ids"`
	SourceIDs    []componentmap.SubsystemID     `json:"source_subsystem_ids,omitempty"`
}

type ArchitectureComponent struct {
	ID                        componentmap.ComponentID   `json:"id"`
	SubsystemID               componentmap.SubsystemID   `json:"subsystem_id"`
	Name                      string                     `json:"name"`
	Description               string                     `json:"description,omitempty"`
	Members                   []componentmap.Candidate   `json:"members"`
	ParticipatingFlowIDs      []componentmap.FlowID      `json:"participating_flow_ids,omitempty"`
	OwnedSurfaceIDs           []string                   `json:"owned_surface_ids,omitempty"`
	ParticipatingSurfaceIDs   []string                   `json:"participating_surface_ids,omitempty"`
	SuggestedInvestigationIDs []string                   `json:"suggested_investigation_ids,omitempty"`
	AnchorIDs                 []string                   `json:"anchor_ids,omitempty"`
	Hypothesis                bool                       `json:"hypothesis,omitempty"`
	SourceIDs                 []componentmap.ComponentID `json:"source_component_ids,omitempty"`
}

// ArchitectureStructuralLocator retains an exact producer-owned source or
// containment node without turning it into a model-authored conceptual
// member. ParticipatingComponentIDs is the complete sorted union recovered
// locally through exact ParentID containment; it never claims ownership.
type ArchitectureStructuralLocator struct {
	Locator                   componentmap.Candidate     `json:"locator"`
	ParticipatingComponentIDs []componentmap.ComponentID `json:"participating_component_ids,omitempty"`
}

// ArchitectureSurface is a presentation join over an existing deterministic
// surface or a saved trace's exact start evidence. It does not introduce a new
// analyzer surface kind or claim that static evidence executed at runtime.
type ArchitectureSurface struct {
	ID                        string                     `json:"id"`
	Name                      string                     `json:"name"`
	Source                    string                     `json:"source"`
	Kind                      string                     `json:"kind,omitempty"`
	Category                  string                     `json:"category"`
	OwningExecutable          string                     `json:"owning_executable,omitempty"`
	OwningComponentID         componentmap.ComponentID   `json:"owning_component_id,omitempty"`
	ParticipatingComponentIDs []componentmap.ComponentID `json:"participating_component_ids,omitempty"`
	RelatedTraceID            componentmap.FlowID        `json:"related_saved_trace_id,omitempty"`
	Status                    string                     `json:"status,omitempty"`
	Certainty                 string                     `json:"certainty,omitempty"`
	Resolution                string                     `json:"resolution,omitempty"`
	Evidence                  []SurfaceLocation          `json:"evidence,omitempty"`
	TraceUnavailableReason    string                     `json:"trace_unavailable_reason,omitempty"`
	SurfaceRole               string                     `json:"surface_role,omitempty"`
	TraceReadiness            string                     `json:"trace_readiness,omitempty"`
	TraceReadinessReason      string                     `json:"trace_readiness_reason,omitempty"`
	Quality                   SurfaceQuality             `json:"quality,omitempty"`
}

// ArchitectureSuggestion binds an accepted untraced direction to exact local
// architecture IDs. It remains distinct from both surfaces and saved traces.
type ArchitectureSuggestion struct {
	ID                     string                     `json:"id"`
	Title                  string                     `json:"title"`
	Reason                 string                     `json:"reason,omitempty"`
	EvidenceReferences     []string                   `json:"evidence_references,omitempty"`
	RelevantAnchorIDs      []string                   `json:"relevant_architecture_anchor_ids,omitempty"`
	RelevantComponentIDs   []componentmap.ComponentID `json:"relevant_component_ids,omitempty"`
	CurrentGrounding       string                     `json:"current_grounding"`
	CanStartTrace          bool                       `json:"can_start_trace"`
	InvestigationAvailable bool                       `json:"investigation_available"`
	UnavailableReason      string                     `json:"unavailable_reason,omitempty"`
	TraceUnavailableReason string                     `json:"trace_unavailable_reason,omitempty"`
	StartLocation          *SurfaceLocation           `json:"start_location,omitempty"`
}

type ArchitectureStructuralEdge struct {
	ID              string                     `json:"id"`
	FromComponentID componentmap.ComponentID   `json:"from_component_id"`
	ToComponentID   componentmap.ComponentID   `json:"to_component_id"`
	Witness         componentmap.LocalRelation `json:"witness"`
}

type ArchitectureFlow struct {
	ID                         componentmap.FlowID        `json:"id"`
	Name                       string                     `json:"name"`
	Archetype                  flowproof.Archetype        `json:"archetype"`
	Trigger                    string                     `json:"trigger,omitempty"`
	Scope                      string                     `json:"scope,omitempty"`
	MentalModel                string                     `json:"mental_model,omitempty"`
	Goal                       string                     `json:"goal,omitempty"`
	Command                    string                     `json:"command,omitempty"`
	Steps                      []ArchitectureFlowStep     `json:"steps"`
	Branches                   []ArchitectureFlowBranch   `json:"branches"`
	Slots                      []flowproof.Slot           `json:"slots"`
	TransitionIDs              []string                   `json:"transition_ids"`
	Status                     string                     `json:"status,omitempty"`
	EvidenceBasis              string                     `json:"evidence_basis,omitempty"`
	WhyInspect                 string                     `json:"why_inspect,omitempty"`
	GroundedAreas              int                        `json:"grounded_areas,omitempty"`
	TotalAreas                 int                        `json:"total_areas,omitempty"`
	FrontierSummary            string                     `json:"frontier_summary,omitempty"`
	ParticipatingComponentIDs  []componentmap.ComponentID `json:"participating_component_ids,omitempty"`
	StartSurfaceID             string                     `json:"start_surface_id,omitempty"`
	SeedSurfaceID              string                     `json:"seed_surface_id,omitempty"`
	TraceEvidenceSurfaceIDs    []string                   `json:"trace_evidence_surface_ids,omitempty"`
	RelatedComponentSurfaceIDs []string                   `json:"related_component_surface_ids,omitempty"`
	TraceQuality               flowproof.TraceQuality     `json:"trace_quality,omitempty"`
	CurrentFrontier            string                     `json:"current_frontier,omitempty"`
}

type ArchitectureFlowStep struct {
	ID                        string                          `json:"id"`
	Kind                      flowproof.AnchorKind            `json:"kind"`
	Label                     string                          `json:"label"`
	QualifiedName             string                          `json:"qualified_name,omitempty"`
	Location                  *evidence.Location              `json:"location,omitempty"`
	BranchID                  string                          `json:"branch_id,omitempty"`
	ComponentID               componentmap.ComponentID        `json:"component_id,omitempty"`
	ParticipatingComponentIDs []componentmap.ComponentID      `json:"participating_component_ids,omitempty"`
	Binding                   *componentmap.FlowAnchorBinding `json:"binding,omitempty"`
}

type ArchitectureFlowBranch struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	RootAnchorID  string   `json:"root_anchor_id,omitempty"`
	RootAnchorIDs []string `json:"root_anchor_ids,omitempty"`
	AnchorIDs     []string `json:"anchor_ids"`
}

type ArchitectureFlowEdge struct {
	ID           string                  `json:"id"`
	FlowID       componentmap.FlowID     `json:"flow_id"`
	From         string                  `json:"from"`
	To           string                  `json:"to"`
	Relation     evidence.RelationKind   `json:"relation"`
	Resolution   evidence.ResolutionKind `json:"resolution"`
	Invocation   evidence.InvocationMode `json:"invocation,omitempty"`
	Condition    *evidence.Condition     `json:"condition,omitempty"`
	Certainty    evidence.Certainty      `json:"certainty"`
	Evidence     evidence.Location       `json:"evidence"`
	Provider     string                  `json:"provider"`
	FromBranchID string                  `json:"from_branch_id"`
	ToBranchID   string                  `json:"to_branch_id"`
	CrossBranch  bool                    `json:"cross_branch"`
}

type ArchitectureFrontier struct {
	ID           string              `json:"id"`
	FlowID       componentmap.FlowID `json:"flow_id,omitempty"`
	Kind         string              `json:"kind"`
	AnchorID     string              `json:"anchor_id,omitempty"`
	TransitionID string              `json:"transition_id,omitempty"`
	Slot         flowproof.SlotKind  `json:"slot,omitempty"`
	Reason       string              `json:"reason"`
	Evidence     *evidence.Location  `json:"evidence,omitempty"`
}

type ArchitectureDiagnostic struct {
	ID       string                 `json:"id"`
	Source   string                 `json:"source"`
	Severity string                 `json:"severity"`
	Code     string                 `json:"code"`
	Message  string                 `json:"message"`
	FlowID   componentmap.FlowID    `json:"flow_id,omitempty"`
	Member   *componentmap.MemberID `json:"member,omitempty"`
}

type architectureCanvasIndex struct {
	memberComponents map[componentmap.MemberID][]componentmap.ComponentID
	bindings         map[architectureBindingKey]componentmap.FlowAnchorBinding
	flowNames        map[componentmap.FlowID]string
}

type architectureBindingKey struct {
	flowID   componentmap.FlowID
	anchorID string
}

type architectureProofView struct {
	anchors        map[string]flowproof.Anchor
	anchorOrder    []string
	transitions    []flowproof.Transition
	transitionByID map[string]flowproof.Transition
	slots          []flowproof.Slot
}

// ProjectArchitectureCanvas creates a presentation projection from already
// validated conceptual membership and explicit FlowProof sessions. It never
// infers component ownership from paths and never adds temporal edges.
func ProjectArchitectureCanvas(input ArchitectureCanvasInput) (ArchitectureCanvas, error) {
	if err := input.CandidateBundle.Validate(); err != nil {
		return ArchitectureCanvas{}, fmt.Errorf("architecture canvas: candidate bundle: %w", err)
	}
	if err := input.Landscape.Validate(input.CandidateBundle); err != nil {
		return ArchitectureCanvas{}, fmt.Errorf("architecture canvas: landscape: %w", err)
	}

	canvas := ArchitectureCanvas{
		Version:                ArchitectureCanvasVersion,
		LandscapeVersion:       componentmap.ContractVersion,
		FlowProofVersion:       flowproof.Version,
		Fallback:               input.Landscape.Fallback,
		FallbackReason:         input.Landscape.FallbackReason,
		ValidationOutcome:      input.Landscape.ValidationOutcome,
		ArchitectureSource:     input.Landscape.Source,
		ArchitectureLevel:      input.Landscape.Level,
		Normalizations:         append([]componentmap.NormalizationOperation(nil), input.Landscape.Normalizations...),
		OriginalProposalSHA256: input.Landscape.OriginalProposalSHA256,
		RepositoryArchetype:    input.CandidateBundle.RepositoryArchetype,
		GroundingMode:          input.CandidateBundle.GroundingMode,
		BehaviorAnchors:        append([]componentmap.BehaviorAnchor(nil), input.CandidateBundle.BehaviorAnchors...),
	}
	remainderComponentID, err := architectureLocalRemainderComponentID(input.Landscape)
	if err != nil {
		return ArchitectureCanvas{}, fmt.Errorf("architecture canvas: local remainder: %w", err)
	}
	canvas.LocalRemainderComponentID = remainderComponentID
	canvas.Title, canvas.Subtitle = architectureGroundingWording(canvas.ArchitectureSource, canvas.GroundingMode)
	index := projectArchitectureLandscape(input.CandidateBundle, input.Landscape, &canvas)
	projectArchitectureStructuralLocators(input.CandidateBundle, input.Landscape, &index, &canvas)
	projectArchitectureStructuralFacts(input.Landscape.Relations, index.memberComponents, &canvas)
	projectArchitectureFlows(input.Flows, index, &canvas)

	sort.Slice(canvas.StructuralFacts, func(i, j int) bool {
		return canvas.StructuralFacts[i].ID < canvas.StructuralFacts[j].ID
	})
	sort.Slice(canvas.StructuralLocators, func(i, j int) bool {
		left := canvas.StructuralLocators[i].Locator.ID
		right := canvas.StructuralLocators[j].Locator.ID
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.Value < right.Value
	})
	sort.Slice(canvas.StructuralEdges, func(i, j int) bool {
		return canvas.StructuralEdges[i].ID < canvas.StructuralEdges[j].ID
	})
	sort.Slice(canvas.Flows, func(i, j int) bool { return canvas.Flows[i].ID < canvas.Flows[j].ID })
	sort.Slice(canvas.FlowEdges, func(i, j int) bool {
		if canvas.FlowEdges[i].FlowID != canvas.FlowEdges[j].FlowID {
			return canvas.FlowEdges[i].FlowID < canvas.FlowEdges[j].FlowID
		}
		return canvas.FlowEdges[i].ID < canvas.FlowEdges[j].ID
	})
	sort.Slice(canvas.Frontiers, func(i, j int) bool { return canvas.Frontiers[i].ID < canvas.Frontiers[j].ID })
	sort.Slice(canvas.Diagnostics, func(i, j int) bool { return canvas.Diagnostics[i].ID < canvas.Diagnostics[j].ID })
	return canvas, nil
}

func architectureLocalRemainderComponentID(landscape componentmap.Landscape) (componentmap.ComponentID, error) {
	if len(landscape.LocalRemainderMemberIDs) == 0 {
		return "", nil
	}
	remainder := make(map[componentmap.MemberID]struct{}, len(landscape.LocalRemainderMemberIDs))
	for _, memberID := range landscape.LocalRemainderMemberIDs {
		remainder[memberID] = struct{}{}
	}
	var matched componentmap.ComponentID
	for _, subsystem := range landscape.Subsystems {
		if subsystem.Category != componentmap.SubsystemCategoryDiagnostic {
			continue
		}
		for _, component := range subsystem.Components {
			if len(component.Members) != len(remainder) {
				continue
			}
			exact := true
			for _, member := range component.Members {
				if _, exists := remainder[member.ID]; !exists {
					exact = false
					break
				}
			}
			if !exact {
				continue
			}
			if matched != "" {
				return "", fmt.Errorf("multiple diagnostic components match exact local remainder identities")
			}
			matched = component.ID
		}
	}
	if matched == "" {
		return "", fmt.Errorf("no diagnostic component matches exact local remainder identities")
	}
	return matched, nil
}

func projectArchitectureStructuralLocators(
	bundle componentmap.CandidateBundle,
	landscape componentmap.Landscape,
	index *architectureCanvasIndex,
	canvas *ArchitectureCanvas,
) {
	if len(landscape.StructuralLocators) == 0 {
		return
	}
	known := make(map[componentmap.MemberID]componentmap.Candidate, len(bundle.Candidates))
	for _, candidate := range bundle.Candidates {
		known[candidate.ID] = candidate
	}

	for _, locator := range landscape.StructuralLocators {
		participants := make([]componentmap.ComponentID, 0)
		// Exact conceptual ancestors provide local containment context.
		for parentID := locator.ParentID; parentID != nil; {
			parent, exists := known[*parentID]
			if !exists {
				break
			}
			if parent.Role == componentmap.CandidateRoleConceptualMember {
				participants = append(participants, index.memberComponents[parent.ID]...)
			}
			parentID = parent.ParentID
		}
		// Exact conceptual descendants provide the other side of a structural
		// source container. No path, name or proposal ordering participates.
		for _, candidate := range bundle.Candidates {
			if candidate.Role != componentmap.CandidateRoleConceptualMember {
				continue
			}
			for parentID := candidate.ParentID; parentID != nil; {
				if *parentID == locator.ID {
					participants = append(participants, index.memberComponents[candidate.ID]...)
					break
				}
				parent, exists := known[*parentID]
				if !exists {
					break
				}
				parentID = parent.ParentID
			}
		}
		participants = uniqueArchitectureComponentIDs(participants)
		index.memberComponents[locator.ID] = append([]componentmap.ComponentID(nil), participants...)
		canvas.StructuralLocators = append(canvas.StructuralLocators, ArchitectureStructuralLocator{
			Locator:                   cloneArchitectureCandidate(locator),
			ParticipatingComponentIDs: append([]componentmap.ComponentID(nil), participants...),
		})
	}
}

func projectArchitectureLandscape(
	bundle componentmap.CandidateBundle,
	landscape componentmap.Landscape,
	canvas *ArchitectureCanvas,
) architectureCanvasIndex {
	index := architectureCanvasIndex{
		memberComponents: make(map[componentmap.MemberID][]componentmap.ComponentID),
		bindings:         make(map[architectureBindingKey]componentmap.FlowAnchorBinding),
		flowNames:        make(map[componentmap.FlowID]string, len(bundle.Flows)),
	}
	for _, flow := range bundle.Flows {
		index.flowNames[flow.ID] = flow.Name
	}
	for _, diagnostic := range landscape.Diagnostics {
		canvas.Diagnostics = append(canvas.Diagnostics, architectureLandscapeDiagnostic(diagnostic))
	}
	for _, subsystem := range landscape.Subsystems {
		projected := ArchitectureSubsystem{
			ID: subsystem.ID, Name: subsystem.Name, Description: subsystem.Description, Category: subsystem.Category,
			SourceIDs: append([]componentmap.SubsystemID(nil), subsystem.SourceIDs...),
		}
		for _, component := range subsystem.Components {
			projected.ComponentIDs = append(projected.ComponentIDs, component.ID)
			members := append([]componentmap.Candidate(nil), component.Members...)
			participatingFlows := make(map[componentmap.FlowID]struct{})
			if component.ID != canvas.LocalRemainderComponentID {
				for _, member := range component.Members {
					index.memberComponents[member.ID] = append(index.memberComponents[member.ID], component.ID)
					for _, participation := range member.Participations {
						participatingFlows[participation.FlowID] = struct{}{}
					}
				}
			}
			canvas.Components = append(canvas.Components, ArchitectureComponent{
				ID: component.ID, SubsystemID: subsystem.ID,
				Name: component.Name, Description: component.Description, Members: members,
				ParticipatingFlowIDs: sortedArchitectureFlowIDs(participatingFlows),
				AnchorIDs:            append([]string(nil), component.AnchorIDs...), Hypothesis: component.Hypothesis,
				SourceIDs: append([]componentmap.ComponentID(nil), component.SourceIDs...),
			})
		}
		canvas.Subsystems = append(canvas.Subsystems, projected)
	}
	for _, binding := range landscape.AnchorBindings {
		index.bindings[architectureBindingKey{flowID: binding.FlowID, anchorID: binding.AnchorID}] = binding
	}
	return index
}

func projectArchitectureStructuralFacts(
	relations []componentmap.LocalRelation,
	memberComponents map[componentmap.MemberID][]componentmap.ComponentID,
	canvas *ArchitectureCanvas,
) {
	for _, relation := range relations {
		canvas.StructuralFacts = append(canvas.StructuralFacts, relation)
		if relation.Kind == componentmap.StructuralRelationPackageImport {
			continue
		}

		fromComponents := uniqueArchitectureComponentIDs(memberComponents[relation.From])
		toComponents := uniqueArchitectureComponentIDs(memberComponents[relation.To])
		if len(fromComponents) != 1 || len(toComponents) != 1 {
			canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
				"projection", "warning", "structural.non_unique_conceptual_endpoint",
				fmt.Sprintf("structural fact %q has no unique conceptual component endpoints", relation.ID), "", nil,
			))
			continue
		}
		if fromComponents[0] == toComponents[0] {
			continue
		}
		canvas.StructuralEdges = append(canvas.StructuralEdges, ArchitectureStructuralEdge{
			ID:              architectureStableID("structural-edge", relation.ID),
			FromComponentID: fromComponents[0],
			ToComponentID:   toComponents[0],
			Witness:         relation,
		})
	}
}

func architectureGroundingWording(source componentmap.ArchitectureSource, mode componentmap.GroundingMode) (string, string) {
	switch source {
	case componentmap.SourceLocalAnchors:
		return "Evidence-backed architecture skeleton", "Built from exact local architecture anchors"
	case componentmap.SourceLocalPackages:
		return "Repository architecture", "Built from exact local package and repository structure"
	case componentmap.SourcePackageFallback:
		return "Package landscape", "Behavioral grounding was insufficient or the architecture proposal was rejected"
	}
	switch mode {
	case componentmap.GroundingBehavior:
		return "Behavioral architecture", "Evidence-backed process and runtime responsibilities"
	case componentmap.GroundingMixed:
		return "Architecture hypotheses and grounded behavior", "Some areas remain package-derived"
	default:
		return "Conceptual architecture", "Model-assisted grouping of exact local repository members"
	}
}

func projectArchitectureFlows(
	inputs []ArchitectureFlowInput,
	index architectureCanvasIndex,
	canvas *ArchitectureCanvas,
) {
	counts := make(map[componentmap.FlowID]int, len(inputs))
	for _, input := range inputs {
		counts[input.ID]++
	}
	ordered := append([]ArchitectureFlowInput(nil), inputs...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	reportedDuplicate := make(map[componentmap.FlowID]struct{})
	for _, input := range ordered {
		if counts[input.ID] != 1 {
			if _, reported := reportedDuplicate[input.ID]; !reported {
				canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
					"projection", "error", "flow.duplicate_input",
					fmt.Sprintf("flow %q occurs more than once in canvas input", input.ID), input.ID, nil,
				))
				reportedDuplicate[input.ID] = struct{}{}
			}
			continue
		}
		flowName, known := index.flowNames[input.ID]
		if !known {
			canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
				"projection", "error", "flow.unknown_id",
				fmt.Sprintf("flow %q is absent from the validated candidate bundle", input.ID), input.ID, nil,
			))
			continue
		}
		if input.Session.Version != flowproof.SessionVersion || input.Session.Proof.Version != flowproof.Version {
			canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
				"projection", "warning", "flow.unsupported_proof_version",
				fmt.Sprintf(
					"flow %q has session/proof versions %d/%d; need %d/%d",
					input.ID, input.Session.Version, input.Session.Proof.Version,
					flowproof.SessionVersion, flowproof.Version,
				), input.ID, nil,
			))
			continue
		}
		if input.Session.Proof.ID != string(input.ID) {
			canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
				"projection", "error", "flow.proof_id_mismatch",
				fmt.Sprintf("flow %q contains proof id %q", input.ID, input.Session.Proof.ID), input.ID, nil,
			))
			continue
		}
		if input.Session.Proof.Archetype != flowproof.ArchetypeCLI &&
			input.Session.Proof.Archetype != flowproof.ArchetypeProcess {
			canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
				"projection", "error", "flow.unsupported_archetype",
				fmt.Sprintf("flow %q has unsupported archetype %q", input.ID, input.Session.Proof.Archetype), input.ID, nil,
			))
			continue
		}
		if input.Name != "" {
			flowName = input.Name
		}
		projectArchitectureFlow(input, flowName, index, canvas)
	}
}

func projectArchitectureFlow(
	input ArchitectureFlowInput,
	name string,
	index architectureCanvasIndex,
	canvas *ArchitectureCanvas,
) {
	proof := input.Session.Proof
	view := validateArchitectureProof(input.ID, proof, canvas)
	branchByAnchor, branches := assignArchitectureBranches(input.ID, view, canvas)
	flow := ArchitectureFlow{
		ID: input.ID, Name: name, Archetype: proof.Archetype,
		Trigger: input.Trigger, Scope: input.Scope,
		MentalModel: input.MentalModel, Goal: proof.Goal, Command: proof.Command,
		Branches:                branches,
		SeedSurfaceID:           proof.SeedSurfaceID,
		TraceEvidenceSurfaceIDs: append([]string(nil), proof.TraceEvidenceSurfaceIDs...),
		TraceQuality:            proof.TraceQuality,
		CurrentFrontier:         proof.CurrentFrontier,
	}
	if flow.TraceQuality == "" {
		flow.TraceQuality = flowproof.AssessTraceQuality(proof)
	}

	for _, anchorID := range view.anchorOrder {
		anchor := view.anchors[anchorID]
		step := ArchitectureFlowStep{
			ID: anchor.ID, Kind: anchor.Kind, Label: anchor.Label,
			QualifiedName: anchor.QualifiedName,
			Location:      cloneArchitectureLocation(anchor.Location),
			BranchID:      branchByAnchor[anchor.ID],
		}
		projectArchitectureAnchorBinding(input.ID, anchor, index, &step, canvas)
		flow.Steps = append(flow.Steps, step)
	}
	for _, slot := range view.slots {
		flow.Slots = append(flow.Slots, cloneArchitectureSlot(slot))
		if slot.Status != flowproof.SlotVerified && slot.Status != flowproof.SlotNotApplicable {
			reason := slot.Missing
			if reason == "" {
				reason = slot.Summary
			}
			if reason == "" {
				reason = "proof slot remains unresolved"
			}
			canvas.Frontiers = append(canvas.Frontiers, newArchitectureFrontier(
				input.ID, "proof_slot", "", "", slot.Kind, reason, nil,
			))
		}
	}
	for _, transition := range view.transitions {
		fromBranchID := branchByAnchor[transition.From]
		toBranchID := branchByAnchor[transition.To]
		if fromBranchID == "" || toBranchID == "" {
			canvas.Frontiers = append(canvas.Frontiers, newArchitectureFrontier(
				input.ID, "unassigned_edge_endpoint", "", transition.ID, "",
				"transition endpoint is outside an evidence-supported branch", &transition.Evidence,
			))
			continue
		}
		canvas.FlowEdges = append(canvas.FlowEdges, ArchitectureFlowEdge{
			ID: transition.ID, FlowID: input.ID, From: transition.From, To: transition.To,
			Relation: transition.Relation, Resolution: transition.Resolution,
			Invocation: transition.Invocation, Condition: cloneArchitectureCondition(transition.Condition),
			Certainty: transition.Certainty, Evidence: transition.Evidence, Provider: transition.Provider,
			FromBranchID: fromBranchID, ToBranchID: toBranchID, CrossBranch: fromBranchID != toBranchID,
		})
		flow.TransitionIDs = append(flow.TransitionIDs, transition.ID)
	}
	for _, warning := range proof.Warnings {
		canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
			"flowproof", "warning", "flowproof.warning", warning, input.ID, nil,
		))
	}
	sort.Slice(flow.Steps, func(i, j int) bool { return flow.Steps[i].ID < flow.Steps[j].ID })
	sort.Strings(flow.TransitionIDs)
	canvas.Flows = append(canvas.Flows, flow)
}

func validateArchitectureProof(
	flowID componentmap.FlowID,
	proof flowproof.Proof,
	canvas *ArchitectureCanvas,
) architectureProofView {
	view := architectureProofView{
		anchors:        make(map[string]flowproof.Anchor),
		transitionByID: make(map[string]flowproof.Transition),
	}
	anchorCounts := make(map[string]int, len(proof.Anchors))
	for _, anchor := range proof.Anchors {
		anchorCounts[anchor.ID]++
	}
	reportedAnchors := make(map[string]struct{})
	for _, anchor := range proof.Anchors {
		if anchorCounts[anchor.ID] != 1 {
			if _, reported := reportedAnchors[anchor.ID]; !reported {
				canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
					"flowproof", "error", "flow.duplicate_anchor_id",
					fmt.Sprintf("flow %q repeats anchor id %q", flowID, anchor.ID), flowID, nil,
				))
				canvas.Frontiers = append(canvas.Frontiers, newArchitectureFrontier(
					flowID, "invalid_anchor", anchor.ID, "", "", "anchor id is not unique", anchor.Location,
				))
				reportedAnchors[anchor.ID] = struct{}{}
			}
			continue
		}
		if err := validateArchitectureAnchor(anchor); err != nil {
			canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
				"flowproof", "error", "flow.invalid_anchor",
				fmt.Sprintf("flow %q anchor %q: %v", flowID, anchor.ID, err), flowID, nil,
			))
			canvas.Frontiers = append(canvas.Frontiers, newArchitectureFrontier(
				flowID, "invalid_anchor", anchor.ID, "", "", err.Error(), anchor.Location,
			))
			continue
		}
		view.anchors[anchor.ID] = cloneArchitectureAnchor(anchor)
		view.anchorOrder = append(view.anchorOrder, anchor.ID)
	}

	transitionCounts := make(map[string]int, len(proof.Transitions))
	for _, transition := range proof.Transitions {
		transitionCounts[transition.ID]++
	}
	reportedTransitions := make(map[string]struct{})
	for _, transition := range proof.Transitions {
		if transitionCounts[transition.ID] != 1 {
			if _, reported := reportedTransitions[transition.ID]; !reported {
				canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
					"flowproof", "error", "flow.duplicate_transition_id",
					fmt.Sprintf("flow %q repeats transition id %q", flowID, transition.ID), flowID, nil,
				))
				canvas.Frontiers = append(canvas.Frontiers, newArchitectureFrontier(
					flowID, "invalid_transition", "", transition.ID, "", "transition id is not unique", &transition.Evidence,
				))
				reportedTransitions[transition.ID] = struct{}{}
			}
			continue
		}
		if err := validateArchitectureTransition(transition); err != nil {
			canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
				"flowproof", "error", "flow.invalid_transition",
				fmt.Sprintf("flow %q transition %q: %v", flowID, transition.ID, err), flowID, nil,
			))
			canvas.Frontiers = append(canvas.Frontiers, newArchitectureFrontier(
				flowID, "invalid_transition", "", transition.ID, "", err.Error(), &transition.Evidence,
			))
			continue
		}
		_, fromExists := view.anchors[transition.From]
		_, toExists := view.anchors[transition.To]
		if !fromExists || !toExists {
			missingID := transition.From
			if fromExists {
				missingID = transition.To
			}
			canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
				"flowproof", "error", "flow.missing_transition_endpoint",
				fmt.Sprintf("flow %q transition %q references missing anchor %q", flowID, transition.ID, missingID), flowID, nil,
			))
			canvas.Frontiers = append(canvas.Frontiers, newArchitectureFrontier(
				flowID, "missing_endpoint", missingID, transition.ID, "",
				"transition endpoint is absent or invalid", &transition.Evidence,
			))
			continue
		}
		cloned := cloneArchitectureTransition(transition)
		view.transitions = append(view.transitions, cloned)
		view.transitionByID[transition.ID] = cloned
		if transition.Resolution == evidence.ResolutionUnknown || transition.Resolution == evidence.ResolutionUnresolved {
			canvas.Frontiers = append(canvas.Frontiers, newArchitectureFrontier(
				flowID, "unresolved_transition", "", transition.ID, "",
				"transition resolution remains unresolved", &transition.Evidence,
			))
		}
	}

	slotCounts := make(map[flowproof.SlotKind]int, len(proof.Slots))
	for _, slot := range proof.Slots {
		slotCounts[slot.Kind]++
	}
	reportedSlots := make(map[flowproof.SlotKind]struct{})
	for _, slot := range proof.Slots {
		if slotCounts[slot.Kind] != 1 {
			if _, reported := reportedSlots[slot.Kind]; !reported {
				canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
					"flowproof", "error", "flow.duplicate_slot_kind",
					fmt.Sprintf("flow %q repeats slot kind %q", flowID, slot.Kind), flowID, nil,
				))
				reportedSlots[slot.Kind] = struct{}{}
			}
			continue
		}
		if err := validateArchitectureSlot(slot); err != nil {
			canvas.Diagnostics = append(canvas.Diagnostics, newArchitectureDiagnostic(
				"flowproof", "error", "flow.invalid_slot",
				fmt.Sprintf("flow %q slot %q: %v", flowID, slot.Kind, err), flowID, nil,
			))
			canvas.Frontiers = append(canvas.Frontiers, newArchitectureFrontier(
				flowID, "invalid_slot", "", "", slot.Kind, err.Error(), nil,
			))
			continue
		}
		view.slots = append(view.slots, cloneArchitectureSlot(slot))
	}
	return view
}

func assignArchitectureBranches(
	flowID componentmap.FlowID,
	view architectureProofView,
	canvas *ArchitectureCanvas,
) (map[string]string, []ArchitectureFlowBranch) {
	transitionsByFrom := make(map[string][]flowproof.Transition)
	incoming := make(map[string]int)
	for _, transition := range view.transitions {
		transitionsByFrom[transition.From] = append(transitionsByFrom[transition.From], transition)
		incoming[transition.To]++
	}
	for anchorID := range transitionsByFrom {
		sort.Slice(transitionsByFrom[anchorID], func(i, j int) bool {
			return transitionsByFrom[anchorID][i].ID < transitionsByFrom[anchorID][j].ID
		})
	}

	taskAnchorIDs := make([]string, 0)
	for anchorID, anchor := range view.anchors {
		if anchor.Kind == flowproof.AnchorTask {
			taskAnchorIDs = append(taskAnchorIDs, anchorID)
		}
	}
	sort.Strings(taskAnchorIDs)

	mainSeedSet := make(map[string]struct{})
	for _, slot := range view.slots {
		for _, evidenceID := range slot.EvidenceIDs {
			if anchor, exists := view.anchors[evidenceID]; exists && anchor.Kind != flowproof.AnchorTask {
				mainSeedSet[evidenceID] = struct{}{}
				continue
			}
			if transition, exists := view.transitionByID[evidenceID]; exists {
				if anchor := view.anchors[transition.From]; anchor.Kind != flowproof.AnchorTask {
					mainSeedSet[transition.From] = struct{}{}
				}
			}
		}
	}
	for sourceID := range transitionsByFrom {
		if incoming[sourceID] == 0 && view.anchors[sourceID].Kind != flowproof.AnchorTask {
			mainSeedSet[sourceID] = struct{}{}
		}
	}
	mainSeeds := sortedArchitectureStringSet(mainSeedSet)
	mainBranchID := architectureMainBranchID(flowID)
	reachability := make(map[string]map[string]struct{}, len(taskAnchorIDs)+1)
	reachability[mainBranchID] = architectureBranchReachability(
		mainSeeds, "", view.anchors, transitionsByFrom,
	)
	for _, taskAnchorID := range taskAnchorIDs {
		branchID := architectureTaskBranchID(flowID, taskAnchorID)
		reachability[branchID] = architectureBranchReachability(
			[]string{taskAnchorID}, taskAnchorID, view.anchors, transitionsByFrom,
		)
	}

	branchByAnchor := make(map[string]string, len(view.anchors))
	sharedBranchID := architectureSharedBranchID(flowID)
	branchIDs := make([]string, 0, len(reachability))
	for branchID := range reachability {
		branchIDs = append(branchIDs, branchID)
	}
	sort.Strings(branchIDs)
	for anchorID, anchor := range view.anchors {
		if anchor.Kind == flowproof.AnchorTask {
			branchByAnchor[anchorID] = architectureTaskBranchID(flowID, anchorID)
			continue
		}
		var reachedBy []string
		for _, branchID := range branchIDs {
			if _, reached := reachability[branchID][anchorID]; reached {
				reachedBy = append(reachedBy, branchID)
			}
		}
		switch len(reachedBy) {
		case 1:
			branchByAnchor[anchorID] = reachedBy[0]
		case 0:
			// Preserved below as an explicit frontier.
		default:
			branchByAnchor[anchorID] = sharedBranchID
		}
	}

	anchorsByBranch := make(map[string][]string)
	for anchorID := range view.anchors {
		branchID := branchByAnchor[anchorID]
		if branchID == "" {
			canvas.Frontiers = append(canvas.Frontiers, newArchitectureFrontier(
				flowID, "unassigned_branch", anchorID, "", "",
				"anchor is not reachable from a proof slot, graph root, or task root", view.anchors[anchorID].Location,
			))
			continue
		}
		anchorsByBranch[branchID] = append(anchorsByBranch[branchID], anchorID)
	}

	branches := make([]ArchitectureFlowBranch, 0, len(taskAnchorIDs)+1)
	if anchorIDs := anchorsByBranch[mainBranchID]; len(anchorIDs) > 0 {
		sort.Strings(anchorIDs)
		rootIDs := make([]string, 0, len(mainSeeds))
		for _, seed := range mainSeeds {
			if branchByAnchor[seed] == mainBranchID {
				rootIDs = append(rootIDs, seed)
			}
		}
		branches = append(branches, ArchitectureFlowBranch{
			ID: mainBranchID, Kind: "main", RootAnchorIDs: rootIDs, AnchorIDs: anchorIDs,
		})
	}
	for _, taskAnchorID := range taskAnchorIDs {
		branchID := architectureTaskBranchID(flowID, taskAnchorID)
		anchorIDs := anchorsByBranch[branchID]
		sort.Strings(anchorIDs)
		branches = append(branches, ArchitectureFlowBranch{
			ID: branchID, Kind: "task", RootAnchorID: taskAnchorID,
			RootAnchorIDs: []string{taskAnchorID}, AnchorIDs: anchorIDs,
		})
	}
	if anchorIDs := anchorsByBranch[sharedBranchID]; len(anchorIDs) > 0 {
		sort.Strings(anchorIDs)
		branches = append(branches, ArchitectureFlowBranch{
			ID: sharedBranchID, Kind: "shared", AnchorIDs: anchorIDs,
		})
	}
	return branchByAnchor, branches
}

func architectureBranchReachability(
	seeds []string,
	taskRootID string,
	anchors map[string]flowproof.Anchor,
	transitionsByFrom map[string][]flowproof.Transition,
) map[string]struct{} {
	queue := append([]string(nil), seeds...)
	seen := make(map[string]struct{}, len(queue))
	for len(queue) > 0 {
		anchorID := queue[0]
		queue = queue[1:]
		if _, duplicate := seen[anchorID]; duplicate {
			continue
		}
		seen[anchorID] = struct{}{}
		_, exists := anchors[anchorID]
		if !exists {
			continue
		}
		for _, transition := range transitionsByFrom[anchorID] {
			target := anchors[transition.To]
			if target.Kind == flowproof.AnchorTask && transition.To != taskRootID {
				continue
			}
			queue = append(queue, transition.To)
		}
	}
	return seen
}

func projectArchitectureAnchorBinding(
	flowID componentmap.FlowID,
	anchor flowproof.Anchor,
	index architectureCanvasIndex,
	step *ArchitectureFlowStep,
	canvas *ArchitectureCanvas,
) {
	binding, exists := index.bindings[architectureBindingKey{flowID: flowID, anchorID: anchor.ID}]
	if !exists {
		canvas.Frontiers = append(canvas.Frontiers, newArchitectureFrontier(
			flowID, "unassigned_component", anchor.ID, "", "",
			"anchor has no exact validated flow-anchor binding", anchor.Location,
		))
		return
	}
	participants := uniqueArchitectureComponentIDs(index.memberComponents[binding.MemberID])
	bindingCopy := binding
	step.Binding = &bindingCopy
	step.ParticipatingComponentIDs = participants
	if len(participants) > 0 {
		return
	}
	canvas.Frontiers = append(canvas.Frontiers, newArchitectureFrontier(
		flowID, "unassigned_component", anchor.ID, "", "",
		"bound member has no participating conceptual component", anchor.Location,
	))
}

func validateArchitectureAnchor(anchor flowproof.Anchor) error {
	if err := validateArchitectureOpaque("anchor id", anchor.ID); err != nil {
		return err
	}
	if !validArchitectureAnchorKind(anchor.Kind) {
		return fmt.Errorf("invalid anchor kind %q", anchor.Kind)
	}
	if strings.TrimSpace(anchor.Label) == "" {
		return fmt.Errorf("anchor label is empty")
	}
	if anchor.Location != nil {
		if err := validateArchitectureLocation(*anchor.Location, false); err != nil {
			return fmt.Errorf("anchor location: %w", err)
		}
	}
	return nil
}

func validateArchitectureTransition(transition flowproof.Transition) error {
	if err := validateArchitectureOpaque("transition id", transition.ID); err != nil {
		return err
	}
	if err := validateArchitectureOpaque("transition source", transition.From); err != nil {
		return err
	}
	if err := validateArchitectureOpaque("transition target", transition.To); err != nil {
		return err
	}
	if transition.From == transition.To {
		return fmt.Errorf("transition is self-referential")
	}
	if !transition.Relation.Valid() {
		return fmt.Errorf("invalid relation %q", transition.Relation)
	}
	if !transition.Resolution.Valid() {
		return fmt.Errorf("invalid resolution %q", transition.Resolution)
	}
	if !transition.Invocation.Valid() {
		return fmt.Errorf("invalid invocation %q", transition.Invocation)
	}
	if !transition.Certainty.Valid() {
		return fmt.Errorf("invalid certainty %q", transition.Certainty)
	}
	if err := validateArchitectureLocation(transition.Evidence, true); err != nil {
		return fmt.Errorf("transition evidence: %w", err)
	}
	if strings.TrimSpace(transition.Provider) == "" {
		return fmt.Errorf("transition provider is empty")
	}
	if transition.Condition != nil {
		if strings.TrimSpace(transition.Condition.Expression) == "" && !transition.Condition.ExpressionOmitted {
			return fmt.Errorf("condition expression is empty")
		}
		if strings.TrimSpace(transition.Condition.Expression) != "" && transition.Condition.ExpressionOmitted {
			return fmt.Errorf("condition expression cannot be present and omitted")
		}
		if err := validateArchitectureLocation(transition.Condition.Location, true); err != nil {
			return fmt.Errorf("condition location: %w", err)
		}
	}
	return nil
}

func validateArchitectureSlot(slot flowproof.Slot) error {
	if !validArchitectureSlotKind(slot.Kind) {
		return fmt.Errorf("invalid slot kind %q", slot.Kind)
	}
	if !validArchitectureSlotStatus(slot.Status) {
		return fmt.Errorf("invalid slot status %q", slot.Status)
	}
	for _, evidenceID := range slot.EvidenceIDs {
		if err := validateArchitectureOpaque("slot evidence id", evidenceID); err != nil {
			return err
		}
	}
	if slot.ApplicabilityReason != "" &&
		slot.ApplicabilityReason != flowproof.ApplicabilityNoConcurrentLifecycleInScope {
		return fmt.Errorf("invalid applicability reason %q", slot.ApplicabilityReason)
	}
	if slot.Status == flowproof.SlotNotApplicable {
		if slot.Kind != flowproof.SlotConcurrency ||
			slot.ApplicabilityReason != flowproof.ApplicabilityNoConcurrentLifecycleInScope ||
			len(slot.Provenance) == 0 {
			return fmt.Errorf("not-applicable slot lacks a supported reason and provenance")
		}
	}
	return nil
}

func validateArchitectureLocation(location evidence.Location, requireLine bool) error {
	if strings.TrimSpace(location.Path) == "" {
		return fmt.Errorf("evidence path is empty")
	}
	for _, char := range location.Path {
		if char < 0x20 || char == 0x7f {
			return fmt.Errorf("evidence path contains control characters")
		}
	}
	if location.Line < 0 || location.Column < 0 || location.EndLine < 0 || location.EndColumn < 0 {
		return fmt.Errorf("source coordinates are negative")
	}
	if requireLine && location.Line == 0 {
		return fmt.Errorf("source line is missing")
	}
	if location.Line == 0 && (location.Column != 0 || location.EndLine != 0 || location.EndColumn != 0) {
		return fmt.Errorf("path-only location has partial coordinates")
	}
	if location.EndLine > 0 && location.EndLine < location.Line {
		return fmt.Errorf("source range ends before it starts")
	}
	if location.EndLine == 0 && location.EndColumn != 0 {
		return fmt.Errorf("source range has end column without end line")
	}
	return nil
}

func validateArchitectureOpaque(field, value string) error {
	if value == "" || strings.TrimSpace(value) != value || len(value) > 256 {
		return fmt.Errorf("%s is empty, malformed, or too long", field)
	}
	for _, char := range value {
		if char < 0x21 || char == 0x7f {
			return fmt.Errorf("%s contains whitespace or control characters", field)
		}
	}
	return nil
}

func validArchitectureAnchorKind(kind flowproof.AnchorKind) bool {
	switch kind {
	case flowproof.AnchorCommand,
		flowproof.AnchorFunction,
		flowproof.AnchorMethod,
		flowproof.AnchorCallsite,
		flowproof.AnchorOperation,
		flowproof.AnchorTask:
		return true
	default:
		return false
	}
}

func validArchitectureSlotKind(kind flowproof.SlotKind) bool {
	switch kind {
	case flowproof.SlotTrigger,
		flowproof.SlotEntrypoint,
		flowproof.SlotDispatch,
		flowproof.SlotApplicationCallable,
		flowproof.SlotCoreOperation,
		flowproof.SlotIOBoundary,
		flowproof.SlotConcurrency,
		flowproof.SlotTermination:
		return true
	default:
		return false
	}
}

func validArchitectureSlotStatus(status flowproof.SlotStatus) bool {
	switch status {
	case flowproof.SlotMissing,
		flowproof.SlotPartial,
		flowproof.SlotVerified,
		flowproof.SlotUnresolved,
		flowproof.SlotNotApplicable:
		return true
	default:
		return false
	}
}

func architectureMainBranchID(flowID componentmap.FlowID) string {
	return "flow:" + string(flowID) + ":main"
}

func architectureTaskBranchID(flowID componentmap.FlowID, anchorID string) string {
	return "flow:" + string(flowID) + ":task:" + anchorID
}

func architectureSharedBranchID(flowID componentmap.FlowID) string {
	return "flow:" + string(flowID) + ":shared"
}

func architectureLandscapeDiagnostic(diagnostic componentmap.Diagnostic) ArchitectureDiagnostic {
	var member *componentmap.MemberID
	if diagnostic.Member != nil {
		copy := *diagnostic.Member
		member = &copy
	}
	severity := "info"
	if diagnostic.Severity == componentmap.FindingFatal {
		severity = "error"
	} else if diagnostic.Severity == componentmap.FindingRecoverable {
		severity = "warning"
	}
	return newArchitectureDiagnostic(
		"landscape", severity, diagnostic.Code, diagnostic.Message, "", member,
	)
}

func newArchitectureDiagnostic(
	source, severity, code, message string,
	flowID componentmap.FlowID,
	member *componentmap.MemberID,
) ArchitectureDiagnostic {
	memberKey := ""
	var memberCopy *componentmap.MemberID
	if member != nil {
		copy := *member
		memberCopy = &copy
		memberKey = string(copy.Kind) + ":" + copy.Value
	}
	return ArchitectureDiagnostic{
		ID: architectureStableID(
			"diagnostic", source, severity, code, message, string(flowID), memberKey,
		),
		Source: source, Severity: severity, Code: code, Message: message,
		FlowID: flowID, Member: memberCopy,
	}
}

func newArchitectureFrontier(
	flowID componentmap.FlowID,
	kind, anchorID, transitionID string,
	slot flowproof.SlotKind,
	reason string,
	evidenceLocation *evidence.Location,
) ArchitectureFrontier {
	return ArchitectureFrontier{
		ID: architectureStableID(
			"frontier", string(flowID), kind, anchorID, transitionID, string(slot), reason,
		),
		FlowID: flowID, Kind: kind, AnchorID: anchorID, TransitionID: transitionID,
		Slot: slot, Reason: reason, Evidence: cloneArchitectureLocation(evidenceLocation),
	}
}

func architectureStableID(kind string, parts ...string) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "architecture-canvas/%d/%s\n", ArchitectureCanvasVersion, kind)
	for _, part := range parts {
		fmt.Fprintf(hash, "%d:%s\n", len(part), part)
	}
	return kind + "-" + hex.EncodeToString(hash.Sum(nil))
}

func cloneArchitectureAnchor(anchor flowproof.Anchor) flowproof.Anchor {
	cloned := anchor
	cloned.Location = cloneArchitectureLocation(anchor.Location)
	return cloned
}

func cloneArchitectureTransition(transition flowproof.Transition) flowproof.Transition {
	cloned := transition
	cloned.Condition = cloneArchitectureCondition(transition.Condition)
	return cloned
}

func cloneArchitectureCondition(condition *evidence.Condition) *evidence.Condition {
	if condition == nil {
		return nil
	}
	cloned := *condition
	return &cloned
}

func cloneArchitectureSlot(slot flowproof.Slot) flowproof.Slot {
	cloned := slot
	cloned.EvidenceIDs = append([]string(nil), slot.EvidenceIDs...)
	cloned.Provenance = cloneArchitectureProvenance(slot.Provenance)
	return cloned
}

func cloneArchitectureCandidate(candidate componentmap.Candidate) componentmap.Candidate {
	cloned := candidate
	if candidate.ParentID != nil {
		parentID := *candidate.ParentID
		cloned.ParentID = &parentID
	}
	if candidate.Facts != nil {
		cloned.Facts = make([]componentmap.LocalFact, len(candidate.Facts))
		for index, fact := range candidate.Facts {
			cloned.Facts[index] = cloneArchitectureLocalFact(fact)
		}
	}
	if candidate.Participations != nil {
		cloned.Participations = make([]componentmap.FlowParticipation, len(candidate.Participations))
		for index, participation := range candidate.Participations {
			cloned.Participations[index] = participation
			cloned.Participations[index].Evidence = cloneArchitectureLocalFact(participation.Evidence)
		}
	}
	return cloned
}

func cloneArchitectureLocalFact(fact componentmap.LocalFact) componentmap.LocalFact {
	cloned := fact
	cloned.Location = cloneArchitectureLocation(fact.Location)
	cloned.Provenance = cloneArchitectureProvenance(fact.Provenance)
	for index := range cloned.Provenance {
		cloned.Provenance[index].Location = cloneArchitectureLocation(cloned.Provenance[index].Location)
	}
	return cloned
}

func cloneArchitectureLocation(location *evidence.Location) *evidence.Location {
	if location == nil {
		return nil
	}
	cloned := *location
	return &cloned
}

func cloneArchitectureProvenance(provenance []evidence.Provenance) []evidence.Provenance {
	return append([]evidence.Provenance(nil), provenance...)
}

func uniqueArchitectureComponentIDs(values []componentmap.ComponentID) []componentmap.ComponentID {
	set := make(map[componentmap.ComponentID]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]componentmap.ComponentID, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func sortedArchitectureFlowIDs(set map[componentmap.FlowID]struct{}) []componentmap.FlowID {
	result := make([]componentmap.FlowID, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func sortedArchitectureStringSet(set map[string]struct{}) []string {
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
