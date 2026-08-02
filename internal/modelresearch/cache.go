package modelresearch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	cacheDirectory = ".model-research"
	// cacheRecordVersion is independent from the persisted research contract.
	// Increment it whenever a previously validated provider response is no
	// longer guaranteed to replay under current stage validators.
	cacheRecordVersion = 2
)

var ErrInvalidCachedRound = errors.New("model research: reject invalid cached round")

type FingerprintInput struct {
	Repository             RepositoryContext `json:"repository"`
	Stage                  string            `json:"stage"`
	PromptVersion          string            `json:"prompt_version"`
	CacheContract          string            `json:"cache_contract,omitempty"`
	Profile                string            `json:"profile"`
	Model                  string            `json:"model"`
	ProviderEndpointSHA256 string            `json:"provider_endpoint_sha256"`
	RequestSHA256          string            `json:"request_sha256"`
	EvidenceBundleHash     string            `json:"evidence_bundle_sha256"`
	PolicyVersion          string            `json:"policy_version"`
	OutputLanguage         string            `json:"output_language,omitempty"`
}

type cacheRecord struct {
	Version               int    `json:"version"`
	CacheKey              string `json:"cache_key"`
	CacheContract         string `json:"cache_contract,omitempty"`
	RequestSHA256         string `json:"request_sha256"`
	BundleSHA256          string `json:"bundle_sha256"`
	ResponseSHA256        string `json:"response_sha256"`
	Response              []byte `json:"response"`
	RequestBytes          int    `json:"request_bytes"`
	ResponseBytes         int    `json:"response_bytes"`
	InputTokens           int    `json:"input_tokens,omitempty"`
	OutputTokens          int    `json:"output_tokens,omitempty"`
	PromptCacheHitTokens  int    `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens int    `json:"prompt_cache_miss_tokens,omitempty"`
	LatencyMillis         int64  `json:"latency_ms,omitempty"`
	RetryCount            int    `json:"retry_count,omitempty"`
}

type StageCacheInput struct {
	RunsDir            string
	Fingerprint        FingerprintInput
	Request            []byte
	EvidenceBundleHash string
}

type StageResponse struct {
	CacheKey              string
	Content               []byte
	RequestBytes          int
	ResponseBytes         int
	InputTokens           int
	OutputTokens          int
	PromptCacheHitTokens  int
	PromptCacheMissTokens int
	LatencyMillis         int64
	RetryCount            int
	Cached                bool
}

func SHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// ProviderEndpointSHA256 returns a stable, non-secret identity for one
// provider endpoint. The raw endpoint is never stored in a cache key or
// record. Transport-irrelevant spelling differences are normalized before
// hashing; user-info and fragments are rejected rather than silently hidden.
func ProviderEndpointSHA256(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Opaque != "" || parsed.User != nil || parsed.Fragment != "" {
		return "", fmt.Errorf("model research: invalid provider endpoint identity")
	}
	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	if (scheme != "http" && scheme != "https") || hostname == "" {
		return "", fmt.Errorf("model research: invalid provider endpoint identity")
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
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", fmt.Errorf("model research: invalid provider endpoint identity")
	}
	canonical := url.URL{
		Scheme: scheme, Host: host, Path: parsed.Path, RawPath: parsed.RawPath,
		RawQuery: query.Encode(),
	}
	return SHA256([]byte(canonical.String())), nil
}

// CacheOutputLanguage keeps the historical default-English fingerprint while
// isolating provider responses whose human-readable prose is requested in
// Russian.
func CacheOutputLanguage(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "ru") {
		return "ru"
	}
	return ""
}

func LoadStageResponse(input StageCacheInput) (StageResponse, bool, error) {
	if err := validateStageCacheInput(input); err != nil {
		return StageResponse{}, false, err
	}
	cacheKey, err := CacheKey(input.Fingerprint)
	if err != nil {
		return StageResponse{}, false, err
	}
	record, found, err := loadCache(
		input.RunsDir,
		cacheKey,
		input.Fingerprint.CacheContract,
		requestHash(input.Request),
		input.EvidenceBundleHash,
	)
	if errors.Is(err, ErrInvalidCachedRound) {
		if removeErr := removeCache(input.RunsDir, cacheKey); removeErr != nil {
			return StageResponse{}, false, removeErr
		}
		return StageResponse{}, false, nil
	}
	if err != nil || !found {
		return StageResponse{}, found, err
	}
	return StageResponse{
		CacheKey: cacheKey, Content: append([]byte(nil), record.Response...),
		RequestBytes: record.RequestBytes, ResponseBytes: record.ResponseBytes,
		InputTokens: record.InputTokens, OutputTokens: record.OutputTokens,
		PromptCacheHitTokens:  record.PromptCacheHitTokens,
		PromptCacheMissTokens: record.PromptCacheMissTokens,
		LatencyMillis:         record.LatencyMillis, RetryCount: record.RetryCount, Cached: true,
	}, true, nil
}

// InvalidateStageResponse removes only the exact generic stage-cache record.
// Semantic validation remains owned by the consuming stage.
func InvalidateStageResponse(input StageCacheInput) error {
	if err := validateStageCacheInput(input); err != nil {
		return err
	}
	cacheKey, err := CacheKey(input.Fingerprint)
	if err != nil {
		return err
	}
	return removeCache(input.RunsDir, cacheKey)
}

func SaveStageResponse(input StageCacheInput, response StageResponse) (StageResponse, error) {
	if err := validateStageCacheInput(input); err != nil {
		return StageResponse{}, err
	}
	cacheKey, err := CacheKey(input.Fingerprint)
	if err != nil {
		return StageResponse{}, err
	}
	record := cacheRecord{
		Version: cacheRecordVersion, CacheKey: cacheKey,
		CacheContract: input.Fingerprint.CacheContract,
		RequestSHA256: requestHash(input.Request), BundleSHA256: input.EvidenceBundleHash,
		ResponseSHA256: requestHash(response.Content), Response: append([]byte(nil), response.Content...),
		RequestBytes: len(input.Request), ResponseBytes: len(response.Content),
		InputTokens: response.InputTokens, OutputTokens: response.OutputTokens,
		PromptCacheHitTokens:  response.PromptCacheHitTokens,
		PromptCacheMissTokens: response.PromptCacheMissTokens,
		LatencyMillis:         response.LatencyMillis, RetryCount: response.RetryCount,
	}
	if err := saveCache(input.RunsDir, record); err != nil {
		return StageResponse{}, err
	}
	response.CacheKey = cacheKey
	response.RequestBytes = len(input.Request)
	response.ResponseBytes = len(response.Content)
	return response, nil
}

func BundleHash(bundle EvidenceBundle) (string, []byte, error) {
	canonical := bundle
	canonical.ProviderAllowedPaths = sortedUnique(canonical.ProviderAllowedPaths)
	canonical.KnownComponentIDs = sortedUnique(canonical.KnownComponentIDs)
	canonical.KnownSurfaceIDs = sortedUnique(canonical.KnownSurfaceIDs)
	canonical.KnownTraceIDs = sortedUnique(canonical.KnownTraceIDs)
	sort.Slice(canonical.Evidence, func(i, j int) bool { return canonical.Evidence[i].ID < canonical.Evidence[j].ID })
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", nil, fmt.Errorf("model research: encode evidence bundle: %w", err)
	}
	return SHA256(encoded), encoded, nil
}

func CacheKey(input FingerprintInput) (string, error) {
	if input.Stage == "" || input.PromptVersion == "" || input.Model == "" ||
		!IsSHA256(input.ProviderEndpointSHA256) || !IsSHA256(input.RequestSHA256) ||
		!IsSHA256(input.EvidenceBundleHash) {
		return "", fmt.Errorf("model research: incomplete cache fingerprint")
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("model research: encode cache fingerprint: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "research-" + hex.EncodeToString(digest[:]), nil
}

func validateStageCacheInput(input StageCacheInput) error {
	if input.Fingerprint.RequestSHA256 != requestHash(input.Request) ||
		input.Fingerprint.EvidenceBundleHash != input.EvidenceBundleHash {
		return fmt.Errorf("model research: cache fingerprint does not match exact request")
	}
	return nil
}

// IsSHA256 reports whether value is the canonical lowercase encoding used by
// cache identities. It permits cache owners to validate a digest without
// persisting or returning its raw source value.
func IsSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func requestHash(request []byte) string {
	return SHA256(request)
}

func cachePath(runsDir, cacheKey string) string {
	return filepath.Join(runsDir, cacheDirectory, cacheKey+".json")
}

func loadCache(
	runsDir,
	cacheKey,
	cacheContract,
	requestSHA,
	bundleSHA string,
) (cacheRecord, bool, error) {
	data, err := os.ReadFile(cachePath(runsDir, cacheKey))
	if errors.Is(err, os.ErrNotExist) {
		return cacheRecord{}, false, nil
	}
	if err != nil {
		return cacheRecord{}, false, fmt.Errorf("model research: read cache: %w", err)
	}
	var record cacheRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return cacheRecord{}, false, fmt.Errorf("%w: decode: %v", ErrInvalidCachedRound, err)
	}
	if record.Version != cacheRecordVersion || record.CacheKey != cacheKey ||
		record.CacheContract != cacheContract ||
		record.RequestSHA256 != requestSHA || record.BundleSHA256 != bundleSHA ||
		record.ResponseSHA256 != requestHash(record.Response) || record.ResponseBytes != len(record.Response) {
		return cacheRecord{}, false, ErrInvalidCachedRound
	}
	return record, true, nil
}

func removeCache(runsDir, cacheKey string) error {
	err := os.Remove(cachePath(runsDir, cacheKey))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("model research: remove rejected cache entry: %w", err)
	}
	return nil
}

func saveCache(runsDir string, record cacheRecord) error {
	if _, found := secretscan.Detect(string(record.Response)); found {
		return fmt.Errorf("model research: provider response contains an obvious credential")
	}
	path := cachePath(runsDir, record.CacheKey)
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("model research: encode cache: %w", err)
	}
	return writeProtected(path, append(encoded, '\n'))
}

func WriteState(runDir string, state State) error {
	state.UpdatedAt = nowUTC()
	if err := state.Validate(); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("model research: encode state: %w", err)
	}
	return writeProtected(filepath.Join(runDir, StateFile), append(encoded, '\n'))
}

func ReadState(runDir string) (State, error) {
	data, err := os.ReadFile(filepath.Join(runDir, StateFile))
	if err != nil {
		return State{}, err
	}
	var state State
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return State{}, fmt.Errorf("model research: decode state: %w", err)
	}
	if err := state.Validate(); err != nil {
		return State{}, err
	}
	return state, nil
}

func writeProtected(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("model research: create artifact directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".repomap-model-research-")
	if err != nil {
		return fmt.Errorf("model research: create temporary artifact: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("model research: protect artifact: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("model research: write artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("model research: close artifact: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("model research: replace artifact: %w", err)
	}
	return nil
}

func sortedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

var nowUTC = func() string { return time.Now().UTC().Format(time.RFC3339) }
