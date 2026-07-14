package index_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseektest"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/freshness"
	localindex "github.com/dvordrova/repomap/internal/index"
	"github.com/dvordrova/repomap/internal/symbol"
)

const fixtureTargetID = "method:server/key.go:90:20:kvServer.Put"

func TestPutAndNeighborhood(t *testing.T) {
	t.Parallel()

	store := newIndex(t, fixtureMetadata())
	bundle := fixtureBundle(t)
	if err := store.Put(bundle); err != nil {
		t.Fatal(err)
	}

	got, err := store.Neighborhood(fixtureTargetID)
	if err != nil {
		t.Fatal(err)
	}
	got.Query = "mutated"
	got.AllowedPaths[0] = "mutated.go"

	again, err := store.Neighborhood(fixtureTargetID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Query != bundle.Query || !reflect.DeepEqual(again.AllowedPaths, bundle.AllowedPaths) {
		t.Fatalf("stored bundle was mutated: %#v", again)
	}
	if store.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", store.Len())
	}
}

func TestLoadReusesSnapshotWhenFactContextIsUnchanged(t *testing.T) {
	t.Parallel()

	metadata := fixtureMetadata()
	store := newIndex(t, metadata)
	bundle := fixtureBundle(t)
	if err := store.Put(bundle); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), ".repomap", "symbol-index.json")
	if err := store.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := localindex.Load(path, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Metadata(), metadata) {
		t.Fatalf("Metadata() = %#v, want %#v", loaded.Metadata(), metadata)
	}
	if !reflect.DeepEqual(loaded.Targets(), []string{fixtureTargetID}) {
		t.Fatalf("Targets() = %v", loaded.Targets())
	}
	got, err := loaded.Neighborhood(fixtureTargetID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, bundle) {
		t.Fatalf("reloaded bundle differs\ngot:  %#v\nwant: %#v", got, bundle)
	}
}

func TestLoadRejectsSameHeadWithDifferentDirtyContent(t *testing.T) {
	t.Parallel()

	saved := fixtureMetadata()
	saved.Facts.Repository.Dirty = []freshness.DirtyFile{fixtureDirtyFile(strings.Repeat("b", 64))}
	current := fixtureMetadata()
	current.Facts.Repository.Dirty = []freshness.DirtyFile{fixtureDirtyFile(strings.Repeat("c", 64))}
	if saved.Facts.Repository.Head != current.Facts.Repository.Head {
		t.Fatal("fixture HEADs differ; test would not isolate dirty content")
	}

	path := saveFixtureIndex(t, saved)
	_, err := localindex.Load(path, current)
	assertStaleReason(t, err, freshness.Reason("repository_dirty"))
}

func TestLoadReportsAnalyzerBuildAndCollectorMismatchReasons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		reason freshness.Reason
		mutate func(*localindex.Metadata)
	}{
		{
			name:   "analyzer version",
			reason: freshness.Reason("analyzer_version"),
			mutate: func(metadata *localindex.Metadata) {
				metadata.Facts.AnalyzerVersion = "v0.24.0"
			},
		},
		{
			name:   "build context",
			reason: freshness.Reason("build_context"),
			mutate: func(metadata *localindex.Metadata) {
				metadata.Facts.Build.GOARCH = "arm64"
			},
		},
		{
			name:   "collector version",
			reason: freshness.Reason("collector_version"),
			mutate: func(metadata *localindex.Metadata) {
				metadata.Facts.CollectorVersion = "v2"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			saved := fixtureMetadata()
			current := fixtureMetadata()
			test.mutate(&current)
			path := saveFixtureIndex(t, saved)

			_, err := localindex.Load(path, current)
			assertStaleReason(t, err, test.reason)
		})
	}
}

func TestMetadataReturnsDefensiveCopies(t *testing.T) {
	t.Parallel()

	metadata := fixtureMetadata()
	metadata.Facts.Build.BuildTags = []string{"integration"}
	store := newIndex(t, metadata)
	metadata.Facts.Build.BuildTags[0] = "caller-mutated"

	got := store.Metadata()
	if !reflect.DeepEqual(got.Facts.Build.BuildTags, []string{"integration"}) {
		t.Fatalf("Metadata().Facts.Build.BuildTags = %v", got.Facts.Build.BuildTags)
	}
	got.Facts.Build.BuildTags[0] = "result-mutated"
	again := store.Metadata()
	if !reflect.DeepEqual(again.Facts.Build.BuildTags, []string{"integration"}) {
		t.Fatalf("stored metadata was mutated: %v", again.Facts.Build.BuildTags)
	}
}

func TestInvalidatePath(t *testing.T) {
	t.Parallel()

	store := newIndex(t, fixtureMetadata())
	bundle := fixtureBundle(t)
	if err := store.Put(bundle); err != nil {
		t.Fatal(err)
	}
	other := minimalBundle("function:server/other.go:10:other", "server/other.go")
	if err := store.Put(other); err != nil {
		t.Fatal(err)
	}

	removed := store.InvalidatePath("server/key.go")
	if !reflect.DeepEqual(removed, []string{fixtureTargetID}) {
		t.Fatalf("InvalidatePath() = %v", removed)
	}
	if _, err := store.Neighborhood(fixtureTargetID); !errors.Is(err, localindex.ErrNotFound) {
		t.Fatalf("Neighborhood(invalidated) error = %v", err)
	}
	if _, err := store.Neighborhood(other.Target.Entity.ID); err != nil {
		t.Fatalf("Neighborhood(unrelated) error = %v", err)
	}
}

func TestPutReplacementUpdatesPathIndex(t *testing.T) {
	t.Parallel()

	store := newIndex(t, fixtureMetadata())
	if err := store.Put(fixtureBundle(t)); err != nil {
		t.Fatal(err)
	}
	replacement := minimalBundle(fixtureTargetID, "server/replacement.go")
	if err := store.Put(replacement); err != nil {
		t.Fatal(err)
	}

	if removed := store.InvalidatePath("server/key.go"); len(removed) != 0 {
		t.Fatalf("InvalidatePath(old path) = %v, want none", removed)
	}
	if _, err := store.Neighborhood(fixtureTargetID); err != nil {
		t.Fatalf("Neighborhood(replacement) error = %v", err)
	}
	if removed := store.InvalidatePath("server/replacement.go"); !reflect.DeepEqual(removed, []string{fixtureTargetID}) {
		t.Fatalf("InvalidatePath(replacement path) = %v", removed)
	}
}

func TestPutRejectsUnsafePath(t *testing.T) {
	t.Parallel()

	store := newIndex(t, fixtureMetadata())
	bundle := minimalBundle("function:outside", "../outside.go")
	if err := store.Put(bundle); err == nil {
		t.Fatal("Put() error = nil, want unsafe path error")
	}
}

func TestLoadRejectsUnknownVersion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "index.json")
	if err := os.WriteFile(path, []byte(`{"version":999,"metadata":{},"symbols":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := localindex.Load(path, fixtureMetadata()); err == nil {
		t.Fatal("Load() error = nil, want version error")
	}
}

func TestNewAndLoadRejectMalformedMetadata(t *testing.T) {
	t.Parallel()

	invalid := localindex.Metadata{}
	if _, err := localindex.New(invalid); err == nil {
		t.Fatal("New() error = nil, want metadata error")
	}
	validPath := saveFixtureIndex(t, fixtureMetadata())
	if _, err := localindex.Load(validPath, invalid); err == nil {
		t.Fatal("Load() error = nil, want current metadata error")
	}

	path := filepath.Join(t.TempDir(), "index.json")
	data := []byte(`{"version":2,"metadata":{"facts":{}},"symbols":[]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := localindex.Load(path, fixtureMetadata()); err == nil {
		t.Fatal("Load() error = nil, want saved metadata error")
	}
}

func TestConcurrentSavesLeaveOneCompleteSnapshot(t *testing.T) {
	t.Parallel()

	metadata := fixtureMetadata()
	path := filepath.Join(t.TempDir(), "index.json")
	stores := []*localindex.Index{
		newIndex(t, metadata),
		newIndex(t, metadata),
	}
	if err := stores[0].Put(fixtureBundle(t)); err != nil {
		t.Fatal(err)
	}
	if err := stores[1].Put(minimalBundle("function:other.go:10:other", "other.go")); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errorsByStore := make([]error, len(stores))
	for index, store := range stores {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsByStore[index] = store.Save(path)
		}()
	}
	wait.Wait()
	for _, err := range errorsByStore {
		if err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := localindex.Load(path, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Len() != 1 {
		t.Fatalf("loaded Len() = %d", loaded.Len())
	}
}

func fixtureBundle(t *testing.T) symbol.Bundle {
	t.Helper()
	var bundle symbol.Bundle
	if err := json.Unmarshal(deepseektest.SymbolBundleJSON, &bundle); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func minimalBundle(targetID, path string) symbol.Bundle {
	return symbol.Bundle{
		Version:  symbol.BundleVersion,
		RepoName: "fixture-repo",
		Query:    targetID,
		Target: symbol.Fact{Entity: evidence.Entity{
			ID:       targetID,
			Kind:     evidence.EntityFunction,
			Name:     targetID,
			Language: "go",
			Location: &evidence.Location{Path: path, Line: 10},
		}},
		AllowedPaths: []string{path},
	}
}

func fixtureMetadata() localindex.Metadata {
	return localindex.Metadata{Facts: freshness.FactContext{
		Version: freshness.FactContextVersion,
		Repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: "/repo",
			Head:     strings.Repeat("a", 40),
			Dirty:    []freshness.DirtyFile{},
		},
		GoVersion:        "go1.24.0",
		Analyzer:         "gopls",
		AnalyzerVersion:  "v0.23.0",
		Collector:        "symbol-neighborhood",
		CollectorVersion: "v1",
		InputsSHA256:     strings.Repeat("d", 64),
		Build: evidence.BuildContext{
			GOOS:   "linux",
			GOARCH: "amd64",
		},
	}}
}

func newIndex(t *testing.T, metadata localindex.Metadata) *localindex.Index {
	t.Helper()
	store, err := localindex.New(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func saveFixtureIndex(t *testing.T, metadata localindex.Metadata) string {
	t.Helper()
	store := newIndex(t, metadata)
	if err := store.Put(fixtureBundle(t)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "index.json")
	if err := store.Save(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func fixtureDirtyFile(contentSHA256 string) freshness.DirtyFile {
	return freshness.DirtyFile{
		Status:        "modified",
		Path:          "server/key.go",
		Kind:          "file",
		ContentSHA256: contentSHA256,
	}
}

func assertStaleReason(t *testing.T, err error, reason freshness.Reason) {
	t.Helper()
	if !errors.Is(err, localindex.ErrStale) {
		t.Fatalf("Load() error = %v, want ErrStale", err)
	}
	var stale *localindex.StaleError
	if !errors.As(err, &stale) {
		t.Fatalf("Load() error type = %T, want *index.StaleError", err)
	}
	for _, difference := range stale.Differences {
		if difference.Reason == reason {
			return
		}
	}
	t.Fatalf("stale differences = %v, want reason %q", stale.Differences, reason)
}
