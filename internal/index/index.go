// Package index persists bounded local evidence that can be selected without
// rerunning an analyzer or sending a repository to a model.
package index

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/symbol"
)

const Version = 1

var ErrNotFound = errors.New("index: target not found")

type Metadata struct {
	Repository string `json:"repository"`
	Revision   string `json:"revision,omitempty"`
}

type Index struct {
	metadata Metadata
	records  map[string]record
	byPath   map[string]map[string]struct{}
}

type record struct {
	bundle json.RawMessage
	paths  []string
}

type snapshot struct {
	Version  int               `json:"version"`
	Metadata Metadata          `json:"metadata"`
	Symbols  []json.RawMessage `json:"symbols"`
}

func New(metadata Metadata) *Index {
	return &Index{
		metadata: metadata,
		records:  make(map[string]record),
		byPath:   make(map[string]map[string]struct{}),
	}
}

func (i *Index) Metadata() Metadata {
	return i.metadata
}

func (i *Index) Len() int {
	return len(i.records)
}

// Put adds or replaces the bounded neighborhood for its resolved target.
func (i *Index) Put(bundle symbol.Bundle) error {
	targetID, paths, err := validateBundle(bundle)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("index: encode target %q: %w", targetID, err)
	}

	if existing, ok := i.records[targetID]; ok {
		i.removePaths(targetID, existing.paths)
	}
	i.records[targetID] = record{bundle: encoded, paths: paths}
	for _, path := range paths {
		if i.byPath[path] == nil {
			i.byPath[path] = make(map[string]struct{})
		}
		i.byPath[path][targetID] = struct{}{}
	}
	return nil
}

// Neighborhood returns a defensive copy of one target's bounded evidence.
func (i *Index) Neighborhood(targetID string) (symbol.Bundle, error) {
	record, ok := i.records[targetID]
	if !ok {
		return symbol.Bundle{}, fmt.Errorf("%w: %s", ErrNotFound, targetID)
	}
	var bundle symbol.Bundle
	if err := json.Unmarshal(record.bundle, &bundle); err != nil {
		return symbol.Bundle{}, fmt.Errorf("index: decode target %q: %w", targetID, err)
	}
	return bundle, nil
}

func (i *Index) Targets() []string {
	targets := make([]string, 0, len(i.records))
	for targetID := range i.records {
		targets = append(targets, targetID)
	}
	sort.Strings(targets)
	return targets
}

// InvalidatePath removes every neighborhood whose bounded evidence references
// path and returns the removed target IDs in stable order.
func (i *Index) InvalidatePath(path string) []string {
	path, err := normalizePath(path)
	if err != nil {
		return nil
	}
	targetSet := i.byPath[path]
	removed := make([]string, 0, len(targetSet))
	for targetID := range targetSet {
		removed = append(removed, targetID)
	}
	sort.Strings(removed)
	for _, targetID := range removed {
		record := i.records[targetID]
		i.removePaths(targetID, record.paths)
		delete(i.records, targetID)
	}
	return removed
}

func (i *Index) Save(path string) error {
	symbols := make([]json.RawMessage, 0, len(i.records))
	for _, targetID := range i.Targets() {
		symbols = append(symbols, append(json.RawMessage(nil), i.records[targetID].bundle...))
	}
	data, err := json.MarshalIndent(snapshot{
		Version:  Version,
		Metadata: i.metadata,
		Symbols:  symbols,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("index: encode snapshot: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("index: create snapshot directory: %w", err)
	}
	temporaryPath := path + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0o600); err != nil {
		return fmt.Errorf("index: write snapshot: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("index: replace snapshot: %w", err)
	}
	return nil
}

func Load(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("index: read snapshot: %w", err)
	}
	var persisted snapshot
	if err := decodeJSON(data, &persisted); err != nil {
		return nil, fmt.Errorf("index: decode snapshot: %w", err)
	}
	if persisted.Version != Version {
		return nil, fmt.Errorf("index: unsupported snapshot version %d", persisted.Version)
	}

	result := New(persisted.Metadata)
	seen := make(map[string]struct{}, len(persisted.Symbols))
	for position, raw := range persisted.Symbols {
		var bundle symbol.Bundle
		if err := json.Unmarshal(raw, &bundle); err != nil {
			return nil, fmt.Errorf("index: decode symbol %d: %w", position, err)
		}
		targetID := bundle.Target.Entity.ID
		if _, exists := seen[targetID]; exists {
			return nil, fmt.Errorf("index: duplicate target %q", targetID)
		}
		seen[targetID] = struct{}{}
		if err := result.Put(bundle); err != nil {
			return nil, fmt.Errorf("index: load symbol %d: %w", position, err)
		}
	}
	return result, nil
}

func validateBundle(bundle symbol.Bundle) (string, []string, error) {
	if bundle.Version != symbol.BundleVersion {
		return "", nil, fmt.Errorf("index: unsupported symbol bundle version %d", bundle.Version)
	}
	targetID := bundle.Target.Entity.ID
	if targetID == "" {
		return "", nil, fmt.Errorf("index: symbol bundle has no target ID")
	}
	if bundle.Target.Entity.Location == nil {
		return "", nil, fmt.Errorf("index: target %q has no location", targetID)
	}
	targetPath, err := normalizePath(bundle.Target.Entity.Location.Path)
	if err != nil {
		return "", nil, fmt.Errorf("index: target %q: %w", targetID, err)
	}

	pathSet := make(map[string]struct{}, len(bundle.AllowedPaths))
	for _, path := range bundle.AllowedPaths {
		normalized, err := normalizePath(path)
		if err != nil {
			return "", nil, fmt.Errorf("index: target %q: %w", targetID, err)
		}
		pathSet[normalized] = struct{}{}
	}
	if _, ok := pathSet[targetPath]; !ok {
		return "", nil, fmt.Errorf("index: target %q path %q is not allowed", targetID, targetPath)
	}
	paths := make([]string, 0, len(pathSet))
	for path := range pathSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return targetID, paths, nil
}

func normalizePath(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return "", fmt.Errorf("invalid repository-relative path %q", path)
	}
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "." || path == ".." || strings.HasPrefix(path, "../") {
		return "", fmt.Errorf("invalid repository-relative path %q", path)
	}
	return path, nil
}

func (i *Index) removePaths(targetID string, paths []string) {
	for _, path := range paths {
		delete(i.byPath[path], targetID)
		if len(i.byPath[path]) == 0 {
			delete(i.byPath, path)
		}
	}
}

func decodeJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
