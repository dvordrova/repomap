package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	presentationLocalizationCacheVersion    = "presentation-localization-cache-v1"
	presentationLocalizationCacheDir        = ".localization-cache"
	presentationLocalizationCacheVersionDir = "v1"
	maxPresentationLocalizationCacheBytes   = 8 << 20
)

type presentationLocalizationProvider interface {
	BuildLocalizationRequest(
		localization.Prompt,
	) (deepseek.LocalizationRequestEvidence, error)
	ExecuteLocalizationRequest(
		context.Context,
		localization.Prompt,
		deepseek.LocalizationRequestEvidence,
	) (modelresearch.ProviderResult, error)
}

var errPresentationLocalizationProviderConfiguration = errors.New(
	"presentation localization provider configuration is unavailable",
)

type lazyPresentationLocalizationProvider struct {
	promptClient  *deepseek.Client
	newLiveClient func() (*deepseek.Client, error)
	onWait        func(deepseek.WaitProgress)
}

func (provider *lazyPresentationLocalizationProvider) BuildLocalizationRequest(
	prompt localization.Prompt,
) (deepseek.LocalizationRequestEvidence, error) {
	if provider == nil || provider.promptClient == nil {
		return deepseek.LocalizationRequestEvidence{}, fmt.Errorf(
			"%w: prompt client is required",
			errPresentationLocalizationProviderConfiguration,
		)
	}
	return provider.promptClient.BuildLocalizationRequest(prompt)
}

func (provider *lazyPresentationLocalizationProvider) ExecuteLocalizationRequest(
	ctx context.Context,
	prompt localization.Prompt,
	evidence deepseek.LocalizationRequestEvidence,
) (modelresearch.ProviderResult, error) {
	if provider == nil || provider.newLiveClient == nil {
		return modelresearch.ProviderResult{}, fmt.Errorf(
			"%w: live client factory is required",
			errPresentationLocalizationProviderConfiguration,
		)
	}
	client, err := provider.newLiveClient()
	if err != nil {
		return modelresearch.ProviderResult{}, fmt.Errorf(
			"%w: %v",
			errPresentationLocalizationProviderConfiguration,
			err,
		)
	}
	if client == nil {
		return modelresearch.ProviderResult{}, fmt.Errorf(
			"%w: live client is required",
			errPresentationLocalizationProviderConfiguration,
		)
	}
	liveEvidence, err := client.BuildLocalizationRequest(prompt)
	if err != nil {
		return modelresearch.ProviderResult{}, fmt.Errorf(
			"%w: rebuild request identity: %v",
			errPresentationLocalizationProviderConfiguration,
			err,
		)
	}
	if !samePresentationLocalizationRequest(liveEvidence, evidence) {
		return modelresearch.ProviderResult{}, fmt.Errorf(
			"%w: live provider identity changed after cache lookup",
			errPresentationLocalizationProviderConfiguration,
		)
	}
	client.OnWait = provider.onWait
	return client.ExecuteLocalizationRequest(ctx, prompt, evidence)
}

type presentationLocalizationCacheIdentity struct {
	Version                    string                               `json:"version"`
	ContractVersion            string                               `json:"contract_version"`
	TranslationContractVersion string                               `json:"translation_contract_version"`
	TargetLocale               string                               `json:"target_locale"`
	Request                    deepseek.LocalizationRequestEvidence `json:"request"`
}

type presentationLocalizationCacheRecord struct {
	Version    string                                `json:"version"`
	Key        string                                `json:"key"`
	Identity   presentationLocalizationCacheIdentity `json:"identity"`
	Projection localization.Projection               `json:"projection"`
}

type presentationLocalizationOutcome struct {
	State         string
	ReasonCode    string
	CacheHit      bool
	CacheCorrupt  bool
	CacheWriteErr bool
	RequestBytes  int
	ResponseBytes int
	InputTokens   int
	OutputTokens  int
	Attempts      int
}

func localizePresentationForRun(
	ctx context.Context,
	runDir,
	cacheRoot string,
	noCache bool,
	stderr io.Writer,
	sourceEpisodeJSON ...[]byte,
) (presentationLocalizationOutcome, error) {
	data, err := readCanonicalReportForLocalization(runDir)
	if err != nil {
		if writeErr := report.WritePresentationLocalizationFailure(
			runDir, report.LocalizationFailurePreparation, "",
		); writeErr != nil {
			return presentationLocalizationOutcome{}, errors.Join(err, writeErr)
		}
		return presentationLocalizationOutcome{
			State:      report.PresentationLocalizationFailed,
			ReasonCode: report.LocalizationFailurePreparation,
		}, nil
	}
	if len(sourceEpisodeJSON) > 0 && len(sourceEpisodeJSON[0]) > 0 {
		if err := report.AttachSourceEpisodePresentation(
			data,
			sourceEpisodeJSON[0],
		); err != nil {
			if writeErr := report.WritePresentationLocalizationFailure(
				runDir, report.LocalizationFailurePreparation, "",
			); writeErr != nil {
				return presentationLocalizationOutcome{}, errors.Join(err, writeErr)
			}
			return presentationLocalizationOutcome{
				State:      report.PresentationLocalizationFailed,
				ReasonCode: report.LocalizationFailurePreparation,
			}, nil
		}
	}
	prepared, err := report.PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		if writeErr := report.WritePresentationLocalizationFailure(
			runDir, report.LocalizationFailurePreparation, "",
		); writeErr != nil {
			return presentationLocalizationOutcome{}, errors.Join(err, writeErr)
		}
		return presentationLocalizationOutcome{
			State:      report.PresentationLocalizationFailed,
			ReasonCode: report.LocalizationFailurePreparation,
		}, nil
	}
	promptClient, err := deepseek.NewPromptFromEnv()
	if err != nil {
		if writeErr := report.WritePresentationLocalizationFailure(
			runDir,
			report.LocalizationFailureProviderConfig,
			prepared.Canonical.SHA256,
		); writeErr != nil {
			return presentationLocalizationOutcome{}, errors.Join(err, writeErr)
		}
		return presentationLocalizationOutcome{
			State:      report.PresentationLocalizationFailed,
			ReasonCode: report.LocalizationFailureProviderConfig,
		}, nil
	}
	provider := &lazyPresentationLocalizationProvider{
		promptClient:  promptClient,
		newLiveClient: deepseek.NewFromEnv,
		onWait: func(progress deepseek.WaitProgress) {
			fmt.Fprintf(
				stderr,
				"repomap: %s still running after %s (Ctrl-C to cancel)\n",
				progress.Stage,
				progress.Elapsed.Round(time.Second),
			)
		},
	}
	return executePresentationLocalization(
		ctx,
		runDir,
		cacheRoot,
		noCache,
		data,
		prepared,
		provider,
	)
}

func markPresentationLocalizationUnavailable(
	runDir,
	reasonCode string,
) error {
	data, err := readCanonicalReportForLocalization(runDir)
	if err != nil {
		return report.WritePresentationLocalizationFailure(runDir, reasonCode, "")
	}
	prepared, prepareErr := report.PreparePresentationLocalization(
		data,
		localization.LocaleRussian,
	)
	if prepareErr != nil {
		return report.WritePresentationLocalizationFailure(runDir, reasonCode, "")
	}
	return report.WritePresentationLocalizationFailure(
		runDir,
		reasonCode,
		prepared.Canonical.SHA256,
	)
}

func readCanonicalReportForLocalization(runDir string) (*report.ReportData, error) {
	raw, err := readBoundedRegularFile(
		filepath.Join(runDir, "report.json"),
		maxDecisionTraceReportBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("localization: read canonical report: %w", err)
	}
	var data report.ReportData
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil {
		return nil, fmt.Errorf("localization: decode canonical report: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("localization: canonical report has trailing JSON")
	}
	if data.FormatVersion != report.CurrentFormatVersion ||
		data.ReportLanguage != "" ||
		data.GitLabSourceLinks != nil ||
		data.GitHubSourceLinks != nil {
		return nil, fmt.Errorf("localization: report is not canonical English")
	}
	if err := report.HydrateRunPresentationMetadata(runDir, &data); err != nil {
		return nil, fmt.Errorf("localization: hydrate presentation metadata: %w", err)
	}
	return &data, nil
}

func executePresentationLocalization(
	ctx context.Context,
	runDir,
	cacheRoot string,
	noCache bool,
	data *report.ReportData,
	prepared report.PreparedPresentationLocalization,
	provider presentationLocalizationProvider,
) (presentationLocalizationOutcome, error) {
	prompt, err := localization.BuildRussianPrompt(prepared.Canonical, prepared.Input)
	if err != nil {
		reasonCode := report.LocalizationFailurePreparation
		if strings.Contains(err.Error(), "prompt exceeds its byte limit") {
			reasonCode = report.LocalizationFailurePayloadTooLarge
		}
		return savePresentationLocalizationFailure(
			runDir, reasonCode, prepared.Canonical.SHA256,
		)
	}
	request, err := provider.BuildLocalizationRequest(prompt)
	if err != nil {
		return savePresentationLocalizationFailure(
			runDir, report.LocalizationFailureProviderConfig, prepared.Canonical.SHA256,
		)
	}
	if err := request.Validate(prompt); err != nil {
		return savePresentationLocalizationFailure(
			runDir, report.LocalizationFailureProviderConfig, prepared.Canonical.SHA256,
		)
	}
	identity := presentationLocalizationCacheIdentity{
		Version:                    presentationLocalizationCacheVersion,
		ContractVersion:            report.PresentationLocalizationContractVersion,
		TranslationContractVersion: localization.PromptVersion,
		TargetLocale:               localization.LocaleRussian,
		Request:                    request,
	}
	key, identityJSON, err := presentationLocalizationCacheKey(identity)
	if err != nil {
		return savePresentationLocalizationFailure(
			runDir, report.LocalizationFailurePreparation, prepared.Canonical.SHA256,
		)
	}
	requestSHA := sha256Hex(request.Body)
	outcome := presentationLocalizationOutcome{
		RequestBytes: len(request.Body),
	}
	if !noCache {
		record, found, corrupt := loadPresentationLocalizationCache(
			cacheRoot,
			key,
			identityJSON,
		)
		outcome.CacheCorrupt = corrupt
		if found {
			if presentationLocalizationCacheProjectionValid(
				data,
				prepared,
				record,
			) {
				if err := report.WritePresentationLocalizationSuccess(
					runDir,
					prepared,
					record.Projection,
					true,
					requestSHA,
					key,
				); err != nil {
					return outcome, err
				}
				outcome.State = report.PresentationLocalizationSucceeded
				outcome.CacheHit = true
				return outcome, nil
			}
			outcome.CacheCorrupt = true
		}
	}

	providerResult, err := provider.ExecuteLocalizationRequest(ctx, prompt, request)
	outcome.ResponseBytes = len(providerResult.Content)
	outcome.InputTokens = providerResult.InputTokens
	outcome.OutputTokens = providerResult.OutputTokens
	outcome.Attempts = providerResult.Attempts
	if err != nil {
		reasonCode := report.LocalizationFailureProviderRequest
		if errors.Is(err, errPresentationLocalizationProviderConfiguration) {
			reasonCode = report.LocalizationFailureProviderConfig
		}
		failure, writeErr := savePresentationLocalizationFailure(
			runDir,
			reasonCode,
			prepared.Canonical.SHA256,
		)
		failure.CacheCorrupt = outcome.CacheCorrupt
		failure.RequestBytes = outcome.RequestBytes
		failure.Attempts = outcome.Attempts
		return failure, writeErr
	}
	if _, found := secretscan.DetectAlways(string(providerResult.Content)); found {
		failure, writeErr := savePresentationLocalizationFailure(
			runDir,
			report.LocalizationFailureInvalidProjection,
			prepared.Canonical.SHA256,
		)
		failure.CacheCorrupt = outcome.CacheCorrupt
		failure.RequestBytes = outcome.RequestBytes
		failure.ResponseBytes = outcome.ResponseBytes
		failure.InputTokens = outcome.InputTokens
		failure.OutputTokens = outcome.OutputTokens
		failure.Attempts = outcome.Attempts
		return failure, writeErr
	}
	projection, err := report.DecodePresentationLocalizationProjection(
		[]byte(providerResult.Content),
	)
	if err != nil {
		failure, writeErr := savePresentationLocalizationFailure(
			runDir,
			report.LocalizationFailureInvalidProjection,
			prepared.Canonical.SHA256,
		)
		failure.CacheCorrupt = outcome.CacheCorrupt
		failure.RequestBytes = outcome.RequestBytes
		failure.ResponseBytes = outcome.ResponseBytes
		failure.InputTokens = outcome.InputTokens
		failure.OutputTokens = outcome.OutputTokens
		failure.Attempts = outcome.Attempts
		return failure, writeErr
	}
	if _, result, err := report.ApplyPresentationLocalization(
		data,
		prepared,
		projection,
	); err != nil || result.Fallback || len(result.Diagnostics) != 0 {
		failure, writeErr := savePresentationLocalizationFailure(
			runDir,
			report.LocalizationFailureInvalidProjection,
			prepared.Canonical.SHA256,
		)
		failure.CacheCorrupt = outcome.CacheCorrupt
		failure.RequestBytes = outcome.RequestBytes
		failure.ResponseBytes = outcome.ResponseBytes
		failure.InputTokens = outcome.InputTokens
		failure.OutputTokens = outcome.OutputTokens
		failure.Attempts = outcome.Attempts
		return failure, writeErr
	}
	if !noCache {
		record := presentationLocalizationCacheRecord{
			Version:    presentationLocalizationCacheVersion,
			Key:        key,
			Identity:   identity,
			Projection: projection,
		}
		cacheAlreadyValid := false
		cacheWriteBlocked := false
		existing, found, corrupt := loadPresentationLocalizationCache(
			cacheRoot,
			key,
			identityJSON,
		)
		if found && presentationLocalizationCacheProjectionValid(
			data,
			prepared,
			existing,
		) {
			// Another writer completed this immutable request while the
			// provider call was in flight. First valid publication wins.
			cacheAlreadyValid = true
		} else if found {
			corrupt = true
		}
		if corrupt {
			outcome.CacheCorrupt = true
			if err := removeCorruptPresentationLocalizationCache(
				cacheRoot,
				key,
			); err != nil {
				outcome.CacheWriteErr = true
				cacheWriteBlocked = true
			}
		}
		if !cacheAlreadyValid && !cacheWriteBlocked {
			if err := writePresentationLocalizationCache(cacheRoot, key, record); err != nil {
				existing, found, _ := loadPresentationLocalizationCache(
					cacheRoot,
					key,
					identityJSON,
				)
				if !found || !presentationLocalizationCacheProjectionValid(
					data,
					prepared,
					existing,
				) {
					outcome.CacheWriteErr = true
				}
			}
		}
	}
	if err := report.WritePresentationLocalizationSuccess(
		runDir,
		prepared,
		projection,
		false,
		requestSHA,
		key,
	); err != nil {
		return outcome, err
	}
	outcome.State = report.PresentationLocalizationSucceeded
	return outcome, nil
}

func samePresentationLocalizationRequest(
	left,
	right deepseek.LocalizationRequestEvidence,
) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func presentationLocalizationCacheProjectionValid(
	data *report.ReportData,
	prepared report.PreparedPresentationLocalization,
	record presentationLocalizationCacheRecord,
) bool {
	encoded, err := json.Marshal(record.Projection)
	if err != nil {
		return false
	}
	if _, found := secretscan.DetectAlways(string(encoded)); found {
		return false
	}
	_, result, err := report.ApplyPresentationLocalization(
		data,
		prepared,
		record.Projection,
	)
	return err == nil && !result.Fallback && len(result.Diagnostics) == 0
}

func savePresentationLocalizationFailure(
	runDir,
	reasonCode,
	canonicalSHA string,
) (presentationLocalizationOutcome, error) {
	err := report.WritePresentationLocalizationFailure(runDir, reasonCode, canonicalSHA)
	return presentationLocalizationOutcome{
		State:      report.PresentationLocalizationFailed,
		ReasonCode: reasonCode,
	}, err
}

func presentationLocalizationCacheKey(
	identity presentationLocalizationCacheIdentity,
) (string, []byte, error) {
	if strings.TrimSpace(identity.Version) == "" ||
		strings.TrimSpace(identity.ContractVersion) == "" ||
		strings.TrimSpace(identity.TranslationContractVersion) == "" ||
		strings.TrimSpace(identity.TargetLocale) == "" {
		return "", nil, fmt.Errorf("localization cache: invalid identity")
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return "", nil, fmt.Errorf("localization cache: encode identity: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > deepseek.MaxLocalizationRequestBodyBytes*2 {
		return "", nil, fmt.Errorf("localization cache: identity exceeds its byte limit")
	}
	return "translation-" + sha256Hex(encoded), encoded, nil
}

func loadPresentationLocalizationCache(
	cacheRoot,
	key string,
	identityJSON []byte,
) (presentationLocalizationCacheRecord, bool, bool) {
	if !validPresentationLocalizationCacheKey(key) {
		return presentationLocalizationCacheRecord{}, false, true
	}
	path := filepath.Join(
		cacheRoot,
		presentationLocalizationCacheVersionDir,
		key+".json",
	)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return presentationLocalizationCacheRecord{}, false, false
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 ||
		!info.Mode().IsRegular() || info.Size() <= 0 ||
		info.Size() > maxPresentationLocalizationCacheBytes {
		return presentationLocalizationCacheRecord{}, false, true
	}
	file, err := os.Open(path)
	if err != nil {
		return presentationLocalizationCacheRecord{}, false, true
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return presentationLocalizationCacheRecord{}, false, true
	}
	data, err := io.ReadAll(io.LimitReader(
		file,
		maxPresentationLocalizationCacheBytes+1,
	))
	if err != nil || len(data) == 0 ||
		len(data) > maxPresentationLocalizationCacheBytes ||
		!utf8.Valid(data) {
		return presentationLocalizationCacheRecord{}, false, true
	}
	var record presentationLocalizationCacheRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil {
		return presentationLocalizationCacheRecord{}, false, true
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return presentationLocalizationCacheRecord{}, false, true
	}
	recordIdentityJSON, err := json.Marshal(record.Identity)
	if err != nil ||
		record.Version != presentationLocalizationCacheVersion ||
		record.Key != key ||
		!bytes.Equal(recordIdentityJSON, identityJSON) {
		return presentationLocalizationCacheRecord{}, false, true
	}
	return record, true, false
}

func removeCorruptPresentationLocalizationCache(cacheRoot, key string) error {
	if !validPresentationLocalizationCacheKey(key) {
		return fmt.Errorf("localization cache: invalid corrupt entry key")
	}
	path := filepath.Join(
		cacheRoot,
		presentationLocalizationCacheVersionDir,
		key+".json",
	)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || info.IsDir() {
		return fmt.Errorf("localization cache: corrupt entry is unavailable")
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("localization cache: remove corrupt entry: %w", err)
	}
	return nil
}

func writePresentationLocalizationCache(
	cacheRoot,
	key string,
	record presentationLocalizationCacheRecord,
) error {
	if !validPresentationLocalizationCacheKey(key) ||
		record.Key != key ||
		record.Version != presentationLocalizationCacheVersion {
		return fmt.Errorf("localization cache: invalid record")
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("localization cache: encode record: %w", err)
	}
	data = append(data, '\n')
	if len(data) == 0 || len(data) > maxPresentationLocalizationCacheBytes {
		return fmt.Errorf("localization cache: record exceeds its byte limit")
	}
	versionDir := filepath.Join(cacheRoot, presentationLocalizationCacheVersionDir)
	if err := os.MkdirAll(versionDir, 0o700); err != nil {
		return fmt.Errorf("localization cache: create directory: %w", err)
	}
	if info, err := os.Lstat(versionDir); err != nil ||
		info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("localization cache: directory is unavailable")
	}
	path := filepath.Join(versionDir, key+".json")
	temporary, err := os.CreateTemp(versionDir, ".translation-")
	if err != nil {
		return fmt.Errorf("localization cache: create temporary record: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("localization cache: protect temporary record: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("localization cache: write record: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("localization cache: sync record: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("localization cache: close record: %w", err)
	}
	// Cache keys are content identities. Publish without replacement so two
	// concurrent misses can never turn one key into last-writer-wins state.
	if err := os.Link(temporaryPath, path); err != nil {
		if presentationLocalizationCacheFileMatches(path, data) {
			return nil
		}
		return fmt.Errorf("localization cache: publish immutable record: %w", err)
	}
	directory, err := os.Open(versionDir)
	if err != nil {
		return fmt.Errorf("localization cache: open directory for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("localization cache: sync directory: %w", err)
	}
	return nil
}

func presentationLocalizationCacheFileMatches(path string, want []byte) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 ||
		!info.Mode().IsRegular() || info.Size() != int64(len(want)) ||
		info.Size() <= 0 || info.Size() > maxPresentationLocalizationCacheBytes {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return false
	}
	got, err := io.ReadAll(io.LimitReader(
		file,
		maxPresentationLocalizationCacheBytes+1,
	))
	return err == nil && bytes.Equal(got, want)
}

func validPresentationLocalizationCacheKey(value string) bool {
	const prefix = "translation-"
	if !strings.HasPrefix(value, prefix) ||
		len(value) != len(prefix)+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
