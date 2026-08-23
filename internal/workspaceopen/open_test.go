package workspaceopen

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

type ErrorKind = errorKind

const (
	ErrorInvalidRequest    = errorInvalidRequest
	ErrorUnauthorized      = errorUnauthorized
	ErrorRootUnavailable   = errorRootUnavailable
	ErrorTargetUnavailable = errorTargetUnavailable
	ErrorCanceled          = errorCanceled
)

func ErrorKindOf(err error) ErrorKind {
	var target *openError
	if errors.As(err, &target) {
		return target.kind
	}
	return ""
}

func TestResolveAuthorizedCurrentTarget(t *testing.T) {
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
	if target.AbsolutePath != wantPath {
		t.Fatalf("unchanged target = %#v", target)
	}

	writeFile(t, filepath.Join(root, filepath.FromSlash(path)), []byte("package changed\n"))
	changed, err := service.Resolve(context.Background(), Request{Path: path})
	if err != nil {
		t.Fatalf("Resolve changed: %v", err)
	}
	if changed.AbsolutePath != wantPath {
		t.Fatalf("changed target = %#v", changed)
	}
	if target.AbsolutePath != wantPath {
		t.Fatalf("first target mutated = %#v", target)
	}
}

func TestResolveUsesExactAuthorizedPath(t *testing.T) {
	t.Parallel()

	repository := canonicalTempDir(t)
	analysisRoot := filepath.Join(repository, "service")
	if err := os.Mkdir(analysisRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	const authorizedPath = " main.go "
	exact := filepath.Join(analysisRoot, authorizedPath)
	trimmedAlias := filepath.Join(analysisRoot, "main.go")
	content := []byte("package exact\n")
	writeFile(t, exact, content)
	writeFile(t, trimmedAlias, []byte("package alias\n"))
	service := testService(t, repository, analysisRoot, authorizedPath, content)

	target, err := service.Resolve(context.Background(), Request{Path: authorizedPath})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if target.AbsolutePath != exact {
		t.Fatalf("target = %#v, want exact authorized path %q", target, exact)
	}
	if target.AbsolutePath == trimmedAlias {
		t.Fatalf("exact authorized path resolved to trimmed alias %q", trimmedAlias)
	}
}

func TestResolveDoesNotOpenTrimmedAliasForUnavailableAuthorizedPath(t *testing.T) {
	t.Parallel()

	root := canonicalTempDir(t)
	const authorizedPath = " missing.go "
	trimmedAlias := filepath.Join(root, "missing.go")
	content := []byte("package alias\n")
	writeFile(t, trimmedAlias, content)
	service := testService(t, root, root, authorizedPath, content)

	target, err := service.Resolve(context.Background(), Request{Path: authorizedPath})
	if ErrorKindOf(err) != ErrorTargetUnavailable || target != (Target{}) {
		t.Fatalf("Resolve = %#v, %v kind=%q", target, err, ErrorKindOf(err))
	}
	assertPrivateError(t, err, root, authorizedPath, trimmedAlias)
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
		if target.AbsolutePath != want {
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

func TestResolveCancellationIsTyped(t *testing.T) {
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
