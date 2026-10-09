package llm

// Categorizer answers closed questions: each question gets one of its listed
// options, or a probability of yes, with the probabilities the owner's
// decision rule reads. It is a Provider, so the executor caches it by State
// and exact prepared bytes, journals it and gates its attempts like any
// other; the owner turns its verdicts into decisions or explicit uncertainty.
//
// Jev is the only implementation. Its questions name the data by the fields
// of Jev's request (`task`, `context.<field>`, `row`), and the owner still
// sizes its requests, concurrency and output for Jev; a second
// implementation would take those over when it exists.
type Categorizer interface {
	Provider
	// Prompt is the exact request asking these questions, by key, over one
	// task and the context every question shares.
	Prompt(task string, context map[string]any, questions map[string]Question) (Prompt, error)
	// Verdicts reads a response by question key. A verdict it cannot read is
	// absent; a response without verdicts is an error.
	Verdicts(response []byte) (map[string]Verdict, error)
}

// Question is one closed question about one item.
type Question struct {
	// Name is what the request calls the item, such as "file" or
	// "declaration", so the question can name it; empty is "row".
	Name string
	Item map[string]any
	Ask  string
	// Options are the choices in order; none makes a yes/no question.
	Options []Option
}

// Option is one choice. Criteria says what choosing it means on the owner's
// terms; else Meaning says it in one text, given only where the task and
// context do not already define the option, such as "none of these".
type Option struct {
	Name, Meaning string
	Criteria      *Criteria
}

// Criteria are an option's explicit terms: what it is, what it includes,
// what it is not for, and examples. The fields are in the order their JSON
// keys sort, the order a request writes them in.
type Criteria struct {
	Examples []string `json:"examples,omitempty"`
	Includes string   `json:"includes,omitempty"`
	NotFor   string   `json:"not_for,omitempty"`
	What     string   `json:"what"`
}

// Verdict answers one question: the choice with every option's probability,
// or a yes/no question's probability of yes. Conflict marks a question the
// response answered more than once, differently: it has no answer, and its
// owner refuses that question's cell or row alone.
type Verdict struct {
	Choice        string
	Probabilities map[string]float64
	// InvalidProbabilities preserves unreadable or conflicting scores until
	// the owner knows which labels belong to this question. Unknown labels
	// cannot invalidate a known decision.
	InvalidProbabilities []string
	Yes                  *float64
	InvalidYes           bool
	Conflict             bool
}
