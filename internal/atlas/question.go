package atlas

// QuestionFilename is an inspectable output of the atlas question pass. It is
// a reading aid over places.json, not an alternative program graph.
const QuestionFilename = "question-routes.json"

// QuestionRouteVersion versions each complete reading and its answer metadata.
const QuestionRouteVersion = 9

// QuestionRoutes keeps independent reading results over one shared graph.
type QuestionRoutes struct {
	Version int             `json:"version"`
	Routes  []QuestionRoute `json:"routes"`
}

type QuestionRoute struct {
	Version      int              `json:"version"`
	Question     string           `json:"question"`
	Repository   string           `json:"repository"`
	Revision     string           `json:"revision"`
	GraphSHA256  string           `json:"graph_sha256"`
	UserQuestion bool             `json:"user_question,omitempty"`
	Origins      []LearningOrigin `json:"origins,omitempty"`
	// Scope describes what was actually inspected, not what the model knows.
	Scope    []string         `json:"scope"`
	Coverage QuestionCoverage `json:"coverage"`
	Stops    []QuestionStop   `json:"stops"`
	// Connections retain their origin: compiler witnesses, producer declarations
	// or corpus membership. They never assert a complete execution trace.
	Connections []QuestionConnection `json:"connections"`
	Guide       *QuestionGuide       `json:"guide,omitempty"`
	Answer      *QuestionAnswer      `json:"answer,omitempty"`
}

// Parts keep the original selected evidence when it needs multiple windows.
// A part never claims to synthesize evidence from the other windows.
type QuestionAnswer struct {
	State string               `json:"state"`
	Parts []QuestionAnswerPart `json:"parts"`
}

type QuestionAnswerPart struct {
	// OriginRequest and OriginRow bind shared terminology to this accepted row.
	// It is local provenance and never appears in a provider prompt.
	OriginRequest string         `json:"origin_request_sha256,omitempty"`
	OriginRow     string         `json:"origin_row,omitempty"`
	State         string         `json:"state"`
	Text          string         `json:"text"`
	Basis         string         `json:"basis"`
	Remaining     string         `json:"remaining"`
	Source        string         `json:"source"`
	Steps         []QuestionStep `json:"steps"`
}

type QuestionCoverage struct {
	Files            int `json:"files"`
	Entities         int `json:"entities"`
	Documents        int `json:"documents"`
	SourceFacts      int `json:"source_facts,omitempty"`
	Chunks           int `json:"chunks"`
	InspectedChunks  int `json:"inspected_chunks"`
	UnresolvedChunks int `json:"unresolved_chunks"`
}

type QuestionStop struct {
	// SubjectID names the internal object being inspected. PlaceID retains
	// the context row's graph location, which can be a different entity/file.
	SubjectID string   `json:"subject_id,omitempty"`
	PlaceID   string   `json:"place_id"`
	Path      string   `json:"path"`
	Line      int      `json:"line"`
	Column    int      `json:"column,omitempty"`
	Name      string   `json:"name,omitempty"`
	Kind      string   `json:"kind"`
	TargetIDs []string `json:"target_ids"`
	// Relevance and Why are MODEL hypotheses. Source retains model/cache.
	Relevance    string         `json:"relevance"`
	Why          string         `json:"why"`
	Source       string         `json:"source"`
	Evidence     map[string]any `json:"evidence"`
	KnowledgeIDs []string       `json:"knowledge_ids,omitempty"`
}

// QuestionGuide projects the answer's ordered sources for supporting reading.
// It makes no separate model decision over the preserved candidate reservoir.
// StopIndexes restore all observations sharing a selected source location.
type QuestionGuide struct {
	State      string         `json:"state"`
	Source     string         `json:"source"`
	Candidates int            `json:"candidates"`
	Steps      []QuestionStep `json:"steps"`
	// Parts retain independently answered evidence windows. Steps is their
	// exact union, not a model-selected order between parts.
	Parts []QuestionGuidePart `json:"parts,omitempty"`
}

type QuestionGuidePart struct {
	Source string         `json:"source"`
	Steps  []QuestionStep `json:"steps"`
}

type QuestionStep struct {
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Column      int    `json:"column,omitempty"`
	StopIndexes []int  `json:"stop_indexes"`
}

type QuestionConnection struct {
	FromID    string        `json:"from_id"`
	ToID      string        `json:"to_id"`
	FromPath  string        `json:"from_path"`
	ToPath    string        `json:"to_path"`
	FromName  string        `json:"from_name,omitempty"`
	ToName    string        `json:"to_name,omitempty"`
	Kind      string        `json:"kind"`
	Count     int           `json:"count"`
	Witnesses []Witness     `json:"witnesses"`
	Evidence  *EdgeEvidence `json:"evidence,omitempty"`
}
