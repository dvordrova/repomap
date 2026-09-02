// Package readmetargetscout sends complete repository guidance documents and
// the complete shared-corpus FileID/path authority through an exhaustive set
// of bounded model calls, then reduces one sparse file-role catalog. It is
// language-neutral and runs in parallel with language target discovery.
package readmetargetscout

import (
	"encoding/json"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
)

const (
	CompilationVersion = 7

	PreparationVersion = "complete-readmes-agents-exhaustive-file-tree-shards-and-prose-ref-authority-v8"
	SchemaVersion      = "readme-file-role-classifications-unbounded-set-valued-ignore-unknown-refs-v5"
	ReducerVersion     = "readme-file-role-classifications-unbounded-known-set-union-with-incompatible-prose-filter-v9"

	// MaxRequestBytes is a deterministic shard-packing window, not an
	// acceptance or transport limit. Larger complete inputs are covered by
	// deterministic shards; an indivisible shard proceeds to the shared
	// semantic-record envelope without losing a byte.
	MaxRequestBytes = 1536 << 10
	// MaxProviderRequestBytes retains the former prepared-request estimate for
	// scale comparisons. Ordinary execution uses llm.SemanticRecordByteLimit.
	MaxProviderRequestBytes = 2*MaxRequestBytes + 64<<10
	MaxResponseBytes        = llm.ProviderResponseByteLimit
	MaxOutputTokens         = 128_000

	// Former local acceptance thresholds are retained only as scale-warning
	// baselines. Crossing one never truncates or rejects accepted data.
	AdvisoryAtomicRequestBytes     = 1536 << 10
	AdvisoryResponseBytes          = 64 << 10
	AdvisoryHypothesisBytes        = 160
	AdvisoryClassificationsPerFile = 3
	AdvisoryHypothesesPerClass     = 2
	AdvisoryArtifactBytes          = 1 << 20
)

const executionContract = "repository-guidance-file-classifier-v13"

const ArtifactFilename = "readme-file-roles.json"

type CompilationState string

const (
	StateReady         CompilationState = "ready"
	StateNotApplicable CompilationState = "not_applicable"
)

type NotApplicableReason string

const NoGuidanceFiles NotApplicableReason = "no_guidance_files"

// Request is either the complete aggregate first-layer evidence identity or
// one provider-visible shard. The aggregate FileTree contains every tracked
// regular corpus file; a shard contains an exact subset and complete
// request-local prose authority. Non-guidance source contents are absent.
type Request struct {
	RepoName          string                    `json:"repo_name"`
	FileCount         int                       `json:"file_count"`
	FileTree          FileTree                  `json:"file_tree"`
	ProseFileRefs     []corpus.FileID           `json:"prose_file_refs"`
	GuidanceDocuments []RequestGuidanceDocument `json:"guidance_documents"`
}

type GuidanceKind string

const (
	GuidanceReadme GuidanceKind = "readme"
	GuidanceAgents GuidanceKind = "agents"
)

type RequestGuidanceDocument struct {
	FileRef corpus.FileID `json:"file_ref"`
	Path    string        `json:"path"`
	Kind    GuidanceKind  `json:"kind"`
	Content string        `json:"content"`
}

// Compilation owns the expanded exact dictionary used to validate response
// refs. Compile returns the complete aggregate authority; the internal batch
// planner derives request-local compilations with equivalent lossless trees.
// A not-applicable compilation has no provider request.
type Compilation struct {
	Version       int
	State         CompilationState
	Reason        NotApplicableReason
	Request       Request
	RequestSHA256 string

	wire      []byte
	authority map[corpus.FileID]string
	corpusRef string
	seal      string
}

type Prompt struct {
	Version string
	System  string
	User    string
}

type FileClass string

const (
	ClassTargetEntry       FileClass = "target_entry"
	ClassExampleEntry      FileClass = "example_entry"
	ClassTestEntry         FileClass = "test_entry"
	ClassSupportToolEntry  FileClass = "support_tool_entry"
	ClassConfiguration     FileClass = "configuration"
	ClassDatabaseAsset     FileClass = "database_asset"
	ClassClientEntry       FileClass = "client_entry"
	ClassDocumentation     FileClass = "documentation"
	ClassDeployment        FileClass = "deployment"
	ClassInterfaceContract FileClass = "interface_contract"
)

type Classification struct {
	Class      FileClass `json:"class"`
	Hypotheses []string  `json:"hypotheses"`
}

type ClassifiedFile struct {
	FileRef         corpus.FileID    `json:"file_ref"`
	Classifications []Classification `json:"classifications"`
}

// Result is a sparse repository-guidance-backed role catalog. One file may have
// several orthogonal roles. Repeated set-valued response rows are normalized
// before this canonical result is built.
type Result []ClassifiedFile

// Execution binds every caller-indexed model outcome to the one merged
// complete result. Any terminal shard failure rejects the result.
type Execution struct {
	Result   Result
	Outcomes []llm.Outcome[Result]
}

// TargetCandidates projects only guidance-backed target_entry classifications
// into the common target-hypothesis merger. All other roles remain available
// in Result for logging and future cubes, but cannot become analysis targets
// through complement semantics.
func (result Result) TargetCandidates() []analysistarget.FileCandidate {
	candidates := make([]analysistarget.FileCandidate, 0)
	for _, file := range result {
		for _, classification := range file.Classifications {
			if classification.Class != ClassTargetEntry {
				continue
			}
			hypotheses := make([]string, len(classification.Hypotheses))
			for index, hypothesis := range classification.Hypotheses {
				hypotheses[index] = "Repository guidance target_entry: " + hypothesis
			}
			candidates = append(candidates, analysistarget.FileCandidate{
				FileRef: file.FileRef, Hypotheses: hypotheses,
			})
		}
	}
	if candidates == nil {
		return []analysistarget.FileCandidate{}
	}
	return candidates
}

func executionStateValue() any {
	promptHash := sha256Hex([]byte(promptSystem + "\x00" + promptUserShape))
	return struct {
		Contract           string `json:"contract"`
		CompilationVersion int    `json:"compilation_version"`
		PromptVersion      string `json:"prompt_version"`
		PromptSHA256       string `json:"prompt_sha256"`
		PreparationVersion string `json:"preparation_version"`
		PreparationSHA256  string `json:"preparation_sha256"`
		SchemaVersion      string `json:"schema_version"`
		SchemaSHA256       string `json:"schema_sha256"`
		ReducerVersion     string `json:"reducer_version"`
		ReducerSHA256      string `json:"reducer_sha256"`
		MaxRequestBytes    int    `json:"max_request_bytes"`
		MaxResponseBytes   int    `json:"max_response_bytes"`
		MaxOutputTokens    int    `json:"max_output_tokens"`
	}{
		Contract: executionContract, CompilationVersion: CompilationVersion,
		PromptVersion: PromptVersion, PromptSHA256: promptHash,
		PreparationVersion: PreparationVersion,
		PreparationSHA256:  sha256Hex([]byte(preparationContract)),
		SchemaVersion:      SchemaVersion, SchemaSHA256: sha256Hex([]byte(schemaContract)),
		ReducerVersion: ReducerVersion, ReducerSHA256: sha256Hex([]byte(reducerContract)),
		MaxRequestBytes: MaxRequestBytes, MaxResponseBytes: MaxResponseBytes,
		MaxOutputTokens: MaxOutputTokens,
	}
}

// ExecutionState binds prompt, complete input preparation, strict list schema,
// and reducer semantics for the shared executor/cache.
func ExecutionState() []byte {
	state, err := json.Marshal(executionStateValue())
	if err != nil {
		panic("readme target scout: encode static execution state")
	}
	return state
}
