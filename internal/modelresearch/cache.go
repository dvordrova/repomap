package modelresearch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/secretscan"
)

const cacheDirectory = ".model-research"

type FingerprintInput struct {
	Repository         RepositoryContext `json:"repository"`
	Stage              string            `json:"stage"`
	PromptVersion      string            `json:"prompt_version"`
	Profile            string            `json:"profile"`
	Model              string            `json:"model"`
	EvidenceBundleHash string            `json:"evidence_bundle_sha256"`
	PolicyVersion      string            `json:"policy_version"`
}

type cacheRecord struct {
	Version        int    `json:"version"`
	CacheKey       string `json:"cache_key"`
	RequestSHA256  string `json:"request_sha256"`
	BundleSHA256   string `json:"bundle_sha256"`
	ResponseSHA256 string `json:"response_sha256"`
	Response       []byte `json:"response"`
	RequestBytes   int    `json:"request_bytes"`
	ResponseBytes  int    `json:"response_bytes"`
	InputTokens    int    `json:"input_tokens,omitempty"`
	OutputTokens   int    `json:"output_tokens,omitempty"`
	LatencyMillis  int64  `json:"latency_ms,omitempty"`
	RetryCount     int    `json:"retry_count,omitempty"`
}

type StageCacheInput struct {
	RunsDir            string
	Fingerprint        FingerprintInput
	Request            []byte
	EvidenceBundleHash string
}

type StageResponse struct {
	CacheKey      string
	Content       []byte
	RequestBytes  int
	ResponseBytes int
	InputTokens   int
	OutputTokens  int
	LatencyMillis int64
	RetryCount    int
	Cached        bool
}

func SHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func LoadStageResponse(input StageCacheInput) (StageResponse, bool, error) {
	cacheKey, err := CacheKey(input.Fingerprint)
	if err != nil {
		return StageResponse{}, false, err
	}
	record, found, err := loadCache(input.RunsDir, cacheKey, requestHash(input.Request), input.EvidenceBundleHash)
	if err != nil || !found {
		return StageResponse{}, found, err
	}
	return StageResponse{
		CacheKey: cacheKey, Content: append([]byte(nil), record.Response...),
		RequestBytes: record.RequestBytes, ResponseBytes: record.ResponseBytes,
		InputTokens: record.InputTokens, OutputTokens: record.OutputTokens,
		LatencyMillis: record.LatencyMillis, RetryCount: record.RetryCount, Cached: true,
	}, true, nil
}

func SaveStageResponse(input StageCacheInput, response StageResponse) (StageResponse, error) {
	cacheKey, err := CacheKey(input.Fingerprint)
	if err != nil {
		return StageResponse{}, err
	}
	record := cacheRecord{
		Version: ContractVersion, CacheKey: cacheKey,
		RequestSHA256: requestHash(input.Request), BundleSHA256: input.EvidenceBundleHash,
		ResponseSHA256: requestHash(response.Content), Response: append([]byte(nil), response.Content...),
		RequestBytes: len(input.Request), ResponseBytes: len(response.Content),
		InputTokens: response.InputTokens, OutputTokens: response.OutputTokens,
		LatencyMillis: response.LatencyMillis, RetryCount: response.RetryCount,
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
	if input.Stage == "" || input.PromptVersion == "" || input.Model == "" || input.EvidenceBundleHash == "" {
		return "", fmt.Errorf("model research: incomplete cache fingerprint")
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("model research: encode cache fingerprint: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "research-" + hex.EncodeToString(digest[:]), nil
}

func requestHash(request []byte) string {
	return SHA256(request)
}

func cachePath(runsDir, cacheKey string) string {
	return filepath.Join(runsDir, cacheDirectory, cacheKey+".json")
}

func loadCache(runsDir, cacheKey, requestSHA, bundleSHA string) (cacheRecord, bool, error) {
	data, err := os.ReadFile(cachePath(runsDir, cacheKey))
	if errors.Is(err, os.ErrNotExist) {
		return cacheRecord{}, false, nil
	}
	if err != nil {
		return cacheRecord{}, false, fmt.Errorf("model research: read cache: %w", err)
	}
	var record cacheRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return cacheRecord{}, false, fmt.Errorf("model research: decode cache: %w", err)
	}
	if record.Version != ContractVersion || record.CacheKey != cacheKey ||
		record.RequestSHA256 != requestSHA || record.BundleSHA256 != bundleSHA ||
		record.ResponseSHA256 != requestHash(record.Response) || record.ResponseBytes != len(record.Response) {
		return cacheRecord{}, false, fmt.Errorf("model research: reject invalid cached round")
	}
	return record, true, nil
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
