// Package readmetargetscout sends complete repository guidance documents and
// the complete shared-corpus FileID/path tree through one bounded model call,
// then reduces a sparse file-role catalog. It is language-neutral and runs in
// parallel with language target discovery.
package readmetargetscout

import (
	"encoding/json"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/lexicalhints"
)

const (
	CompilationVersion = 4

	PreparationVersion = "complete-readmes-and-agents-prefix-compressed-corpus-file-tree-and-sparse-grep-stats-v4"
	SchemaVersion      = "readme-file-role-classifications-v2"
	ReducerVersion     = "readme-file-role-classifications-strict-with-prose-rejection-v4"

	// Keep one complete atomic semantic body below the empirically reliable
	// provider envelope. The former flat dictionary made a measured Airflow
	// request 1.96 MiB; the lossless path-component tree reduces the same
	// complete request to about 1.45 MiB without chunking or omission.
	// JSON transport escaping can at most double the body; the remaining
	// provider allowance covers prompts and envelope fields.
	MaxRequestBytes                = 1536 << 10
	MaxProviderRequestBytes        = 2*MaxRequestBytes + 64<<10
	MaxResponseBytes               = 64 << 10
	MaxHypothesisBytes             = 160
	MaxClassifiedFiles             = 48
	MaxClassificationsPerFile      = 3
	MaxHypothesesPerClassification = 2
	MaxOutputTokens                = 32_000
)

const executionContract = "repository-guidance-file-classifier-v6"

const ArtifactFilename = "readme-file-roles.json"

type CompilationState string

const (
	StateReady         CompilationState = "ready"
	StateNotApplicable CompilationState = "not_applicable"
)

type NotApplicableReason string

const NoGuidanceFiles NotApplicableReason = "no_guidance_files"

// Request is the complete provider-visible first-layer evidence. FileTree is a
// lossless prefix-compressed encoding of every tracked regular corpus file,
// not a locally selected subset. GrepStats contains weak sparse counts only;
// it never contains source bytes.
type Request struct {
	RepoName          string                             `json:"repo_name"`
	FileCount         int                                `json:"file_count"`
	FileTree          FileTree                           `json:"file_tree"`
	GrepStats         map[corpus.FileID]map[string]uint8 `json:"grep_stats"`
	GuidanceDocuments []RequestGuidanceDocument          `json:"guidance_documents"`
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
// refs. The provider sees the equivalent lossless tree. A not-applicable
// compilation has no provider request.
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

// Result is a sparse repository-guidance-backed role catalog. One file may have several
// orthogonal roles, but each role appears at most once for that file.
type Result []ClassifiedFile

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
	lexicalState, err := json.Marshal(lexicalhints.State())
	if err != nil {
		panic("readme target scout: encode lexical-hints state")
	}
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
		LexicalStateSHA256 string `json:"lexical_state_sha256"`
		MaxRequestBytes    int    `json:"max_request_bytes"`
		MaxResponseBytes   int    `json:"max_response_bytes"`
		MaxHypothesisBytes int    `json:"max_hypothesis_bytes"`
		MaxClassifiedFiles int    `json:"max_classified_files"`
		MaxClassifications int    `json:"max_classifications_per_file"`
		MaxHypotheses      int    `json:"max_hypotheses_per_classification"`
		MaxOutputTokens    int    `json:"max_output_tokens"`
	}{
		Contract: executionContract, CompilationVersion: CompilationVersion,
		PromptVersion: PromptVersion, PromptSHA256: promptHash,
		PreparationVersion: PreparationVersion,
		PreparationSHA256:  sha256Hex([]byte(preparationContract)),
		SchemaVersion:      SchemaVersion, SchemaSHA256: sha256Hex([]byte(schemaContract)),
		ReducerVersion: ReducerVersion, ReducerSHA256: sha256Hex([]byte(reducerContract)),
		LexicalStateSHA256: sha256Hex(lexicalState),
		MaxRequestBytes:    MaxRequestBytes, MaxResponseBytes: MaxResponseBytes,
		MaxHypothesisBytes: MaxHypothesisBytes, MaxClassifiedFiles: MaxClassifiedFiles,
		MaxClassifications: MaxClassificationsPerFile,
		MaxHypotheses:      MaxHypothesesPerClassification,
		MaxOutputTokens:    MaxOutputTokens,
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
