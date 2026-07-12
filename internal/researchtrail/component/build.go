// Package component adapts the bounded component study, probe, and teacher
// artifacts into one presentation-neutral research trail.
package component

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/componentprobe"
	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/componentteach"
	"github.com/dvordrova/repomap/internal/researchtrail"
)

type Input struct {
	Binding     researchtrail.Binding
	StudyBundle componentstudy.Bundle
	StudyResult componentstudy.Result
	Round1      componentprobe.Bundle
	Round2      *componentprobe.Bundle
	TeachBundle componentteach.Bundle
	TeachIndex  componentteach.Index
	TeachResult componentteach.ParseResult
}

// Build validates every stage boundary before combining the artifacts. The
// returned LocalIndex remains separate so a future renderer can omit local
// paths without changing the trail contract.
func Build(input Input) (researchtrail.Trail, researchtrail.LocalIndex, error) {
	if err := validateInput(input); err != nil {
		return researchtrail.Trail{}, researchtrail.LocalIndex{}, err
	}

	componentNodeID := nodeID("component", input.StudyBundle.Component.ID)
	primaryQuestionNodeID := nodeID("question", input.StudyResult.Plan.PrimaryQuestionID)
	trail := researchtrail.Trail{
		Version: researchtrail.Version,
		Binding: input.Binding,
		Goal: researchtrail.Goal{
			ID:        input.StudyBundle.Goal.ID,
			Kind:      string(input.StudyBundle.Goal.Kind),
			Objective: input.StudyBundle.Goal.Objective,
		},
		Component: researchtrail.Component{
			ID:      input.StudyBundle.Component.ID,
			Name:    input.StudyBundle.Component.Name,
			Purpose: input.StudyBundle.Component.Purpose,
			Basis:   researchtrail.SupportOrientationHypothesis,
		},
		Framing:               input.StudyResult.Plan.Framing,
		PrimaryQuestionNodeID: primaryQuestionNodeID,
		Nodes: []researchtrail.Node{{
			ID:       componentNodeID,
			SourceID: input.StudyBundle.Component.ID,
			Kind:     researchtrail.NodeComponent,
			Section:  researchtrail.SectionPlanning,
			Label:    input.StudyBundle.Component.Name,
			Detail:   input.StudyBundle.Component.Purpose,
			Basis:    researchtrail.SupportOrientationHypothesis,
		}},
		Edges:       []researchtrail.Edge{},
		Diagnostics: diagnostics(input),
	}

	candidateNodes := addPlannerNodes(&trail, input, componentNodeID)
	addQuestions(&trail, input, componentNodeID, candidateNodes)
	evidenceNodes, err := addTeacherEvidence(&trail, input.TeachBundle)
	if err != nil {
		return researchtrail.Trail{}, researchtrail.LocalIndex{}, err
	}
	frontierNodes, err := addFrontier(&trail, input.TeachBundle)
	if err != nil {
		return researchtrail.Trail{}, researchtrail.LocalIndex{}, err
	}
	acceptedLocators := addAcceptedFrontier(&trail, input, frontierNodes)
	addClaims(&trail, input.TeachResult.Report, primaryQuestionNodeID, evidenceNodes, frontierNodes)
	addUnreferencedFrontier(&trail, primaryQuestionNodeID, frontierNodes)
	probeProjection := addProbeNodes(&trail, input)
	addSteps(&trail, input, candidateNodes, evidenceNodes, frontierNodes, probeProjection)

	index := buildLocalIndex(input, candidateNodes, evidenceNodes, frontierNodes)
	index.Entries = append(index.Entries, acceptedLocators...)
	index.Entries = append(index.Entries, probeProjection.locators...)
	sortTrail(&trail)
	sortLocalIndex(&index)
	if err := trail.Validate(); err != nil {
		return researchtrail.Trail{}, researchtrail.LocalIndex{}, fmt.Errorf("component research trail: validate trail: %w", err)
	}
	if err := index.Validate(trail); err != nil {
		return researchtrail.Trail{}, researchtrail.LocalIndex{}, fmt.Errorf("component research trail: validate local index: %w", err)
	}
	return trail, index, nil
}

func validateInput(input Input) error {
	if !validSHA256(input.Binding.RepositoryStateSHA256) || !validSHA256(input.Binding.ReportSHA256) {
		return fmt.Errorf("component research trail: repository/report binding is invalid")
	}
	if err := input.StudyBundle.Validate(); err != nil {
		return fmt.Errorf("component research trail: invalid study bundle: %w", err)
	}
	if err := input.StudyResult.Plan.Validate(input.StudyBundle); err != nil {
		return fmt.Errorf("component research trail: invalid study result: %w", err)
	}
	if err := input.Round1.Validate(); err != nil {
		return fmt.Errorf("component research trail: invalid probe round 1: %w", err)
	}
	if input.Round1.Round != componentprobe.RoundInitial {
		return fmt.Errorf("component research trail: first probe bundle must be round 1")
	}
	if input.Round2 != nil {
		if err := input.Round2.ValidateAgainst(input.Round1); err != nil {
			return fmt.Errorf("component research trail: invalid probe round 2: %w", err)
		}
	}
	if err := input.TeachBundle.Validate(); err != nil {
		return fmt.Errorf("component research trail: invalid teacher bundle: %w", err)
	}
	if err := input.TeachIndex.Validate(input.TeachBundle); err != nil {
		return fmt.Errorf("component research trail: invalid teacher index: %w", err)
	}
	if err := input.TeachResult.Report.Validate(input.TeachBundle); err != nil {
		return fmt.Errorf("component research trail: invalid teacher result: %w", err)
	}

	primary, ok := primaryQuestion(input.StudyResult.Plan)
	if !ok {
		return fmt.Errorf("component research trail: primary planner question is missing")
	}
	if !reflect.DeepEqual(input.Round1.Focus.Goal, input.StudyBundle.Goal) ||
		!reflect.DeepEqual(input.Round1.Focus.Component, input.StudyBundle.Component) ||
		!reflect.DeepEqual(input.Round1.Focus.PrimaryQuestion, primary) ||
		!reflect.DeepEqual(input.Round1.Focus.SelectedFiles, input.StudyResult.Plan.SelectedFiles) {
		return fmt.Errorf("component research trail: probe focus differs from the study plan")
	}
	selectedSymbols := make(map[string]componentstudy.SymbolCandidate, len(input.StudyResult.Plan.SelectedSymbols))
	for _, selected := range input.StudyResult.Plan.SelectedSymbols {
		selectedSymbols[selected.ID] = selected
	}
	for _, probe := range input.Round1.SymbolProbes {
		selected, exists := selectedSymbols[probe.SelectedSymbol.ID]
		if !exists || !reflect.DeepEqual(selected, probe.SelectedSymbol) {
			return fmt.Errorf("component research trail: probe symbol is absent from the study plan")
		}
	}
	if input.TeachBundle.GoalObjective != input.Round1.Focus.Goal.Objective ||
		input.TeachBundle.Component.Name != input.Round1.Focus.Component.Name ||
		input.TeachBundle.Component.PurposeHypothesis != input.Round1.Focus.Component.Purpose ||
		input.TeachBundle.PrimaryQuestion.ID != primary.ID ||
		input.TeachBundle.PrimaryQuestion.Question != primary.Question ||
		input.TeachBundle.PrimaryQuestion.Why != primary.Why {
		return fmt.Errorf("component research trail: teacher focus differs from the probe focus")
	}
	if err := validateIndexOrigins(input); err != nil {
		return err
	}
	return nil
}

func validateIndexOrigins(input Input) error {
	known := make(map[originKey]struct{})
	addRoundOrigins(known, input.Round1)
	if input.Round2 != nil {
		addRoundOrigins(known, *input.Round2)
	}
	for _, entry := range input.TeachIndex.Entries {
		for _, origin := range entry.Origins {
			key := originKey{
				round: origin.Round, probeID: origin.ProbeID,
				artifact: string(origin.Artifact), localID: origin.LocalID,
			}
			if _, exists := known[key]; !exists {
				return fmt.Errorf("component research trail: teacher index origin is absent from the probe chain")
			}
		}
	}
	return nil
}

type originKey struct {
	round    int
	probeID  string
	artifact string
	localID  string
}

func addRoundOrigins(known map[originKey]struct{}, round componentprobe.Bundle) {
	for _, probe := range round.SymbolProbes {
		for _, ref := range probe.EvidenceIndex {
			known[originKey{
				round: round.Round, probeID: ref.Origin.ProbeID,
				artifact: string(ref.Origin.Artifact), localID: ref.Origin.LocalID,
			}] = struct{}{}
		}
	}
}

func addPlannerNodes(
	trail *researchtrail.Trail,
	input Input,
	componentNodeID string,
) map[string]string {
	used := make(map[string]struct{})
	for _, question := range input.StudyResult.Plan.Questions {
		for _, id := range question.EvidenceIDs {
			used[id] = struct{}{}
		}
	}
	for _, candidate := range input.StudyResult.Plan.SelectedFiles {
		used[candidate.ID] = struct{}{}
	}
	for _, candidate := range input.StudyResult.Plan.SelectedSymbols {
		used[candidate.ID] = struct{}{}
	}

	bySource := make(map[string]string, len(used))
	for _, candidate := range input.StudyBundle.Anchors {
		if _, ok := used[candidate.ID]; !ok {
			continue
		}
		addPlannerNode(trail, bySource, researchtrail.Node{
			ID:        nodeID("planner", candidate.ID),
			SourceID:  candidate.ID,
			Kind:      researchtrail.NodePlannerAnchor,
			Section:   researchtrail.SectionPlanning,
			Label:     locationLabel(candidate.Path, candidate.Line),
			Detail:    candidate.Reason,
			Certainty: string(candidate.Certainty),
		})
	}
	for _, candidate := range input.StudyBundle.Files {
		if _, ok := used[candidate.ID]; !ok {
			continue
		}
		addPlannerNode(trail, bySource, researchtrail.Node{
			ID:        nodeID("planner", candidate.ID),
			SourceID:  candidate.ID,
			Kind:      researchtrail.NodePlannerFile,
			Section:   researchtrail.SectionPlanning,
			Label:     candidate.Path,
			Detail:    candidate.Reason,
			Certainty: string(candidate.Certainty),
		})
	}
	for _, candidate := range input.StudyBundle.Symbols {
		if _, ok := used[candidate.ID]; !ok {
			continue
		}
		addPlannerNode(trail, bySource, researchtrail.Node{
			ID:        nodeID("planner", candidate.ID),
			SourceID:  candidate.ID,
			Kind:      researchtrail.NodePlannerSymbol,
			Section:   researchtrail.SectionPlanning,
			Label:     candidate.Name,
			Detail:    locationLabel(candidate.Path, candidate.Line) + " · " + candidate.Reason,
			Certainty: string(candidate.Certainty),
		})
	}
	for _, candidate := range input.StudyBundle.Evidence {
		if _, ok := used[candidate.ID]; !ok {
			continue
		}
		addPlannerNode(trail, bySource, researchtrail.Node{
			ID:        nodeID("planner", candidate.ID),
			SourceID:  candidate.ID,
			Kind:      researchtrail.NodePlannerEvidence,
			Section:   researchtrail.SectionPlanning,
			Label:     candidate.Statement,
			Detail:    candidate.Reason,
			Certainty: string(candidate.Certainty),
		})
	}

	selected := make(map[string]struct{}, len(input.StudyResult.Plan.SelectedFiles)+len(input.StudyResult.Plan.SelectedSymbols))
	for _, candidate := range input.StudyResult.Plan.SelectedFiles {
		selected[candidate.ID] = struct{}{}
	}
	for _, candidate := range input.StudyResult.Plan.SelectedSymbols {
		selected[candidate.ID] = struct{}{}
	}
	for sourceID := range selected {
		candidateNodeID := bySource[sourceID]
		trail.Edges = append(trail.Edges, newEdge(
			researchtrail.EdgeSelects,
			componentNodeID,
			candidateNodeID,
			researchtrail.SupportOrientationHypothesis,
			sourceID,
		))
	}
	return bySource
}

func addPlannerNode(trail *researchtrail.Trail, bySource map[string]string, node researchtrail.Node) {
	trail.Nodes = append(trail.Nodes, node)
	bySource[node.SourceID] = node.ID
}

func addQuestions(
	trail *researchtrail.Trail,
	input Input,
	componentNodeID string,
	candidateNodes map[string]string,
) {
	for _, question := range input.StudyResult.Plan.Questions {
		questionNodeID := nodeID("question", question.ID)
		trail.Nodes = append(trail.Nodes, researchtrail.Node{
			ID:       questionNodeID,
			SourceID: question.ID,
			Kind:     researchtrail.NodeQuestion,
			Section:  researchtrail.SectionPlanning,
			Label:    question.Question,
			Detail:   question.Why,
			Basis:    researchtrail.SupportModelSynthesis,
			Primary:  question.ID == input.StudyResult.Plan.PrimaryQuestionID,
		})
		trail.Edges = append(trail.Edges, newEdge(
			researchtrail.EdgeFramesQuestion,
			componentNodeID,
			questionNodeID,
			researchtrail.SupportOrientationHypothesis,
			question.ID,
		))
		for _, sourceID := range question.EvidenceIDs {
			trail.Edges = append(trail.Edges, newEdge(
				researchtrail.EdgeMotivates,
				candidateNodes[sourceID],
				questionNodeID,
				researchtrail.SupportOrientationHypothesis,
				sourceID,
			))
		}
	}
}

func addTeacherEvidence(
	trail *researchtrail.Trail,
	bundle componentteach.Bundle,
) (map[string]string, error) {
	result := make(map[string]string, len(bundle.Evidence))
	for _, item := range bundle.Evidence {
		basis, err := teachBasis(item.SupportBasis)
		if err != nil {
			return nil, err
		}
		node := researchtrail.Node{
			ID:             nodeID("evidence", item.ID),
			SourceID:       item.ID,
			Kind:           researchtrail.NodeEvidence,
			Label:          item.Summary,
			Detail:         evidenceDetail(item),
			Basis:          basis,
			NavigationOnly: item.NavigationOnly,
		}
		trail.Nodes = append(trail.Nodes, node)
		result[item.ID] = node.ID
	}
	return result, nil
}

func addFrontier(
	trail *researchtrail.Trail,
	bundle componentteach.Bundle,
) (map[string]frontierNode, error) {
	result := make(map[string]frontierNode, len(bundle.UnresolvedFrontiers))
	for _, hint := range bundle.UnresolvedFrontiers {
		basis, err := teachBasis(hint.SupportBasis)
		if err != nil {
			return nil, err
		}
		node := researchtrail.Node{
			ID:             nodeID("frontier", hint.ID),
			SourceID:       hint.ID,
			Kind:           researchtrail.NodeFrontier,
			Section:        researchtrail.SectionNextDive,
			Label:          hint.Name,
			Detail:         strings.Join([]string{hint.Kind, hint.Direction, hint.EntityKind}, " · "),
			Basis:          basis,
			NavigationOnly: hint.NavigationOnly,
		}
		trail.Nodes = append(trail.Nodes, node)
		result[hint.ID] = frontierNode{id: node.ID, basis: basis}
	}
	return result, nil
}

type frontierNode struct {
	id         string
	basis      researchtrail.SupportBasis
	referenced bool
	accepted   bool
}

type reportSection struct {
	section researchtrail.Section
	kind    researchtrail.NodeKind
	items   []componentteach.Item
}

func addClaims(
	trail *researchtrail.Trail,
	report componentteach.Report,
	primaryQuestionNodeID string,
	evidenceNodes map[string]string,
	frontierNodes map[string]frontierNode,
) {
	sections := []reportSection{
		{section: researchtrail.SectionMentalModel, kind: researchtrail.NodeClaim, items: report.MentalModel},
		{section: researchtrail.SectionLifecycle, kind: researchtrail.NodeLifecycleStep, items: report.LifecycleSteps},
		{section: researchtrail.SectionBoundaries, kind: researchtrail.NodeClaim, items: report.Boundaries},
		{section: researchtrail.SectionDesignNotes, kind: researchtrail.NodeClaim, items: report.DesignNotes},
		{section: researchtrail.SectionFailuresObservability, kind: researchtrail.NodeClaim, items: report.FailuresAndObservability},
		{section: researchtrail.SectionTestsChecks, kind: researchtrail.NodeClaim, items: report.TestsAndChecks},
		{section: researchtrail.SectionUnknowns, kind: researchtrail.NodeClaim, items: report.Unknowns},
		{section: researchtrail.SectionNextDive, kind: researchtrail.NodeClaim, items: report.NextDive},
	}
	for _, section := range sections {
		var previousLifecycle string
		for _, item := range section.items {
			claimNodeID := nodeID("claim", item.ID)
			trail.Nodes = append(trail.Nodes, researchtrail.Node{
				ID:       claimNodeID,
				SourceID: item.ID,
				Kind:     section.kind,
				Section:  section.section,
				Label:    item.Text,
				Basis:    researchtrail.SupportModelSynthesis,
			})
			trail.Edges = append(trail.Edges, newEdge(
				researchtrail.EdgeAnswers,
				primaryQuestionNodeID,
				claimNodeID,
				researchtrail.SupportModelSynthesis,
				item.ID,
			))
			for _, evidenceID := range item.EvidenceIDs {
				evidenceNodeID := evidenceNodes[evidenceID]
				basis := trailNodeBasis(trail.Nodes, evidenceNodeID)
				trail.Edges = append(trail.Edges, newEdge(
					researchtrail.EdgeSupports,
					evidenceNodeID,
					claimNodeID,
					basis,
					evidenceID,
				))
			}
			for _, frontierID := range item.FrontierIDs {
				frontier := frontierNodes[frontierID]
				trail.Edges = append(trail.Edges, newEdge(
					researchtrail.EdgeLeavesOpen,
					claimNodeID,
					frontier.id,
					frontier.basis,
					frontierID,
				))
				frontier.referenced = true
				frontierNodes[frontierID] = frontier
			}
			if section.kind == researchtrail.NodeLifecycleStep && previousLifecycle != "" {
				trail.Edges = append(trail.Edges, newEdge(
					researchtrail.EdgeTeachingNext,
					previousLifecycle,
					claimNodeID,
					researchtrail.SupportModelSynthesis,
					item.ID,
				))
			}
			if section.kind == researchtrail.NodeLifecycleStep {
				previousLifecycle = claimNodeID
			}
		}
	}
}

func addUnreferencedFrontier(
	trail *researchtrail.Trail,
	primaryQuestionNodeID string,
	frontierNodes map[string]frontierNode,
) {
	for sourceID, frontier := range frontierNodes {
		if frontier.referenced || frontier.accepted {
			continue
		}
		trail.Edges = append(trail.Edges, newEdge(
			researchtrail.EdgeFrontier,
			primaryQuestionNodeID,
			frontier.id,
			frontier.basis,
			sourceID,
		))
	}
}

func addAcceptedFrontier(
	trail *researchtrail.Trail,
	input Input,
	frontierNodes map[string]frontierNode,
) []researchtrail.LocatorEntry {
	if input.Round2 == nil || input.Round2.Parent == nil {
		return []researchtrail.LocatorEntry{}
	}
	acceptedID := input.Round2.Parent.AcceptedFrontierID
	for _, candidate := range input.Round1.Frontier {
		if candidate.ID != acceptedID {
			continue
		}
		node := researchtrail.Node{
			ID: nodeID("frontier", candidate.ID), SourceID: candidate.ID,
			Kind: researchtrail.NodeFrontier, Section: researchtrail.SectionNextDive,
			Label:  candidate.Name,
			Detail: strings.Join([]string{string(candidate.Kind), string(candidate.Direction), string(candidate.EntityKind)}, " · "),
			Basis:  researchtrail.SupportStaticActiveBuild, NavigationOnly: candidate.NavigationOnly,
		}
		if existing, exists := frontierNodes[candidate.ID]; exists {
			existing.accepted = true
			frontierNodes[candidate.ID] = existing
			return []researchtrail.LocatorEntry{}
		} else {
			trail.Nodes = append(trail.Nodes, node)
			frontierNodes[candidate.ID] = frontierNode{
				id: node.ID, basis: researchtrail.SupportStaticActiveBuild, accepted: true,
			}
		}
		origins := make([]researchtrail.Origin, 0, len(candidate.Origins))
		for _, origin := range candidate.Origins {
			origins = append(origins, researchtrail.Origin{
				Stage: "componentprobe", Round: 1, ProbeID: origin.ProbeID,
				Artifact: string(origin.Artifact), LocalID: origin.LocalID,
			})
		}
		return []researchtrail.LocatorEntry{{
			NodeID: node.ID, SourceID: candidate.ID, Path: candidate.Location.Path,
			StartLine: candidate.Location.Line, EndLine: max(candidate.Location.Line, candidate.Location.EndLine),
			Column: candidate.Location.Column, Origins: origins,
		}}
	}
	return []researchtrail.LocatorEntry{}
}

type probeProjection struct {
	nodesByRound map[int][]string
	locators     []researchtrail.LocatorEntry
}

func addProbeNodes(trail *researchtrail.Trail, input Input) probeProjection {
	projection := probeProjection{
		nodesByRound: make(map[int][]string),
		locators:     []researchtrail.LocatorEntry{},
	}
	rounds := []componentprobe.Bundle{input.Round1}
	if input.Round2 != nil {
		rounds = append(rounds, *input.Round2)
	}
	for _, round := range rounds {
		for _, probe := range round.SymbolProbes {
			selected := probe.SelectedSymbol
			node := researchtrail.Node{
				ID: nodeID("exact", probe.ID), SourceID: selected.ID,
				Kind: researchtrail.NodeExactSymbol, Section: researchtrail.SectionPlanning,
				Label: selected.Name, Detail: selected.Kind,
				Basis: researchtrail.SupportStaticActiveBuild, Certainty: string(selected.Certainty),
			}
			trail.Nodes = append(trail.Nodes, node)
			projection.nodesByRound[round.Round] = append(projection.nodesByRound[round.Round], node.ID)
			projection.locators = append(projection.locators, researchtrail.LocatorEntry{
				NodeID: node.ID, SourceID: selected.ID, Path: selected.Path,
				StartLine: selected.Line, EndLine: selected.Line, Column: selected.Column,
				Origins: []researchtrail.Origin{{
					Stage: "componentprobe", Round: round.Round, ProbeID: probe.ID,
					Artifact: string(componentprobe.ArtifactStructural), LocalID: probe.Structural.Target.EvidenceID,
				}},
			})
		}
		sort.Strings(projection.nodesByRound[round.Round])
	}
	return projection
}

func addSteps(
	trail *researchtrail.Trail,
	input Input,
	candidateNodes map[string]string,
	evidenceNodes map[string]string,
	frontierNodes map[string]frontierNode,
	probes probeProjection,
) {
	planFocus := []string{trail.PrimaryQuestionNodeID}
	for _, selected := range input.StudyResult.Plan.SelectedFiles {
		planFocus = append(planFocus, candidateNodes[selected.ID])
	}
	for _, selected := range input.StudyResult.Plan.SelectedSymbols {
		planFocus = append(planFocus, candidateNodes[selected.ID])
	}
	planFocus = sortedUnique(planFocus)
	evidenceByRound := make(map[int][]string)
	for _, entry := range input.TeachIndex.Entries {
		if entry.Kind != componentteach.LocatorEvidence {
			continue
		}
		for _, origin := range entry.Origins {
			evidenceByRound[origin.Round] = append(evidenceByRound[origin.Round], evidenceNodes[entry.ID])
		}
	}
	for round := range evidenceByRound {
		evidenceByRound[round] = sortedUnique(evidenceByRound[round])
	}
	teachFocus := make([]string, 0)
	for _, node := range trail.Nodes {
		if node.Kind == researchtrail.NodeClaim || node.Kind == researchtrail.NodeLifecycleStep {
			teachFocus = append(teachFocus, node.ID)
		}
	}
	sort.Strings(teachFocus)
	trail.Steps = []researchtrail.Step{
		{ID: "step:plan", Kind: researchtrail.StepPlan, Status: "planned", Label: "Plan the bounded question", FocusNodeIDs: planFocus, EvidenceNodeIDs: []string{}},
		{ID: "step:probe:1", Kind: researchtrail.StepProbe, Round: 1, Status: string(input.Round1.Status), Label: "Collect exact local evidence", FocusNodeIDs: probes.nodesByRound[1], EvidenceNodeIDs: evidenceByRound[1]},
	}
	if input.Round2 != nil {
		trail.Steps = append(trail.Steps, researchtrail.Step{
			ID: "step:probe:2", Kind: researchtrail.StepProbe, Round: 2, Status: string(input.Round2.Status),
			Label: "Follow one accepted frontier", FocusNodeIDs: probes.nodesByRound[2], EvidenceNodeIDs: evidenceByRound[2],
		})
	}
	trail.Steps = append(trail.Steps, researchtrail.Step{
		ID: "step:teach", Kind: researchtrail.StepTeach, Status: "taught", Label: "Explain grounded findings",
		FocusNodeIDs: teachFocus, EvidenceNodeIDs: []string{},
	})
	trail.Transitions = []researchtrail.Transition{continuedTransition("step:plan", "step:probe:1")}
	if input.Round2 == nil {
		trail.Transitions = append(trail.Transitions, continuedTransition("step:probe:1", "step:teach"))
		return
	}
	accepted := frontierNodes[input.Round2.Parent.AcceptedFrontierID]
	target := ""
	if len(probes.nodesByRound[2]) > 0 {
		target = probes.nodesByRound[2][0]
	}
	trail.Transitions = append(trail.Transitions, researchtrail.Transition{
		ID:   transitionID(researchtrail.TransitionAcceptedFrontier, "step:probe:1", "step:probe:2", input.Round2.Parent.AcceptedFrontierID),
		Kind: researchtrail.TransitionAcceptedFrontier, FromStepID: "step:probe:1", ToStepID: "step:probe:2",
		SourceNodeID: accepted.id, TargetNodeID: target, SourceID: input.Round2.Parent.AcceptedFrontierID,
		Basis: researchtrail.SupportStaticActiveBuild,
	})
	trail.Transitions = append(trail.Transitions, continuedTransition("step:probe:2", "step:teach"))
}

func continuedTransition(from, to string) researchtrail.Transition {
	return researchtrail.Transition{
		ID: transitionID(researchtrail.TransitionContinues, from, to, ""), Kind: researchtrail.TransitionContinues,
		FromStepID: from, ToStepID: to, Basis: researchtrail.SupportWorkflowRecord,
	}
}

func transitionID(kind researchtrail.TransitionKind, from, to, source string) string {
	return stableID("transition", string(kind), from, to, source)
}

func sortedUnique(values []string) []string {
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if value == "" || (len(result) > 0 && result[len(result)-1] == value) {
			continue
		}
		result = append(result, value)
	}
	return result
}

func buildLocalIndex(
	input Input,
	candidateNodes map[string]string,
	evidenceNodes map[string]string,
	frontierNodes map[string]frontierNode,
) researchtrail.LocalIndex {
	result := researchtrail.LocalIndex{
		Version: researchtrail.LocalIndexVersion,
		Entries: make([]researchtrail.LocatorEntry, 0, len(input.TeachIndex.Entries)+
			len(input.StudyResult.Plan.SelectedFiles)+len(input.StudyResult.Plan.SelectedSymbols)),
	}
	for _, selected := range input.StudyResult.Plan.SelectedFiles {
		result.Entries = append(result.Entries, researchtrail.LocatorEntry{
			NodeID: candidateNodes[selected.ID], SourceID: selected.ID, Path: selected.Path,
			StartLine: 1, EndLine: 1,
			Origins: []researchtrail.Origin{plannerOrigin(
				"file_candidate",
				selected.ID,
				selected.Provenance,
			)},
		})
	}
	for _, selected := range input.StudyResult.Plan.SelectedSymbols {
		result.Entries = append(result.Entries, researchtrail.LocatorEntry{
			NodeID: candidateNodes[selected.ID], SourceID: selected.ID, Path: selected.Path,
			StartLine: selected.Line, EndLine: selected.Line, Column: selected.Column,
			Origins: []researchtrail.Origin{plannerOrigin(
				"symbol_candidate",
				selected.ID,
				selected.Provenance,
			)},
		})
	}
	for _, entry := range input.TeachIndex.Entries {
		nodeID := evidenceNodes[entry.ID]
		if entry.Kind == componentteach.LocatorFrontier {
			nodeID = frontierNodes[entry.ID].id
		}
		origins := make([]researchtrail.Origin, 0, len(entry.Origins))
		for _, origin := range entry.Origins {
			origins = append(origins, researchtrail.Origin{
				Stage: "componentprobe",
				Round: origin.Round, ProbeID: origin.ProbeID,
				Artifact: string(origin.Artifact), LocalID: origin.LocalID,
			})
		}
		result.Entries = append(result.Entries, researchtrail.LocatorEntry{
			NodeID: nodeID, SourceID: entry.ID, Path: entry.Path,
			StartLine: entry.StartLine, EndLine: entry.EndLine, Column: entry.Column,
			Origins: origins,
		})
	}
	return result
}

func plannerOrigin(
	artifact string,
	localID string,
	provenance componentstudy.Provenance,
) researchtrail.Origin {
	return researchtrail.Origin{
		Stage: "componentstudy", Artifact: artifact, LocalID: localID,
		Source: provenance.Source, Operation: provenance.Operation, Detail: provenance.Detail,
	}
}

func diagnostics(input Input) []researchtrail.Diagnostic {
	result := make([]researchtrail.Diagnostic, 0)
	for _, diagnostic := range input.StudyResult.Diagnostics {
		result = append(result, researchtrail.Diagnostic{
			Stage: "componentstudy", Code: diagnostic.Code,
			Field: diagnostic.Field, Message: diagnostic.Message,
		})
	}
	for _, unknown := range input.StudyResult.Plan.Unknowns {
		result = append(result, researchtrail.Diagnostic{
			Stage: "componentstudy", Code: "plan.unknown", Message: unknown,
		})
	}
	for _, warning := range input.StudyResult.Plan.Warnings {
		result = append(result, researchtrail.Diagnostic{
			Stage: "componentstudy", Code: "plan.warning", Message: warning,
		})
	}
	result = appendProbeDiagnostics(result, input.Round1)
	if input.Round2 != nil {
		result = appendProbeDiagnostics(result, *input.Round2)
	}
	for _, warning := range input.TeachBundle.Warnings {
		result = append(result, researchtrail.Diagnostic{
			Stage: "componentteach_bundle", Code: "bundle.warning", Message: warning,
		})
	}
	for _, diagnostic := range input.TeachResult.Diagnostics {
		result = append(result, researchtrail.Diagnostic{
			Stage: "componentteach", Code: diagnostic.Code,
			Field: diagnostic.Field, Message: diagnostic.Message,
		})
	}
	return result
}

func appendProbeDiagnostics(
	result []researchtrail.Diagnostic,
	bundle componentprobe.Bundle,
) []researchtrail.Diagnostic {
	stage := fmt.Sprintf("componentprobe_round_%d", bundle.Round)
	for _, warning := range bundle.Warnings {
		field := ""
		if warning.SymbolID != "" {
			field = "symbol:" + warning.SymbolID
		}
		result = append(result, researchtrail.Diagnostic{
			Stage: stage, Code: warning.Code, Field: field, Message: warning.Message,
		})
	}
	return result
}

func primaryQuestion(plan componentstudy.Plan) (componentstudy.Question, bool) {
	for _, question := range plan.Questions {
		if question.ID == plan.PrimaryQuestionID {
			return question, true
		}
	}
	return componentstudy.Question{}, false
}

func teachBasis(value componentteach.SupportBasis) (researchtrail.SupportBasis, error) {
	switch value {
	case componentteach.SupportOrientationHypothesis:
		return researchtrail.SupportOrientationHypothesis, nil
	case componentteach.SupportStaticActiveBuild:
		return researchtrail.SupportStaticActiveBuild, nil
	case componentteach.SupportSource:
		return researchtrail.SupportSource, nil
	case componentteach.SupportTestNavigation:
		return researchtrail.SupportTestNavigation, nil
	default:
		return "", fmt.Errorf("component research trail: unsupported teacher basis %q", value)
	}
}

func evidenceDetail(item componentteach.EvidenceItem) string {
	parts := make([]string, 0, 3)
	if item.Caller != "" {
		parts = append(parts, item.Caller)
	}
	if item.Callee != "" {
		parts = append(parts, item.Callee)
	}
	if item.Direction != "" {
		parts = append(parts, item.Direction)
	}
	return strings.Join(parts, " · ")
}

func locationLabel(path string, line int) string {
	if line <= 0 {
		return path
	}
	return fmt.Sprintf("%s:%d", path, line)
}

func trailNodeBasis(nodes []researchtrail.Node, id string) researchtrail.SupportBasis {
	for _, node := range nodes {
		if node.ID == id {
			return node.Basis
		}
	}
	return ""
}

func nodeID(kind, sourceID string) string {
	return kind + ":" + sourceID
}

func newEdge(
	kind researchtrail.EdgeKind,
	from string,
	to string,
	basis researchtrail.SupportBasis,
	source string,
) researchtrail.Edge {
	return researchtrail.Edge{
		ID: edgeID(kind, from, to, source), Kind: kind,
		From: from, To: to, Basis: basis, Source: source,
	}
}

func edgeID(kind researchtrail.EdgeKind, from, to, source string) string {
	return stableID("edge", string(kind), from, to, source)
}

func stableID(prefix string, parts ...string) string {
	hash := sha256.New()
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(part))
	}
	return fmt.Sprintf("%s-%x", prefix, hash.Sum(nil)[:10])
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func sortTrail(trail *researchtrail.Trail) {
	sort.Slice(trail.Nodes, func(i, j int) bool {
		if trail.Nodes[i].Kind != trail.Nodes[j].Kind {
			return trail.Nodes[i].Kind < trail.Nodes[j].Kind
		}
		return trail.Nodes[i].ID < trail.Nodes[j].ID
	})
	sort.Slice(trail.Edges, func(i, j int) bool {
		if trail.Edges[i].Kind != trail.Edges[j].Kind {
			return trail.Edges[i].Kind < trail.Edges[j].Kind
		}
		if trail.Edges[i].From != trail.Edges[j].From {
			return trail.Edges[i].From < trail.Edges[j].From
		}
		if trail.Edges[i].To != trail.Edges[j].To {
			return trail.Edges[i].To < trail.Edges[j].To
		}
		return trail.Edges[i].ID < trail.Edges[j].ID
	})
	sort.Slice(trail.Diagnostics, func(i, j int) bool {
		left := trail.Diagnostics[i]
		right := trail.Diagnostics[j]
		if left.Stage != right.Stage {
			return left.Stage < right.Stage
		}
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.Field != right.Field {
			return left.Field < right.Field
		}
		return left.Message < right.Message
	})
}

func sortLocalIndex(index *researchtrail.LocalIndex) {
	for entryIndex := range index.Entries {
		entry := &index.Entries[entryIndex]
		sort.Slice(entry.Origins, func(i, j int) bool {
			left := entry.Origins[i]
			right := entry.Origins[j]
			if left.Stage != right.Stage {
				return left.Stage < right.Stage
			}
			if left.Round != right.Round {
				return left.Round < right.Round
			}
			if left.ProbeID != right.ProbeID {
				return left.ProbeID < right.ProbeID
			}
			if left.Artifact != right.Artifact {
				return left.Artifact < right.Artifact
			}
			if left.LocalID != right.LocalID {
				return left.LocalID < right.LocalID
			}
			if left.Source != right.Source {
				return left.Source < right.Source
			}
			if left.Operation != right.Operation {
				return left.Operation < right.Operation
			}
			return left.Detail < right.Detail
		})
	}
	sort.Slice(index.Entries, func(i, j int) bool {
		if index.Entries[i].NodeID != index.Entries[j].NodeID {
			return index.Entries[i].NodeID < index.Entries[j].NodeID
		}
		return index.Entries[i].Path < index.Entries[j].Path
	})
}
