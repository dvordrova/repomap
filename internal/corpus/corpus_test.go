package corpus

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/gitfiles"
)

func TestNewIsPermutationStableAndBuildsBothIndexes(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCorpusFile(t, repo, "z.go", "package z\n", 0o600)
	writeCorpusFile(t, repo, "a.go", "package a\n", 0o600)
	writeCorpusFile(t, repo, "bin/tool", "#!/bin/sh\n", 0o700)

	left := newTestCorpus(t, repo, gitfiles.Listing{
		Paths:           []string{"z.go", "vendor/submodule", "bin/tool", "a.go"},
		RegularPaths:    []string{"z.go", "bin/tool", "a.go"},
		ExecutablePaths: []string{"bin/tool"},
		Gitlinks: []gitfiles.Gitlink{{
			Path: "vendor/submodule", ObjectID: strings.Repeat("a", 40),
		}},
	})
	right := newTestCorpus(t, repo, gitfiles.Listing{
		Paths:           []string{"a.go", "z.go", "bin/tool", "vendor/submodule"},
		RegularPaths:    []string{"a.go", "z.go", "bin/tool"},
		ExecutablePaths: []string{"bin/tool"},
		Gitlinks: []gitfiles.Gitlink{{
			Path: "vendor/submodule", ObjectID: strings.Repeat("a", 40),
		}},
	})

	wantEntries := []Entry{
		{ID: "f1", Path: "a.go"},
		{ID: "f2", Path: "bin/tool", Executable: true},
		{ID: "f3", Path: "z.go"},
	}
	if got := left.Entries(); !reflect.DeepEqual(got, wantEntries) {
		t.Fatalf("Entries() = %#v, want %#v", got, wantEntries)
	}
	wantGitlinks := []Gitlink{{Path: "vendor/submodule", RecordedObjectID: strings.Repeat("a", 40)}}
	if got := left.Gitlinks(); !reflect.DeepEqual(got, wantGitlinks) {
		t.Fatalf("Gitlinks() = %#v, want %#v", got, wantGitlinks)
	}
	if left.Ref() != right.Ref() || left.SHA256() != right.SHA256() {
		t.Fatalf("permutation changed seal: %q/%q != %q/%q", left.Ref(), left.SHA256(), right.Ref(), right.SHA256())
	}
	leftJSON, err := left.Snapshot().CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	rightJSON, err := right.Snapshot().CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leftJSON, rightJSON) {
		t.Fatalf("permutation changed canonical JSON:\n%s\n%s", leftJSON, rightJSON)
	}

	for _, want := range wantEntries {
		id, ok := left.ID(want.Path)
		if !ok || id != want.ID {
			t.Fatalf("ID(%q) = %q, %t; want %q", want.Path, id, ok, want.ID)
		}
		info, ok := left.Info(want.ID)
		if !ok || info.Entry != want {
			t.Fatalf("Info(%q) = %#v, %t", want.ID, info, ok)
		}
	}
	for _, invalid := range []string{"./a.go", "../a.go", "/a.go", `dir\a.go`} {
		if id, ok := left.ID(invalid); ok {
			t.Fatalf("ID(%q) = %q, true", invalid, id)
		}
	}
	if _, ok := left.Info("f4"); ok {
		t.Fatal("Info(f4) resolved an unknown ID")
	}

	entries := left.Entries()
	entries[0].Path = "changed.go"
	snapshot := left.Snapshot()
	snapshot.Entries[0].Path = "changed.go"
	if got := left.Entries(); !reflect.DeepEqual(got, wantEntries) {
		t.Fatalf("caller mutation changed corpus: %#v", got)
	}
}

func TestNewSupportsAnEmptyTrackedRegularCorpus(t *testing.T) {
	t.Parallel()

	corpus := newTestCorpus(t, t.TempDir(), gitfiles.Listing{})
	if len(corpus.Entries()) != 0 {
		t.Fatalf("empty corpus has entries: %#v", corpus.Entries())
	}
	snapshot := corpus.Snapshot()
	if snapshot.Entries == nil {
		t.Fatal("empty snapshot lost its canonical empty entry array")
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	wire, err := snapshot.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire, []byte(`"entries":[]`)) {
		t.Fatalf("empty canonical JSON = %s", wire)
	}
}

func TestCorpusExcludesCredentialConfigurationDependenciesAndGeneratedOutputs(t *testing.T) {
	repo := t.TempDir()
	allowed := []string{
		"client/src/App.tsx", "package.json", "shared/schema.ts", "tsconfig.json",
	}
	for _, filePath := range append(append([]string(nil), allowed...),
		".npmrc", ".env", ".env.local", "client/.env.production",
		"node_modules/typescript/lib/typescript.js", "dist/index.js", "client/build/app.js",
		"coverage/report.json", "cache.tsbuildinfo", "client/cache.tsbuildinfo",
	) {
		writeCorpusFile(t, repo, filePath, "content", 0o600)
	}
	listing := gitfiles.Listing{RegularPaths: []string{
		".env", ".env.local", ".npmrc", "cache.tsbuildinfo", "client/.env.production",
		"client/build/app.js", "client/cache.tsbuildinfo", "client/src/App.tsx",
		"coverage/report.json", "dist/index.js", "node_modules/typescript/lib/typescript.js",
		"package.json", "shared/schema.ts", "tsconfig.json",
	}}
	listing.Paths = append([]string(nil), listing.RegularPaths...)
	value := newTestCorpus(t, repo, listing)
	entries := value.Entries()
	got := make([]string, len(entries))
	for index, entry := range entries {
		got[index] = entry.Path
	}
	if !reflect.DeepEqual(got, allowed) {
		t.Fatalf("filtered corpus paths = %#v, want %#v", got, allowed)
	}
	if !reflect.DeepEqual(value.VisiblePaths(), allowed) {
		t.Fatalf("filtered visible paths = %#v, want %#v", value.VisiblePaths(), allowed)
	}
	for _, forbidden := range listing.RegularPaths {
		if slices.Contains(allowed, forbidden) {
			continue
		}
		if _, ok := value.ID(forbidden); ok {
			t.Fatalf("forbidden path %q received a FileID", forbidden)
		}
	}
}

func TestForbiddenPathIsComponentExact(t *testing.T) {
	tests := map[string]bool{
		".npmrc": true, "ui/.npmrc": true, ".env": true, ".env.test": true,
		"src/.environment.ts": true, "node_modules/pkg/index.js": true,
		"src/node_modules_helper.ts": false, "dist/app.js": true, "src/dist/app.js": true,
		"build/app.js": true, "coverage/report.json": true, "app.tsbuildinfo": true,
		"src/build.ts": false, "src/coverage.ts": false,
	}
	for filePath, want := range tests {
		if got := ForbiddenPath(filePath); got != want {
			t.Errorf("ForbiddenPath(%q) = %v, want %v", filePath, got, want)
		}
	}
}

func TestExecutableModeChangesCorpusIdentityButContentDoesNot(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCorpusFile(t, repo, "tool", "one", 0o700)
	plain := newTestCorpus(t, repo, gitfiles.Listing{
		Paths:        []string{"tool"},
		RegularPaths: []string{"tool"},
	})
	executable := newTestCorpus(t, repo, gitfiles.Listing{
		Paths:           []string{"tool"},
		RegularPaths:    []string{"tool"},
		ExecutablePaths: []string{"tool"},
	})
	if plain.Ref() == executable.Ref() || plain.SHA256() == executable.SHA256() {
		t.Fatal("executable mode did not change corpus identity")
	}

	beforeRef, beforeSHA := plain.Ref(), plain.SHA256()
	writeCorpusFile(t, repo, "tool", strings.Repeat("changed", 20), 0o700)
	changedSize := newTestCorpus(t, repo, gitfiles.Listing{
		Paths:        []string{"tool"},
		RegularPaths: []string{"tool"},
	})
	if changedSize.Ref() != beforeRef || changedSize.SHA256() != beforeSHA {
		t.Fatal("working-tree content or size changed sealed path/mode identity")
	}
}

func TestSnapshotRejectsTamper(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCorpusFile(t, repo, "a.go", "a", 0o600)
	writeCorpusFile(t, repo, "b.go", "b", 0o600)
	corpus := newTestCorpus(t, repo, gitfiles.Listing{
		Paths:        []string{"b.go", "a.go"},
		RegularPaths: []string{"b.go", "a.go"},
	})
	valid := corpus.Snapshot()

	tests := []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{name: "path", mutate: func(value *Snapshot) { value.Entries[0].Path = "changed.go" }},
		{name: "executable", mutate: func(value *Snapshot) { value.Entries[0].Executable = true }},
		{name: "file ID", mutate: func(value *Snapshot) { value.Entries[0].ID = "f2" }},
		{name: "order", mutate: func(value *Snapshot) { value.Entries[0], value.Entries[1] = value.Entries[1], value.Entries[0] }},
		{name: "ref", mutate: func(value *Snapshot) { value.Ref = refPrefix + strings.Repeat("0", 24) }},
		{name: "hash", mutate: func(value *Snapshot) { value.SHA256 = strings.Repeat("0", 64) }},
		{name: "version", mutate: func(value *Snapshot) { value.Version++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			tampered := cloneSnapshot(valid)
			test.mutate(&tampered)
			if err := tampered.Validate(); err == nil {
				t.Fatal("Validate() accepted tampered snapshot")
			}
		})
	}

}

func TestNewRejectsInvalidListingAndWorkingTreeSymlink(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCorpusFile(t, repo, "real.go", "real", 0o600)
	if err := os.Symlink("real.go", filepath.Join(repo, "alias.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if corpus, err := New(context.Background(), repo, gitfiles.Listing{
		Paths:        []string{"real.go", "alias.go"},
		RegularPaths: []string{"real.go"},
	}); err != nil {
		t.Fatalf("tracked non-regular path should be ignored: %v", err)
	} else {
		if len(corpus.Entries()) != 1 {
			t.Fatalf("entry count = %d, want 1", len(corpus.Entries()))
		}
		if got := corpus.VisiblePaths(); !reflect.DeepEqual(got, []string{"alias.go", "real.go"}) {
			t.Fatalf("VisiblePaths() = %#v", got)
		}
		_ = corpus.Close()
	}

	symlinkCorpus, err := New(context.Background(), repo, gitfiles.Listing{
		Paths:        []string{"alias.go"},
		RegularPaths: []string{"alias.go"},
	})
	if err != nil {
		t.Fatalf("index inventory should not inspect working-tree files: %v", err)
	}
	defer symlinkCorpus.Close()
	if _, err := symlinkCorpus.ReadFile("f1", 32); err == nil || !strings.Contains(err.Error(), "symbolic links") {
		t.Fatalf("lazy symlink read error = %v", err)
	}

	tests := []gitfiles.Listing{
		{RegularPaths: []string{"../real.go"}},
		{RegularPaths: []string{"real.go", "real.go"}},
		{RegularPaths: []string{"real.go"}, ExecutablePaths: []string{"other"}},
		{Paths: []string{"other"}, RegularPaths: []string{"real.go"}},
		{Gitlinks: []gitfiles.Gitlink{{Path: "sub", ObjectID: "invalid"}}},
		{Gitlinks: []gitfiles.Gitlink{
			{Path: "sub", ObjectID: strings.Repeat("a", 40)},
			{Path: "sub", ObjectID: strings.Repeat("a", 40)},
		}},
		{Paths: []string{"real.go"}, Gitlinks: []gitfiles.Gitlink{{
			Path: "sub", ObjectID: strings.Repeat("a", 40),
		}}},
	}
	for index, listing := range tests {
		if _, err := New(context.Background(), repo, listing); err == nil {
			t.Fatalf("invalid listing %d was accepted", index)
		}
	}
}

func TestReadFileIsBoundedCurrentAndRejectsSymlinkReplacement(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCorpusFile(t, repo, "source.py", "abcdef", 0o600)
	corpus := newTestCorpus(t, repo, gitfiles.Listing{
		Paths:        []string{"source.py"},
		RegularPaths: []string{"source.py"},
	})
	identityRef := corpus.Ref()

	content, err := corpus.ReadFile("f1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(content.Bytes) != "abc" || !content.Truncated || content.Entry.Path != "source.py" {
		t.Fatalf("bounded content = %#v", content)
	}
	if cap(content.Bytes) != len(content.Bytes) {
		t.Fatalf("content capacity = %d, want %d", cap(content.Bytes), len(content.Bytes))
	}

	writeCorpusFile(t, repo, "source.py", "new-current-bytes", 0o600)
	content, err = corpus.ReadFile("f1", MaxReadBytes)
	if err != nil {
		t.Fatal(err)
	}
	if string(content.Bytes) != "new-current-bytes" || content.Truncated {
		t.Fatalf("current content = %#v", content)
	}
	if corpus.Ref() != identityRef {
		t.Fatal("ordinary repository change mutated corpus identity")
	}

	if _, err := corpus.ReadFile("f2", 1); err == nil || !strings.Contains(err.Error(), "unknown file ID") {
		t.Fatalf("unknown ID error = %v", err)
	}
	if _, err := corpus.ReadFile("f01", 1); err == nil || !strings.Contains(err.Error(), "unknown file ID") {
		t.Fatalf("non-canonical ID error = %v", err)
	}
	content, err = corpus.ReadFile("f1", MaxReadBytes+1)
	if err != nil || string(content.Bytes) != "new-current-bytes" || content.Truncated {
		t.Fatalf("read above former advisory threshold = %#v, %v", content, err)
	}
	content, err = corpus.ReadFileAll("f1")
	if err != nil || string(content.Bytes) != "new-current-bytes" || content.Truncated {
		t.Fatalf("complete read = %#v, %v", content, err)
	}

	if err := os.Remove(filepath.Join(repo, "source.py")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other.py", filepath.Join(repo, "source.py")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeCorpusFile(t, repo, "other.py", "outside identity", 0o600)
	if _, err := corpus.ReadFile("f1", 32); err == nil || !strings.Contains(err.Error(), "symbolic links") {
		t.Fatalf("symlink replacement error = %v", err)
	}
}

func TestCorpusScaleWarningsAreDiagnosticOnly(t *testing.T) {
	warnings := corpusScaleWarnings(
		MaxFiles+1,
		MaxVisiblePaths+1,
		MaxFiles+2,
		MaxSnapshotBytes+1,
		2,
		MaxReadBytes+1,
	)
	if len(warnings) != 5 {
		t.Fatalf("warnings = %#v", warnings)
	}
	for _, warning := range warnings {
		if warning.Retained <= warning.AdvisorySize || warning.MaximumRetained <= warning.AdvisorySize {
			t.Fatalf("invalid warning = %#v", warning)
		}
	}
	if warnings[4].Kind != ScaleWarningReadBytes || warnings[4].AffectedCollections != 2 {
		t.Fatalf("complete-read warning = %#v", warnings[4])
	}
}

func TestConcurrentReadsAndClose(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCorpusFile(t, repo, "source.go", strings.Repeat("x", 1024), 0o600)
	corpus := newTestCorpus(t, repo, gitfiles.Listing{
		Paths:        []string{"source.go"},
		RegularPaths: []string{"source.go"},
	})

	var wait sync.WaitGroup
	errorsByRead := make(chan error, 16)
	for index := 0; index < 16; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			content, err := corpus.ReadFile("f1", 64)
			if err == nil && (len(content.Bytes) != 64 || !content.Truncated) {
				err = errors.New("unexpected bounded content")
			}
			errorsByRead <- err
		}()
	}
	wait.Wait()
	close(errorsByRead)
	for err := range errorsByRead {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := corpus.Close(); err != nil {
		t.Fatal(err)
	}
	if err := corpus.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := corpus.ReadFile("f1", 1); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("read after Close error = %v", err)
	}
}

func TestOpenPropagatesCancellationAndListingFailure(t *testing.T) {
	t.Parallel()

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Open(canceled, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Open canceled error = %v", err)
	}
	if _, err := Open(context.Background(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "tracked files") {
		t.Fatalf("Open non-repository error = %v", err)
	}
	if _, err := New(canceled, t.TempDir(), gitfiles.Listing{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("New canceled error = %v", err)
	}
}

func TestOpenUsesOnlyStageZeroRegularIndexModes(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	runCorpusGit(t, repo, "init", "--quiet")
	writeCorpusFile(t, repo, "main.go", "package main\n", 0o600)
	writeCorpusFile(t, repo, "tool", "#!/bin/sh\n", 0o700)
	if err := os.Symlink("main.go", filepath.Join(repo, "linked.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	runCorpusGit(t, repo, "add", "main.go", "tool", "linked.go")
	runCorpusGit(t, repo, "update-index", "--chmod=+x", "tool")

	corpus, err := Open(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = corpus.Close() })
	if got := corpus.Entries(); !reflect.DeepEqual(got, []Entry{
		{ID: "f1", Path: "main.go"},
		{ID: "f2", Path: "tool", Executable: true},
	}) {
		t.Fatalf("Open entries = %#v", got)
	}
	if _, ok := corpus.ID("linked.go"); ok {
		t.Fatal("tracked symlink entered the regular corpus")
	}
}

func newTestCorpus(t *testing.T, repo string, listing gitfiles.Listing) *Corpus {
	t.Helper()
	value, err := New(context.Background(), repo, listing)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := value.Close(); err != nil {
			t.Error(err)
		}
	})
	return value
}

func writeCorpusFile(t *testing.T, repo, name, content string, mode os.FileMode) {
	t.Helper()
	filePath := filepath.Join(repo, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func runCorpusGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repo}, args...)
	command := exec.Command("git", commandArgs...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}
