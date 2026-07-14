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

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/symbol"
)

const Version = 2

var (
	ErrNotFound = errors.New("index: target not found")
	ErrStale    = errors.New("index: stale snapshot")
)

type Metadata struct {
	Facts freshness.FactContext `json:"facts"`
}

func (m Metadata) Validate() error {
	if err := m.Facts.Validate(); err != nil {
		return fmt.Errorf("facts: %w", err)
	}
	return nil
}

type StaleError struct {
	Differences []freshness.Difference
}

func (e StaleError) Error() string {
	if len(e.Differences) == 0 {
		return ErrStale.Error()
	}
	differences := make([]string, 0, len(e.Differences))
	for _, difference := range e.Differences {
		differences = append(differences, difference.String())
	}
	return fmt.Sprintf("%s: %s", ErrStale, strings.Join(differences, "; "))
}

func (e StaleError) Unwrap() error {
	return ErrStale
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

func New(metadata Metadata) (*Index, error) {
	if err := metadata.Validate(); err != nil {
		return nil, fmt.Errorf("index: invalid metadata: %w", err)
	}
	return &Index{
		metadata: cloneMetadata(metadata),
		records:  make(map[string]record),
		byPath:   make(map[string]map[string]struct{}),
	}, nil
}

func (i *Index) Metadata() Metadata {
	return cloneMetadata(i.metadata)
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
	return writeAtomic(path, data)
}

func Load(path string, current Metadata) (*Index, error) {
	if err := current.Validate(); err != nil {
		return nil, fmt.Errorf("index: invalid current metadata: %w", err)
	}
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
	if err := persisted.Metadata.Validate(); err != nil {
		return nil, fmt.Errorf("index: invalid saved metadata: %w", err)
	}
	differences := freshness.CompareFactContext(persisted.Metadata.Facts, current.Facts)
	if len(differences) > 0 {
		return nil, &StaleError{Differences: append([]freshness.Difference(nil), differences...)}
	}

	result, err := New(persisted.Metadata)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(persisted.Symbols))
	for position, raw := range persisted.Symbols {
		var bundle symbol.Bundle
		if err := decodeJSON(raw, &bundle); err != nil {
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

func cloneMetadata(metadata Metadata) Metadata {
	if metadata.Facts.Repository.Dirty != nil {
		metadata.Facts.Repository.Dirty = append(
			make([]freshness.DirtyFile, 0, len(metadata.Facts.Repository.Dirty)),
			metadata.Facts.Repository.Dirty...,
		)
	}
	if metadata.Facts.Build.BuildTags != nil {
		metadata.Facts.Build.BuildTags = append(
			make([]string, 0, len(metadata.Facts.Build.BuildTags)),
			metadata.Facts.Build.BuildTags...,
		)
	}
	return metadata
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	temporary, err := os.CreateTemp(dir, ".repomap-index-*")
	if err != nil {
		return fmt.Errorf("index: create temporary snapshot: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("index: protect temporary snapshot: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("index: write snapshot: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("index: sync snapshot: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("index: close snapshot: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("index: replace snapshot: %w", err)
	}
	return nil
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
