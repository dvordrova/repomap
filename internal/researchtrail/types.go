// Package researchtrail defines a presentation-neutral record of one bounded
// repository research step. It contains no repository, model, or browser I/O.
package researchtrail

const (
	Version           = 1
	LocalIndexVersion = 1
)

type SupportBasis string

const (
	SupportOrientationHypothesis SupportBasis = "orientation_hypothesis"
	SupportStaticActiveBuild     SupportBasis = "static_active_build"
	SupportSource                SupportBasis = "source_supported"
	SupportTestNavigation        SupportBasis = "test_navigation_only"
	SupportModelSynthesis        SupportBasis = "model_synthesis"
	SupportWorkflowRecord        SupportBasis = "saved_workflow"
)

type NodeKind string

const (
	NodeComponent       NodeKind = "component"
	NodeQuestion        NodeKind = "question"
	NodePlannerAnchor   NodeKind = "planner_anchor"
	NodePlannerFile     NodeKind = "planner_file"
	NodePlannerSymbol   NodeKind = "planner_symbol"
	NodePlannerEvidence NodeKind = "planner_evidence"
	NodeExactSymbol     NodeKind = "exact_symbol"
	NodeEvidence        NodeKind = "evidence"
	NodeLifecycleStep   NodeKind = "lifecycle_step"
	NodeClaim           NodeKind = "claim"
	NodeFrontier        NodeKind = "frontier"
)

type Section string

const (
	SectionPlanning              Section = "planning"
	SectionMentalModel           Section = "mental_model"
	SectionLifecycle             Section = "lifecycle"
	SectionBoundaries            Section = "boundaries"
	SectionDesignNotes           Section = "design_notes"
	SectionFailuresObservability Section = "failures_and_observability"
	SectionTestsChecks           Section = "tests_and_checks"
	SectionUnknowns              Section = "unknowns"
	SectionNextDive              Section = "next_dive"
)

type EdgeKind string

const (
	EdgeFramesQuestion EdgeKind = "frames_question"
	EdgeSelects        EdgeKind = "selects"
	EdgeMotivates      EdgeKind = "motivates"
	EdgeAnswers        EdgeKind = "answers"
	EdgeSupports       EdgeKind = "supports"
	// EdgeTeachingNext records explanation order, not runtime order.
	EdgeTeachingNext EdgeKind = "teaching_next"
	EdgeLeavesOpen   EdgeKind = "leaves_open"
	EdgeFrontier     EdgeKind = "frontier"
)

type StepKind string

const (
	StepPlan  StepKind = "plan"
	StepProbe StepKind = "probe"
	StepTeach StepKind = "teach"
)

type TransitionKind string

const (
	TransitionContinues        TransitionKind = "continues"
	TransitionAcceptedFrontier TransitionKind = "accepted_frontier"
)

type Goal struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Objective string `json:"objective"`
}

// Binding commits the trail to the repository state and exact onboarding
// report that authorized the focused research step.
type Binding struct {
	RepositoryStateSHA256 string `json:"repository_state_sha256"`
	ReportSHA256          string `json:"report_sha256"`
}

type Component struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Purpose string       `json:"purpose"`
	Basis   SupportBasis `json:"support_basis"`
}

type Trail struct {
	Version               int          `json:"version"`
	Binding               Binding      `json:"binding"`
	Goal                  Goal         `json:"goal"`
	Component             Component    `json:"component"`
	Framing               string       `json:"framing,omitempty"`
	PrimaryQuestionNodeID string       `json:"primary_question_node_id"`
	Nodes                 []Node       `json:"nodes"`
	Edges                 []Edge       `json:"edges"`
	Steps                 []Step       `json:"steps"`
	Transitions           []Transition `json:"transitions"`
	Diagnostics           []Diagnostic `json:"diagnostics"`
}

type Node struct {
	ID             string       `json:"id"`
	SourceID       string       `json:"source_id,omitempty"`
	Kind           NodeKind     `json:"kind"`
	Section        Section      `json:"section,omitempty"`
	Label          string       `json:"label"`
	Detail         string       `json:"detail,omitempty"`
	Basis          SupportBasis `json:"support_basis,omitempty"`
	Certainty      string       `json:"certainty,omitempty"`
	Primary        bool         `json:"primary,omitempty"`
	NavigationOnly bool         `json:"navigation_only,omitempty"`
}

type Edge struct {
	ID     string       `json:"id"`
	Kind   EdgeKind     `json:"kind"`
	From   string       `json:"from"`
	To     string       `json:"to"`
	Basis  SupportBasis `json:"support_basis"`
	Source string       `json:"source_id,omitempty"`
}

// Step is one bounded research stage. FocusNodeIDs name what the stage worked
// on; EvidenceNodeIDs expose challengeable facts without turning them into a
// teacher claim.
type Step struct {
	ID              string   `json:"id"`
	Kind            StepKind `json:"kind"`
	Round           int      `json:"round,omitempty"`
	Status          string   `json:"status"`
	Label           string   `json:"label"`
	FocusNodeIDs    []string `json:"focus_node_ids"`
	EvidenceNodeIDs []string `json:"evidence_node_ids"`
}

// Transition records research order, not repository runtime order. An
// accepted-frontier transition retains both the opaque frontier and the exact
// round-two symbol when resolution succeeded.
type Transition struct {
	ID           string         `json:"id"`
	Kind         TransitionKind `json:"kind"`
	FromStepID   string         `json:"from_step_id"`
	ToStepID     string         `json:"to_step_id"`
	SourceNodeID string         `json:"source_node_id,omitempty"`
	TargetNodeID string         `json:"target_node_id,omitempty"`
	SourceID     string         `json:"source_id,omitempty"`
	Basis        SupportBasis   `json:"support_basis"`
}

type Diagnostic struct {
	Stage   string `json:"stage"`
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// LocalIndex keeps repository locators and artifact-local origins out of the
// presentation-neutral trail while retaining a lossless way to open evidence.
type LocalIndex struct {
	Version int            `json:"version"`
	Entries []LocatorEntry `json:"entries"`
}

type LocatorEntry struct {
	NodeID    string   `json:"node_id"`
	SourceID  string   `json:"source_id"`
	Path      string   `json:"path"`
	StartLine int      `json:"start_line"`
	EndLine   int      `json:"end_line"`
	Column    int      `json:"column,omitempty"`
	Origins   []Origin `json:"origins"`
}

type Origin struct {
	Stage     string `json:"stage"`
	Round     int    `json:"round,omitempty"`
	ProbeID   string `json:"probe_id,omitempty"`
	Artifact  string `json:"artifact"`
	LocalID   string `json:"local_id"`
	Source    string `json:"source,omitempty"`
	Operation string `json:"operation,omitempty"`
	Detail    string `json:"detail,omitempty"`
}
