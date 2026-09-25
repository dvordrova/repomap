package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// RunPayloadDirectoryName holds, inside one run directory, the exact bytes of
// that run's exchanges that no accepted cache record owns.
const RunPayloadDirectoryName = "payloads"

// SavePayload stores exact request or response bytes once in the shared cache.
// Accepted records and the run journals of accepted exchanges reference them.
func SavePayload(rootDir string, raw []byte) (string, error) {
	cacheDir, err := ensureCacheDirectory(rootDir)
	if err != nil {
		return "", err
	}
	return savePayloadIn(filepath.Join(cacheDir, "payloads"), raw)
}

// SaveRunPayload stores the exact bytes of a refused or failed exchange once
// in its own run directory. No accepted record owns them, so they are a
// developer's diagnostics, not the user's cache: cache clear leaves them and a
// later run never reads them.
func SaveRunPayload(runDir string, raw []byte) (string, error) {
	if runDir == "" {
		return "", errors.New("llm: run directory is empty")
	}
	return savePayloadIn(filepath.Join(runDir, RunPayloadDirectoryName), raw)
}

func savePayloadIn(dir string, raw []byte) (string, error) {
	extension := ".txt"
	if json.Valid(raw) {
		extension = ".json"
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := filepath.Join(dir, sha256Hex(raw)+extension)
	if _, err := os.Stat(name); err == nil {
		return filepath.Abs(name)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	file, err := os.CreateTemp(dir, ".payload-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(raw); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(file.Name(), name); err != nil {
		return "", err
	}
	return filepath.Abs(name)
}

func readPayload(cacheDir, filename string, limit int) ([]byte, error) {
	if filename == "" || filepath.Base(filename) != filename || !filepath.IsLocal(filename) {
		return nil, corruptCache(fmt.Errorf("llm: invalid payload filename"))
	}
	raw, found, err := readBoundedRegularFile(filepath.Join(cacheDir, "payloads", filename), limit)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, corruptCache(fmt.Errorf("llm: cached payload is missing"))
	}
	return raw, nil
}
