package debugdump

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/secretscan"
)

type RunMeta struct {
	RunID                      string           `json:"run_id"`
	CreatedAt                  string           `json:"created_at"`
	RepoName                   string           `json:"repo_name"`
	RepoPath                   string           `json:"repo_path"`
	Command                    string           `json:"command"`
	Model                      string           `json:"model"`
	Endpoint                   string           `json:"endpoint"`
	PromptVersion              string           `json:"prompt_version,omitempty"`
	CompactContextBytes        int              `json:"compact_context_bytes,omitempty"`
	ExternalRequestBytes       int              `json:"external_request_bytes,omitempty"`
	ProviderRequestCount       int              `json:"provider_request_count,omitempty"`
	ProviderAccountingComplete bool             `json:"provider_accounting_complete,omitempty"`
	CandidateDirectionCount    int              `json:"candidate_direction_count,omitempty"`
	AcceptedDirectionCount     int              `json:"accepted_direction_count,omitempty"`
	RejectedDirectionCount     int              `json:"rejected_direction_count,omitempty"`
	ProviderLatencyMillis      *int64           `json:"provider_latency_ms,omitempty"`
	SurfaceDiscoveryRan        bool             `json:"surface_discovery_ran,omitempty"`
	SurfaceDiscoveryCount      int              `json:"surface_discovery_count,omitempty"`
	SurfaceDiscoveryMillis     *int64           `json:"surface_discovery_ms,omitempty"`
	Warnings                   []string         `json:"warnings,omitempty"`
	SnapshotOnly               bool             `json:"snapshot_only"`
	LLMBundleOnly              bool             `json:"llm_bundle_only"`
	AuthMode                   string           `json:"auth_mode,omitempty"`
	TimeoutMillis              int64            `json:"timeout_ms,omitempty"`
	MaxTokens                  int              `json:"max_tokens,omitempty"`
	EffectiveOptions           EffectiveOptions `json:"effective_options,omitempty"`
	RequestAttempts            []RequestAttempt `json:"request_attempts,omitempty"`
	BuildIdentity              BuildIdentity    `json:"build_identity"`
}

// BuildIdentity binds a run to the exact local binary without exposing build
// environment paths or command-line arguments. A NewWriter owns and enforces
// the current runtime identity. OpenWriter preserves the validated identity
// already read from that run; an old metadata file without one remains
// truthfully unavailable, while a not-yet-initialized run binds the process
// that creates its first metadata file.
type BuildIdentity struct {
	Available     bool   `json:"available"`
	GoVersion     string `json:"go_version,omitempty"`
	ModulePath    string `json:"module_path,omitempty"`
	ModuleVersion string `json:"module_version,omitempty"`
	VCSRevision   string `json:"vcs_revision,omitempty"`
	VCSTime       string `json:"vcs_time,omitempty"`
	VCSModified   *bool  `json:"vcs_modified,omitempty"`
}

type EffectiveOptions struct {
	Offline          bool   `json:"offline"`
	NoCache          bool   `json:"no_cache"`
	FlowCount        int    `json:"flows"`
	DiscoverSurfaces bool   `json:"discover_surfaces"`
	GuidedTour       bool   `json:"guided_tour"`
	NoSecrets        bool   `json:"no_secrets,omitempty"`
	ReportLanguage   string `json:"report_language,omitempty"`
	GitLabURL        string `json:"gitlab_url,omitempty"`
	GitHubURL        string `json:"github_url,omitempty"`
	OutputJSON       bool   `json:"json_output"`
	PreviewRequest   bool   `json:"preview_request"`
	NoOpen           bool   `json:"no_open"`
	NoServe          bool   `json:"no_serve"`
	Port             int    `json:"port"`
	DebugEnabled     bool   `json:"debug_enabled"`
}

type RequestAttempt struct {
	Stage                 string           `json:"stage"`
	State                 string           `json:"state"`
	RequestBytes          int              `json:"request_bytes,omitempty"`
	ProviderCallCount     int              `json:"provider_call_count,omitempty"`
	TransportAttemptCount int              `json:"transport_attempt_count,omitempty"`
	LatencyMillis         *int64           `json:"latency_ms,omitempty"`
	Outcome               *SemanticOutcome `json:"outcome,omitempty"`
}

const (
	SemanticExchangesDir     = "semantic_exchanges"
	SemanticExchangeMetaFile = "exchange.v2.json"

	SemanticStageOrientation       = "orientation"
	SemanticStageNavigator         = "navigator"
	SemanticStageAtlasStudy        = "atlas_study"
	SemanticStageTargetedResearch  = "targeted_research"
	SemanticStageArchitecture      = "architecture_synthesis"
	SemanticStageGuidedTour        = "guided_tour"
	SemanticStageStudyBrief        = "repository_brief_shape"
	SemanticStageStudyDirections   = "study_direction_candidates"
	SemanticStageStudyReview       = "reading_pack_review"
	SemanticStageLocalization      = "localization"
	SemanticRequestPrepared        = "prepared_request"
	SemanticRequestExactSent       = "exact_sent_request"
	SemanticStateAccepted          = "accepted"
	SemanticStateRejected          = "rejected"
	SemanticStateCacheHit          = "cache_hit"
	SemanticStateCanceled          = "canceled"
	SemanticStateProviderFailed    = "provider_failed"
	SemanticValidationAccepted     = "accepted"
	SemanticValidationCache        = "cache_validated"
	SemanticValidationCanceled     = "canceled"
	SemanticValidationProvider     = "provider_failed"
	SemanticValidationSecret       = "response_secret_scan"
	SemanticValidationDecode       = "response_decode"
	SemanticValidationResponse     = "response_validation"
	SemanticValidationApply        = "projection_apply"
	SemanticValidationQuality      = "projection_quality"
	SemanticUnavailableNoContent   = "provider_no_content"
	SemanticUnavailableCanceled    = "canceled"
	SemanticUnavailableCache       = "cache_raw_unavailable"
	SemanticUnavailableProjection  = "cache_projection_only"
	SemanticUnavailableOmitted     = "cache_response_omitted"
	SemanticUnavailableSize        = "size_limit"
	SemanticExchangeWarningCode    = "artifact_write_failed"
	semanticExchangeVersion        = 2
	semanticPayloadMarkerVersion   = 1
	maxSemanticExchangePayloadSize = 16 << 20
)

// SemanticUnavailable describes response bytes that the current stage seam
// truthfully does not possess. It may carry an already-known original identity
// but never reconstructed content.
type SemanticUnavailable struct {
	Code           string
	OriginalSHA256 string
	OriginalBytes  int
}

// SemanticExchange is one observation at the semantic request owner. The
// recorder is diagnostic-only: callers give it their existing outcome after
// validation and never read a value back into execution.
type SemanticExchange struct {
	Stage                  string
	InstanceOrdinal        int
	SemanticAttemptOrdinal int
	RequestProvenance      string
	State                  string
	ValidationCode         string
	SemanticCalls          int
	TransportAttempts      int
	Request                []byte
	Response               []byte
	ResponseUnavailable    *SemanticUnavailable
	Outcome                SemanticOutcome
}

// SemanticOutcome is the closed, safe explanation of what the stage decided.
// Raw provider/error text never belongs here: payload bytes already live in
// separately redacted and secret-scanned files.
type SemanticOutcome struct {
	Phase   string           `json:"phase"`
	Code    string           `json:"code"`
	Detail  string           `json:"detail,omitempty"`
	Metrics []SemanticMetric `json:"metrics,omitempty"`
}

type SemanticMetric struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type SemanticPayloadRecord struct {
	Storage         string `json:"storage"`
	File            string `json:"file"`
	MediaType       string `json:"media_type"`
	OriginalSHA256  string `json:"original_sha256,omitempty"`
	OriginalBytes   int    `json:"original_bytes"`
	SavedSHA256     string `json:"saved_sha256"`
	SavedBytes      int    `json:"saved_bytes"`
	UnsafeKind      string `json:"unsafe_kind,omitempty"`
	UnavailableCode string `json:"unavailable_code,omitempty"`
}

type SemanticExchangeRecord struct {
	Version                int                   `json:"version"`
	Stage                  string                `json:"stage"`
	InstanceOrdinal        int                   `json:"instance_ordinal"`
	SemanticAttemptOrdinal int                   `json:"semantic_attempt_ordinal"`
	RequestSHA256          string                `json:"request_sha256"`
	RequestProvenance      string                `json:"request_provenance"`
	State                  string                `json:"state"`
	ValidationCode         string                `json:"validation_code"`
	SemanticCalls          int                   `json:"semantic_calls"`
	TransportAttempts      int                   `json:"transport_attempts"`
	Outcome                SemanticOutcome       `json:"outcome"`
	Request                SemanticPayloadRecord `json:"request"`
	Response               SemanticPayloadRecord `json:"response"`
}

type semanticPayloadMarker struct {
	Version         int    `json:"version"`
	Storage         string `json:"storage"`
	OriginalSHA256  string `json:"original_sha256,omitempty"`
	OriginalBytes   int    `json:"original_bytes"`
	UnsafeKind      string `json:"unsafe_kind,omitempty"`
	UnavailableCode string `json:"unavailable_code,omitempty"`
}

type preparedSemanticPayload struct {
	name   string
	data   []byte
	record SemanticPayloadRecord
}

type Writer struct {
	BaseDir  string
	RunID    string
	Redacted bool
	runDir   string
	root     *os.Root

	semanticMu     sync.Mutex
	warningMu      sync.Mutex
	warningWriter  io.Writer
	warnedSemantic map[string]struct{}
	buildIdentity  *BuildIdentity
}

func NewWriter(baseDir, runID string, redacted bool) (*Writer, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("debug dir is empty")
	}
	if runID == "" || !filepath.IsLocal(runID) || filepath.Clean(runID) != runID || filepath.Base(runID) != runID {
		return nil, fmt.Errorf("debug run id must be one local path component")
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, fmt.Errorf("create debug base dir: %w", err)
	}
	baseRoot, err := os.OpenRoot(baseDir)
	if err != nil {
		return nil, fmt.Errorf("open debug base dir: %w", err)
	}
	defer baseRoot.Close()
	// The caller binds metadata, report routing, and browser authority to this
	// exact ID. Fail closed on collision instead of reusing or silently
	// suffixing a directory that those callers did not authorize.
	if err := baseRoot.Mkdir(runID, 0o700); err != nil {
		return nil, fmt.Errorf("create unique debug run dir: %w", err)
	}
	runRoot, err := baseRoot.OpenRoot(runID)
	if err != nil {
		return nil, fmt.Errorf("open debug run dir: %w", err)
	}
	runDir := filepath.Join(baseDir, runID)
	identity := currentBuildIdentity()
	return &Writer{
		BaseDir: baseDir, RunID: runID, Redacted: redacted,
		runDir: runDir, root: runRoot, warningWriter: os.Stderr,
		warnedSemantic: make(map[string]struct{}), buildIdentity: &identity,
	}, nil
}

// OpenWriter opens an existing run directory while retaining os.Root
// confinement. It never creates or follows a replacement run-directory
// symlink.
func OpenWriter(runDir string, redacted bool) (*Writer, error) {
	if runDir == "" {
		return nil, fmt.Errorf("debug run dir is empty")
	}
	absRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return nil, fmt.Errorf("resolve debug run dir: %w", err)
	}
	info, err := os.Lstat(absRunDir)
	if err != nil {
		return nil, fmt.Errorf("inspect debug run dir: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("debug run dir must be an existing directory")
	}
	parentRoot, err := os.OpenRoot(filepath.Dir(absRunDir))
	if err != nil {
		return nil, fmt.Errorf("open debug run parent: %w", err)
	}
	defer parentRoot.Close()
	root, err := parentRoot.OpenRoot(filepath.Base(absRunDir))
	if err != nil {
		return nil, fmt.Errorf("open debug run dir: %w", err)
	}
	identity, err := readPreservedBuildIdentity(root)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	if identity == nil {
		current := currentBuildIdentity()
		identity = &current
	}
	return &Writer{
		BaseDir: filepath.Dir(absRunDir), RunID: filepath.Base(absRunDir),
		Redacted: redacted, runDir: absRunDir, root: root,
		warningWriter: os.Stderr, warnedSemantic: make(map[string]struct{}),
		buildIdentity: identity,
	}, nil
}

func readPreservedBuildIdentity(root *os.Root) (*BuildIdentity, error) {
	if root == nil {
		return nil, fmt.Errorf("debug writer is closed")
	}
	file, err := root.Open("metadata.json")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read existing debug metadata identity: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect existing debug metadata identity: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 4<<20 {
		return nil, fmt.Errorf("existing debug metadata is not a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read existing debug metadata identity: %w", err)
	}
	var metadata struct {
		BuildIdentity BuildIdentity `json:"build_identity"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("decode existing debug metadata identity: %w", err)
	}
	if err := validateBuildIdentity(metadata.BuildIdentity); err != nil {
		return nil, fmt.Errorf("existing debug metadata build identity: %w", err)
	}
	identity := metadata.BuildIdentity
	return &identity, nil
}

func (w *Writer) WriteFile(name string, data []byte) error {
	return w.writeRootFile(name, data)
}

// WriteValidatedFile applies the writer's existing persisted-artifact
// redaction first, validates those exact prepared bytes, and only then
// publishes them. Callers use this for canonical artifacts whose redacted
// representation must remain a valid instance of their contract.
func (w *Writer) WriteValidatedFile(
	name string,
	data []byte,
	validate func([]byte) error,
) error {
	if validate == nil {
		return fmt.Errorf("debug artifact validator is required")
	}
	prepared := data
	if w != nil && w.Redacted {
		prepared = redactJSON(prepared)
	}
	if err := validate(append([]byte(nil), prepared...)); err != nil {
		return fmt.Errorf("validate %s: %w", name, err)
	}
	return w.writePreparedRootFile(name, prepared)
}

func (w *Writer) writeRootFile(name string, data []byte) error {
	if w != nil && w.Redacted {
		data = redactJSON(data)
	}
	return w.writePreparedRootFile(name, data)
}

func (w *Writer) writePreparedRootFile(name string, data []byte) error {
	if w == nil || w.root == nil {
		return fmt.Errorf("debug writer is closed")
	}
	localName := filepath.FromSlash(name)
	if name == "" || !filepath.IsLocal(localName) || filepath.Clean(localName) != localName {
		return fmt.Errorf("invalid debug artifact path %q", name)
	}
	tmpName := localName + ".tmp"
	file, err := w.root.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	written := false
	defer func() {
		if !written {
			_ = w.root.Remove(tmpName)
		}
	}()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", name, err)
	}
	if err := w.root.Rename(tmpName, localName); err != nil {
		return fmt.Errorf("publish %s: %w", name, err)
	}
	written = true
	return nil
}

// RecordSemanticExchange publishes one best-effort human-debug journal entry.
// A write failure is deliberately not returned to semantic execution; instead
// one bounded warning is emitted for the closed stage in this writer.
func (w *Writer) RecordSemanticExchange(exchange SemanticExchange) string {
	if w == nil {
		return ""
	}
	if err := w.writeSemanticExchange(exchange, nil); err != nil {
		w.warnSemanticExchange(exchange.Stage)
		return ""
	}
	return filepath.ToSlash(filepath.Join(
		SemanticExchangesDir,
		semanticExchangeKey(exchange),
		SemanticExchangeMetaFile,
	))
}

// SetWarningWriter routes bounded recorder warnings. A nil writer restores the
// default stderr destination.
func (w *Writer) SetWarningWriter(writer io.Writer) {
	if w == nil {
		return
	}
	w.warningMu.Lock()
	defer w.warningMu.Unlock()
	if writer == nil {
		writer = os.Stderr
	}
	w.warningWriter = writer
}

func (w *Writer) writeSemanticExchange(
	exchange SemanticExchange,
	afterPayloads func() error,
) error {
	if err := validateSemanticExchange(exchange); err != nil {
		return err
	}
	request, err := prepareSemanticPayload("request", exchange.Request, nil)
	if err != nil {
		return err
	}
	response, err := prepareSemanticPayload(
		"response",
		exchange.Response,
		exchange.ResponseUnavailable,
	)
	if err != nil {
		return err
	}
	record := SemanticExchangeRecord{
		Version: semanticExchangeVersion,
		Stage:   exchange.Stage, InstanceOrdinal: exchange.InstanceOrdinal,
		SemanticAttemptOrdinal: exchange.SemanticAttemptOrdinal,
		RequestSHA256:          sha256Hex(exchange.Request),
		RequestProvenance:      exchange.RequestProvenance,
		State:                  exchange.State, ValidationCode: exchange.ValidationCode,
		SemanticCalls: exchange.SemanticCalls, TransportAttempts: exchange.TransportAttempts,
		Outcome: normalizedSemanticOutcome(exchange),
		Request: request.record, Response: response.record,
	}
	metadata, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode semantic exchange metadata: %w", err)
	}
	metadata = append(metadata, '\n')

	w.semanticMu.Lock()
	defer w.semanticMu.Unlock()
	if w.root == nil {
		return fmt.Errorf("debug writer is closed")
	}
	if err := w.root.MkdirAll(SemanticExchangesDir, 0o700); err != nil {
		return fmt.Errorf("create semantic exchange directory: %w", err)
	}
	directory := filepath.Join(SemanticExchangesDir, semanticExchangeKey(exchange))
	if err := w.root.Mkdir(directory, 0o700); err != nil {
		return fmt.Errorf("create semantic exchange: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = w.root.RemoveAll(directory)
		}
	}()
	if err := w.writePreparedRootFile(filepath.Join(directory, request.name), request.data); err != nil {
		return err
	}
	if err := w.writePreparedRootFile(filepath.Join(directory, response.name), response.data); err != nil {
		return err
	}
	if afterPayloads != nil {
		if err := afterPayloads(); err != nil {
			return err
		}
	}
	// Metadata is the commit marker and is always published last.
	if err := w.writePreparedRootFile(filepath.Join(directory, SemanticExchangeMetaFile), metadata); err != nil {
		return err
	}
	committed = true
	return nil
}

func (w *Writer) warnSemanticExchange(stage string) {
	if !validSemanticStage(stage) {
		stage = "unknown"
	}
	w.warningMu.Lock()
	defer w.warningMu.Unlock()
	if w.warnedSemantic == nil {
		w.warnedSemantic = make(map[string]struct{})
	}
	if _, found := w.warnedSemantic[stage]; found {
		return
	}
	w.warnedSemantic[stage] = struct{}{}
	writer := w.warningWriter
	if writer == nil {
		writer = os.Stderr
	}
	fmt.Fprintf(
		writer,
		"warning: semantic exchange journal unavailable: stage=%s code=%s\n",
		stage,
		SemanticExchangeWarningCode,
	)
}

func validateSemanticExchange(exchange SemanticExchange) error {
	if !validSemanticStage(exchange.Stage) {
		return fmt.Errorf("semantic exchange: invalid stage")
	}
	if exchange.InstanceOrdinal < 1 || exchange.InstanceOrdinal > 4096 ||
		exchange.SemanticAttemptOrdinal < 1 || exchange.SemanticAttemptOrdinal > 16 {
		return fmt.Errorf("semantic exchange: invalid ordinal")
	}
	if exchange.RequestProvenance != SemanticRequestPrepared &&
		exchange.RequestProvenance != SemanticRequestExactSent {
		return fmt.Errorf("semantic exchange: invalid request provenance")
	}
	if !validSemanticState(exchange.State) || !validSemanticValidationCode(exchange.ValidationCode) {
		return fmt.Errorf("semantic exchange: invalid outcome")
	}
	if exchange.SemanticCalls < 0 || exchange.SemanticCalls > 1 ||
		exchange.TransportAttempts < 0 || exchange.TransportAttempts > 64 ||
		exchange.SemanticCalls == 0 && exchange.TransportAttempts != 0 {
		return fmt.Errorf("semantic exchange: invalid call counts")
	}
	if len(exchange.Request) == 0 ||
		len(exchange.Response) > 0 && exchange.ResponseUnavailable != nil ||
		len(exchange.Response) == 0 && exchange.ResponseUnavailable == nil {
		return fmt.Errorf("semantic exchange: invalid payload availability")
	}
	if exchange.ResponseUnavailable != nil {
		if !validSemanticUnavailableCode(exchange.ResponseUnavailable.Code) ||
			exchange.ResponseUnavailable.OriginalBytes < 0 ||
			exchange.ResponseUnavailable.OriginalSHA256 != "" &&
				!validSHA256(exchange.ResponseUnavailable.OriginalSHA256) {
			return fmt.Errorf("semantic exchange: invalid unavailable response identity")
		}
	}
	if err := validateSemanticOutcomeForStage(exchange.Stage, normalizedSemanticOutcome(exchange)); err != nil {
		return err
	}
	return nil
}

var semanticDiagnosticNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)

var semanticOutcomeRegistry = map[string]map[string]struct{}{
	"complete": {"accepted": {}, "accepted_partial": {}},
	"cache":    {"cache_hit": {}},
	"input_preparation": {
		"input_preparation_failed": {}, "architecture.preparation_failed": {},
	},
	"provider_configuration": {
		"provider_configuration_failed": {}, "architecture.provider_configuration_failed": {},
	},
	"provider_call": {
		"canceled": {}, "provider_failed": {}, "response_received": {},
		"architecture.provider_call_failed":  {},
		"architecture.provider_output_limit": {},
	},
	"response_secret_scan": {"response_secret_scan": {}},
	"response_decode": {
		"response_decode": {}, "architecture.empty_response": {},
	},
	"response_validation": {
		"response_validation":                     {},
		"architecture.proposal_rejected":          {},
		"componentmap.response_evaluation_failed": {},
	},
	"landscape_validation": {
		"componentmap.partial_model_inconsistent": {},
	},
	"projection_apply":   {"projection_apply": {}},
	"projection_quality": {"projection_quality": {}},
	"availability":       {"unavailable": {}, "not_called": {}},
	"request_build":      {"prepared": {}, "request_build_failed": {}},
	"resource_exhausted": {"resource_exhausted": {}},
	"stage_state":        {"unclassified": {}},
}

var semanticOutcomeDetails = map[string]string{
	"componentmap.partial_model_inconsistent": "full or normalized coverage was classified as partial without a local remainder or item-local salvage",
}

var semanticOutcomeMetricRegistry = map[string]map[string]struct{}{
	"accepted":                                architectureSemanticMetricNames(),
	"accepted_partial":                        architectureSemanticMetricNames(),
	"architecture.provider_call_failed":       architectureSemanticMetricNames(),
	"architecture.provider_output_limit":      architectureSemanticMetricNames(),
	"architecture.proposal_rejected":          architectureSemanticMetricNames(),
	"componentmap.response_evaluation_failed": architectureSemanticMetricNames(),
	"componentmap.partial_model_inconsistent": architectureSemanticMetricNames(),
	"cache_hit": architectureSemanticMetricNames(),
}

func architectureSemanticMetricNames() map[string]struct{} {
	return map[string]struct{}{
		"anchor_ref_count": {}, "component_count": {},
		"covered_conceptual_count": {}, "covered_primary_scope_count": {},
		"covered_supporting_evidence_count": {},
		"explicit_empty_anchor_refs_count":  {}, "json_valid": {},
		"member_ref_count": {}, "missing_anchor_refs_count": {},
		"null_anchor_refs_count": {}, "requested_conceptual_count": {},
		"requested_primary_scope_count": {},
		"subsystem_count":               {}, "uncovered_conceptual_count": {},
		"uncovered_primary_scope_count": {}, "unit_ref_count": {},
	}
}

func normalizedSemanticOutcome(exchange SemanticExchange) SemanticOutcome {
	outcome := exchange.Outcome
	if outcome.Phase != "" || outcome.Code != "" {
		return outcome
	}
	// State is lifecycle; ValidationCode is the stage owner's more precise
	// classification. In particular, a provider_failed lifecycle can still
	// carry response_decode when a completed response fails during recovery.
	switch exchange.ValidationCode {
	case SemanticValidationAccepted:
		return SemanticOutcome{Phase: "complete", Code: "accepted"}
	case SemanticValidationCache:
		return SemanticOutcome{Phase: "cache", Code: "cache_hit"}
	case SemanticValidationCanceled:
		return SemanticOutcome{Phase: "provider_call", Code: "canceled"}
	case SemanticValidationProvider:
		return SemanticOutcome{Phase: "provider_call", Code: "provider_failed"}
	case SemanticValidationSecret:
		return SemanticOutcome{Phase: "response_secret_scan", Code: "response_secret_scan"}
	case SemanticValidationDecode:
		return SemanticOutcome{Phase: "response_decode", Code: "response_decode"}
	case SemanticValidationResponse:
		return SemanticOutcome{Phase: "response_validation", Code: "response_validation"}
	case SemanticValidationApply:
		return SemanticOutcome{Phase: "projection_apply", Code: "projection_apply"}
	case SemanticValidationQuality:
		return SemanticOutcome{Phase: "projection_quality", Code: "projection_quality"}
	}
	return SemanticOutcome{Phase: "stage_state", Code: "unclassified"}
}

func validateSemanticOutcome(outcome SemanticOutcome) error {
	return validateSemanticOutcomeForStage("", outcome)
}

func validateSemanticOutcomeForStage(stage string, outcome SemanticOutcome) error {
	if !semanticDiagnosticNamePattern.MatchString(outcome.Phase) ||
		!semanticDiagnosticNamePattern.MatchString(outcome.Code) {
		return fmt.Errorf("semantic exchange: invalid outcome diagnostic")
	}
	if codes, exists := semanticOutcomeRegistry[outcome.Phase]; !exists {
		return fmt.Errorf("semantic exchange: unknown outcome phase")
	} else if _, exists := codes[outcome.Code]; !exists {
		return fmt.Errorf("semantic exchange: unknown outcome code")
	}
	if len(outcome.Detail) > 512 || strings.TrimSpace(outcome.Detail) != outcome.Detail ||
		strings.ContainsAny(outcome.Detail, "\r\n\t") || !utf8.ValidString(outcome.Detail) {
		return fmt.Errorf("semantic exchange: invalid outcome detail")
	}
	if outcome.Detail != "" {
		if semanticOutcomeDetails[outcome.Code] != outcome.Detail {
			return fmt.Errorf("semantic exchange: unregistered outcome detail")
		}
		if _, found := secretscan.DetectAlways(outcome.Detail); found {
			return fmt.Errorf("semantic exchange: unsafe outcome detail")
		}
	}
	if len(outcome.Metrics) > 32 {
		return fmt.Errorf("semantic exchange: too many outcome metrics")
	}
	if len(outcome.Metrics) > 0 && stage != "" && stage != SemanticStageArchitecture {
		return fmt.Errorf("semantic exchange: outcome metrics are not registered for stage")
	}
	allowedMetrics := semanticOutcomeMetricRegistry[outcome.Code]
	for index, metric := range outcome.Metrics {
		if !semanticDiagnosticNamePattern.MatchString(metric.Name) || metric.Value < 0 ||
			index > 0 && outcome.Metrics[index-1].Name >= metric.Name {
			return fmt.Errorf("semantic exchange: invalid outcome metrics")
		}
		if _, allowed := allowedMetrics[metric.Name]; !allowed {
			return fmt.Errorf("semantic exchange: outcome metric is not registered for code")
		}
	}
	return nil
}

func validSemanticStage(stage string) bool {
	switch stage {
	case SemanticStageOrientation,
		SemanticStageNavigator,
		SemanticStageAtlasStudy,
		SemanticStageTargetedResearch,
		SemanticStageArchitecture,
		SemanticStageGuidedTour,
		SemanticStageStudyBrief,
		SemanticStageStudyDirections,
		SemanticStageStudyReview,
		SemanticStageLocalization:
		return true
	default:
		return false
	}
}

func validSemanticState(state string) bool {
	switch state {
	case SemanticStateAccepted,
		SemanticStateRejected,
		SemanticStateCacheHit,
		SemanticStateCanceled,
		SemanticStateProviderFailed:
		return true
	default:
		return false
	}
}

func validSemanticValidationCode(code string) bool {
	switch code {
	case SemanticValidationAccepted,
		SemanticValidationCache,
		SemanticValidationCanceled,
		SemanticValidationProvider,
		SemanticValidationSecret,
		SemanticValidationDecode,
		SemanticValidationResponse,
		SemanticValidationApply,
		SemanticValidationQuality:
		return true
	default:
		return false
	}
}

func validSemanticUnavailableCode(code string) bool {
	switch code {
	case SemanticUnavailableNoContent,
		SemanticUnavailableCanceled,
		SemanticUnavailableCache,
		SemanticUnavailableProjection,
		SemanticUnavailableOmitted,
		SemanticUnavailableSize:
		return true
	default:
		return false
	}
}

func prepareSemanticPayload(
	label string,
	raw []byte,
	unavailable *SemanticUnavailable,
) (preparedSemanticPayload, error) {
	if label != "request" && label != "response" {
		return preparedSemanticPayload{}, fmt.Errorf("semantic exchange: invalid payload label")
	}
	if unavailable != nil {
		marker := semanticPayloadMarker{
			Version: semanticPayloadMarkerVersion, Storage: "raw_unavailable",
			OriginalSHA256: unavailable.OriginalSHA256,
			OriginalBytes:  unavailable.OriginalBytes, UnavailableCode: unavailable.Code,
		}
		return prepareSemanticMarker(label, marker)
	}
	originalSHA := sha256Hex(raw)
	redacted := sensitiveKeyPattern.ReplaceAll(raw, []byte(`"$1": "[redacted]"`))
	if kind, found := secretscan.DetectAlways(string(redacted)); found {
		return prepareSemanticMarker(label, semanticPayloadMarker{
			Version: semanticPayloadMarkerVersion, Storage: "unsafe_marker",
			OriginalSHA256: originalSHA, OriginalBytes: len(raw),
			UnsafeKind: secretscan.ClosedKind(kind),
		})
	}
	if len(redacted) > maxSemanticExchangePayloadSize {
		return prepareSemanticMarker(label, semanticPayloadMarker{
			Version: semanticPayloadMarkerVersion, Storage: "raw_unavailable",
			OriginalSHA256: originalSHA, OriginalBytes: len(raw),
			UnavailableCode: SemanticUnavailableSize,
		})
	}
	extension := ".txt"
	mediaType := "text/plain"
	if json.Valid(redacted) {
		extension = ".json"
		mediaType = "application/json"
	}
	name := label + extension
	return preparedSemanticPayload{
		name: name, data: append([]byte(nil), redacted...),
		record: SemanticPayloadRecord{
			Storage: "raw_content", File: name, MediaType: mediaType,
			OriginalSHA256: originalSHA, OriginalBytes: len(raw),
			SavedSHA256: sha256Hex(redacted), SavedBytes: len(redacted),
		},
	}, nil
}

func prepareSemanticMarker(
	label string,
	marker semanticPayloadMarker,
) (preparedSemanticPayload, error) {
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return preparedSemanticPayload{}, fmt.Errorf("encode semantic payload marker: %w", err)
	}
	data = append(data, '\n')
	name := label + ".marker.json"
	return preparedSemanticPayload{
		name: name, data: data,
		record: SemanticPayloadRecord{
			Storage: marker.Storage, File: name, MediaType: "application/json",
			OriginalSHA256: marker.OriginalSHA256, OriginalBytes: marker.OriginalBytes,
			SavedSHA256: sha256Hex(data), SavedBytes: len(data),
			UnsafeKind: marker.UnsafeKind, UnavailableCode: marker.UnavailableCode,
		},
	}, nil
}

func semanticExchangeKey(exchange SemanticExchange) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%d\x00%d\x00%s",
		exchange.Stage,
		exchange.InstanceOrdinal,
		exchange.SemanticAttemptOrdinal,
		sha256Hex(exchange.Request),
	)))
	return hex.EncodeToString(digest[:])
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (w *Writer) WriteMetadata(meta RunMeta) error {
	if w != nil && w.buildIdentity != nil {
		meta.BuildIdentity = *w.buildIdentity
	}
	if err := validateBuildIdentity(meta.BuildIdentity); err != nil {
		return fmt.Errorf("debug metadata: build identity: %w", err)
	}
	// RunMeta is passed by value, but its attempt slice may still share the
	// caller's backing array. Fill persistence-only defaults on a copy so a
	// later caller transition (prepared -> response_received -> rejected, for
	// example) cannot inherit an outcome derived from an earlier state.
	meta.RequestAttempts = append([]RequestAttempt(nil), meta.RequestAttempts...)
	for index := range meta.RequestAttempts {
		attempt := &meta.RequestAttempts[index]
		if attempt.Outcome == nil {
			outcome := requestAttemptOutcome(*attempt)
			attempt.Outcome = &outcome
		}
		if err := validateSemanticOutcomeForStage(attempt.Stage, *attempt.Outcome); err != nil {
			return fmt.Errorf("debug metadata: request attempt outcome: %w", err)
		}
	}
	data, _ := json.MarshalIndent(meta, "", "  ")
	return w.WriteFile("metadata.json", append(data, '\n'))
}

func validateBuildIdentity(identity BuildIdentity) error {
	if !identity.Available {
		if identity.GoVersion != "" || identity.ModulePath != "" || identity.ModuleVersion != "" ||
			identity.VCSRevision != "" || identity.VCSTime != "" || identity.VCSModified != nil {
			return fmt.Errorf("unavailable build identity contains fields")
		}
		return nil
	}
	fields := []struct {
		value string
		limit int
	}{
		{identity.GoVersion, 64},
		{identity.ModulePath, 512},
		{identity.ModuleVersion, 256},
		{identity.VCSRevision, 64},
		{identity.VCSTime, 64},
	}
	if identity.GoVersion == "" || identity.ModulePath == "" {
		return fmt.Errorf("available build identity is incomplete")
	}
	for _, field := range fields {
		if len(field.value) > field.limit || !utf8.ValidString(field.value) ||
			strings.TrimSpace(field.value) != field.value || strings.ContainsAny(field.value, "\r\n\t") {
			return fmt.Errorf("build identity contains an invalid field")
		}
	}
	if strings.HasPrefix(identity.ModulePath, "/") || strings.Contains(identity.ModulePath, "\\") ||
		strings.Contains(identity.ModulePath, "..") {
		return fmt.Errorf("build identity module path is invalid")
	}
	if identity.VCSRevision != "" && !validLowerHex(identity.VCSRevision, 40, 64) {
		return fmt.Errorf("build identity revision is invalid")
	}
	if identity.VCSTime != "" {
		if _, err := time.Parse(time.RFC3339, identity.VCSTime); err != nil {
			return fmt.Errorf("build identity time is invalid")
		}
	}
	joined := strings.Join([]string{
		identity.GoVersion, identity.ModulePath, identity.ModuleVersion,
		identity.VCSRevision, identity.VCSTime,
	}, "\n")
	if _, found := secretscan.DetectAlways(joined); found {
		return fmt.Errorf("build identity contains unsafe material")
	}
	return nil
}

func validLowerHex(value string, lengths ...int) bool {
	lengthAllowed := false
	for _, length := range lengths {
		if len(value) == length {
			lengthAllowed = true
			break
		}
	}
	if !lengthAllowed {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') ||
			(character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func requestAttemptOutcome(attempt RequestAttempt) SemanticOutcome {
	if attempt.Stage == "configuration" && attempt.State == "failed" {
		return SemanticOutcome{
			Phase: "provider_configuration", Code: "provider_configuration_failed",
		}
	}
	switch attempt.State {
	case "accepted", "completed", "empty", "no_new_evidence", "succeeded":
		return SemanticOutcome{Phase: "complete", Code: "accepted"}
	case "accepted_partial", "accepted_with_rejections":
		return SemanticOutcome{Phase: "complete", Code: "accepted_partial"}
	case "cache_hit", "cached":
		return SemanticOutcome{Phase: "cache", Code: "cache_hit"}
	case "canceled":
		return SemanticOutcome{Phase: "provider_call", Code: "canceled"}
	case "response_received":
		return SemanticOutcome{Phase: "provider_call", Code: "response_received"}
	case "failed", "provider_failed":
		return SemanticOutcome{Phase: "provider_call", Code: "provider_failed"}
	case "response_rejected":
		return SemanticOutcome{Phase: "response_secret_scan", Code: "response_secret_scan"}
	case "response_parse_failed":
		return SemanticOutcome{Phase: "response_decode", Code: "response_decode"}
	case "rejected", "response_validation_failed":
		return SemanticOutcome{Phase: "response_validation", Code: "response_validation"}
	case "budget_exhausted", "resource_exhausted":
		return SemanticOutcome{Phase: "resource_exhausted", Code: "resource_exhausted"}
	case "unavailable":
		return SemanticOutcome{Phase: "availability", Code: "unavailable"}
	case "not_called", "skipped", "skipped_insufficient_evidence", "skipped_local_complete", "skipped_offline":
		return SemanticOutcome{Phase: "availability", Code: "not_called"}
	case "prepared":
		return SemanticOutcome{Phase: "request_build", Code: "prepared"}
	case "request_build_failed":
		return SemanticOutcome{Phase: "request_build", Code: "request_build_failed"}
	default:
		return SemanticOutcome{Phase: "stage_state", Code: "unclassified"}
	}
}

func currentBuildIdentity() BuildIdentity {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return BuildIdentity{}
	}
	identity := BuildIdentity{
		Available:     true,
		GoVersion:     info.GoVersion,
		ModulePath:    info.Main.Path,
		ModuleVersion: info.Main.Version,
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			identity.VCSRevision = setting.Value
		case "vcs.time":
			identity.VCSTime = setting.Value
		case "vcs.modified":
			modified := setting.Value == "true"
			identity.VCSModified = &modified
		}
	}
	return identity
}

func (w *Writer) WriteSnapshot(snapshotJSON []byte) error {
	return w.WriteFile("snapshot.json", snapshotJSON)
}

// WriteLLMBundleWithSidecar writes the exact persisted bundle bytes and a
// producer-built sidecar derived from those same post-redaction bytes.
func (w *Writer) WriteLLMBundleWithSidecar(
	bundleJSON []byte,
	sidecarName string,
	buildSidecar func([]byte) ([]byte, error),
) error {
	return w.writeRootFileWithSidecar(
		"llm_bundle.json",
		bundleJSON,
		sidecarName,
		buildSidecar,
	)
}

func (w *Writer) WriteOrientationReport(reportJSON []byte) error {
	return w.WriteFile("orientation_report.json", reportJSON)
}

// WriteOrientationReportWithSidecar writes the exact report bytes and then a
// producer-built sidecar derived from those same bytes. The callback observes
// the post-redaction payload that is actually persisted, so a sidecar hash can
// never accidentally bind the pre-redaction input.
func (w *Writer) WriteOrientationReportWithSidecar(
	reportJSON []byte,
	sidecarName string,
	buildSidecar func([]byte) ([]byte, error),
) error {
	return w.writeRootFileWithSidecar(
		"orientation_report.json",
		reportJSON,
		sidecarName,
		buildSidecar,
	)
}

func (w *Writer) writeRootFileWithSidecar(
	primaryName string,
	primaryJSON []byte,
	sidecarName string,
	buildSidecar func([]byte) ([]byte, error),
) error {
	if w == nil || w.root == nil {
		return fmt.Errorf("debug writer is closed")
	}
	if buildSidecar == nil {
		return fmt.Errorf("debug sidecar builder is required")
	}
	preparedPrimary := primaryJSON
	if w.Redacted {
		preparedPrimary = redactJSON(preparedPrimary)
	}
	// The callback may retain or mutate its argument. Give it an exact copy so
	// the prepared primary remains the immutable write authority.
	sidecar, err := buildSidecar(append([]byte(nil), preparedPrimary...))
	if err != nil {
		return fmt.Errorf("build %s: %w", sidecarName, err)
	}
	if w.Redacted {
		sidecar = redactJSON(sidecar)
	}
	if err := w.writePreparedRootFile(primaryName, preparedPrimary); err != nil {
		return err
	}
	return w.writePreparedRootFile(sidecarName, sidecar)
}

func (w *Writer) WriteOrientationValidation(validationJSON []byte) error {
	return w.WriteFile("orientation_validation.json", validationJSON)
}

func (w *Writer) WriteDirFile(subdir, name string, data []byte) error {
	localSubdir := filepath.FromSlash(subdir)
	if w == nil || w.root == nil || subdir == "" || !filepath.IsLocal(localSubdir) ||
		filepath.Clean(localSubdir) != localSubdir || localSubdir == "." ||
		name == "" || filepath.Base(name) != name {
		return fmt.Errorf("invalid debug artifact subpath")
	}
	if err := w.root.MkdirAll(localSubdir, 0o700); err != nil {
		return err
	}
	return w.writeRootFile(filepath.Join(localSubdir, name), data)
}

func (w *Writer) WriteDirError(subdir string, err error) {
	data := []byte(fmt.Sprintf("error: %v\n", err))
	_ = w.WriteDirFile(subdir, "error.txt", data)
}

func (w *Writer) RunDir() string {
	return w.runDir
}

// Close releases the directory handle that confines artifact writes.
func (w *Writer) Close() error {
	if w == nil {
		return nil
	}
	w.semanticMu.Lock()
	defer w.semanticMu.Unlock()
	if w.root == nil {
		return nil
	}
	err := w.root.Close()
	w.root = nil
	return err
}

func (w *Writer) WriteError(err error) {
	data := []byte(fmt.Sprintf("error: %v\n", err))
	_ = w.WriteFile("error.txt", data)
}

var sensitiveKeyPattern = regexp.MustCompile(
	`(?i)"(` + strings.Join([]string{
		`api_key`,
		`apikey`,
		`api-key`,
		`authorization`,
		`bearer`,
		`token`,
		`secret`,
		`password`,
		`passwd`,
		`private_key`,
		`private-key`,
		`access_key`,
		`access-key`,
		`refresh_token`,
		`refresh-token`,
	}, "|") + `)"\s*:\s*"[^"]*"`,
)

func redactJSON(data []byte) []byte {
	redacted := sensitiveKeyPattern.ReplaceAll(data, []byte(`"$1": "[redacted]"`))
	if kind, found := secretscan.DetectAlways(string(redacted)); found {
		return []byte(fmt.Sprintf("[redacted: %s detected]\n", kind))
	}
	return redacted
}

func GenerateRunID(repoName string) string {
	ts := time.Now().UTC().Format("20060102-150405")
	var nonce [6]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fmt.Sprintf("%s-%s-%x", ts, sanitize(repoName), time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("%s-%s-%s", ts, sanitize(repoName), hex.EncodeToString(nonce[:]))
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, s)
}
