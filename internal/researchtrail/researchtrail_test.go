package researchtrail

import (
	"strings"
	"testing"
)

// This is an MVP contract smoke test, not a golden rendering fixture. It is
// intentionally cheap to replace when the deeper-research projection changes.
func TestPebbleBatchTrailContract(t *testing.T) {
	t.Parallel()

	trail := Trail{
		Version: Version,
		Binding: Binding{
			RepositoryStateSHA256: strings.Repeat("a", 64),
			ReportSHA256:          strings.Repeat("b", 64),
		},
		Goal: Goal{
			ID:        "goal-onboarding",
			Kind:      "onboarding",
			Objective: "Understand the Pebble batch commit lifecycle.",
		},
		Component: Component{
			ID:      "component-batch",
			Name:    "Batch Operations",
			Purpose: "Groups writes and commits them atomically.",
			Basis:   SupportOrientationHypothesis,
		},
		PrimaryQuestionNodeID: "question:commit",
		Nodes: []Node{
			{
				ID: "component:batch", SourceID: "component-batch", Kind: NodeComponent,
				Section: SectionPlanning, Label: "Batch Operations",
				Detail: "Groups writes and commits them atomically.", Basis: SupportOrientationHypothesis,
			},
			{
				ID: "planner:batch-commit", SourceID: "symbol-batch-commit", Kind: NodePlannerSymbol,
				Section: SectionPlanning, Label: "Batch.Commit", Certainty: "static",
			},
			{
				ID: "question:commit", SourceID: "question-commit", Kind: NodeQuestion,
				Section: SectionPlanning, Label: "How does Batch.Commit reach the commit pipeline?",
				Basis: SupportModelSynthesis, Primary: true,
			},
			{
				ID: "exact:batch", SourceID: "symbol-batch-commit", Kind: NodeExactSymbol,
				Section: SectionPlanning, Label: "Batch.Commit", Basis: SupportStaticActiveBuild,
			},
			{
				ID: "exact:apply-internal", SourceID: "symbol-apply-internal", Kind: NodeExactSymbol,
				Section: SectionPlanning, Label: "applyInternal", Basis: SupportStaticActiveBuild,
			},
			{
				ID: "evidence:apply", SourceID: "teach-apply", Kind: NodeEvidence,
				Label: "Static call relation: Batch.Commit calls DB.Apply.", Basis: SupportStaticActiveBuild,
			},
			{
				ID: "evidence:apply-internal", SourceID: "teach-apply-internal", Kind: NodeEvidence,
				Label: "Source slice for applyInternal", Basis: SupportSource,
			},
			{
				ID: "claim:apply", SourceID: "item-lifecycle-001", Kind: NodeLifecycleStep,
				Section: SectionLifecycle, Label: "Batch.Commit delegates to DB.Apply.", Basis: SupportModelSynthesis,
			},
			{
				ID: "claim:pipeline", SourceID: "item-lifecycle-002", Kind: NodeLifecycleStep,
				Section: SectionLifecycle, Label: "applyInternal enters commitPipeline.Commit.", Basis: SupportModelSynthesis,
			},
			{
				ID: "frontier:apply-internal", SourceID: "frontier-apply-internal", Kind: NodeFrontier,
				Section: SectionNextDive, Label: "applyInternal", Basis: SupportStaticActiveBuild,
			},
			{
				ID: "frontier:prepare", SourceID: "frontier-prepare", Kind: NodeFrontier,
				Section: SectionNextDive, Label: "commitPipeline.prepare", Basis: SupportStaticActiveBuild,
			},
		},
		Edges: []Edge{
			{ID: "edge-select", Kind: EdgeSelects, From: "component:batch", To: "planner:batch-commit", Basis: SupportOrientationHypothesis, Source: "symbol-batch-commit"},
			{ID: "edge-motivate", Kind: EdgeMotivates, From: "planner:batch-commit", To: "question:commit", Basis: SupportOrientationHypothesis, Source: "symbol-batch-commit"},
			{ID: "edge-frame", Kind: EdgeFramesQuestion, From: "component:batch", To: "question:commit", Basis: SupportOrientationHypothesis, Source: "question-commit"},
			{ID: "edge-answer-1", Kind: EdgeAnswers, From: "question:commit", To: "claim:apply", Basis: SupportModelSynthesis, Source: "item-lifecycle-001"},
			{ID: "edge-answer-2", Kind: EdgeAnswers, From: "question:commit", To: "claim:pipeline", Basis: SupportModelSynthesis, Source: "item-lifecycle-002"},
			{ID: "edge-support-1", Kind: EdgeSupports, From: "evidence:apply", To: "claim:apply", Basis: SupportStaticActiveBuild, Source: "teach-apply"},
			{ID: "edge-support-2", Kind: EdgeSupports, From: "evidence:apply-internal", To: "claim:pipeline", Basis: SupportSource, Source: "teach-apply-internal"},
			{ID: "edge-order", Kind: EdgeTeachingNext, From: "claim:apply", To: "claim:pipeline", Basis: SupportModelSynthesis, Source: "item-lifecycle-002"},
			{ID: "edge-open", Kind: EdgeLeavesOpen, From: "claim:pipeline", To: "frontier:prepare", Basis: SupportStaticActiveBuild, Source: "frontier-prepare"},
		},
		Steps: []Step{
			{
				ID: "step:plan", Kind: StepPlan, Status: "planned", Label: "Plan the bounded question",
				FocusNodeIDs: []string{"planner:batch-commit", "question:commit"}, EvidenceNodeIDs: []string{},
			},
			{
				ID: "step:probe:1", Kind: StepProbe, Round: 1, Status: "frontier", Label: "Collect exact local evidence",
				FocusNodeIDs: []string{"exact:batch"}, EvidenceNodeIDs: []string{"evidence:apply"},
			},
			{
				ID: "step:probe:2", Kind: StepProbe, Round: 2, Status: "connected", Label: "Follow one accepted frontier",
				FocusNodeIDs: []string{"exact:apply-internal"}, EvidenceNodeIDs: []string{"evidence:apply-internal"},
			},
			{
				ID: "step:teach", Kind: StepTeach, Status: "taught", Label: "Explain grounded findings",
				FocusNodeIDs: []string{"claim:apply", "claim:pipeline"}, EvidenceNodeIDs: []string{},
			},
		},
		Transitions: []Transition{
			{ID: "transition-plan", Kind: TransitionContinues, FromStepID: "step:plan", ToStepID: "step:probe:1", Basis: SupportWorkflowRecord},
			{
				ID: "transition-frontier", Kind: TransitionAcceptedFrontier,
				FromStepID: "step:probe:1", ToStepID: "step:probe:2",
				SourceNodeID: "frontier:apply-internal", TargetNodeID: "exact:apply-internal",
				SourceID: "frontier-apply-internal", Basis: SupportStaticActiveBuild,
			},
			{ID: "transition-teach", Kind: TransitionContinues, FromStepID: "step:probe:2", ToStepID: "step:teach", Basis: SupportWorkflowRecord},
		},
		Diagnostics: []Diagnostic{},
	}
	index := LocalIndex{
		Version: LocalIndexVersion,
		Entries: []LocatorEntry{
			{
				NodeID: "planner:batch-commit", SourceID: "symbol-batch-commit", Path: "batch.go",
				StartLine: 1275, EndLine: 1275, Column: 1,
				Origins: []Origin{{
					Stage: "componentstudy", Artifact: "symbol_candidate", LocalID: "symbol-batch-commit",
					Source: "report", Operation: "component_symbols",
				}},
			},
			{
				NodeID: "exact:batch", SourceID: "symbol-batch-commit", Path: "batch.go",
				StartLine: 1275, EndLine: 1275, Column: 1,
				Origins: []Origin{{Stage: "componentprobe", Round: 1, ProbeID: "probe-batch", Artifact: "structural", LocalID: "resolution-batch"}},
			},
			{
				NodeID: "exact:apply-internal", SourceID: "symbol-apply-internal", Path: "db.go",
				StartLine: 760, EndLine: 760, Column: 1,
				Origins: []Origin{{Stage: "componentprobe", Round: 2, ProbeID: "probe-apply-internal", Artifact: "structural", LocalID: "resolution-apply-internal"}},
			},
			{
				NodeID: "evidence:apply", SourceID: "teach-apply", Path: "batch.go",
				StartLine: 1280, EndLine: 1280, Column: 2,
				Origins: []Origin{{Stage: "componentprobe", Round: 1, ProbeID: "probe-batch", Artifact: "structural", LocalID: "call-apply"}},
			},
			{
				NodeID: "evidence:apply-internal", SourceID: "teach-apply-internal", Path: "db.go",
				StartLine: 760, EndLine: 790, Column: 1,
				Origins: []Origin{{Stage: "componentprobe", Round: 2, ProbeID: "probe-apply-internal", Artifact: "source", LocalID: "source-760"}},
			},
			{
				NodeID: "frontier:apply-internal", SourceID: "frontier-apply-internal", Path: "db.go",
				StartLine: 760, EndLine: 760, Column: 1,
				Origins: []Origin{{Stage: "componentprobe", Round: 1, ProbeID: "probe-batch", Artifact: "structural", LocalID: "call-apply-internal"}},
			},
			{
				NodeID: "frontier:prepare", SourceID: "frontier-prepare", Path: "commit.go",
				StartLine: 320, EndLine: 320, Column: 1,
				Origins: []Origin{{Stage: "componentprobe", Round: 2, ProbeID: "probe-apply-internal", Artifact: "structural", LocalID: "call-prepare"}},
			},
		},
	}

	if err := trail.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := index.Validate(trail); err != nil {
		t.Fatalf("LocalIndex.Validate() error = %v", err)
	}
}
