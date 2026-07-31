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
	presentationLocalizationCacheVersion    = "presentation-localization-cache-v3"
	presentationLocalizationCacheDir        = ".localization-cache"
	presentationLocalizationCacheVersionDir = "v3"
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
	BatchContractVersion       int                                  `json:"batch_contract_version"`
	BatchContentSHA256         string                               `json:"batch_content_sha256"`
	Request                    deepseek.LocalizationRequestEvidence `json:"request"`
}

type presentationLocalizationCacheRecord struct {
	Version    string                                `json:"version"`
	Key        string                                `json:"key"`
	Identity   presentationLocalizationCacheIdentity `json:"identity"`
	Projection localization.Projection               `json:"projection"`
}

// presentationLocalizationCacheObservation binds a cache decision to the
// exact bounded regular file that was inspected. The file identity prevents a
// replacement inode from being removed, while data preserves the exact bytes
// used to classify the observed inode as corrupt.
type presentationLocalizationCacheObservation struct {
	info os.FileInfo
	data []byte
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
	ProviderCalls int
	Batches       []presentationLocalizationBatchOutcome
}

type presentationLocalizationBatchOutcome struct {
	Index                int
	Count                int
	FieldCount           int
	PredictedOutputBytes int
	CacheHit             bool
	RequestBytes         int
	ResponseBytes        int
	InputTokens          int
	OutputTokens         int
	Attempts             int
	ProviderCalls        int
}

type presentationLocalizationBatchPlan struct {
	Batch        localization.Batch
	Prompt       localization.Prompt
	Request      deepseek.LocalizationRequestEvidence
	Identity     presentationLocalizationCacheIdentity
	Key          string
	IdentityJSON []byte
	RequestSHA   string
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
	outcome := presentationLocalizationOutcome{}
	fail := func(reasonCode string) (presentationLocalizationOutcome, error) {
		outcome.State = report.PresentationLocalizationFailed
		outcome.ReasonCode = reasonCode
		return outcome, report.WritePresentationLocalizationFailure(
			runDir,
			reasonCode,
			prepared.Canonical.SHA256,
		)
	}

	plans, err := buildPresentationLocalizationBatchPlans(prepared, provider)
	if err != nil {
		reasonCode := report.LocalizationFailurePreparation
		if strings.Contains(err.Error(), "byte limit") ||
			strings.Contains(err.Error(), "output budget") {
			reasonCode = report.LocalizationFailurePayloadTooLarge
		} else if errors.Is(err, errPresentationLocalizationProviderConfiguration) {
			reasonCode = report.LocalizationFailureProviderConfig
		}
		return fail(reasonCode)
	}
	projections := make([]localization.Projection, len(plans))
	requestSHAs := make([]string, len(plans))
	cacheKeys := make([]string, len(plans))
	outcome.Batches = make([]presentationLocalizationBatchOutcome, len(plans))
	allCacheHits := true

	for index, plan := range plans {
		requestSHAs[index] = plan.RequestSHA
		cacheKeys[index] = plan.Key
		batchOutcome := presentationLocalizationBatchOutcome{
			Index: index, Count: len(plans),
			FieldCount:           plan.Batch.Manifest.FieldCount,
			PredictedOutputBytes: plan.Batch.Manifest.PredictedOutputBytes,
		}

		if !noCache {
			record, found, corrupt := loadPresentationLocalizationCache(
				cacheRoot,
				plan.Key,
				plan.IdentityJSON,
			)
			outcome.CacheCorrupt = outcome.CacheCorrupt || corrupt
			if found && presentationLocalizationBatchCacheProjectionValid(
				plan.Batch,
				record,
			) {
				projections[index] = record.Projection
				batchOutcome.CacheHit = true
				outcome.Batches[index] = batchOutcome
				continue
			}
			if found {
				outcome.CacheCorrupt = true
			}
		}

		allCacheHits = false
		batchOutcome.RequestBytes = len(plan.Request.Body)
		outcome.RequestBytes += batchOutcome.RequestBytes
		providerResult, executeErr := provider.ExecuteLocalizationRequest(
			ctx,
			plan.Prompt,
			plan.Request,
		)
		batchOutcome.ResponseBytes = len(providerResult.Content)
		batchOutcome.ProviderCalls++
		outcome.ProviderCalls++
		batchOutcome.InputTokens = providerResult.InputTokens
		batchOutcome.OutputTokens = providerResult.OutputTokens
		batchOutcome.Attempts = providerResult.Attempts
		outcome.ResponseBytes += batchOutcome.ResponseBytes
		outcome.InputTokens += batchOutcome.InputTokens
		outcome.OutputTokens += batchOutcome.OutputTokens
		outcome.Attempts += batchOutcome.Attempts
		outcome.Batches[index] = batchOutcome
		if executeErr != nil {
			reasonCode := report.LocalizationFailureProviderRequest
			if errors.Is(executeErr, errPresentationLocalizationProviderConfiguration) {
				reasonCode = report.LocalizationFailureProviderConfig
			}
			return fail(reasonCode)
		}
		if _, found := secretscan.DetectAlways(string(providerResult.Content)); found {
			return fail(report.LocalizationFailureInvalidProjection)
		}
		projection, decodeErr := localization.DecodeRussianProviderResponse(
			plan.Batch.Canonical,
			plan.Batch.Input,
			[]byte(providerResult.Content),
		)
		if decodeErr != nil {
			return fail(report.LocalizationFailureInvalidProjection)
		}
		validation, validationErr := localization.Apply(
			plan.Batch.Canonical,
			plan.Batch.Input,
			projection,
		)
		if validationErr != nil {
			return fail(report.LocalizationFailureInvalidProjection)
		}
		if validation.Fallback || len(validation.Diagnostics) != 0 {
			return fail(report.LocalizationFailureInvalidProjection)
		}
		projections[index] = projection

		if !noCache {
			record := presentationLocalizationCacheRecord{
				Version: presentationLocalizationCacheVersion,
				Key:     plan.Key, Identity: plan.Identity,
				Projection: projection,
			}
			cacheAlreadyValid := false
			cacheWriteBlocked := false
			var cacheWinner *localization.Projection
			existing, found, corrupt, observed := loadPresentationLocalizationCacheObserved(
				cacheRoot,
				plan.Key,
				plan.IdentityJSON,
			)
			if found && presentationLocalizationBatchCacheProjectionValid(
				plan.Batch,
				existing,
			) {
				cacheAlreadyValid = true
				cacheWinner = &existing.Projection
			} else if found {
				corrupt = true
			}
			if corrupt {
				outcome.CacheCorrupt = true
				removed, err := removeCorruptPresentationLocalizationCache(
					cacheRoot,
					plan.Key,
					observed,
				)
				if err != nil {
					outcome.CacheWriteErr = true
					cacheWriteBlocked = true
				} else if !removed {
					// A different inode or byte sequence won the race. Never
					// unlink it. A valid winner is first-valid-wins; an invalid
					// winner blocks this write and will be observed afresh by a
					// later run.
					winner, winnerFound, _ := loadPresentationLocalizationCache(
						cacheRoot,
						plan.Key,
						plan.IdentityJSON,
					)
					if winnerFound && presentationLocalizationBatchCacheProjectionValid(
						plan.Batch,
						winner,
					) {
						cacheAlreadyValid = true
						cacheWinner = &winner.Projection
					} else {
						outcome.CacheWriteErr = true
						cacheWriteBlocked = true
					}
				}
			}
			if !cacheAlreadyValid && !cacheWriteBlocked {
				if err := writePresentationLocalizationCache(
					cacheRoot,
					plan.Key,
					record,
				); err != nil {
					existing, found, _ := loadPresentationLocalizationCache(
						cacheRoot,
						plan.Key,
						plan.IdentityJSON,
					)
					if !found || !presentationLocalizationBatchCacheProjectionValid(
						plan.Batch,
						existing,
					) {
						outcome.CacheWriteErr = true
					} else {
						cacheAlreadyValid = true
						cacheWinner = &existing.Projection
					}
				}
			}
			if cacheAlreadyValid && cacheWinner != nil {
				projection = *cacheWinner
				projections[index] = projection
			}
		}
	}

	projection, err := localization.CombineBatchProjections(
		prepared.Canonical,
		prepared.Input,
		localizationBatches(plans),
		projections,
	)
	if err != nil {
		return fail(report.LocalizationFailureInvalidProjection)
	}
	if _, result, err := report.ApplyPresentationLocalization(
		data,
		prepared,
		projection,
	); err != nil || result.Fallback || len(result.Diagnostics) != 0 {
		return fail(report.LocalizationFailureInvalidProjection)
	}
	requestSetJSON, err := json.Marshal(requestSHAs)
	if err != nil {
		return fail(report.LocalizationFailurePreparation)
	}
	cacheSetJSON, err := json.Marshal(cacheKeys)
	if err != nil {
		return fail(report.LocalizationFailurePreparation)
	}
	requestSHA := sha256Hex(requestSetJSON)
	cacheKey := "translation-" + sha256Hex(cacheSetJSON)
	if err := report.WritePresentationLocalizationSuccess(
		runDir,
		prepared,
		projection,
		allCacheHits,
		requestSHA,
		cacheKey,
	); err != nil {
		return outcome, err
	}
	outcome.State = report.PresentationLocalizationSucceeded
	outcome.CacheHit = allCacheHits
	return outcome, nil
}

func buildPresentationLocalizationBatchPlans(
	prepared report.PreparedPresentationLocalization,
	provider presentationLocalizationProvider,
) ([]presentationLocalizationBatchPlan, error) {
	batches, err := localization.BuildBatches(prepared.Canonical, prepared.Input)
	if err != nil {
		return nil, err
	}
	plans := make([]presentationLocalizationBatchPlan, 0, len(batches))
	for _, batch := range batches {
		prompt, err := localization.BuildRussianPrompt(batch.Canonical, batch.Input)
		if err != nil {
			return nil, err
		}
		request, err := provider.BuildLocalizationRequest(prompt)
		if err != nil {
			return nil, errors.Join(errPresentationLocalizationProviderConfiguration, err)
		}
		if err := request.Validate(prompt); err != nil {
			return nil, errors.Join(errPresentationLocalizationProviderConfiguration, err)
		}
		identity := presentationLocalizationCacheIdentity{
			Version:                    presentationLocalizationCacheVersion,
			ContractVersion:            report.PresentationLocalizationContractVersion,
			TranslationContractVersion: localization.PromptVersion,
			TargetLocale:               localization.LocaleRussian,
			BatchContractVersion:       batch.Manifest.Version,
			BatchContentSHA256:         batch.Manifest.ContentSHA256,
			Request:                    request,
		}
		key, identityJSON, err := presentationLocalizationCacheKey(identity)
		if err != nil {
			return nil, err
		}
		plans = append(plans, presentationLocalizationBatchPlan{
			Batch: batch, Prompt: prompt, Request: request, Identity: identity,
			Key: key, IdentityJSON: identityJSON, RequestSHA: sha256Hex(request.Body),
		})
	}
	return plans, nil
}

func localizationBatches(plans []presentationLocalizationBatchPlan) []localization.Batch {
	batches := make([]localization.Batch, len(plans))
	for index, plan := range plans {
		batches[index] = plan.Batch
	}
	return batches
}

func samePresentationLocalizationRequest(
	left,
	right deepseek.LocalizationRequestEvidence,
) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func presentationLocalizationBatchProjectionValid(
	batch localization.Batch,
	projection localization.Projection,
) bool {
	encoded, err := json.Marshal(projection)
	if err != nil {
		return false
	}
	if _, found := secretscan.DetectAlways(string(encoded)); found {
		return false
	}
	result, err := localization.Apply(batch.Canonical, batch.Input, projection)
	return err == nil && !result.Fallback && len(result.Diagnostics) == 0
}

func presentationLocalizationBatchCacheProjectionValid(
	batch localization.Batch,
	record presentationLocalizationCacheRecord,
) bool {
	return record.Version == presentationLocalizationCacheVersion &&
		presentationLocalizationBatchProjectionValid(batch, record.Projection)
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
		strings.TrimSpace(identity.TargetLocale) == "" ||
		identity.BatchContractVersion != localization.BatchManifestVersion ||
		!validSHA256Hex(identity.BatchContentSHA256) {
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

func validSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func loadPresentationLocalizationCache(
	cacheRoot,
	key string,
	identityJSON []byte,
) (presentationLocalizationCacheRecord, bool, bool) {
	record, found, corrupt, _ := loadPresentationLocalizationCacheObserved(
		cacheRoot,
		key,
		identityJSON,
	)
	return record, found, corrupt
}

func loadPresentationLocalizationCacheObserved(
	cacheRoot,
	key string,
	identityJSON []byte,
) (
	presentationLocalizationCacheRecord,
	bool,
	bool,
	presentationLocalizationCacheObservation,
) {
	if !validPresentationLocalizationCacheKey(key) {
		return presentationLocalizationCacheRecord{}, false, true,
			presentationLocalizationCacheObservation{}
	}
	path := filepath.Join(
		cacheRoot,
		presentationLocalizationCacheVersionDir,
		key+".json",
	)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return presentationLocalizationCacheRecord{}, false, false,
			presentationLocalizationCacheObservation{}
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 ||
		!info.Mode().IsRegular() || info.Size() <= 0 ||
		info.Size() > maxPresentationLocalizationCacheBytes {
		return presentationLocalizationCacheRecord{}, false, true,
			presentationLocalizationCacheObservation{}
	}
	file, err := os.Open(path)
	if err != nil {
		return presentationLocalizationCacheRecord{}, false, true,
			presentationLocalizationCacheObservation{}
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return presentationLocalizationCacheRecord{}, false, true,
			presentationLocalizationCacheObservation{}
	}
	data, err := io.ReadAll(io.LimitReader(
		file,
		maxPresentationLocalizationCacheBytes+1,
	))
	if err != nil || len(data) == 0 ||
		len(data) > maxPresentationLocalizationCacheBytes {
		return presentationLocalizationCacheRecord{}, false, true,
			presentationLocalizationCacheObservation{}
	}
	observed := presentationLocalizationCacheObservation{info: opened, data: data}
	if !utf8.Valid(data) {
		return presentationLocalizationCacheRecord{}, false, true, observed
	}
	var record presentationLocalizationCacheRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil {
		return presentationLocalizationCacheRecord{}, false, true, observed
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return presentationLocalizationCacheRecord{}, false, true, observed
	}
	recordIdentityJSON, err := json.Marshal(record.Identity)
	if err != nil ||
		record.Version != presentationLocalizationCacheVersion ||
		record.Key != key ||
		!bytes.Equal(recordIdentityJSON, identityJSON) {
		return presentationLocalizationCacheRecord{}, false, true, observed
	}
	return record, true, false, observed
}

func removeCorruptPresentationLocalizationCache(
	cacheRoot,
	key string,
	observed presentationLocalizationCacheObservation,
) (bool, error) {
	if !validPresentationLocalizationCacheKey(key) {
		return false, fmt.Errorf("localization cache: invalid corrupt entry key")
	}
	if observed.info == nil || len(observed.data) == 0 ||
		len(observed.data) > maxPresentationLocalizationCacheBytes {
		return false, fmt.Errorf(
			"localization cache: corrupt entry has no stable observation",
		)
	}
	path := filepath.Join(
		cacheRoot,
		presentationLocalizationCacheVersionDir,
		key+".json",
	)
	// Serialize corrupt-entry cleanup for this content identity. Writers publish with a
	// no-replace hard link, so while the observed corrupt path exists they
	// cannot replace it. Holding this claim through the conditional unlink
	// closes the only writer/remover race: a second cleanup cannot remove a
	// valid entry published after this cleanup releases the path.
	claimPath := path + ".cleanup.lock"
	claim, err := os.OpenFile(
		claimPath,
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0o600,
	)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf(
			"localization cache: claim corrupt entry cleanup: %w",
			err,
		)
	}
	defer func() {
		_ = claim.Close()
		_ = os.Remove(claimPath)
	}()
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 ||
		!info.Mode().IsRegular() || !os.SameFile(observed.info, info) ||
		info.Size() != int64(len(observed.data)) {
		return false, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return false, nil
	}
	opened, statErr := file.Stat()
	data, readErr := io.ReadAll(io.LimitReader(
		file,
		maxPresentationLocalizationCacheBytes+1,
	))
	closeErr := file.Close()
	if statErr != nil || !opened.Mode().IsRegular() ||
		!os.SameFile(info, opened) || readErr != nil || closeErr != nil ||
		!bytes.Equal(data, observed.data) {
		return false, nil
	}
	// Recheck the directory entry after reading. This closes the practical
	// replacement window and, together with the inode and byte comparison,
	// ensures a different first-valid-wins entry is never intentionally
	// removed.
	current, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil || !os.SameFile(opened, current) {
		return false, nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("localization cache: remove corrupt entry: %w", err)
	}
	return true, nil
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
