package atlas

// LearningPlan records why the reader was offered a question. It annotates the
// existing atlas; sources and answers still refer to the same places graph.
type LearningPlan struct {
	Version     int                 `json:"version"`
	GraphSHA256 string              `json:"graph_sha256"`
	State       string              `json:"state"`
	Questions   []LearningQuestion  `json:"questions"`
	Reviews     []LearningReview    `json:"reviews"`
	Selections  []LearningSelection `json:"selections,omitempty"`
}

// Selection records whether a proposal belongs to a selected introduction.
// Reason explains the whole menu decision, not an individual rejection.
// Unselected material remains inspectable without claiming it is inapplicable.
type LearningSelection struct {
	LearningQuestion
	Intent         string `json:"intent"`
	Title          string `json:"title"`
	Audience       string `json:"audience"`
	Reason         string `json:"reason"`
	Source         string `json:"source"`
	PartialContext bool   `json:"partial_context,omitempty"`
}

type LearningQuestion struct {
	Question string           `json:"question"`
	Origins  []LearningOrigin `json:"origins"`
}

type LearningOrigin struct {
	Intent   string         `json:"intent"`
	Title    string         `json:"title"`
	Question string         `json:"question"`
	Why      string         `json:"why"`
	Source   string         `json:"source"`
	Sources  []QuestionStop `json:"sources"`
}

// Review is explicitly local to one context window. A local lack of evidence
// must never become a repository-wide assertion that a topic does not apply.
type LearningReview struct {
	Intent         string `json:"intent"`
	Title          string `json:"title"`
	Window         int    `json:"window"`
	PartialContext bool   `json:"partial_context"`
	State          string `json:"state"`
	Reason         string `json:"reason"`
	// ReasonFrom names the response field the reason was taken from when the
	// review came back without one: "why" is the first sentence of its first
	// accepted question's why. Empty when the model wrote the reason itself.
	ReasonFrom string         `json:"reason_from,omitempty"`
	Source     string         `json:"source"`
	Sources    []QuestionStop `json:"sources,omitempty"`
}
