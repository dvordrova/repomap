package report

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	ArchitectureLocalizationRecordVersion    = 1
	ArchitectureLocalizationProjectorVersion = "architecture-canvas-localization-v1"

	architectureLocalizationRecordRoot       = ".localization-projections"
	architectureLocalizationRecordVersionDir = "v1"
	architectureLocalizationRecordStage      = "architecture-localization-projection"

	maxArchitectureLocalizationRecordBytes    = 5 << 20
	maxArchitectureLocalizationIdentityScalar = 4 << 10
)

const (
	ArchitectureLocalizationRecordMiss   = "miss_not_found"
	ArchitectureLocalizationRecordStored = "stored"
	ArchitectureLocalizationRecordHit    = "hit_exact"
)

type ArchitectureLocalizationProjectionRecordIdentity struct {
	Version               int      `json:"version"`
	Stage                 string   `json:"stage"`
	ProjectorVersion      string   `json:"projector_version"`
	CanonicalSHA256       string   `json:"canonical_sha256"`
	InputVersion          int      `json:"input_version"`
	InputBytes            int      `json:"input_bytes"`
	InputSHA256           string   `json:"input_sha256"`
	ProjectionVersion     int      `json:"projection_version"`
	SourceLocale          string   `json:"source_locale"`
	TargetLocale          string   `json:"target_locale"`
	PromptVersion         string   `json:"prompt_version"`
	PromptBytes           int      `json:"prompt_bytes"`
	PromptSHA256          string   `json:"prompt_sha256"`
	RequestVersion        string   `json:"request_version"`
	Provider              string   `json:"provider"`
	Endpoint              string   `json:"endpoint"`
	AuthMode              string   `json:"auth_mode"`
	Model                 string   `json:"model"`
	Temperature           *float64 `json:"temperature"`
	MaxTokens             int      `json:"max_tokens"`
	ResponseFormat        string   `json:"response_format"`
	Thinking              string   `json:"thinking,omitempty"`
	ReasoningEffort       string   `json:"reasoning_effort,omitempty"`
	ProviderRequestBytes  int      `json:"provider_request_bytes"`
	ProviderRequestSHA256 string   `json:"provider_request_sha256"`
}

type ArchitectureLocalizationProjectionRecord struct {
	Version          int                                              `json:"version"`
	Key              string                                           `json:"key"`
	Identity         ArchitectureLocalizationProjectionRecordIdentity `json:"identity"`
	ProviderRequest  []byte                                           `json:"provider_request"`
	Projection       []byte                                           `json:"projection"`
	ProjectionSHA256 string                                           `json:"projection_sha256"`
}

type ArchitectureLocalizationRecordResult struct {
	Version          int                             `json:"version"`
	Status           string                          `json:"status"`
	Key              string                          `json:"key"`
	RecordPath       string                          `json:"record_path"`
	RequestSHA256    string                          `json:"provider_request_sha256"`
	ProjectionSHA256 string                          `json:"projection_sha256,omitempty"`
	Replay           *ArchitectureLocalizationReplay `json:"replay,omitempty"`
}

type architectureLocalizationRequestBuilder func(
	localization.Prompt,
) (deepseek.LocalizationRequestEvidence, error)

type architectureLocalizationRecordResponse func(context.Context) ([]byte, error)

type preparedArchitectureLocalizationRecord struct {
	runDir       string
	runInfo      os.FileInfo
	localization preparedArchitectureLocalizationRussian
	prompt       localization.Prompt
	promptJSON   []byte
	request      deepseek.LocalizationRequestEvidence
	identity     ArchitectureLocalizationProjectionRecordIdentity
	key          string
	recordPath   string
}

type validatedArchitectureLocalizationRecord struct {
	record ArchitectureLocalizationProjectionRecord
	replay ArchitectureLocalizationReplay
}

// ReplayArchitectureLocalizationRussianRecordFile performs one explicit
// provider-free immutable-record lookup. A supplied local response is opened
// only after an exact miss and is never consulted on an exact hit.
func ReplayArchitectureLocalizationRussianRecordFile(
	ctx context.Context,
	runDir,
	responsePath string,
	buildRequest architectureLocalizationRequestBuilder,
) ([]byte, error) {
	var response architectureLocalizationRecordResponse
	if responsePath != "" {
		response = func(context.Context) ([]byte, error) {
			return readArchitectureLocalizationProjectionFile(responsePath)
		}
	}
	return replayArchitectureLocalizationRussianRecord(
		ctx,
		runDir,
		buildRequest,
		response,
	)
}

func replayArchitectureLocalizationRussianRecord(
	ctx context.Context,
	runDir string,
	buildRequest architectureLocalizationRequestBuilder,
	response architectureLocalizationRecordResponse,
) ([]byte, error) {
	if buildRequest == nil {
		return nil, fmt.Errorf("architecture localization record: request builder is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prepared, err := prepareArchitectureLocalizationRecord(runDir, buildRequest)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cached, found, err := loadArchitectureLocalizationProjectionRecord(prepared)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if found {
		return encodeArchitectureLocalizationRecordResult(
			prepared,
			ArchitectureLocalizationRecordHit,
			cached.record.ProjectionSHA256,
			cached.replay,
		)
	}
	if response == nil {
		return encodeArchitectureLocalizationRecordResult(
			prepared,
			ArchitectureLocalizationRecordMiss,
			"",
			ArchitectureLocalizationReplay{},
		)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	responseJSON, err := response(ctx)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization record: saved response unavailable",
		)
	}
	projection, err := localization.DecodeRussianProviderResponse(
		prepared.localization.canonical,
		prepared.localization.input,
		responseJSON,
	)
	if err != nil {
		return nil, err
	}
	projectionJSON, replay, err := acceptedArchitectureLocalizationRecordProjection(
		prepared.localization,
		projection,
	)
	if err != nil {
		return nil, err
	}
	record := ArchitectureLocalizationProjectionRecord{
		Version:          ArchitectureLocalizationRecordVersion,
		Key:              prepared.key,
		Identity:         prepared.identity,
		ProviderRequest:  append([]byte(nil), prepared.request.Body...),
		Projection:       projectionJSON,
		ProjectionSHA256: architectureLocalizationSHA256(projectionJSON),
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	winner, published, err := publishArchitectureLocalizationProjectionRecord(
		ctx,
		prepared,
		record,
	)
	if err != nil {
		return nil, err
	}
	if !published {
		replay = winner.replay
		record = winner.record
	}
	status := ArchitectureLocalizationRecordStored
	if !published {
		status = ArchitectureLocalizationRecordHit
	}
	return encodeArchitectureLocalizationRecordResult(
		prepared,
		status,
		record.ProjectionSHA256,
		replay,
	)
}

func prepareArchitectureLocalizationRecord(
	runDir string,
	buildRequest architectureLocalizationRequestBuilder,
) (preparedArchitectureLocalizationRecord, error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return preparedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: resolve run dir: %w",
			err,
		)
	}
	runInfo, err := os.Lstat(absDir)
	if err != nil {
		return preparedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: inspect run dir: %w",
			err,
		)
	}
	if runInfo.Mode()&os.ModeSymlink != 0 || !runInfo.IsDir() {
		return preparedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: run path is not a real directory",
		)
	}
	prepared, err := prepareArchitectureLocalizationRussian(absDir)
	if err != nil {
		return preparedArchitectureLocalizationRecord{}, err
	}
	prompt, promptJSON, err := buildArchitectureLocalizationRussianPrompt(prepared)
	if err != nil {
		return preparedArchitectureLocalizationRecord{}, err
	}
	request, err := buildRequest(prompt)
	if err != nil {
		return preparedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: build provider request: %w",
			err,
		)
	}
	if err := request.Validate(prompt); err != nil {
		return preparedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: provider request evidence was rejected",
		)
	}
	identity, err := buildArchitectureLocalizationProjectionRecordIdentity(
		prepared,
		promptJSON,
		request,
	)
	if err != nil {
		return preparedArchitectureLocalizationRecord{}, err
	}
	identityJSON, err := json.Marshal(identity)
	if err != nil {
		return preparedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: encode identity: %w",
			err,
		)
	}
	key := "architecture-" + architectureLocalizationSHA256(identityJSON)
	recordPath := filepath.Join(
		architectureLocalizationRecordRoot,
		architectureLocalizationRecordVersionDir,
		key+".json",
	)
	currentRunInfo, err := os.Stat(absDir)
	if err != nil {
		return preparedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: reinspect run dir: %w",
			err,
		)
	}
	if !os.SameFile(runInfo, currentRunInfo) {
		return preparedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: run directory changed during preparation",
		)
	}
	return preparedArchitectureLocalizationRecord{
		runDir:       absDir,
		runInfo:      runInfo,
		localization: prepared,
		prompt:       prompt,
		promptJSON:   promptJSON,
		request:      request,
		identity:     identity,
		key:          key,
		recordPath:   recordPath,
	}, nil
}

func buildArchitectureLocalizationProjectionRecordIdentity(
	prepared preparedArchitectureLocalizationRussian,
	promptJSON []byte,
	request deepseek.LocalizationRequestEvidence,
) (ArchitectureLocalizationProjectionRecordIdentity, error) {
	inputJSON, err := json.Marshal(prepared.input)
	if err != nil {
		return ArchitectureLocalizationProjectionRecordIdentity{}, fmt.Errorf(
			"architecture localization record: encode localization input: %w",
			err,
		)
	}
	if len(inputJSON) == 0 || len(inputJSON) > maxArchitectureLocalizationArtifactBytes {
		return ArchitectureLocalizationProjectionRecordIdentity{}, fmt.Errorf(
			"architecture localization record: input exceeds its byte limit",
		)
	}
	if len(promptJSON) == 0 || len(promptJSON) > maxArchitectureLocalizationArtifactBytes ||
		len(request.Body) == 0 ||
		len(request.Body) > deepseek.MaxLocalizationRequestBodyBytes {
		return ArchitectureLocalizationProjectionRecordIdentity{}, fmt.Errorf(
			"architecture localization record: request exceeds its byte limit",
		)
	}
	scalars := []string{
		request.Version,
		request.Provider,
		request.Endpoint,
		request.AuthMode,
		request.Model,
		request.ResponseFormat,
		request.Thinking,
		request.ReasoningEffort,
	}
	for _, value := range scalars {
		if len(value) > maxArchitectureLocalizationIdentityScalar ||
			!utf8.ValidString(value) ||
			strings.ContainsAny(value, "\x00\r\n") {
			return ArchitectureLocalizationProjectionRecordIdentity{}, fmt.Errorf(
				"architecture localization record: invalid provider request identity",
			)
		}
		if kind, found := secretscan.DetectAlways(value); found {
			return ArchitectureLocalizationProjectionRecordIdentity{}, fmt.Errorf(
				"architecture localization record: request identity contains an obvious %s",
				kind,
			)
		}
	}
	if request.Version == "" ||
		request.Provider == "" ||
		request.Endpoint == "" ||
		request.AuthMode == "" ||
		request.Model == "" ||
		request.ResponseFormat != "json_object" ||
		request.MaxTokens <= 0 ||
		request.Temperature == nil ||
		*request.Temperature != 0 {
		return ArchitectureLocalizationProjectionRecordIdentity{}, fmt.Errorf(
			"architecture localization record: incomplete provider request identity",
		)
	}
	if kind, found := secretscan.DetectAlways(string(request.Body)); found {
		return ArchitectureLocalizationProjectionRecordIdentity{}, fmt.Errorf(
			"architecture localization record: request contains an obvious %s",
			kind,
		)
	}
	return ArchitectureLocalizationProjectionRecordIdentity{
		Version:               ArchitectureLocalizationRecordVersion,
		Stage:                 architectureLocalizationRecordStage,
		ProjectorVersion:      ArchitectureLocalizationProjectorVersion,
		CanonicalSHA256:       prepared.canonical.SHA256,
		InputVersion:          prepared.input.Version,
		InputBytes:            len(inputJSON),
		InputSHA256:           architectureLocalizationSHA256(inputJSON),
		ProjectionVersion:     localization.ProjectionVersion,
		SourceLocale:          prepared.input.SourceLocale,
		TargetLocale:          prepared.input.TargetLocale,
		PromptVersion:         localization.PromptVersion,
		PromptBytes:           len(promptJSON),
		PromptSHA256:          architectureLocalizationSHA256(promptJSON),
		RequestVersion:        request.Version,
		Provider:              request.Provider,
		Endpoint:              request.Endpoint,
		AuthMode:              request.AuthMode,
		Model:                 request.Model,
		Temperature:           cloneArchitectureLocalizationFloat(request.Temperature),
		MaxTokens:             request.MaxTokens,
		ResponseFormat:        request.ResponseFormat,
		Thinking:              request.Thinking,
		ReasoningEffort:       request.ReasoningEffort,
		ProviderRequestBytes:  len(request.Body),
		ProviderRequestSHA256: architectureLocalizationSHA256(request.Body),
	}, nil
}

func acceptedArchitectureLocalizationRecordProjection(
	prepared preparedArchitectureLocalizationRussian,
	projection localization.Projection,
) ([]byte, ArchitectureLocalizationReplay, error) {
	replayJSON, err := replayPreparedArchitectureLocalizationRussian(prepared, projection)
	if err != nil {
		return nil, ArchitectureLocalizationReplay{}, err
	}
	var replay ArchitectureLocalizationReplay
	if err := decodeArchitectureLocalizationJSON(replayJSON, &replay); err != nil {
		return nil, ArchitectureLocalizationReplay{}, fmt.Errorf(
			"architecture localization record: replay result is not strict JSON",
		)
	}
	if replay.Locale != localization.LocaleRussian ||
		replay.Fallback ||
		len(replay.Diagnostics) != 0 {
		return nil, ArchitectureLocalizationReplay{}, fmt.Errorf(
			"architecture localization record: projection was not fully accepted",
		)
	}
	projectionJSON, err := json.Marshal(projection)
	if err != nil {
		return nil, ArchitectureLocalizationReplay{}, fmt.Errorf(
			"architecture localization record: encode projection: %w",
			err,
		)
	}
	if len(projectionJSON) == 0 ||
		len(projectionJSON) > maxArchitectureLocalizationArtifactBytes {
		return nil, ArchitectureLocalizationReplay{}, fmt.Errorf(
			"architecture localization record: projection exceeds its byte limit",
		)
	}
	if kind, found := architectureLocalizationCredential(
		prepared.canonical,
		prepared.input,
		projection,
	); found {
		return nil, ArchitectureLocalizationReplay{}, fmt.Errorf(
			"architecture localization record: projection contains an obvious %s",
			kind,
		)
	}
	if kind, found := secretscan.DetectAlways(string(projectionJSON)); found {
		return nil, ArchitectureLocalizationReplay{}, fmt.Errorf(
			"architecture localization record: encoded projection contains an obvious %s",
			kind,
		)
	}
	return projectionJSON, replay, nil
}

func openPreparedArchitectureLocalizationRoot(
	prepared preparedArchitectureLocalizationRecord,
) (*os.Root, error) {
	root, err := os.OpenRoot(prepared.runDir)
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization record: open run root: %w",
			err,
		)
	}
	info, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf(
			"architecture localization record: stat run root: %w",
			err,
		)
	}
	if prepared.runInfo == nil || !info.IsDir() ||
		!os.SameFile(prepared.runInfo, info) {
		_ = root.Close()
		return nil, fmt.Errorf(
			"architecture localization record: run root identity mismatch",
		)
	}
	return root, nil
}

func loadArchitectureLocalizationProjectionRecord(
	prepared preparedArchitectureLocalizationRecord,
) (validatedArchitectureLocalizationRecord, bool, error) {
	root, err := openPreparedArchitectureLocalizationRoot(prepared)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, err
	}
	defer root.Close()
	versionDir := filepath.Join(
		architectureLocalizationRecordRoot,
		architectureLocalizationRecordVersionDir,
	)
	for _, dir := range []string{architectureLocalizationRecordRoot, versionDir} {
		info, inspectErr := root.Lstat(dir)
		if os.IsNotExist(inspectErr) {
			return validatedArchitectureLocalizationRecord{}, false, nil
		}
		if inspectErr != nil {
			return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
				"architecture localization record: inspect record directory: %w",
				inspectErr,
			)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
				"architecture localization record: record directory is not a real directory",
			)
		}
	}
	versionInfo, err := root.Lstat(versionDir)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: reinspect record directory: %w",
			err,
		)
	}
	versionRoot, err := root.OpenRoot(versionDir)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: open record directory: %w",
			err,
		)
	}
	defer versionRoot.Close()
	openedVersionInfo, err := versionRoot.Stat(".")
	if err != nil || !openedVersionInfo.IsDir() ||
		!os.SameFile(versionInfo, openedVersionInfo) {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: record directory changed before open",
		)
	}
	recordName := filepath.Base(prepared.recordPath)
	info, err := versionRoot.Lstat(recordName)
	if os.IsNotExist(err) {
		return validatedArchitectureLocalizationRecord{}, false, nil
	}
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: inspect exact record: %w",
			err,
		)
	}
	if info.Mode()&os.ModeSymlink != 0 ||
		!info.Mode().IsRegular() ||
		info.Size() <= 0 ||
		info.Size() > maxArchitectureLocalizationRecordBytes {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: exact record is not a bounded regular file",
		)
	}
	file, err := versionRoot.Open(recordName)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: open exact record: %w",
			err,
		)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: stat exact record: %w",
			err,
		)
	}
	if !openedInfo.Mode().IsRegular() ||
		openedInfo.Size() <= 0 ||
		openedInfo.Size() > maxArchitectureLocalizationRecordBytes ||
		!os.SameFile(info, openedInfo) {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: exact record changed before open",
		)
	}
	data, err := io.ReadAll(io.LimitReader(
		file,
		maxArchitectureLocalizationRecordBytes+1,
	))
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: read exact record: %w",
			err,
		)
	}
	if len(data) == 0 || len(data) > maxArchitectureLocalizationRecordBytes {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: exact record exceeds its byte limit",
		)
	}
	if kind, found := secretscan.DetectAlways(string(data)); found {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: exact record contains an obvious %s",
			kind,
		)
	}
	var record ArchitectureLocalizationProjectionRecord
	if err := decodeArchitectureLocalizationJSON(data, &record); err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: exact record is not strict JSON",
		)
	}
	canonicalRecord, err := json.Marshal(record)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: exact record cannot be normalized",
		)
	}
	canonicalRecord = append(canonicalRecord, '\n')
	if !bytes.Equal(data, canonicalRecord) {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: exact record is not canonical JSON",
		)
	}
	validated, err := validateArchitectureLocalizationProjectionRecord(
		prepared,
		record,
	)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, err
	}
	return validated, true, nil
}

func validateArchitectureLocalizationProjectionRecord(
	prepared preparedArchitectureLocalizationRecord,
	record ArchitectureLocalizationProjectionRecord,
) (validatedArchitectureLocalizationRecord, error) {
	recordIdentityJSON, recordIdentityErr := json.Marshal(record.Identity)
	preparedIdentityJSON, preparedIdentityErr := json.Marshal(prepared.identity)
	if recordIdentityErr != nil ||
		preparedIdentityErr != nil ||
		record.Version != ArchitectureLocalizationRecordVersion ||
		record.Key != prepared.key ||
		!bytes.Equal(recordIdentityJSON, preparedIdentityJSON) {
		return validatedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: exact record identity mismatch",
		)
	}
	if len(record.ProviderRequest) == 0 ||
		len(record.ProviderRequest) > deepseek.MaxLocalizationRequestBodyBytes ||
		record.Identity.ProviderRequestBytes != len(record.ProviderRequest) ||
		record.Identity.ProviderRequestSHA256 !=
			architectureLocalizationSHA256(record.ProviderRequest) ||
		!bytes.Equal(record.ProviderRequest, prepared.request.Body) {
		return validatedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: exact provider request mismatch",
		)
	}
	if len(record.Projection) == 0 ||
		len(record.Projection) > maxArchitectureLocalizationArtifactBytes ||
		record.ProjectionSHA256 != architectureLocalizationSHA256(record.Projection) {
		return validatedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: exact projection hash mismatch",
		)
	}
	if kind, found := secretscan.DetectAlways(string(record.ProviderRequest)); found {
		return validatedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: exact provider request contains an obvious %s",
			kind,
		)
	}
	projection, err := decodeArchitectureLocalizationProjection(record.Projection)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: exact projection was rejected",
		)
	}
	projectionJSON, replay, err := acceptedArchitectureLocalizationRecordProjection(
		prepared.localization,
		projection,
	)
	if err != nil || !bytes.Equal(projectionJSON, record.Projection) {
		return validatedArchitectureLocalizationRecord{}, fmt.Errorf(
			"architecture localization record: exact projection replay was rejected",
		)
	}
	return validatedArchitectureLocalizationRecord{
		record: record,
		replay: replay,
	}, nil
}

func publishArchitectureLocalizationProjectionRecord(
	ctx context.Context,
	prepared preparedArchitectureLocalizationRecord,
	record ArchitectureLocalizationProjectionRecord,
) (validatedArchitectureLocalizationRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return validatedArchitectureLocalizationRecord{}, false, err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: encode exact record: %w",
			err,
		)
	}
	data = append(data, '\n')
	if len(data) > maxArchitectureLocalizationRecordBytes {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: encoded record exceeds its byte limit",
		)
	}
	if kind, found := secretscan.DetectAlways(string(data)); found {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: encoded record contains an obvious %s",
			kind,
		)
	}
	root, err := openPreparedArchitectureLocalizationRoot(prepared)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, err
	}
	defer root.Close()
	if err := ctx.Err(); err != nil {
		return validatedArchitectureLocalizationRecord{}, false, err
	}
	recordRootCreated, err := ensureArchitectureLocalizationRecordDirectory(
		root,
		architectureLocalizationRecordRoot,
	)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, err
	}
	if recordRootCreated {
		if err := syncArchitectureLocalizationDirectory(root, "."); err != nil {
			return validatedArchitectureLocalizationRecord{}, false, err
		}
	}
	versionDir := filepath.Join(
		architectureLocalizationRecordRoot,
		architectureLocalizationRecordVersionDir,
	)
	versionCreated, err := ensureArchitectureLocalizationRecordDirectory(root, versionDir)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, err
	}
	if versionCreated {
		if err := syncArchitectureLocalizationDirectory(
			root,
			architectureLocalizationRecordRoot,
		); err != nil {
			return validatedArchitectureLocalizationRecord{}, false, err
		}
	}
	if err := ctx.Err(); err != nil {
		return validatedArchitectureLocalizationRecord{}, false, err
	}
	versionInfo, err := root.Lstat(versionDir)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: inspect record directory: %w",
			err,
		)
	}
	versionRoot, err := root.OpenRoot(versionDir)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: open record directory: %w",
			err,
		)
	}
	defer versionRoot.Close()
	openedVersionInfo, err := versionRoot.Stat(".")
	if err != nil || !openedVersionInfo.IsDir() ||
		!os.SameFile(versionInfo, openedVersionInfo) {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: record directory changed before open",
		)
	}
	tempName, file, err := createArchitectureLocalizationRecordTemp(versionRoot)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, err
	}
	defer func() {
		_ = file.Close()
		if tempName != "" {
			_ = versionRoot.Remove(tempName)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: write temporary record: %w",
			err,
		)
	}
	if err := file.Chmod(0o600); err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: protect temporary record: %w",
			err,
		)
	}
	if err := file.Sync(); err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: sync temporary record: %w",
			err,
		)
	}
	if err := file.Close(); err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: close temporary record: %w",
			err,
		)
	}
	if err := ctx.Err(); err != nil {
		return validatedArchitectureLocalizationRecord{}, false, err
	}
	recordName := filepath.Base(prepared.recordPath)
	if err := versionRoot.Link(tempName, recordName); err != nil {
		if os.IsExist(err) {
			winner, found, loadErr := loadArchitectureLocalizationProjectionRecord(
				prepared,
			)
			if loadErr != nil {
				return validatedArchitectureLocalizationRecord{}, false, loadErr
			}
			if !found {
				return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
					"architecture localization record: concurrent winner disappeared",
				)
			}
			return winner, false, nil
		}
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: publish immutable record: %w",
			err,
		)
	}
	if err := versionRoot.Remove(tempName); err != nil {
		return validatedArchitectureLocalizationRecord{}, false, fmt.Errorf(
			"architecture localization record: remove temporary record link: %w",
			err,
		)
	}
	tempName = ""
	if err := syncArchitectureLocalizationDirectory(versionRoot, "."); err != nil {
		return validatedArchitectureLocalizationRecord{}, false, err
	}
	validated, err := validateArchitectureLocalizationProjectionRecord(
		prepared,
		record,
	)
	if err != nil {
		return validatedArchitectureLocalizationRecord{}, false, err
	}
	return validated, true, nil
}

func ensureArchitectureLocalizationRecordDirectory(
	root *os.Root,
	name string,
) (bool, error) {
	info, err := root.Lstat(name)
	created := false
	if os.IsNotExist(err) {
		if mkdirErr := root.Mkdir(name, 0o700); mkdirErr != nil &&
			!os.IsExist(mkdirErr) {
			return false, fmt.Errorf(
				"architecture localization record: create record directory: %w",
				mkdirErr,
			)
		} else if mkdirErr == nil {
			created = true
		}
		info, err = root.Lstat(name)
	}
	switch {
	case err != nil:
		return false, fmt.Errorf(
			"architecture localization record: inspect record directory: %w",
			err,
		)
	case info.Mode()&os.ModeSymlink != 0 || !info.IsDir():
		return false, fmt.Errorf(
			"architecture localization record: record directory is not a real directory",
		)
	}
	if err := root.Chmod(name, 0o700); err != nil {
		return false, fmt.Errorf(
			"architecture localization record: protect record directory: %w",
			err,
		)
	}
	return created, nil
}

func syncArchitectureLocalizationDirectory(root *os.Root, name string) error {
	directory, err := root.Open(name)
	if err != nil {
		return fmt.Errorf(
			"architecture localization record: open record directory for sync: %w",
			err,
		)
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return fmt.Errorf(
			"architecture localization record: sync record directory: %w",
			err,
		)
	}
	if err := directory.Close(); err != nil {
		return fmt.Errorf(
			"architecture localization record: close record directory: %w",
			err,
		)
	}
	return nil
}

func createArchitectureLocalizationRecordTemp(
	root *os.Root,
) (string, *os.File, error) {
	for attempt := 0; attempt < 8; attempt++ {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return "", nil, fmt.Errorf(
				"architecture localization record: create random temp name: %w",
				err,
			)
		}
		name := ".architecture-" + hex.EncodeToString(nonce[:]) + ".tmp"
		file, err := root.OpenFile(
			name,
			os.O_WRONLY|os.O_CREATE|os.O_EXCL,
			0o600,
		)
		if err == nil {
			return name, file, nil
		}
		if !os.IsExist(err) {
			return "", nil, fmt.Errorf(
				"architecture localization record: create temporary record: %w",
				err,
			)
		}
	}
	return "", nil, fmt.Errorf(
		"architecture localization record: temporary name collision",
	)
}

func encodeArchitectureLocalizationRecordResult(
	prepared preparedArchitectureLocalizationRecord,
	status,
	projectionSHA string,
	replay ArchitectureLocalizationReplay,
) ([]byte, error) {
	result := ArchitectureLocalizationRecordResult{
		Version:          ArchitectureLocalizationRecordVersion,
		Status:           status,
		Key:              prepared.key,
		RecordPath:       prepared.recordPath,
		RequestSHA256:    prepared.identity.ProviderRequestSHA256,
		ProjectionSHA256: projectionSHA,
	}
	if status != ArchitectureLocalizationRecordMiss {
		copied := replay
		result.Replay = &copied
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization record: encode result: %w",
			err,
		)
	}
	if len(encoded) == 0 || len(encoded) > maxArchitectureLocalizationRecordBytes {
		return nil, fmt.Errorf(
			"architecture localization record: result exceeds its byte limit",
		)
	}
	if kind, found := secretscan.DetectAlways(string(encoded)); found {
		return nil, fmt.Errorf(
			"architecture localization record: result contains an obvious %s",
			kind,
		)
	}
	return encoded, nil
}

func architectureLocalizationSHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func cloneArchitectureLocalizationFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
