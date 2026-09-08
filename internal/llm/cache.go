package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	// CacheDirectoryName is the single accepted-response cache owned by the
	// shared model executor. The ordinary `repomap cache clear` command removes
	// this directory as one explicit persistent cache target.
	CacheDirectoryName = ".llm-cache"
	cacheDirectoryName = CacheDirectoryName
	cacheRecordVersion = 2
	// Accepted records reference exact request/response files in payloads/.
	maxCacheRecordBytes = SemanticRecordByteLimit
)

// Only proven corruption authorizes removing a saved response. An I/O failure,
// a concurrent replacement or a stricter local limit is merely a cache miss.
type cacheCorruptionError struct{ error }

func (err *cacheCorruptionError) Unwrap() error { return err.error }

func corruptCache(err error) error { return &cacheCorruptionError{err} }

func isCacheCorruption(err error) bool {
	var corrupt *cacheCorruptionError
	return errors.As(err, &corrupt)
}

type acceptedCacheRecord struct {
	Version        int          `json:"version"`
	Contract       string       `json:"contract"`
	CacheKey       string       `json:"cache_key"`
	Accepted       bool         `json:"accepted"`
	RequestSHA256  string       `json:"request_sha256"`
	ResponseSHA256 string       `json:"response_sha256"`
	RequestBytes   int          `json:"request_bytes"`
	ResponseBytes  int          `json:"response_bytes"`
	RequestFile    string       `json:"request_file"`
	ResponseFile   string       `json:"response_file"`
	Request        []byte       `json:"-"`
	Response       []byte       `json:"-"`
	FinishReason   FinishReason `json:"finish_reason"`
	ChoiceCount    int          `json:"choice_count"`
	Metrics        Metrics      `json:"metrics"`
}

// CachedExchange reads the current answer behind an entity's request reference.
// Its owning stage must still validate the response against its own schema.
func CachedExchange(rootDir, key string) (Outcome[json.RawMessage], bool, error) {
	var outcome Outcome[json.RawMessage]
	if !validSHA256(key) {
		return outcome, false, fmt.Errorf("llm: invalid request cache key")
	}
	record, found, err := readAcceptedCache(rootDir, key, Limits{MaxResponseBytes: ProviderResponseByteLimit})
	if err != nil || !found {
		return outcome, false, err
	}
	outcome.CacheKey = key
	setOutcomeRequest(&outcome, record.Request)
	outcome.Response, outcome.ResponseSHA256 = record.Response, record.ResponseSHA256
	return outcome, true, nil
}

func loadAcceptedCache(
	rootDir,
	cacheKey string,
	request []byte,
	limits Limits,
) (acceptedCacheRecord, bool, error) {
	record, found, err := readAcceptedCache(rootDir, cacheKey, limits)
	if err != nil || !found {
		return record, found, err
	}
	if !bytes.Equal(record.Request, request) {
		return acceptedCacheRecord{}, false, corruptCache(errors.New("llm: cached request differs from prepared request"))
	}
	return record, true, nil
}

func readAcceptedCache(rootDir, cacheKey string, limits Limits) (acceptedCacheRecord, bool, error) {
	cacheDir, found, err := existingCacheDirectory(rootDir)
	if err != nil || !found {
		return acceptedCacheRecord{}, false, err
	}
	path := filepath.Join(cacheDir, cacheKey+".json")
	data, found, err := readBoundedRegularFile(path, maxCacheRecordBytes)
	if err != nil || !found {
		return acceptedCacheRecord{}, false, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record acceptedCacheRecord
	if err := decoder.Decode(&record); err != nil {
		return acceptedCacheRecord{}, false, corruptCache(fmt.Errorf("llm: decode accepted cache: %w", err))
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return acceptedCacheRecord{}, false, corruptCache(errors.New("llm: accepted cache contains multiple JSON values"))
		}
		return acceptedCacheRecord{}, false, corruptCache(fmt.Errorf("llm: decode accepted cache tail: %w", err))
	}
	record.Request, err = readPayload(cacheDir, record.RequestFile, SemanticRecordByteLimit)
	if err != nil {
		return acceptedCacheRecord{}, false, err
	}
	record.Response, err = readPayload(cacheDir, record.ResponseFile, limits.MaxResponseBytes)
	if err != nil {
		return acceptedCacheRecord{}, false, err
	}
	if err := validateAcceptedCacheRecord(record, cacheKey, record.Request, limits); err != nil {
		return acceptedCacheRecord{}, false, err
	}
	return record, true, nil
}

func validateAcceptedCacheRecord(
	record acceptedCacheRecord,
	cacheKey string,
	request []byte,
	limits Limits,
) error {
	if record.Version != cacheRecordVersion || record.Contract != executorContract ||
		record.CacheKey != cacheKey || !record.Accepted ||
		record.RequestSHA256 != sha256Hex(request) ||
		!bytes.Equal(record.Request, request) ||
		record.ResponseSHA256 != sha256Hex(record.Response) ||
		record.RequestBytes != len(request) ||
		record.ResponseBytes != len(record.Response) ||
		record.RequestBytes < 0 || record.ResponseBytes < 0 ||
		record.FinishReason != FinishStop || record.ChoiceCount != 1 ||
		len(record.Response) > hardMaxResponseBytes {
		return corruptCache(errors.New("llm: rejected cache identity or byte accounting"))
	}
	if err := validateMetrics(record.Metrics); err != nil {
		return corruptCache(fmt.Errorf("llm: rejected cache metrics: %w", err))
	}
	if len(record.Response) > limits.MaxResponseBytes {
		return errors.New("llm: cached response exceeds the current byte limit")
	}
	return nil
}

func saveAcceptedCache(rootDir string, record acceptedCacheRecord) error {
	if record.Version != cacheRecordVersion || record.Contract != executorContract ||
		!validSHA256(record.CacheKey) || !record.Accepted ||
		record.RequestSHA256 != sha256Hex(record.Request) ||
		record.ResponseSHA256 != sha256Hex(record.Response) ||
		record.RequestBytes != len(record.Request) || record.ResponseBytes != len(record.Response) ||
		record.FinishReason != FinishStop || record.ChoiceCount != 1 ||
		len(record.Response) > hardMaxResponseBytes {
		return errors.New("llm: refuse invalid accepted cache record")
	}
	if err := validateMetrics(record.Metrics); err != nil {
		return fmt.Errorf("llm: refuse invalid accepted cache metrics: %w", err)
	}
	requestFile, err := SavePayload(rootDir, record.Request)
	if err != nil {
		return err
	}
	responseFile, err := SavePayload(rootDir, record.Response)
	if err != nil {
		return err
	}
	record.RequestFile, record.ResponseFile = filepath.Base(requestFile), filepath.Base(responseFile)
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("llm: encode accepted cache: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maxCacheRecordBytes {
		return fmt.Errorf("llm: accepted cache record exceeds %d bytes", maxCacheRecordBytes)
	}

	cacheDir, err := ensureCacheDirectory(rootDir)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(cacheDir, ".llm-cache-*.tmp")
	if err != nil {
		return fmt.Errorf("llm: create temporary cache: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("llm: protect temporary cache: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("llm: write temporary cache: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("llm: sync temporary cache: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("llm: close temporary cache: %w", err)
	}
	path := filepath.Join(cacheDir, record.CacheKey+".json")
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("llm: publish accepted cache: %w", err)
	}
	return nil
}

func removeAcceptedCache(rootDir, cacheKey string) error {
	cacheDir, found, err := existingCacheDirectory(rootDir)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if err := os.RemoveAll(filepath.Join(cacheDir, cacheKey+".json")); err != nil {
		return fmt.Errorf("llm: remove invalid accepted cache: %w", err)
	}
	return nil
}

func existingCacheDirectory(rootDir string) (string, bool, error) {
	if rootDir == "" {
		return "", false, errors.New("llm: cache root is empty")
	}
	rootInfo, err := os.Lstat(rootDir)
	if err != nil {
		return "", false, fmt.Errorf("llm: inspect cache root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return "", false, errors.New("llm: cache root is not a regular directory")
	}
	cacheDir := filepath.Join(rootDir, cacheDirectoryName)
	info, err := os.Lstat(cacheDir)
	if errors.Is(err, os.ErrNotExist) {
		return cacheDir, false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("llm: inspect cache directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", false, errors.New("llm: cache directory is not a regular directory")
	}
	return cacheDir, true, nil
}

func ensureCacheDirectory(rootDir string) (string, error) {
	cacheDir, found, err := existingCacheDirectory(rootDir)
	if err != nil {
		return "", err
	}
	if found {
		return cacheDir, nil
	}
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("llm: create cache directory: %w", err)
		}
	}
	cacheDir, found, err = existingCacheDirectory(rootDir)
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("llm: cache directory was not created")
	}
	return cacheDir, nil
}

func readBoundedRegularFile(path string, limit int) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("llm: inspect cache entry: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, false, corruptCache(errors.New("llm: cache entry is not a regular file"))
	}
	if info.Size() < 0 || info.Size() > int64(limit) {
		return nil, false, errors.New("llm: cache entry is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false, fmt.Errorf("llm: open cache entry: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("llm: inspect opened cache entry: %w", err)
	}
	if err := validateOpenedCacheFile(info, opened, limit); err != nil {
		return nil, false, err
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, false, fmt.Errorf("llm: read cache entry: %w", err)
	}
	if len(data) > limit {
		return nil, false, errors.New("llm: cache entry exceeds its byte limit")
	}
	return data, true, nil
}

func validateOpenedCacheFile(before, opened os.FileInfo, limit int) error {
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) ||
		opened.Size() < 0 || opened.Size() > int64(limit) {
		return errors.New("llm: cache entry changed before read")
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
