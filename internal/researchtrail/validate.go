package researchtrail

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (t Trail) Validate() error {
	if t.Version != Version {
		return fmt.Errorf("research trail: unsupported version %d", t.Version)
	}
	if !sha256Pattern.MatchString(t.Binding.RepositoryStateSHA256) ||
		!sha256Pattern.MatchString(t.Binding.ReportSHA256) {
		return fmt.Errorf("research trail: repository/report binding is invalid")
	}
	if strings.TrimSpace(t.Goal.ID) == "" || strings.TrimSpace(t.Goal.Kind) == "" ||
		strings.TrimSpace(t.Goal.Objective) == "" {
		return fmt.Errorf("research trail: goal is incomplete")
	}
	if strings.TrimSpace(t.Component.ID) == "" || strings.TrimSpace(t.Component.Name) == "" ||
		strings.TrimSpace(t.Component.Purpose) == "" || t.Component.Basis != SupportOrientationHypothesis {
		return fmt.Errorf("research trail: component focus is incomplete or overstated")
	}
	if strings.TrimSpace(t.PrimaryQuestionNodeID) == "" {
		return fmt.Errorf("research trail: primary question is missing")
	}

	nodes := make(map[string]Node, len(t.Nodes))
	primaryCount := 0
	componentCount := 0
	for index, node := range t.Nodes {
		if err := validateNode(node); err != nil {
			return fmt.Errorf("research trail: nodes[%d]: %w", index, err)
		}
		if _, exists := nodes[node.ID]; exists {
			return fmt.Errorf("research trail: duplicate node id %q", node.ID)
		}
		nodes[node.ID] = node
		if node.Primary {
			primaryCount++
		}
		if node.Kind == NodeComponent {
			componentCount++
			if node.SourceID != t.Component.ID || node.Label != t.Component.Name || node.Detail != t.Component.Purpose {
				return fmt.Errorf("research trail: component node differs from the trail focus")
			}
		}
	}
	primary, exists := nodes[t.PrimaryQuestionNodeID]
	if !exists || primary.Kind != NodeQuestion || !primary.Primary || primaryCount != 1 {
		return fmt.Errorf("research trail: primary question binding is invalid")
	}
	if componentCount != 1 {
		return fmt.Errorf("research trail: expected exactly one component node")
	}

	edges := make(map[string]struct{}, len(t.Edges))
	incomingSupport := make(map[string]int)
	for index, edge := range t.Edges {
		if _, exists := edges[edge.ID]; exists {
			return fmt.Errorf("research trail: duplicate edge id %q", edge.ID)
		}
		edges[edge.ID] = struct{}{}
		from, fromExists := nodes[edge.From]
		to, toExists := nodes[edge.To]
		if !fromExists || !toExists {
			return fmt.Errorf("research trail: edges[%d] references an unknown node", index)
		}
		if err := validateEdge(edge, from, to); err != nil {
			return fmt.Errorf("research trail: edges[%d]: %w", index, err)
		}
		if edge.Kind == EdgeSupports {
			incomingSupport[edge.To]++
		}
	}
	for _, node := range t.Nodes {
		if (node.Kind == NodeClaim || node.Kind == NodeLifecycleStep) && incomingSupport[node.ID] == 0 {
			return fmt.Errorf("research trail: grounded node %q has no support edge", node.ID)
		}
	}
	if err := validateSteps(t, nodes); err != nil {
		return err
	}
	for index, diagnostic := range t.Diagnostics {
		if strings.TrimSpace(diagnostic.Stage) == "" || strings.TrimSpace(diagnostic.Code) == "" ||
			strings.TrimSpace(diagnostic.Message) == "" {
			return fmt.Errorf("research trail: diagnostics[%d] is incomplete", index)
		}
	}
	return nil
}

func (i LocalIndex) Validate(trail Trail) error {
	if err := trail.Validate(); err != nil {
		return err
	}
	if i.Version != LocalIndexVersion {
		return fmt.Errorf("research trail: unsupported local index version %d", i.Version)
	}
	nodes := make(map[string]Node, len(trail.Nodes))
	want := make(map[string]struct{})
	for _, node := range trail.Nodes {
		nodes[node.ID] = node
		if node.Kind == NodeEvidence || node.Kind == NodeFrontier || node.Kind == NodeExactSymbol {
			want[node.ID] = struct{}{}
		}
	}
	for _, edge := range trail.Edges {
		if edge.Kind == EdgeSelects {
			want[edge.To] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(i.Entries))
	for index, entry := range i.Entries {
		node, exists := nodes[entry.NodeID]
		_, wanted := want[entry.NodeID]
		if !wanted || !exists || !locatableNode(node.Kind) || node.SourceID != entry.SourceID {
			return fmt.Errorf("research trail: local index entry[%d] is not addressable", index)
		}
		if _, exists := seen[entry.NodeID]; exists {
			return fmt.Errorf("research trail: duplicate locator for node %q", entry.NodeID)
		}
		if !validPath(entry.Path) || entry.StartLine <= 0 || entry.EndLine < entry.StartLine ||
			entry.Column < 0 || len(entry.Origins) == 0 {
			return fmt.Errorf("research trail: local index entry[%d] has an invalid locator", index)
		}
		for originIndex, origin := range entry.Origins {
			if !validOrigin(origin) {
				return fmt.Errorf("research trail: local index entry[%d].origins[%d] is invalid", index, originIndex)
			}
		}
		seen[entry.NodeID] = struct{}{}
	}
	if len(seen) != len(want) {
		return fmt.Errorf("research trail: local index does not locate every evidence and frontier node")
	}
	return nil
}

func locatableNode(kind NodeKind) bool {
	return kind == NodeEvidence || kind == NodeFrontier || kind == NodeExactSymbol ||
		kind == NodePlannerFile || kind == NodePlannerSymbol
}

func validOrigin(origin Origin) bool {
	if strings.TrimSpace(origin.Artifact) == "" || strings.TrimSpace(origin.LocalID) == "" {
		return false
	}
	switch origin.Stage {
	case "componentstudy":
		return origin.Round == 0 && origin.ProbeID == "" &&
			strings.TrimSpace(origin.Source) != "" && strings.TrimSpace(origin.Operation) != ""
	case "componentprobe":
		return (origin.Round == 1 || origin.Round == 2) && strings.TrimSpace(origin.ProbeID) != ""
	default:
		return false
	}
}

func validateNode(node Node) error {
	if strings.TrimSpace(node.ID) == "" || strings.TrimSpace(node.Label) == "" || !node.Kind.valid() {
		return fmt.Errorf("node identity is incomplete")
	}
	if node.Section != "" && !node.Section.valid() {
		return fmt.Errorf("invalid section %q", node.Section)
	}
	if node.Basis != "" && !node.Basis.valid() {
		return fmt.Errorf("invalid support basis %q", node.Basis)
	}
	switch node.Kind {
	case NodeComponent:
		if node.Basis != SupportOrientationHypothesis {
			return fmt.Errorf("component node overstates its support")
		}
	case NodeQuestion, NodeClaim, NodeLifecycleStep:
		if node.Basis != SupportModelSynthesis {
			return fmt.Errorf("model-derived node lost its synthesis basis")
		}
	case NodeEvidence, NodeFrontier:
		if node.SourceID == "" || node.Basis == "" || node.Basis == SupportModelSynthesis {
			return fmt.Errorf("locatable node has no evidence support basis")
		}
	case NodeExactSymbol:
		if node.SourceID == "" || node.Basis != SupportStaticActiveBuild {
			return fmt.Errorf("exact symbol lost its active-build support")
		}
	}
	return nil
}

func validateEdge(edge Edge, from, to Node) error {
	if strings.TrimSpace(edge.ID) == "" || !edge.Kind.valid() || !edge.Basis.valid() {
		return fmt.Errorf("edge identity is incomplete")
	}
	switch edge.Kind {
	case EdgeFramesQuestion:
		if from.Kind != NodeComponent || to.Kind != NodeQuestion || edge.Basis != SupportOrientationHypothesis ||
			edge.Source != to.SourceID {
			return fmt.Errorf("invalid component-to-question edge")
		}
	case EdgeSelects:
		selectedKind := to.Kind == NodePlannerFile || to.Kind == NodePlannerSymbol
		if from.Kind != NodeComponent || !selectedKind || edge.Basis != SupportOrientationHypothesis ||
			edge.Source != to.SourceID {
			return fmt.Errorf("invalid planner selection edge")
		}
	case EdgeMotivates:
		if !from.Kind.plannerCandidate() || to.Kind != NodeQuestion || edge.Basis != SupportOrientationHypothesis ||
			edge.Source != from.SourceID {
			return fmt.Errorf("invalid planner motivation edge")
		}
	case EdgeAnswers:
		if from.Kind != NodeQuestion || (to.Kind != NodeClaim && to.Kind != NodeLifecycleStep) ||
			edge.Basis != SupportModelSynthesis || edge.Source != to.SourceID {
			return fmt.Errorf("invalid answer edge")
		}
	case EdgeSupports:
		if from.Kind != NodeEvidence || (to.Kind != NodeClaim && to.Kind != NodeLifecycleStep) ||
			edge.Basis != from.Basis || edge.Source != from.SourceID {
			return fmt.Errorf("invalid evidence support edge")
		}
	case EdgeTeachingNext:
		if from.Kind != NodeLifecycleStep || to.Kind != NodeLifecycleStep || edge.Basis != SupportModelSynthesis ||
			edge.Source != to.SourceID {
			return fmt.Errorf("invalid teaching-order edge")
		}
	case EdgeLeavesOpen:
		if (from.Kind != NodeClaim && from.Kind != NodeLifecycleStep) || to.Kind != NodeFrontier ||
			edge.Basis != to.Basis || edge.Source != to.SourceID {
			return fmt.Errorf("invalid claim frontier edge")
		}
	case EdgeFrontier:
		if from.Kind != NodeQuestion || to.Kind != NodeFrontier || edge.Basis != to.Basis || edge.Source != to.SourceID {
			return fmt.Errorf("invalid unresolved frontier edge")
		}
	}
	return nil
}

func (b SupportBasis) valid() bool {
	switch b {
	case SupportOrientationHypothesis, SupportStaticActiveBuild, SupportSource,
		SupportTestNavigation, SupportModelSynthesis, SupportWorkflowRecord:
		return true
	default:
		return false
	}
}

func (k NodeKind) valid() bool {
	switch k {
	case NodeComponent, NodeQuestion, NodePlannerAnchor, NodePlannerFile, NodePlannerSymbol,
		NodePlannerEvidence, NodeExactSymbol, NodeEvidence, NodeLifecycleStep, NodeClaim, NodeFrontier:
		return true
	default:
		return false
	}
}

func validateSteps(trail Trail, nodes map[string]Node) error {
	if len(trail.Steps) != 3 && len(trail.Steps) != 4 {
		return fmt.Errorf("research trail: expected plan, probe, optional second probe, and teach steps")
	}
	wantKinds := []StepKind{StepPlan, StepProbe, StepTeach}
	wantRounds := []int{0, 1, 0}
	wantIDs := []string{"step:plan", "step:probe:1", "step:teach"}
	if len(trail.Steps) == 4 {
		wantKinds = []StepKind{StepPlan, StepProbe, StepProbe, StepTeach}
		wantRounds = []int{0, 1, 2, 0}
		wantIDs = []string{"step:plan", "step:probe:1", "step:probe:2", "step:teach"}
	}
	exactSeen := make(map[string]struct{})
	for index, step := range trail.Steps {
		if step.ID != wantIDs[index] || step.Kind != wantKinds[index] || step.Round != wantRounds[index] ||
			strings.TrimSpace(step.Status) == "" || strings.TrimSpace(step.Label) == "" {
			return fmt.Errorf("research trail: step %d is invalid", index)
		}
		if step.Kind == StepPlan && step.Status != "planned" {
			return fmt.Errorf("research trail: plan step has invalid status")
		}
		if step.Kind == StepTeach && step.Status != "taught" {
			return fmt.Errorf("research trail: teach step has invalid status")
		}
		if step.Kind == StepProbe && step.Status != "connected" && step.Status != "frontier" && step.Status != "blocked" {
			return fmt.Errorf("research trail: probe step has invalid status")
		}
		if err := validateStepNodeIDs(step.FocusNodeIDs, nodes, func(node Node) bool {
			switch step.Kind {
			case StepPlan:
				return node.Kind == NodeQuestion || node.Kind == NodePlannerFile || node.Kind == NodePlannerSymbol
			case StepProbe:
				if node.Kind == NodeExactSymbol {
					exactSeen[node.ID] = struct{}{}
					return true
				}
				return false
			case StepTeach:
				return node.Kind == NodeClaim || node.Kind == NodeLifecycleStep
			default:
				return false
			}
		}); err != nil {
			return fmt.Errorf("research trail: step %q focus: %w", step.ID, err)
		}
		if err := validateStepNodeIDs(step.EvidenceNodeIDs, nodes, func(node Node) bool {
			return node.Kind == NodeEvidence
		}); err != nil {
			return fmt.Errorf("research trail: step %q evidence: %w", step.ID, err)
		}
	}
	for _, node := range trail.Nodes {
		if node.Kind == NodeExactSymbol {
			if _, exists := exactSeen[node.ID]; !exists {
				return fmt.Errorf("research trail: exact symbol %q is not bound to a probe step", node.ID)
			}
		}
	}
	return validateTransitions(trail, nodes)
}

func validateStepNodeIDs(ids []string, nodes map[string]Node, allowed func(Node) bool) error {
	previous := ""
	for _, id := range ids {
		if id == "" || (previous != "" && id <= previous) {
			return fmt.Errorf("node ids must be uniquely sorted")
		}
		node, exists := nodes[id]
		if !exists || !allowed(node) {
			return fmt.Errorf("node %q has an invalid kind", id)
		}
		previous = id
	}
	return nil
}

func validateTransitions(trail Trail, nodes map[string]Node) error {
	if len(trail.Transitions) != len(trail.Steps)-1 {
		return fmt.Errorf("research trail: stage transition count is invalid")
	}
	seen := make(map[string]struct{}, len(trail.Transitions))
	for index, transition := range trail.Transitions {
		from := trail.Steps[index]
		to := trail.Steps[index+1]
		if strings.TrimSpace(transition.ID) == "" || transition.FromStepID != from.ID || transition.ToStepID != to.ID {
			return fmt.Errorf("research trail: transition %d is not bound to adjacent steps", index)
		}
		if _, exists := seen[transition.ID]; exists {
			return fmt.Errorf("research trail: duplicate transition id %q", transition.ID)
		}
		seen[transition.ID] = struct{}{}
		accepted := from.Kind == StepProbe && from.Round == 1 && to.Kind == StepProbe && to.Round == 2
		if !accepted {
			if transition.Kind != TransitionContinues || transition.Basis != SupportWorkflowRecord ||
				transition.SourceNodeID != "" || transition.TargetNodeID != "" || transition.SourceID != "" {
				return fmt.Errorf("research trail: ordinary transition %d is invalid", index)
			}
			continue
		}
		if transition.Kind != TransitionAcceptedFrontier || transition.Basis != SupportStaticActiveBuild {
			return fmt.Errorf("research trail: round-two transition is not an accepted frontier")
		}
		source, sourceExists := nodes[transition.SourceNodeID]
		if !sourceExists || source.Kind != NodeFrontier || transition.SourceID != source.SourceID {
			return fmt.Errorf("research trail: accepted frontier source is invalid")
		}
		if transition.TargetNodeID == "" {
			if to.Status != "blocked" {
				return fmt.Errorf("research trail: accepted frontier has no exact round-two target")
			}
			continue
		}
		target, targetExists := nodes[transition.TargetNodeID]
		if !targetExists || target.Kind != NodeExactSymbol || !containsSorted(to.FocusNodeIDs, target.ID) {
			return fmt.Errorf("research trail: accepted frontier target is invalid")
		}
	}
	return nil
}

func containsSorted(values []string, target string) bool {
	index := sort.SearchStrings(values, target)
	return index < len(values) && values[index] == target
}

func (k NodeKind) plannerCandidate() bool {
	return k == NodePlannerAnchor || k == NodePlannerFile || k == NodePlannerSymbol || k == NodePlannerEvidence
}

func (s Section) valid() bool {
	switch s {
	case SectionPlanning, SectionMentalModel, SectionLifecycle, SectionBoundaries, SectionDesignNotes,
		SectionFailuresObservability, SectionTestsChecks, SectionUnknowns, SectionNextDive:
		return true
	default:
		return false
	}
}

func (k EdgeKind) valid() bool {
	switch k {
	case EdgeFramesQuestion, EdgeSelects, EdgeMotivates, EdgeAnswers, EdgeSupports,
		EdgeTeachingNext, EdgeLeavesOpen, EdgeFrontier:
		return true
	default:
		return false
	}
}

func validPath(path string) bool {
	local := filepath.FromSlash(path)
	return path != "" && !strings.Contains(path, `\`) && !filepath.IsAbs(local) &&
		filepath.IsLocal(local) && filepath.ToSlash(filepath.Clean(local)) == path
}
