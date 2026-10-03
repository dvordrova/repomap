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

// RunPayloadDirectoryName holds, inside one run directory, the exact bytes of
// that run's exchanges that no accepted cache record owns.
const RunPayloadDirectoryName = "payloads"

// SavePayload stores exact request or response bytes once in the shared cache.
// Accepted records and the run journals of accepted exchanges reference them.
// The bytes reach the disk before the caller writes a record that names them,
// so a power loss cannot leave a synced record over a torn payload.
func SavePayload(rootDir string, raw []byte) (string, error) {
	cacheDir, err := ensureCacheDirectory(rootDir)
	if err != nil {
		return "", err
	}
	return savePayloadIn(filepath.Join(cacheDir, "payloads"), raw, true)
}

// SaveRunPayload stores the exact bytes of a refused or failed exchange once
// in its own run directory. No accepted record owns them, so they are a
// developer's diagnostics, not the user's cache: cache clear leaves them and a
// later run never reads them.
func SaveRunPayload(runDir string, raw []byte) (string, error) {
	if runDir == "" {
		return "", errors.New("llm: run directory is empty")
	}
	return savePayloadIn(filepath.Join(runDir, RunPayloadDirectoryName), raw, false)
}

// savePayloadIn returns the file named by raw's hash once it holds exactly
// raw. A file of that name is shared: accepted records, run journals and
// window refs of every run that used these bytes link it. So an existing file
// is reused only after its bytes are compared with raw, and one that differs
// or cannot be read is replaced, atomically, by the bytes its name promises
// (a corrupt payload once made the same request pay the provider on every
// run: the record was evicted and rewritten over the same corrupt file).
// Nothing is deleted and no owner's link changes. Only an entry that cannot
// be inspected, or a directory in its place, is an error.
func savePayloadIn(dir string, raw []byte, durable bool) (string, error) {
	extension := ".txt"
	if json.Valid(raw) {
		extension = ".json"
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := filepath.Join(dir, sha256Hex(raw)+extension)
	holds, err := payloadHolds(name, raw)
	if err != nil {
		return "", err
	}
	if holds {
		return filepath.Abs(name)
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
	if durable {
		if err := file.Sync(); err != nil {
			file.Close()
			return "", err
		}
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(file.Name(), name); err != nil {
		return "", err
	}
	return filepath.Abs(name)
}

// payloadHolds reports whether name is a regular file holding exactly raw.
// Absent, a link or another special file, a different size or different
// bytes, and a file that cannot be opened or read all answer false: writing
// raw under its own hash loses nothing. A special file is never opened.
func payloadHolds(name string, raw []byte) (bool, error) {
	info, err := os.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("llm: inspect payload: %w", err)
	}
	if info.IsDir() {
		return false, fmt.Errorf("llm: payload %s is a directory", filepath.Base(name))
	}
	if !info.Mode().IsRegular() || info.Size() != int64(len(raw)) {
		return false, nil
	}
	file, err := os.Open(name)
	if err != nil {
		return false, nil
	}
	defer file.Close()
	if opened, err := file.Stat(); err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return false, nil
	}
	buffer := make([]byte, 64<<10)
	rest := raw
	for {
		n, err := file.Read(buffer)
		if n > len(rest) || !bytes.Equal(buffer[:n], rest[:n]) {
			return false, nil
		}
		rest = rest[n:]
		if errors.Is(err, io.EOF) {
			return len(rest) == 0, nil
		}
		if err != nil {
			return false, nil
		}
	}
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
