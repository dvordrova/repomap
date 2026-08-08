// Package mechanismstudy compiles bounded, context-local direct-call graphs
// and validates plural model-proposed mechanisms against their exact local
// authority. A context can be a final Study card or a caller-supplied exact
// root; the package does not choose a repository action or build a follow-up
// worklist.
package mechanismstudy

import "github.com/dvordrova/repomap/internal/surfacediscovery"

const (
	CompilationVersion  = 2
	ExactContextVersion = 1
	RequestVersion      = 2
	ResultVersion       = 2

	MaxCards                             = 8
	MaxDirectReadingsPerCard             = 5
	MaxDepth                             = 2
	MaxRootNeighborsPerDirection         = 8
	MaxContinuationNeighborsPerDirection = 8
	MaxNodesPerCard                      = 32
	MaxEdgesPerCard                      = 48
	MaxCardsPerRequest                   = 4
	MaxNodesPerRequest                   = 128
	MaxEdgesPerRequest                   = 192
	MaxRequestBytes                      = 64 << 10
	MaxResponseBytes                     = 64 << 10
	MaxProviderCalls                     = 4
	MaxMechanismsPerCard                 = 3
	MaxEdgesPerMechanism                 = 8
	MaxFrontierRecordsPerCard            = 16
	maxCardLabelRunes                    = 80
	maxCardQuestionRunes                 = 200
	maxReadingLabelRunes                 = 80
	maxNodeLabelRunes                    = 96
)

type OutcomeState string

const (
	OutcomePrepared  OutcomeState = "prepared_investigation"
	OutcomeMechanism OutcomeState = "mechanism"
)

type FrontierReason string

const (
	FrontierNoExactFunction    FrontierReason = "no_exact_function"
	FrontierUnsupportedReading FrontierReason = "unsupported_reading_kind"
	FrontierDynamicInvoke      FrontierReason = "dynamic_invoke"
	FrontierExternalCallee     FrontierReason = "external_callee"
	FrontierShallowBound       FrontierReason = "shallow_bound"
	FrontierDepthBound         FrontierReason = "depth_bound"
	FrontierIndexUnavailable   FrontierReason = "index_unavailable"
	FrontierResponseInvalid    FrontierReason = "response_invalid"
)

func (reason FrontierReason) valid() bool {
	switch reason {
	case FrontierNoExactFunction,
		FrontierUnsupportedReading,
		FrontierDynamicInvoke,
		FrontierExternalCallee,
		FrontierShallowBound,
		FrontierDepthBound,
		FrontierIndexUnavailable,
		FrontierResponseInvalid:
		return true
	default:
		return false
	}
}

// Binding is local request authority. It is deliberately absent from the
// provider-visible wire; the wire receives only the derived catalog digest.
type ContextKind string

const (
	ContextStudy    ContextKind = "study"
	ContextExplicit ContextKind = "explicit_root"
)

type Binding struct {
	ContextKind               ContextKind `json:"context_kind"`
	ContextSHA256             string      `json:"context_sha256"`
	StudyThemesSHA256         string      `json:"study_themes_sha256"`
	AtlasStudyCatalogSHA256   string      `json:"atlas_study_catalog_sha256"`
	RepositoryRevision        string      `json:"repository_revision"`
	RepositoryFreshnessSHA256 string      `json:"repository_freshness_sha256"`
}

// RepositoryBinding is enough for an explicit-root experiment. The compiler
// derives ContextSHA256 from the exact supplied locators; no Study provider
// stage is needed merely to manufacture a card.
type RepositoryBinding struct {
	RepositoryRevision        string `json:"repository_revision"`
	RepositoryFreshnessSHA256 string `json:"repository_freshness_sha256"`
}

type ExactReading struct {
	Label  string `json:"label"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol"`
}

// ExactContext is caller-supplied context, not a candidate list. Every root is
// used exactly as supplied and must resolve exactly; this API performs no
// ranking or root selection.
type ExactContext struct {
	Label    string         `json:"label"`
	Question string         `json:"question"`
	Readings []ExactReading `json:"readings"`
}

// Frontier is an aggregated, complete count of evidence excluded for one
// closed reason. Aggregation keeps the final card below sixteen records while
// avoiding path, symbol, or canonical-ID disclosure.
type Frontier struct {
	Reason FrontierReason `json:"reason"`
	Count  int            `json:"count"`
}

// Reading is one request-local direct reading. RootNodeRef is present
// only when the exact saved locator resolves to one DirectCallIndex node.
type Reading struct {
	Ref         string `json:"ref"`
	Label       string `json:"label"`
	RootNodeRef string `json:"root_node_ref,omitempty"`
}

// Node is the provider-safe projection of one exact local function. Label is
// derived from a sanitized package leaf plus Symbol.Name; full import path,
// source path, full symbol ID and source stay in the private authority table.
type Node struct {
	Ref   string `json:"ref"`
	Label string `json:"label"`
}

// Edge is one advertised exact directed StaticCallee relation. Its endpoints
// are request-local refs; no repository-global edge identity is exposed.
type Edge struct {
	Ref          string                                `json:"ref"`
	CallerRef    string                                `json:"caller_ref"`
	CalleeRef    string                                `json:"callee_ref"`
	Invocation   surfacediscovery.DirectCallInvocation `json:"invocation"`
	WitnessCount int                                   `json:"witness_count"`
}

// Card is the complete bounded graph the model may cite for one final Study
// card or one explicitly supplied exact-root context. There is no chosen root,
// next action, or expansion request.
type Card struct {
	Ref      string     `json:"ref"`
	Label    string     `json:"label"`
	Question string     `json:"question"`
	Readings []Reading  `json:"readings"`
	Nodes    []Node     `json:"nodes"`
	Edges    []Edge     `json:"edges"`
	Frontier []Frontier `json:"frontier,omitempty"`
}

// Compilation is the provider-free product of exact reading binding and
// bounded graph collection. Authority maps are unexported and never marshal.
type Compilation struct {
	Version               int                       `json:"version"`
	CatalogRef            string                    `json:"catalog_ref"`
	CatalogSHA256         string                    `json:"catalog_sha256"`
	Binding               Binding                   `json:"binding"`
	DirectCallIndexSHA256 string                    `json:"direct_call_index_sha256"`
	Scenario              surfacediscovery.Scenario `json:"scenario"`
	Cards                 []Card                    `json:"cards"`
	OmittedCards          int                       `json:"omitted_cards,omitempty"`
	authority             map[string]cardAuthority
	catalogAuthorityJSON  string
}

type cardAuthority struct {
	nodeIDByRef      map[string]string
	nodeRefByID      map[string]string
	edgeByRef        map[string]surfacediscovery.DirectCallEdge
	readingRootByRef map[string]string
}

// Request is the only graph value embedded in the provider prompt.
type Request struct {
	Version       int    `json:"version"`
	PromptVersion string `json:"prompt_version"`
	CatalogRef    string `json:"catalog_ref"`
	CatalogSHA256 string `json:"catalog_sha256"`
	RequestRef    string `json:"request_ref"`
	Cards         []Card `json:"cards"`
}

// RequestBatch binds exact provider-visible JSON without placing private
// restoration authority on the wire.
type RequestBatch struct {
	Request    Request `json:"request"`
	WireJSON   string  `json:"wire_json"`
	WireSHA256 string  `json:"wire_sha256"`
	sealed     string
}

type Prompt struct {
	Version string `json:"version"`
	System  string `json:"system"`
	User    string `json:"user"`
}

// Candidate is one member of an unordered plural result set, not a selection
// or next step. The backend owns endpoints, direction, reading ties and prose;
// the provider can only cite advertised exact edge refs.
type Candidate struct {
	EdgeRefs []string `json:"edge_refs"`
}

type ResponseCard struct {
	CardRef    string      `json:"card_ref"`
	Mechanisms []Candidate `json:"mechanisms"`
}

type Response struct {
	Version       int            `json:"version"`
	CatalogRef    string         `json:"catalog_ref"`
	CatalogSHA256 string         `json:"catalog_sha256"`
	RequestRef    string         `json:"request_ref"`
	Cards         []ResponseCard `json:"cards"`
}

type IssueCode string

const (
	IssueInvalidShape  IssueCode = "invalid_shape"
	IssueUnknownRef    IssueCode = "unknown_ref"
	IssueDisconnected  IssueCode = "disconnected"
	IssueDuplicateRef  IssueCode = "duplicate_ref"
	IssueDuplicatePath IssueCode = "duplicate_path"
	IssueOverBound     IssueCode = "over_bound"
	IssueNoReadingTie  IssueCode = "no_reading_tie"
	IssueMissingCard   IssueCode = "missing_card"
	IssueDuplicateCard IssueCode = "duplicate_card"
)

type Issue struct {
	Code IssueCode `json:"code"`
}

type Mechanism struct {
	ReadingRefs []string `json:"reading_refs"`
	NodeRefs    []string `json:"node_refs"`
	EdgeRefs    []string `json:"edge_refs"`
}

type CardResult struct {
	CardRef    string       `json:"card_ref"`
	State      OutcomeState `json:"state"`
	Mechanisms []Mechanism  `json:"mechanisms"`
	Frontier   []Frontier   `json:"frontier,omitempty"`
	Issues     []Issue      `json:"issues,omitempty"`
}

type Result struct {
	Version       int          `json:"version"`
	PromptVersion string       `json:"prompt_version"`
	CatalogRef    string       `json:"catalog_ref"`
	CatalogSHA256 string       `json:"catalog_sha256"`
	RequestRef    string       `json:"request_ref"`
	RequestSHA256 string       `json:"request_sha256"`
	Cards         []CardResult `json:"cards"`
}
