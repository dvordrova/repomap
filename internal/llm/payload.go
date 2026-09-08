package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SavePayload stores exact request or response bytes once in the shared cache.
// Run journals retain references to these files, including failed exchanges.
func SavePayload(rootDir string, raw []byte) (string, error) {
	cacheDir, err := ensureCacheDirectory(rootDir)
	if err != nil {
		return "", err
	}
	extension := ".txt"
	if json.Valid(raw) {
		extension = ".json"
	}
	dir := filepath.Join(cacheDir, "payloads")
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
