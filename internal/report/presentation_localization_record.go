package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	PresentationLocalizationStatusFile       = "presentation_localization_status.v2.json"
	presentationLocalizationProjectionPrefix = "presentation_localization_projection.v1."

	PresentationLocalizationStatusVersion = 2
	PresentationLocalizationRecordVersion = 1

	PresentationLocalizationSucceeded  = "succeeded"
	PresentationLocalizationFailed     = "failed"
	presentationLocalizationStageOwned = "stage_owned"

	maxPresentationLocalizationStatusBytes = 64 << 10
	maxPresentationLocalizationRecordBytes = 5 << 20
)

const (
	LocalizationStageCanonicalRead         = "canonical_read"
	LocalizationStageCanonicalAuthority    = "canonical_authority"
	LocalizationStagePresentationHydration = "presentation_hydration"
	LocalizationStageInventoryBuild        = "inventory_build"
	LocalizationStageBatchPartition        = "batch_partition"
	LocalizationStagePromptBuild           = "prompt_build"
	LocalizationStageProviderConfiguration = "provider_configuration"
	LocalizationStageProviderRequest       = "provider_request"
	LocalizationStageResponseSecretScan    = "response_secret_scan"
	LocalizationStageResponseDecode        = "response_decode"
	LocalizationStageProjectionApply       = "projection_apply"
	LocalizationStageProjectionQuality     = "projection_quality"
	LocalizationStageStatusWrite           = "status_write"
	LocalizationStageUnavailable           = "unavailable"
)

const (
	LocalizationValidationCanonicalReport       = "canonical_report"
	LocalizationValidationPresentationInventory = "presentation_inventory"
	LocalizationValidationPayloadBudget         = "payload_budget"
	LocalizationValidationRequestIdentity       = "request_identity"
	LocalizationValidationTransport             = "transport"
	LocalizationValidationUnsafeResponse        = "unsafe_response"
	LocalizationValidationResponseDecode        = "response_decode"
	LocalizationValidationProjectionApply       = "projection_apply"
	LocalizationValidationProjectionDiagnostics = "projection_diagnostics"
	LocalizationValidationBatchCombination      = "batch_combination"
	LocalizationValidationPresentationApply     = "presentation_apply"
	LocalizationValidationOffline               = "offline"
	LocalizationValidationCache                 = "cache"
	LocalizationValidationSavedProjection       = "saved_projection"
	LocalizationValidationStatus                = "status"
)

var presentationLocalizationFailureStages = map[string]struct{}{
	LocalizationStageCanonicalRead:         {},
	LocalizationStageCanonicalAuthority:    {},
	LocalizationStagePresentationHydration: {},
	LocalizationStageInventoryBuild:        {},
	LocalizationStageBatchPartition:        {},
	LocalizationStagePromptBuild:           {},
	LocalizationStageProviderConfiguration: {},
	LocalizationStageProviderRequest:       {},
	LocalizationStageResponseSecretScan:    {},
	LocalizationStageResponseDecode:        {},
	LocalizationStageProjectionApply:       {},
	LocalizationStageProjectionQuality:     {},
	LocalizationStageStatusWrite:           {},
	LocalizationStageUnavailable:           {},
}

var presentationLocalizationValidationCodes = map[string]struct{}{
	LocalizationValidationCanonicalReport:       {},
	LocalizationValidationPresentationInventory: {},
	LocalizationValidationPayloadBudget:         {},
	LocalizationValidationRequestIdentity:       {},
	LocalizationValidationTransport:             {},
	LocalizationValidationUnsafeResponse:        {},
	LocalizationValidationResponseDecode:        {},
	LocalizationValidationProjectionApply:       {},
	LocalizationValidationProjectionDiagnostics: {},
	LocalizationValidationBatchCombination:      {},
	LocalizationValidationPresentationApply:     {},
	LocalizationValidationOffline:               {},
	LocalizationValidationCache:                 {},
	LocalizationValidationSavedProjection:       {},
	LocalizationValidationStatus:                {},
}

const (
	LocalizationFailurePreparation       = "preparation_failed"
	LocalizationFailurePayloadTooLarge   = "payload_too_large"
	LocalizationFailureOfflineRequested  = "offline_requested"
	LocalizationFailureProviderConfig    = "provider_configuration_failed"
	LocalizationFailureProviderRequest   = "provider_request_failed"
	LocalizationFailureInvalidProjection = "invalid_projection"
	LocalizationFailureCacheCorrupt      = "cache_corrupt"
	LocalizationFailureCacheUnavailable  = "cache_unavailable"
	LocalizationFailureSavedProjection   = "saved_projection_invalid"
	LocalizationFailureStatusUnavailable = "status_unavailable"
)

var presentationLocalizationFailureCodes = map[string]struct{}{
	LocalizationFailurePreparation:       {},
	LocalizationFailurePayloadTooLarge:   {},
	LocalizationFailureOfflineRequested:  {},
	LocalizationFailureProviderConfig:    {},
	LocalizationFailureProviderRequest:   {},
	LocalizationFailureInvalidProjection: {},
	LocalizationFailureCacheCorrupt:      {},
	LocalizationFailureCacheUnavailable:  {},
	LocalizationFailureSavedProjection:   {},
	LocalizationFailureStatusUnavailable: {},
}

// PresentationLocalizationStatus is a bounded, non-semantic account of the
// optional render projection. It deliberately contains no provider response,
// repository prose, path, symbol, evidence, or error string.
type PresentationLocalizationStatus struct {
	Version          int    `json:"version"`
	ContractVersion  string `json:"contract_version"`
	RequestedLocale  string `json:"requested_locale"`
	State            string `json:"state"`
	ReasonCode       string `json:"reason_code,omitempty"`
	CacheHit         bool   `json:"cache_hit"`
	CanonicalSHA256  string `json:"canonical_sha256,omitempty"`
	RequestSHA256    string `json:"request_sha256,omitempty"`
	ProjectionSHA256 string `json:"projection_sha256,omitempty"`
	CacheKey         string `json:"cache_key,omitempty"`
	FailureStage     string `json:"failure_stage,omitempty"`
	ValidationCode   string `json:"validation_code,omitempty"`
	BatchTotal       int    `json:"batch_total"`
	BatchAttempted   int    `json:"batch_attempted"`
	BatchCompleted   int    `json:"batch_completed"`
	FailedBatch      int    `json:"failed_batch,omitempty"`
}

// PresentationLocalizationProgress contains only bounded stage counters. A
// failed batch is one-based so zero remains an unambiguous "none" value.
type PresentationLocalizationProgress struct {
	BatchTotal     int
	BatchAttempted int
	BatchCompleted int
	FailedBatch    int
}

// PresentationLocalizationFailure contains only closed diagnostic values.
// Provider text, paths, endpoints, and errors never enter the status record.
type PresentationLocalizationFailure struct {
	ReasonCode      string
	FailureStage    string
	ValidationCode  string
	CanonicalSHA256 string
	Progress        PresentationLocalizationProgress
}

type PresentationLocalizationProjectionRecord struct {
	Version         int                     `json:"version"`
	ContractVersion string                  `json:"contract_version"`
	TargetLocale    string                  `json:"target_locale"`
	CanonicalSHA256 string                  `json:"canonical_sha256"`
	Projection      localization.Projection `json:"projection"`
}

func WritePresentationLocalizationSuccess(
	runDir string,
	prepared PreparedPresentationLocalization,
	projection localization.Projection,
	cacheHit bool,
	requestSHA256,
	cacheKey string,
	progress ...PresentationLocalizationProgress,
) error {
	result, err := localization.Apply(prepared.Canonical, prepared.Input, projection)
	if err != nil || result.Fallback || len(result.Diagnostics) != 0 ||
		result.Locale != localization.LocaleRussian {
		return fmt.Errorf("report localization: cannot save a rejected projection")
	}
	record := PresentationLocalizationProjectionRecord{
		Version:         PresentationLocalizationRecordVersion,
		ContractVersion: PresentationLocalizationContractVersion,
		TargetLocale:    localization.LocaleRussian,
		CanonicalSHA256: prepared.Canonical.SHA256,
		Projection:      projection,
	}
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("report localization: encode saved projection: %w", err)
	}
	recordJSON = append(recordJSON, '\n')
	if len(recordJSON) == 0 || len(recordJSON) > maxPresentationLocalizationRecordBytes {
		return fmt.Errorf("report localization: saved projection exceeds its byte limit")
	}
	if _, found := secretscan.DetectAlways(string(recordJSON)); found {
		return fmt.Errorf("report localization: saved projection contains unsafe material")
	}
	batchProgress := PresentationLocalizationProgress{BatchTotal: 1, BatchAttempted: 1, BatchCompleted: 1}
	if len(progress) > 0 {
		batchProgress = progress[0]
	}
	status := PresentationLocalizationStatus{
		Version:          PresentationLocalizationStatusVersion,
		ContractVersion:  PresentationLocalizationContractVersion,
		RequestedLocale:  localization.LocaleRussian,
		State:            PresentationLocalizationSucceeded,
		CacheHit:         cacheHit,
		CanonicalSHA256:  prepared.Canonical.SHA256,
		RequestSHA256:    requestSHA256,
		ProjectionSHA256: presentationLocalizationSHA256(recordJSON),
		CacheKey:         cacheKey,
		BatchTotal:       batchProgress.BatchTotal,
		BatchAttempted:   batchProgress.BatchAttempted,
		BatchCompleted:   batchProgress.BatchCompleted,
	}
	statusJSON, err := marshalPresentationLocalizationStatus(status)
	if err != nil {
		return err
	}
	// The status is the commit marker. Each projection is published under its
	// content address before status changes, so a crash or concurrent writer can
	// leave only an unreferenced generation; it cannot break the prior valid
	// status/projection pair.
	if err := writePresentationLocalizationFile(
		runDir,
		presentationLocalizationProjectionFilename(status.ProjectionSHA256),
		recordJSON,
		maxPresentationLocalizationRecordBytes,
	); err != nil {
		return err
	}
	return writePresentationLocalizationFile(
		runDir,
		PresentationLocalizationStatusFile,
		statusJSON,
		maxPresentationLocalizationStatusBytes,
	)
}

func WritePresentationLocalizationFailure(
	runDir string,
	failure PresentationLocalizationFailure,
) error {
	if _, ok := presentationLocalizationFailureCodes[failure.ReasonCode]; !ok {
		return fmt.Errorf("report localization: invalid failure code")
	}
	statusJSON, err := marshalPresentationLocalizationStatus(PresentationLocalizationStatus{
		Version:         PresentationLocalizationStatusVersion,
		ContractVersion: PresentationLocalizationContractVersion,
		RequestedLocale: localization.LocaleRussian,
		State:           PresentationLocalizationFailed,
		ReasonCode:      failure.ReasonCode,
		CanonicalSHA256: failure.CanonicalSHA256,
		FailureStage:    failure.FailureStage,
		ValidationCode:  failure.ValidationCode,
		BatchTotal:      failure.Progress.BatchTotal,
		BatchAttempted:  failure.Progress.BatchAttempted,
		BatchCompleted:  failure.Progress.BatchCompleted,
		FailedBatch:     failure.Progress.FailedBatch,
	})
	if err != nil {
		return err
	}
	return writePresentationLocalizationFile(
		runDir,
		PresentationLocalizationStatusFile,
		statusJSON,
		maxPresentationLocalizationStatusBytes,
	)
}

func marshalPresentationLocalizationStatus(
	status PresentationLocalizationStatus,
) ([]byte, error) {
	if err := validatePresentationLocalizationStatus(status); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		return nil, fmt.Errorf("report localization: encode status: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maxPresentationLocalizationStatusBytes {
		return nil, fmt.Errorf("report localization: status exceeds its byte limit")
	}
	return encoded, nil
}

func validatePresentationLocalizationStatus(
	status PresentationLocalizationStatus,
) error {
	if status.Version != PresentationLocalizationStatusVersion ||
		status.ContractVersion != PresentationLocalizationContractVersion ||
		status.RequestedLocale != localization.LocaleRussian ||
		!validPresentationLocalizationScalar(status.CanonicalSHA256, 128) ||
		!validPresentationLocalizationScalar(status.RequestSHA256, 128) ||
		!validPresentationLocalizationScalar(status.ProjectionSHA256, 128) ||
		!validPresentationLocalizationScalar(status.CacheKey, 256) ||
		!validPresentationLocalizationScalar(status.FailureStage, 64) ||
		!validPresentationLocalizationScalar(status.ValidationCode, 64) ||
		!validPresentationLocalizationProgress(status) {
		return fmt.Errorf("report localization: invalid status")
	}
	switch status.State {
	case PresentationLocalizationSucceeded:
		if status.ReasonCode != "" || status.CanonicalSHA256 == "" ||
			status.RequestSHA256 == "" || status.ProjectionSHA256 == "" ||
			!validPresentationLocalizationSHA256(status.ProjectionSHA256) ||
			status.CacheKey == "" || status.FailureStage != "" ||
			status.ValidationCode != "" || status.FailedBatch != 0 ||
			status.BatchTotal == 0 || status.BatchAttempted != status.BatchTotal ||
			status.BatchCompleted != status.BatchTotal {
			return fmt.Errorf("report localization: invalid success status")
		}
	case PresentationLocalizationFailed:
		if _, ok := presentationLocalizationFailureCodes[status.ReasonCode]; !ok ||
			status.CacheHit || status.RequestSHA256 != "" ||
			status.ProjectionSHA256 != "" || status.CacheKey != "" {
			return fmt.Errorf("report localization: invalid failure status")
		}
		if _, ok := presentationLocalizationFailureStages[status.FailureStage]; !ok {
			return fmt.Errorf("report localization: invalid failure status")
		}
		if _, ok := presentationLocalizationValidationCodes[status.ValidationCode]; !ok {
			return fmt.Errorf("report localization: invalid failure status")
		}
	default:
		return fmt.Errorf("report localization: invalid status state")
	}
	return nil
}

func validPresentationLocalizationProgress(status PresentationLocalizationStatus) bool {
	if status.BatchTotal < 0 || status.BatchTotal > 4096 ||
		status.BatchAttempted < 0 || status.BatchAttempted > status.BatchTotal ||
		status.BatchCompleted < 0 || status.BatchCompleted > status.BatchAttempted ||
		status.FailedBatch < 0 || status.FailedBatch > status.BatchTotal {
		return false
	}
	if status.FailedBatch > 0 && status.FailedBatch > status.BatchAttempted {
		return false
	}
	return true
}

func validPresentationLocalizationSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') &&
			(character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validPresentationLocalizationScalar(value string, limit int) bool {
	return len(value) <= limit && utf8.ValidString(value) &&
		!strings.ContainsAny(value, "\x00\r\n")
}

// LoadPresentationLocalization applies a successful run-local projection to a
// fresh copy of canonical data only when Russian was explicitly requested.
// Missing, failed, corrupt, or stale projection state keeps the requested
// Russian product catalog while leaving canonical model-authored prose in
// English with an explicit localization warning. Atlas-first reports are the
// exception for a missing status: their stages own their Russian prose and
// intentionally do not produce the legacy whole-report localization sidecar.
func LoadPresentationLocalization(
	runDir string,
	data *ReportData,
	requestedLocale string,
) (*ReportData, PresentationLocalizationStatus) {
	if data == nil {
		return data, PresentationLocalizationStatus{}
	}
	if normalizedReportLanguage(requestedLocale) != localization.LocaleRussian {
		return canonicalEnglishPresentation(data), PresentationLocalizationStatus{}
	}
	if data.presentationMetadataErr != nil {
		return failedRussianPresentation(data), failedPresentationLocalizationStatus(
			LocalizationFailureSavedProjection,
			LocalizationValidationSavedProjection,
		)
	}
	statusJSON, statusErr := readPresentationLocalizationFile(
		runDir,
		PresentationLocalizationStatusFile,
		maxPresentationLocalizationStatusBytes,
	)
	if os.IsNotExist(statusErr) {
		if hasStageOwnedPresentation(data) {
			return stageOwnedRussianPresentation(data), PresentationLocalizationStatus{}
		}
		return failedRussianPresentation(data), failedPresentationLocalizationStatus(
			LocalizationFailureStatusUnavailable,
			LocalizationValidationStatus,
		)
	}
	var status PresentationLocalizationStatus
	if statusErr != nil ||
		decodePresentationLocalizationJSON(statusJSON, &status) != nil ||
		validatePresentationLocalizationStatus(status) != nil {
		return failedRussianPresentation(data), failedPresentationLocalizationStatus(
			LocalizationFailureSavedProjection,
			LocalizationValidationSavedProjection,
		)
	}
	if status.State == PresentationLocalizationFailed {
		return failedRussianPresentation(data), status
	}
	recordName := presentationLocalizationProjectionFilename(
		status.ProjectionSHA256,
	)
	recordJSON, err := readPresentationLocalizationFile(
		runDir,
		recordName,
		maxPresentationLocalizationRecordBytes,
	)
	if err != nil {
		status = rejectedSavedPresentationLocalizationStatus(status)
		return failedRussianPresentation(data), status
	}
	if _, found := secretscan.DetectAlways(string(recordJSON)); found {
		status = rejectedSavedPresentationLocalizationStatus(status)
		return failedRussianPresentation(data), status
	}
	var record PresentationLocalizationProjectionRecord
	if decodePresentationLocalizationJSON(recordJSON, &record) != nil ||
		presentationLocalizationSHA256(recordJSON) != status.ProjectionSHA256 ||
		record.Version != PresentationLocalizationRecordVersion ||
		record.ContractVersion != PresentationLocalizationContractVersion ||
		record.TargetLocale != localization.LocaleRussian ||
		record.CanonicalSHA256 != status.CanonicalSHA256 {
		status = rejectedSavedPresentationLocalizationStatus(status)
		return failedRussianPresentation(data), status
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil || prepared.Canonical.SHA256 != status.CanonicalSHA256 {
		status = rejectedSavedPresentationLocalizationStatus(status)
		return failedRussianPresentation(data), status
	}
	projected, result, err := ApplyPresentationLocalization(data, prepared, record.Projection)
	if err != nil || result.Fallback || len(result.Diagnostics) != 0 {
		status = rejectedSavedPresentationLocalizationStatus(status)
		return failedRussianPresentation(data), status
	}
	return projected, status
}

func failedPresentationLocalizationStatus(
	reasonCode,
	validationCode string,
) PresentationLocalizationStatus {
	return PresentationLocalizationStatus{
		Version:         PresentationLocalizationStatusVersion,
		ContractVersion: PresentationLocalizationContractVersion,
		RequestedLocale: localization.LocaleRussian,
		State:           PresentationLocalizationFailed,
		ReasonCode:      reasonCode,
		FailureStage:    LocalizationStageUnavailable,
		ValidationCode:  validationCode,
	}
}

func rejectedSavedPresentationLocalizationStatus(
	status PresentationLocalizationStatus,
) PresentationLocalizationStatus {
	failed := failedPresentationLocalizationStatus(
		LocalizationFailureSavedProjection,
		LocalizationValidationSavedProjection,
	)
	failed.CanonicalSHA256 = status.CanonicalSHA256
	return failed
}

func presentationLocalizationSHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return fmt.Sprintf("%x", digest[:])
}

func presentationLocalizationProjectionFilename(sha256 string) string {
	return presentationLocalizationProjectionPrefix + sha256 + ".json"
}

func DecodePresentationLocalizationProjection(
	data []byte,
) (localization.Projection, error) {
	if len(data) == 0 || len(data) > maxPresentationLocalizationRecordBytes {
		return localization.Projection{}, fmt.Errorf(
			"report localization: projection exceeds its byte limit",
		)
	}
	var projection localization.Projection
	if err := decodePresentationLocalizationJSON(data, &projection); err != nil {
		return localization.Projection{}, fmt.Errorf(
			"report localization: projection is not strict JSON",
		)
	}
	return projection, nil
}

func decodePresentationLocalizationJSON(data []byte, target any) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("invalid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func canonicalEnglishPresentation(data *ReportData) *ReportData {
	cloned := *data
	cloned.ReportLanguage = ""
	cloned.presentationLocalizationState = ""
	cloned.presentationLocalizationMessageID = ""
	return &cloned
}

func hasStageOwnedPresentation(data *ReportData) bool {
	return data != nil && data.RepositoryAtlas != nil && data.Navigator != nil
}

func stageOwnedRussianPresentation(data *ReportData) *ReportData {
	cloned := *data
	cloned.ReportLanguage = localization.LocaleRussian
	cloned.presentationLocalizationState = presentationLocalizationStageOwned
	cloned.presentationLocalizationMessageID = "main.localization.ru_active"
	return &cloned
}

func failedRussianPresentation(data *ReportData) *ReportData {
	cloned := *data
	cloned.ReportLanguage = localization.LocaleRussian
	cloned.presentationLocalizationState = PresentationLocalizationFailed
	cloned.presentationLocalizationMessageID =
		"main.localization.ru_unavailable_canonical_en"
	return &cloned
}

func readPresentationLocalizationFile(
	runDir,
	name string,
	limit int64,
) ([]byte, error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(absDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
		info.Size() <= 0 || info.Size() > limit {
		return nil, fmt.Errorf("report localization: saved artifact is not a bounded regular file")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("report localization: saved artifact changed before open")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(data) == 0 || int64(len(data)) > limit {
		return nil, fmt.Errorf("report localization: saved artifact exceeds its byte limit")
	}
	return data, nil
}

func writePresentationLocalizationFile(
	runDir,
	name string,
	data []byte,
	limit int,
) error {
	if filepath.Base(name) != name || len(data) == 0 || len(data) > limit {
		return fmt.Errorf("report localization: invalid saved artifact")
	}
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("report localization: resolve run dir: %w", err)
	}
	info, err := os.Lstat(absDir)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("report localization: run path is not a real directory")
	}
	root, err := os.OpenRoot(absDir)
	if err != nil {
		return fmt.Errorf("report localization: open run root: %w", err)
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil || !opened.IsDir() || !os.SameFile(info, opened) {
		return fmt.Errorf("report localization: run directory changed before open")
	}
	tempName, file, err := createArchitectureLocalizationRecordTemp(root)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
		if tempName != "" {
			_ = root.Remove(tempName)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("report localization: write temporary artifact: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("report localization: protect temporary artifact: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("report localization: sync temporary artifact: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("report localization: close temporary artifact: %w", err)
	}
	if err := root.Rename(tempName, name); err != nil {
		return fmt.Errorf("report localization: publish artifact: %w", err)
	}
	tempName = ""
	return syncArchitectureLocalizationDirectory(root, ".")
}
