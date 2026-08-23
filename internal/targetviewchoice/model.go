// Package targetviewchoice chooses one default presentation view when one
// exact selected file belongs to several exact analysis targets. It does not
// discover, rank, filter, or synthesize targets: every input view remains an
// analysis target and the model selects only one request-local short ref.
package targetviewchoice

const (
	PreparationVersion    = 2
	ResponseSchemaVersion = 1

	MaxViews                = 32
	MaxFileHypotheses       = 64
	MaxRootSummaries        = 24
	MaxBasisSummaries       = 32
	MaxLabelBytes           = 256
	MaxSummaryBytes         = 512
	MaxAnchorPathBytes      = 4096
	MaxRequestBytes         = 128 << 10
	MaxProviderRequestBytes = 2*MaxRequestBytes + 32<<10
	MaxResponseBytes        = 1024
	MaxOutputTokens         = 256
)

const executionContract = "repomap.target-view-choice.v2"

// View is one exact local target view. It deliberately has no canonical
// target ID: the caller keeps that authority and can bind the returned
// language/selector pair back to its own sealed target catalog.
type View struct {
	Language       string   `json:"language"`
	Kind           string   `json:"kind"`
	DisplayName    string   `json:"display_name"`
	Selector       string   `json:"selector"`
	AnchorPath     string   `json:"anchor_path"`
	RootSummaries  []string `json:"root_summaries"`
	BasisSummaries []string `json:"basis_summaries"`
}

// VisibleView is the provider-visible row. Ref is generated locally after
// canonicalization and is the only value the model must copy.
type VisibleView struct {
	Ref            string   `json:"ref"`
	Language       string   `json:"language"`
	Kind           string   `json:"kind"`
	DisplayName    string   `json:"display_name"`
	Selector       string   `json:"selector"`
	AnchorPath     string   `json:"anchor_path"`
	RootSummaries  []string `json:"root_summaries"`
	BasisSummaries []string `json:"basis_summaries"`
}

// Request is the complete bounded provider-visible catalog. Candidate order
// carries no preference or ranking authority.
type Request struct {
	SelectedFileHypotheses []string      `json:"selected_file_hypotheses"`
	Views                  []VisibleView `json:"views"`
}

// Response is the complete provider response schema. No path, selector,
// explanation, confidence, target ID, or ranking is accepted from the model.
type Response struct {
	DefaultViewRef string `json:"default_view_ref"`
}

// Selection restores the chosen view byte-for-byte from local authority.
// DefaultViewRef remains request-local and is never a target identity.
type Selection struct {
	DefaultViewRef string
	DefaultView    View
}

// Cube is one immutable compiled semantic choice. Its unexported authority
// binds the request, cache state, and exact locally restorable views.
type Cube struct {
	views          []View
	fileHypotheses []string
	request        Request
	wire           []byte
	state          []byte
	authority      map[string]View
	seal           string
}
