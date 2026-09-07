package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MemoIdentity binds a reusable interpretation to its provider configuration,
// semantic state and exact isolated input. Preparing this input does not make
// a provider call. The owning cube decides whether an answer is independent
// of the other items with which it was originally batched.
func MemoIdentity(provider Provider, state []byte, prompt Prompt, limits Limits) (string, error) {
	providerState, err := canonicalProviderState(provider.State())
	if err != nil {
		return "", err
	}
	prepared, err := provider.Prepare(prompt, limits)
	if err != nil {
		return "", err
	}
	return executionCacheKey(providerState, state, prepared.Bytes()), nil
}

// A memo stores a cube-owned index into shared responses. The atlas uses it
// to find the original batch and row for an unchanged entity input.
type memoRecord struct {
	Version int             `json:"version"`
	Key     string          `json:"key"`
	SHA256  string          `json:"sha256"`
	Value   json.RawMessage `json:"value"`
}

func LoadMemo[T any](executor Executor, key string, validate DecodeValidate[T]) (T, bool, error) {
	var zero T
	if !executor.Enabled {
		return zero, false, nil
	}
	if !validSHA256(key) || validate == nil {
		return zero, false, fmt.Errorf("llm: memo needs an identity and validator")
	}
	dir, found, err := existingCacheDirectory(executor.RootDir)
	if err != nil || !found {
		return zero, false, err
	}
	raw, found, err := readBoundedRegularFile(filepath.Join(dir, "memo-"+key+".json"), maxCacheRecordBytes)
	if err != nil || !found {
		return zero, false, err
	}
	record, err := decodeJSONValue(raw, func(record memoRecord) error {
		if record.Version != 1 || record.Key != key || record.SHA256 != sha256Hex(record.Value) {
			return fmt.Errorf("llm: memo identity or value changed")
		}
		return nil
	})
	if err != nil {
		return zero, false, err
	}
	value, err := validate(record.Value)
	return value, err == nil, err
}

// SaveMemo persists an already accepted, cube-owned value in the executor's
// single cache directory. --no-cache bypasses it; cache clear removes it.
func SaveMemo(executor Executor, key string, value []byte) error {
	if !executor.Enabled {
		return nil
	}
	if !validSHA256(key) || !json.Valid(value) {
		return fmt.Errorf("llm: invalid memo identity or JSON value")
	}
	// Compact before hashing: RawMessage is compacted again by Marshal.
	var compact json.RawMessage
	if err := json.Unmarshal(value, &compact); err != nil {
		return err
	}
	value, err := json.Marshal(compact)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(memoRecord{Version: 1, Key: key, SHA256: sha256Hex(value), Value: value})
	if err != nil {
		return err
	}
	if len(raw) > maxCacheRecordBytes {
		return fmt.Errorf("llm: memo exceeds cache record envelope")
	}
	dir, err := ensureCacheDirectory(executor.RootDir)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".memo-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(dir, "memo-"+key+".json"))
}
