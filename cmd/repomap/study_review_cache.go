package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/studymap"
)

const (
	studyReviewCacheContractVersion  = "study-reading-pack-review-cache-v1"
	studyReviewValidatorVersion      = "study-reading-pack-review-validator-v1"
	studyReviewStageContractVersion  = "study-reading-pack-review-stage-v1"
	studyReviewCacheParentDirectory  = ".model-research"
	studyReviewCacheVersionDirectory = "study-reviews-v1"
	maxStudyReviewCacheRecordBytes   = 6 << 20
)

type studyReviewCacheIdentity struct {
	CacheContractVersion   string                            `json:"cache_contract_version"`
	ValidatorVersion       string                            `json:"validator_version"`
	StageContractVersion   string                            `json:"stage_contract_version"`
	PromptVersion          string                            `json:"prompt_version"`
	ThinkingProfile        semanticdiscovery.ThinkingProfile `json:"thinking_profile"`
	EnglishContractVersion string                            `json:"english_contract_version"`
	EndpointSHA256         string                            `json:"endpoint_sha256"`
	Model                  string                            `json:"model"`
	ProviderProfile        string                            `json:"provider_profile"`
	MaxTokens              int                               `json:"max_tokens"`
	Request                []byte                            `json:"request"`
	ReviewBundleSHA256     string                            `json:"review_bundle_sha256"`
	SourceFragmentsSHA256  string                            `json:"source_fragments_sha256"`
}

type studyReviewProviderIdentity struct {
	EndpointSHA256  string
	Model           string
	ProviderProfile string
	MaxTokens       int
}

type studyReviewCacheRecord struct {
	Version        string                   `json:"version"`
	Key            string                   `json:"key"`
	Identity       studyReviewCacheIdentity `json:"identity"`
	ResponseSHA256 string                   `json:"response_sha256"`
	Response       []byte                   `json:"response"`
}

type studyReviewCacheStats struct {
	Hits          int
	Misses        int
	Bypassed      int
	Corrupt       int
	WriteFailures int
}

// studyReviewCachingEditor keeps the existing semantic editor contract and
// adds one narrow replay seam used only by reading-pack reviews. Live provider
// construction remains lazy so a review hit itself never requires an API key.
type studyReviewCachingEditor struct {
	promptClient *deepseek.Client
	newLive      func() (semanticDiscoveryEditor, error)
	runsDir      string
	disableCache bool

	liveOnce sync.Once
	live     semanticDiscoveryEditor
	liveErr  error

	mu     sync.Mutex
	stats  studyReviewCacheStats
	stderr io.Writer

	cacheContractVersion string
	validatorVersion     string
	stageContractVersion string
	englishContract      string
}

type studyReviewCacheReplay interface {
	loadStudyReview(
		semanticdiscovery.Prompt,
		[]byte,
		studymap.Bundle,
		studymap.DirectionCandidate,
	) (modelresearch.ProviderResult, bool, error)
	storeStudyReview(
		context.Context,
		semanticdiscovery.Prompt,
		[]byte,
		studymap.Bundle,
		studymap.DirectionCandidate,
		[]byte,
	)
}

func newStudyReviewCachingEditor(
	promptClient *deepseek.Client,
	newLive func() (semanticDiscoveryEditor, error),
	runsDir string,
	disableCache bool,
	stderr io.Writer,
) *studyReviewCachingEditor {
	return &studyReviewCachingEditor{
		promptClient:         promptClient,
		newLive:              newLive,
		runsDir:              runsDir,
		disableCache:         disableCache,
		stderr:               stderr,
		cacheContractVersion: studyReviewCacheContractVersion,
		validatorVersion:     studyReviewValidatorVersion,
		stageContractVersion: studyReviewStageContractVersion,
		englishContract:      deepseek.SemanticOutputLanguageContractVersion,
	}
}

func (editor *studyReviewCachingEditor) SemanticDiscoveryPromptJSON(
	prompt semanticdiscovery.Prompt,
) ([]byte, error) {
	if editor == nil || editor.promptClient == nil {
		return nil, fmt.Errorf("study review cache: prompt client is required")
	}
	return editor.promptClient.SemanticDiscoveryPromptJSON(prompt)
}

func (editor *studyReviewCachingEditor) DiscoverSemanticsMeasured(
	ctx context.Context,
	prompt semanticdiscovery.Prompt,
) (modelresearch.ProviderResult, error) {
	live, err := editor.liveEditor()
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	if prompt.Version != semanticdiscovery.ReadingPackReviewPromptVersion {
		return live.DiscoverSemanticsMeasured(ctx, prompt)
	}
	planned, err := editor.SemanticDiscoveryPromptJSON(prompt)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	liveRequest, err := live.SemanticDiscoveryPromptJSON(prompt)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	if !bytes.Equal(planned, liveRequest) {
		return modelresearch.ProviderResult{}, fmt.Errorf(
			"study review cache: live provider identity changed after request planning",
		)
	}
	plannedProvider, err := studyReviewProviderIdentityFromConfig(
		editor.promptClient.EffectiveConfig(),
	)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	liveConfigProvider, ok := live.(interface {
		EffectiveConfig() deepseek.EffectiveConfig
	})
	if !ok {
		return modelresearch.ProviderResult{}, fmt.Errorf(
			"study review cache: live provider does not expose its effective identity",
		)
	}
	liveProvider, err := studyReviewProviderIdentityFromConfig(
		liveConfigProvider.EffectiveConfig(),
	)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	if plannedProvider != liveProvider {
		return modelresearch.ProviderResult{}, fmt.Errorf(
			"study review cache: live provider identity changed after request planning",
		)
	}
	return live.DiscoverSemanticsMeasured(ctx, prompt)
}

func (editor *studyReviewCachingEditor) liveEditor() (semanticDiscoveryEditor, error) {
	if editor == nil {
		return nil, fmt.Errorf("study review cache: editor is required")
	}
	editor.liveOnce.Do(func() {
		if editor.newLive == nil {
			editor.liveErr = fmt.Errorf("study review cache: live provider factory is required")
			return
		}
		editor.live, editor.liveErr = editor.newLive()
		if editor.liveErr == nil && editor.live == nil {
			editor.liveErr = fmt.Errorf("study review cache: live provider is required")
		}
	})
	return editor.live, editor.liveErr
}

func (editor *studyReviewCachingEditor) loadStudyReview(
	prompt semanticdiscovery.Prompt,
	request []byte,
	bundle studymap.Bundle,
	direction studymap.DirectionCandidate,
) (modelresearch.ProviderResult, bool, error) {
	if editor.disableCache {
		editor.addStat(func(stats *studyReviewCacheStats) { stats.Bypassed++ })
		return modelresearch.ProviderResult{}, false, nil
	}
	_, key, identityJSON, err := editor.reviewCacheIdentity(
		prompt, request, bundle, direction,
	)
	if err != nil {
		editor.addStat(func(stats *studyReviewCacheStats) { stats.Misses++ })
		editor.writeDiagnostic(
			"warning: Study review cache could not validate one request identity; the ordinary review request will be used",
		)
		return modelresearch.ProviderResult{}, false, nil
	}
	record, found, corrupt := loadStudyReviewCache(editor.runsDir, key, identityJSON)
	if found && !studyReviewResponseAccepted(bundle, direction, record.Response) {
		found = false
		corrupt = true
	}
	if found {
		editor.addStat(func(stats *studyReviewCacheStats) { stats.Hits++ })
		return modelresearch.ProviderResult{
			Content: append([]byte(nil), record.Response...),
		}, true, nil
	}
	editor.addStat(func(stats *studyReviewCacheStats) {
		stats.Misses++
		if corrupt {
			stats.Corrupt++
		}
	})
	if corrupt {
		editor.writeDiagnostic(
			"warning: Study review cache ignored an invalid entry; the ordinary review request will be recomputed",
		)
		if err := repairCorruptStudyReviewCache(
			editor.runsDir, key, identityJSON, bundle, direction,
		); err != nil {
			editor.addStat(func(stats *studyReviewCacheStats) { stats.WriteFailures++ })
		}
	}
	return modelresearch.ProviderResult{}, false, nil
}

func (editor *studyReviewCachingEditor) storeStudyReview(
	ctx context.Context,
	prompt semanticdiscovery.Prompt,
	request []byte,
	bundle studymap.Bundle,
	direction studymap.DirectionCandidate,
	response []byte,
) {
	if editor.disableCache || ctx == nil || ctx.Err() != nil ||
		!studyReviewResponseAccepted(bundle, direction, response) {
		return
	}
	identity, key, _, err := editor.reviewCacheIdentity(
		prompt, request, bundle, direction,
	)
	if err != nil {
		editor.cacheWriteFailed()
		return
	}
	if _, found := secretscan.DetectAlways(string(response)); found {
		editor.cacheWriteFailed()
		return
	}
	if ctx.Err() != nil {
		return
	}
	record := studyReviewCacheRecord{
		Version: studyReviewCacheContractVersion,
		Key:     key, Identity: identity,
		ResponseSHA256: sha256Hex(response),
		Response:       append([]byte(nil), response...),
	}
	if err := writeStudyReviewCache(ctx, editor.runsDir, key, record); err != nil {
		if ctx.Err() != nil {
			return
		}
		editor.cacheWriteFailed()
	}
}

func (editor *studyReviewCachingEditor) reviewCacheIdentity(
	prompt semanticdiscovery.Prompt,
	request []byte,
	bundle studymap.Bundle,
	direction studymap.DirectionCandidate,
) (studyReviewCacheIdentity, string, []byte, error) {
	if prompt.Version != semanticdiscovery.ReadingPackReviewPromptVersion ||
		prompt.ThinkingProfile != semanticdiscovery.ThinkingDisabled {
		return studyReviewCacheIdentity{}, "", nil,
			fmt.Errorf("study review cache: unsupported review prompt contract")
	}
	planned, err := editor.SemanticDiscoveryPromptJSON(prompt)
	if err != nil {
		return studyReviewCacheIdentity{}, "", nil, err
	}
	if !bytes.Equal(planned, request) {
		return studyReviewCacheIdentity{}, "", nil,
			fmt.Errorf("study review cache: exact planned request changed")
	}
	if _, found := secretscan.DetectAlways(string(request)); found {
		return studyReviewCacheIdentity{}, "", nil,
			fmt.Errorf("study review cache: provider request contains an obvious credential")
	}
	reviewBundle, err := studyReviewBundleFromPrompt(prompt)
	if err != nil {
		return studyReviewCacheIdentity{}, "", nil, err
	}
	expectedReviewBundle, err := studymap.BuildReviewBundle(bundle, direction)
	if err != nil {
		return studyReviewCacheIdentity{}, "", nil,
			fmt.Errorf("study review cache: rebuild current review bundle: %w", err)
	}
	bundleJSON, err := json.Marshal(reviewBundle)
	if err != nil {
		return studyReviewCacheIdentity{}, "", nil,
			fmt.Errorf("study review cache: encode review bundle: %w", err)
	}
	expectedBundleJSON, err := json.Marshal(expectedReviewBundle)
	if err != nil {
		return studyReviewCacheIdentity{}, "", nil,
			fmt.Errorf("study review cache: encode current review bundle: %w", err)
	}
	if !bytes.Equal(bundleJSON, expectedBundleJSON) {
		return studyReviewCacheIdentity{}, "", nil,
			fmt.Errorf("study review cache: planned review bundle differs from current source")
	}
	fragmentsJSON, err := studyReviewSourceFragmentsJSON(reviewBundle)
	if err != nil {
		return studyReviewCacheIdentity{}, "", nil, err
	}
	providerIdentity, err := studyReviewProviderIdentityFromConfig(
		editor.promptClient.EffectiveConfig(),
	)
	if err != nil {
		return studyReviewCacheIdentity{}, "", nil, err
	}
	identity := studyReviewCacheIdentity{
		CacheContractVersion:   editor.cacheContractVersion,
		ValidatorVersion:       editor.validatorVersion,
		StageContractVersion:   editor.stageContractVersion,
		PromptVersion:          prompt.Version,
		ThinkingProfile:        prompt.ThinkingProfile,
		EnglishContractVersion: editor.englishContract,
		EndpointSHA256:         providerIdentity.EndpointSHA256,
		Model:                  providerIdentity.Model,
		ProviderProfile:        providerIdentity.ProviderProfile,
		MaxTokens:              providerIdentity.MaxTokens,
		Request:                append([]byte(nil), request...),
		ReviewBundleSHA256:     sha256Hex(bundleJSON),
		SourceFragmentsSHA256:  sha256Hex(fragmentsJSON),
	}
	identityJSON, err := json.Marshal(identity)
	if err != nil {
		return studyReviewCacheIdentity{}, "", nil,
			fmt.Errorf("study review cache: encode identity: %w", err)
	}
	if len(identityJSON) == 0 || len(identityJSON) > semanticDiscoveryMaxRequestBytes*2 {
		return studyReviewCacheIdentity{}, "", nil,
			fmt.Errorf("study review cache: identity exceeds its byte limit")
	}
	key := "study-review-" + sha256Hex(identityJSON)
	return identity, key, identityJSON, nil
}

func studyReviewProviderIdentityFromConfig(
	config deepseek.EffectiveConfig,
) (studyReviewProviderIdentity, error) {
	endpoint, err := canonicalStudyReviewEndpoint(config.Endpoint)
	if err != nil {
		return studyReviewProviderIdentity{}, fmt.Errorf(
			"study review cache: invalid provider endpoint identity",
		)
	}
	model := strings.TrimSpace(config.Model)
	authMode := strings.TrimSpace(config.AuthMode)
	if model == "" || len(model) > 1024 ||
		(authMode != "bearer" && authMode != "none") || config.MaxTokens <= 0 {
		return studyReviewProviderIdentity{}, fmt.Errorf(
			"study review cache: invalid provider identity",
		)
	}
	return studyReviewProviderIdentity{
		EndpointSHA256:  sha256Hex([]byte(endpoint)),
		Model:           model,
		ProviderProfile: "openai-compatible/" + authMode,
		MaxTokens:       config.MaxTokens,
	}, nil
}

func canonicalStudyReviewEndpoint(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Opaque != "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid endpoint")
	}
	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	if (scheme != "http" && scheme != "https") || hostname == "" {
		return "", fmt.Errorf("invalid endpoint")
	}
	port := parsed.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	host := hostname
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	return scheme + "://" + host + parsed.EscapedPath(), nil
}

func studyReviewBundleFromPrompt(
	prompt semanticdiscovery.Prompt,
) (studymap.ReviewBundle, error) {
	markerIndex := strings.LastIndex(prompt.User, studyMapReviewBundleMarker)
	if markerIndex < 0 {
		return studymap.ReviewBundle{}, fmt.Errorf("study review cache: review bundle is absent")
	}
	return studymap.DecodeReviewBundle(
		[]byte(prompt.User[markerIndex+len(studyMapReviewBundleMarker):]),
	)
}

func studyReviewSourceFragmentsJSON(bundle studymap.ReviewBundle) ([]byte, error) {
	type sourceIdentity struct {
		AnchorID       string                      `json:"anchor_id"`
		Path           string                      `json:"path"`
		Line           int                         `json:"line"`
		SourceFragment []studymap.ReviewSourceLine `json:"source_fragment"`
	}
	fragments := make([]sourceIdentity, 0, len(bundle.Anchors))
	for _, anchor := range bundle.Anchors {
		fragments = append(fragments, sourceIdentity{
			AnchorID: anchor.AnchorID, Path: anchor.Path, Line: anchor.Line,
			SourceFragment: anchor.SourceFragment,
		})
	}
	encoded, err := json.Marshal(fragments)
	if err != nil {
		return nil, fmt.Errorf("study review cache: encode source fragments: %w", err)
	}
	return encoded, nil
}

func studyReviewResponseAccepted(
	bundle studymap.Bundle,
	direction studymap.DirectionCandidate,
	response []byte,
) bool {
	if _, found := secretscan.DetectAlways(string(response)); found {
		return false
	}
	proposal, err := studymap.DecodeReviewProposal(response)
	if err != nil || proposal.DirectionID != direction.DirectionID {
		return false
	}
	reduction, err := studymap.ApplyReviews(
		bundle,
		studymap.DirectionProposal{
			Version:    studymap.DirectionProposalVersion,
			Directions: []studymap.DirectionCandidate{direction},
		},
		[]studymap.ReviewProposal{proposal},
	)
	return err == nil && reduction.Reviewed == 1 && len(reduction.Issues) == 0
}

func (editor *studyReviewCachingEditor) addStat(
	update func(*studyReviewCacheStats),
) {
	editor.mu.Lock()
	defer editor.mu.Unlock()
	update(&editor.stats)
}

func (editor *studyReviewCachingEditor) cacheWriteFailed() {
	editor.addStat(func(stats *studyReviewCacheStats) { stats.WriteFailures++ })
	editor.writeDiagnostic(
		"warning: Study review cache could not persist one validated response; the current Study result remains valid",
	)
}

func (editor *studyReviewCachingEditor) writeDiagnostic(message string) {
	editor.mu.Lock()
	defer editor.mu.Unlock()
	if editor.stderr != nil {
		fmt.Fprintln(editor.stderr, message)
	}
}

func (editor *studyReviewCachingEditor) writeSummary(stderr io.Writer) {
	if editor == nil || stderr == nil {
		return
	}
	editor.mu.Lock()
	stats := editor.stats
	editor.mu.Unlock()
	if stats.Hits == 0 && stats.Misses == 0 && stats.Bypassed == 0 &&
		stats.Corrupt == 0 && stats.WriteFailures == 0 {
		return
	}
	fmt.Fprintf(
		stderr,
		"repomap: Study review cache hits=%d misses=%d bypassed=%d corrupt=%d write_failures=%d\n",
		stats.Hits,
		stats.Misses,
		stats.Bypassed,
		stats.Corrupt,
		stats.WriteFailures,
	)
}

func studyReviewCachePath(runsDir, key string) string {
	return filepath.Join(
		runsDir,
		studyReviewCacheParentDirectory,
		studyReviewCacheVersionDirectory,
		key+".json",
	)
}

func loadStudyReviewCache(
	runsDir,
	key string,
	identityJSON []byte,
) (studyReviewCacheRecord, bool, bool) {
	if !validStudyReviewCacheKey(key) {
		return studyReviewCacheRecord{}, false, true
	}
	versionDir := filepath.Dir(studyReviewCachePath(runsDir, key))
	if _, err := os.Lstat(versionDir); os.IsNotExist(err) {
		return studyReviewCacheRecord{}, false, false
	}
	if _, safe := studyReviewCacheDirectory(runsDir, false); !safe {
		return studyReviewCacheRecord{}, false, true
	}
	path := studyReviewCachePath(runsDir, key)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return studyReviewCacheRecord{}, false, false
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 ||
		!info.Mode().IsRegular() || info.Size() <= 0 ||
		info.Size() > maxStudyReviewCacheRecordBytes {
		return studyReviewCacheRecord{}, false, true
	}
	file, err := os.Open(path)
	if err != nil {
		return studyReviewCacheRecord{}, false, true
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return studyReviewCacheRecord{}, false, true
	}
	data, err := io.ReadAll(io.LimitReader(file, maxStudyReviewCacheRecordBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxStudyReviewCacheRecordBytes || !utf8.Valid(data) {
		return studyReviewCacheRecord{}, false, true
	}
	var record studyReviewCacheRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil {
		return studyReviewCacheRecord{}, false, true
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return studyReviewCacheRecord{}, false, true
	}
	recordIdentityJSON, err := json.Marshal(record.Identity)
	if err != nil || record.Version != studyReviewCacheContractVersion ||
		record.Key != key || !bytes.Equal(recordIdentityJSON, identityJSON) ||
		record.ResponseSHA256 != sha256Hex(record.Response) ||
		len(record.Response) == 0 ||
		len(record.Response) > maxStudyReviewCacheRecordBytes ||
		!utf8.Valid(record.Response) {
		return studyReviewCacheRecord{}, false, true
	}
	if _, found := secretscan.DetectAlways(string(record.Response)); found {
		return studyReviewCacheRecord{}, false, true
	}
	return record, true, false
}

func writeStudyReviewCache(
	ctx context.Context,
	runsDir,
	key string,
	record studyReviewCacheRecord,
) error {
	if ctx == nil || ctx.Err() != nil {
		return context.Canceled
	}
	if !validStudyReviewCacheKey(key) || record.Key != key ||
		record.Version != studyReviewCacheContractVersion ||
		record.ResponseSHA256 != sha256Hex(record.Response) {
		return fmt.Errorf("study review cache: invalid record")
	}
	if _, found := secretscan.DetectAlways(string(record.Identity.Request)); found {
		return fmt.Errorf("study review cache: request contains an obvious credential")
	}
	if _, found := secretscan.DetectAlways(string(record.Response)); found {
		return fmt.Errorf("study review cache: response contains an obvious credential")
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("study review cache: encode record: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) == 0 || len(encoded) > maxStudyReviewCacheRecordBytes {
		return fmt.Errorf("study review cache: record exceeds its byte limit")
	}
	versionDir, safe := studyReviewCacheDirectory(runsDir, true)
	if !safe {
		return fmt.Errorf("study review cache: directory is unavailable")
	}
	temporary, err := os.CreateTemp(versionDir, ".study-review-")
	if err != nil {
		return fmt.Errorf("study review cache: create temporary record: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("study review cache: protect temporary record: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		return fmt.Errorf("study review cache: write record: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("study review cache: sync record: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("study review cache: close record: %w", err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	path := studyReviewCachePath(runsDir, key)
	if err := os.Link(temporaryPath, path); err != nil {
		if studyReviewCacheFileMatches(path, encoded) {
			return nil
		}
		return fmt.Errorf("study review cache: publish immutable record: %w", err)
	}
	directory, err := os.Open(versionDir)
	if err != nil {
		return fmt.Errorf("study review cache: open directory for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("study review cache: sync directory: %w", err)
	}
	return nil
}

func repairCorruptStudyReviewCache(
	runsDir,
	key string,
	identityJSON []byte,
	bundle studymap.Bundle,
	direction studymap.DirectionCandidate,
) error {
	if !validStudyReviewCacheKey(key) {
		return fmt.Errorf("study review cache: invalid corrupt entry key")
	}
	versionDir, safe := studyReviewCacheDirectory(runsDir, false)
	if !safe {
		return fmt.Errorf("study review cache: corrupt entry directory is unavailable")
	}
	lockPath := filepath.Join(versionDir, key+".repair-lock")
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		if os.IsExist(err) {
			return nil
		}
		return fmt.Errorf("study review cache: lock corrupt entry repair: %w", err)
	}
	defer func() { _ = os.Remove(lockPath) }()

	record, found, corrupt := loadStudyReviewCache(runsDir, key, identityJSON)
	if found && studyReviewResponseAccepted(bundle, direction, record.Response) {
		return nil
	}
	if !found && !corrupt {
		return nil
	}
	path := studyReviewCachePath(runsDir, key)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || info.IsDir() {
		return fmt.Errorf("study review cache: corrupt entry is unavailable")
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("study review cache: remove corrupt entry: %w", err)
	}
	return nil
}

func studyReviewCacheFileMatches(path string, want []byte) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 ||
		!info.Mode().IsRegular() || info.Size() != int64(len(want)) ||
		info.Size() <= 0 || info.Size() > maxStudyReviewCacheRecordBytes {
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
	got, err := io.ReadAll(io.LimitReader(file, maxStudyReviewCacheRecordBytes+1))
	return err == nil && bytes.Equal(got, want)
}

func validStudyReviewCacheKey(value string) bool {
	const prefix = "study-review-"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil
}

func studyReviewCacheDirectory(runsDir string, create bool) (string, bool) {
	root := filepath.Join(runsDir, studyReviewCacheParentDirectory)
	versionDir := filepath.Join(root, studyReviewCacheVersionDirectory)
	for _, directory := range []string{root, versionDir} {
		info, err := os.Lstat(directory)
		if os.IsNotExist(err) && create {
			if err := os.Mkdir(directory, 0o700); err != nil && !os.IsExist(err) {
				return "", false
			}
			info, err = os.Lstat(directory)
		}
		if err != nil {
			return "", false
		}
		if info == nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", false
		}
	}
	return versionDir, true
}
