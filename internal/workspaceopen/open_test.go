package workspaceopen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

func TestResolveAuthorizedTargetAndChangeFlag(t *testing.T) {
	t.Parallel()

	root := canonicalTempDir(t)
	path := "cmd/main.go"
	original := []byte("package main\n")
	writeFile(t, filepath.Join(root, filepath.FromSlash(path)), original)
	service := testService(t, root, root, path, original)

	target, err := service.Resolve(context.Background(), Request{Path: path})
	if err != nil {
		t.Fatalf("Resolve unchanged: %v", err)
	}
	wantPath, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	if target.Path != path || target.AbsolutePath != wantPath || target.SourceChanged {
		t.Fatalf("unchanged target = %#v", target)
	}

	writeFile(t, filepath.Join(root, filepath.FromSlash(path)), []byte("package changed\n"))
	changed, err := service.Resolve(context.Background(), Request{
		Path: path, MaxHashBytes: MaxHashBytes,
	})
	if err != nil {
		t.Fatalf("Resolve changed: %v", err)
	}
	if changed.Path != path || changed.AbsolutePath != wantPath || !changed.SourceChanged {
		t.Fatalf("changed target = %#v", changed)
	}
	if target.Path != path || target.AbsolutePath != wantPath || target.SourceChanged {
		t.Fatalf("first target mutated = %#v", target)
	}
}

func TestResolvePreservesSubdirectoryAndAuthorizedWhitespaceSemantics(t *testing.T) {
	t.Parallel()

	repository := canonicalTempDir(t)
	analysisRoot := filepath.Join(repository, "service")
	if err := os.Mkdir(analysisRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	const authorizedPath = " main.go "
	untrimmed := filepath.Join(analysisRoot, authorizedPath)
	trimmed := filepath.Join(analysisRoot, "main.go")
	content := []byte("package service\n")
	writeFile(t, untrimmed, []byte("package captured_name\n"))
	writeFile(t, trimmed, content)
	service := testService(t, repository, analysisRoot, authorizedPath, content)

	target, err := service.Resolve(context.Background(), Request{Path: authorizedPath})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if target.Path != authorizedPath || target.AbsolutePath != trimmed || target.SourceChanged {
		t.Fatalf("target = %#v, want authorized path and trimmed local target", target)
	}
}

func TestResolveRejectsInvalidUnauthorizedAndUnavailableTargets(t *testing.T) {
	t.Parallel()

	root := canonicalTempDir(t)
	content := []byte("package safe\n")
	writeFile(t, filepath.Join(root, "safe.go"), content)
	service := testService(t, root, root, "safe.go", content)

	invalid := []string{
		"",
		".",
		"/absolute.go",
		"../escape.go",
		"nested/../escape.go",
		`nested\file.go`,
		"bad\nname.go",
		string([]byte{0xff, 'x'}),
		strings.Repeat("x", maxPathBytes+1),
	}
	for _, requestPath := range invalid {
		_, err := service.Resolve(context.Background(), Request{Path: requestPath})
		if ErrorKindOf(err) != ErrorInvalidRequest {
			t.Fatalf("Resolve(%q) error = %v kind=%q", requestPath, err, ErrorKindOf(err))
		}
		assertPrivateError(t, err, root, requestPath)
	}
	for _, limit := range []int64{-1, MaxHashBytes + 1} {
		_, err := service.Resolve(context.Background(), Request{Path: "safe.go", MaxHashBytes: limit})
		if ErrorKindOf(err) != ErrorInvalidRequest {
			t.Fatalf("MaxHashBytes %d error = %v kind=%q", limit, err, ErrorKindOf(err))
		}
	}
	_, err := service.Resolve(context.Background(), Request{Path: "other.go"})
	if ErrorKindOf(err) != ErrorUnauthorized {
		t.Fatalf("unauthorized error = %v kind=%q", err, ErrorKindOf(err))
	}

	t.Run("missing", func(t *testing.T) {
		root := canonicalTempDir(t)
		writeFile(t, filepath.Join(root, "target.go"), content)
		service := testService(t, root, root, "target.go", content)
		if err := os.Remove(filepath.Join(root, "target.go")); err != nil {
			t.Fatal(err)
		}
		_, err := service.Resolve(context.Background(), Request{Path: "target.go"})
		if ErrorKindOf(err) != ErrorTargetUnavailable {
			t.Fatalf("missing error = %v kind=%q", err, ErrorKindOf(err))
		}
	})

	t.Run("directory", func(t *testing.T) {
		root := canonicalTempDir(t)
		writeFile(t, filepath.Join(root, "target.go"), content)
		service := testService(t, root, root, "target.go", content)
		if err := os.Remove(filepath.Join(root, "target.go")); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(root, "target.go"), 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := service.Resolve(context.Background(), Request{Path: "target.go"})
		if ErrorKindOf(err) != ErrorTargetUnavailable {
			t.Fatalf("directory error = %v kind=%q", err, ErrorKindOf(err))
		}
	})
}

func TestResolvePreservesMappedSymlinkSemantics(t *testing.T) {
	t.Parallel()

	t.Run("final symlink rejected", func(t *testing.T) {
		root := canonicalTempDir(t)
		content := []byte("package target\n")
		writeFile(t, filepath.Join(root, "target.go"), content)
		writeFile(t, filepath.Join(root, "other.go"), content)
		service := testService(t, root, root, "target.go", content)
		if err := os.Remove(filepath.Join(root, "target.go")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("other.go", filepath.Join(root, "target.go")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		_, err := service.Resolve(context.Background(), Request{Path: "target.go"})
		if ErrorKindOf(err) != ErrorTargetUnavailable {
			t.Fatalf("final symlink error = %v kind=%q", err, ErrorKindOf(err))
		}
	})

	t.Run("contained intermediate symlink accepted", func(t *testing.T) {
		root := canonicalTempDir(t)
		content := []byte("package target\n")
		writeFile(t, filepath.Join(root, "real", "target.go"), content)
		if err := os.Symlink("real", filepath.Join(root, "alias")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		service := testService(t, root, root, "alias/target.go", content)
		target, err := service.Resolve(context.Background(), Request{Path: "alias/target.go"})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		want := filepath.Join(root, "real", "target.go")
		if target.Path != "alias/target.go" || target.AbsolutePath != want || target.SourceChanged {
			t.Fatalf("target = %#v, want absolute %q", target, want)
		}
	})

	t.Run("escaping intermediate symlink rejected", func(t *testing.T) {
		root := canonicalTempDir(t)
		outside := canonicalTempDir(t)
		content := []byte("package outside\n")
		writeFile(t, filepath.Join(outside, "target.go"), content)
		if err := os.Symlink(outside, filepath.Join(root, "alias")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		service := testService(t, root, root, "alias/target.go", content)
		_, err := service.Resolve(context.Background(), Request{Path: "alias/target.go"})
		if ErrorKindOf(err) != ErrorTargetUnavailable {
			t.Fatalf("escape error = %v kind=%q", err, ErrorKindOf(err))
		}
		assertPrivateError(t, err, root, outside)
	})
}

func TestResolveRejectsReplacedAnalysisRoot(t *testing.T) {
	t.Parallel()

	parent := canonicalTempDir(t)
	root := filepath.Join(parent, "repo")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("package original\n")
	writeFile(t, filepath.Join(root, "target.go"), content)
	service := testService(t, root, root, "target.go", content)

	held := filepath.Join(parent, "held")
	if err := os.Rename(root, held); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(parent, "replacement")
	if err := os.Mkdir(replacement, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(replacement, "target.go"), content)
	if err := os.Symlink("replacement", root); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := service.Resolve(context.Background(), Request{Path: "target.go"})
	if ErrorKindOf(err) != ErrorRootUnavailable {
		t.Fatalf("replaced root error = %v kind=%q", err, ErrorKindOf(err))
	}
	assertPrivateError(t, err, root, replacement, held)
}

func TestResolvePreservesBoundedHashFailureSemantics(t *testing.T) {
	t.Parallel()

	t.Run("oversized current file is not reported changed", func(t *testing.T) {
		root := canonicalTempDir(t)
		path := filepath.Join(root, "large.bin")
		writeFile(t, path, []byte("captured"))
		service := testService(t, root, root, "large.bin", []byte("captured"))
		if err := os.Truncate(path, MaxHashBytes+1); err != nil {
			t.Fatal(err)
		}
		target, err := service.Resolve(context.Background(), Request{Path: "large.bin"})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if target.SourceChanged {
			t.Fatalf("oversized target = %#v", target)
		}
	})

	t.Run("empty hash skips the reader", func(t *testing.T) {
		reader := &countingReader{remaining: 1}
		changed, err := hashChanged(context.Background(), reader, "", MaxHashBytes)
		if err != nil || changed || reader.readBytes != 0 {
			t.Fatalf("hashChanged = %t, %v reads=%d", changed, err, reader.readBytes)
		}
	})

	t.Run("read error is not reported changed", func(t *testing.T) {
		changed, err := hashChanged(
			context.Background(),
			errorReader{},
			strings.Repeat("a", 64),
			MaxHashBytes,
		)
		if err != nil || changed {
			t.Fatalf("hashChanged = %t, %v", changed, err)
		}
	})

	t.Run("read work stops at bound plus one", func(t *testing.T) {
		reader := &countingReader{remaining: MaxHashBytes + 1024}
		changed, err := hashChanged(
			context.Background(),
			reader,
			strings.Repeat("a", 64),
			MaxHashBytes,
		)
		if err != nil || changed || reader.readBytes != MaxHashBytes+1 {
			t.Fatalf("hashChanged = %t, %v reads=%d", changed, err, reader.readBytes)
		}
	})

	t.Run("missing hash source is unverifiable not changed", func(t *testing.T) {
		changed, err := sourceChanged(
			context.Background(),
			filepath.Join(canonicalTempDir(t), "missing.go"),
			strings.Repeat("a", 64),
			MaxHashBytes,
		)
		if err != nil || changed {
			t.Fatalf("sourceChanged = %t, %v", changed, err)
		}
	})
}

func TestResolveCancellationIsTypedAndObservedDuringHash(t *testing.T) {
	t.Parallel()

	root := canonicalTempDir(t)
	content := []byte("package safe\n")
	writeFile(t, filepath.Join(root, "safe.go"), content)
	service := testService(t, root, root, "safe.go", content)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service.Resolve(ctx, Request{Path: "safe.go"})
	if ErrorKindOf(err) != ErrorCanceled {
		t.Fatalf("canceled Resolve error = %v kind=%q", err, ErrorKindOf(err))
	}

	ctx, cancel = context.WithCancel(context.Background())
	reader := &cancelingReader{
		reader: bytes.NewReader(bytes.Repeat([]byte("x"), hashBufferSize*2)),
		cancel: cancel,
	}
	_, err = hashChanged(ctx, reader, strings.Repeat("a", 64), MaxHashBytes)
	if ErrorKindOf(err) != ErrorCanceled || reader.reads != 1 {
		t.Fatalf("hash cancellation error = %v kind=%q reads=%d", err, ErrorKindOf(err), reader.reads)
	}
}

func TestNewAndErrorsDoNotLeakAuthority(t *testing.T) {
	t.Parallel()

	if service, err := New(workspacesnapshot.Snapshot{}); service != nil ||
		ErrorKindOf(err) != ErrorInvalidRequest {
		t.Fatalf("New(zero) = %#v, %v kind=%q", service, err, ErrorKindOf(err))
	}
	var foreign error = errors.New("/Users/example/private.go")
	if ErrorKindOf(foreign) != "" {
		t.Fatalf("foreign error kind = %q", ErrorKindOf(foreign))
	}
}

type countingReader struct {
	remaining int64
	readBytes int64
}

func (reader *countingReader) Read(buffer []byte) (int, error) {
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	read := int64(len(buffer))
	if read > reader.remaining {
		read = reader.remaining
	}
	for index := 0; index < int(read); index++ {
		buffer[index] = 'x'
	}
	reader.remaining -= read
	reader.readBytes += read
	return int(read), nil
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("private read failure")
}

type cancelingReader struct {
	reader io.Reader
	cancel context.CancelFunc
	reads  int
}

func (reader *cancelingReader) Read(buffer []byte) (int, error) {
	read, err := reader.reader.Read(buffer)
	reader.reads++
	reader.cancel()
	return read, err
}

func testService(
	t *testing.T,
	repositoryRoot, analysisRoot, sourcePath string,
	capturedContent []byte,
) *Service {
	t.Helper()
	repositoryPath, err := filepath.Rel(
		repositoryRoot,
		filepath.Join(analysisRoot, filepath.FromSlash(sourcePath)),
	)
	if err != nil {
		t.Fatal(err)
	}
	repositoryPath = filepath.ToSlash(repositoryPath)
	inputID := sha256.Sum256([]byte("workspace-open\x00" + repositoryPath))
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot: analysisRoot,
		Repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: repositoryRoot,
			Head:     strings.Repeat("a", 40),
		},
		CapturedInputs: []freshness.CapturedInput{{
			Version:       freshness.CapturedInputVersion,
			ID:            fmt.Sprintf("%x", inputID),
			Path:          repositoryPath,
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: fmt.Sprintf("%x", sha256.Sum256(capturedContent)),
			Stages:        []string{"report_evidence"},
		}},
		AllowedPaths: []string{sourcePath},
	})
	if err != nil {
		t.Fatalf("workspacesnapshot.New: %v", err)
	}
	service, err := New(snapshot)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return service
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(root)
}

func writeFile(t *testing.T, name string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertPrivateError(t *testing.T, err error, values ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, value := range values {
		if value != "" && strings.Contains(err.Error(), value) {
			t.Fatalf("error %q leaked %q", err, value)
		}
	}
}
