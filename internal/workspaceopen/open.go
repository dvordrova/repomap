// Package workspaceopen resolves one snapshot-authorized local file target
// without owning HTTP, report, browser, or editor-launch behavior.
package workspaceopen

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/sourcecatalog"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

const (
	// MaxHashBytes is the largest current file prefix that may be hashed.
	MaxHashBytes int64 = 8 << 20

	maxPathBytes   = 4096
	hashBufferSize = 32 << 10
)

// ErrorKind is the closed failure contract for authorized target resolution.
type ErrorKind string

const (
	ErrorInvalidRequest    ErrorKind = "invalid_request"
	ErrorUnauthorized      ErrorKind = "unauthorized"
	ErrorRootUnavailable   ErrorKind = "root_unavailable"
	ErrorTargetUnavailable ErrorKind = "target_unavailable"
	ErrorCanceled          ErrorKind = "canceled"
)

type openError struct {
	kind ErrorKind
}

func (err *openError) Error() string {
	if err == nil {
		return "workspace open: target_unavailable"
	}
	return "workspace open: " + string(err.kind)
}

func workspaceOpenError(kind ErrorKind) error {
	return &openError{kind: kind}
}

// ErrorKindOf returns the closed kind for an error produced by this package.
func ErrorKindOf(err error) ErrorKind {
	var target *openError
	if errors.As(err, &target) {
		return target.kind
	}
	return ""
}

// Request selects one exact catalog-authorized analysis-relative path. A zero
// MaxHashBytes selects MaxHashBytes; a positive value may only narrow it.
type Request struct {
	Path         string
	MaxHashBytes int64
}

// Target contains no source bytes. AbsolutePath is intended only for a local
// adapter that has separately authorized its presentation-level action.
type Target struct {
	Path          string
	AbsolutePath  string
	SourceChanged bool
}

// Service is bound to one immutable workspace snapshot and catalog.
type Service struct {
	analysisRoot string
	catalog      sourcecatalog.Catalog
}

// New constructs a concrete target resolver without reading the filesystem.
func New(snapshot workspacesnapshot.Snapshot) (*Service, error) {
	analysisRoot := snapshot.AnalysisRoot()
	catalog := snapshot.Catalog()
	if analysisRoot == "" || snapshot.RepositoryRoot() == "" ||
		!filepath.IsAbs(analysisRoot) || filepath.Clean(analysisRoot) != analysisRoot ||
		catalog.AnalysisRoot() != analysisRoot {
		return nil, workspaceOpenError(ErrorInvalidRequest)
	}
	return &Service{analysisRoot: analysisRoot, catalog: catalog}, nil
}

// Resolve verifies one authorized current local file and reports whether its
// bounded current hash is known to differ from the captured catalog hash.
func (service *Service) Resolve(ctx context.Context, request Request) (Target, error) {
	// Path is caller-controlled. Bound it before trimming, Unicode scanning,
	// canonicalization, filesystem calls, or path-derived allocations.
	if len(request.Path) > maxPathBytes {
		return Target{}, workspaceOpenError(ErrorInvalidRequest)
	}
	if service == nil || service.analysisRoot == "" || ctx == nil {
		return Target{}, workspaceOpenError(ErrorInvalidRequest)
	}
	hashLimit, ok := normalizeHashLimit(request.MaxHashBytes)
	if !ok || !validPath(request.Path) {
		return Target{}, workspaceOpenError(ErrorInvalidRequest)
	}
	if err := contextError(ctx); err != nil {
		return Target{}, err
	}
	source, ok := service.catalog.Lookup(request.Path)
	if !ok || source.Path != request.Path {
		return Target{}, workspaceOpenError(ErrorUnauthorized)
	}

	liveRoot, err := filepath.EvalSymlinks(service.analysisRoot)
	if err != nil || liveRoot != service.analysisRoot {
		return Target{}, workspaceOpenError(ErrorRootUnavailable)
	}
	if err := contextError(ctx); err != nil {
		return Target{}, err
	}

	// Resolve the exact path authorized by the catalog.
	localPath := filepath.FromSlash(request.Path)
	if !filepath.IsLocal(localPath) || localPath == "." {
		return Target{}, workspaceOpenError(ErrorTargetUnavailable)
	}
	unresolved := filepath.Join(liveRoot, localPath)
	entryInfo, err := os.Lstat(unresolved)
	if err != nil || entryInfo.Mode()&os.ModeSymlink != 0 {
		return Target{}, workspaceOpenError(ErrorTargetUnavailable)
	}
	absolutePath, err := filepath.EvalSymlinks(unresolved)
	if err != nil {
		return Target{}, workspaceOpenError(ErrorTargetUnavailable)
	}
	relative, err := filepath.Rel(liveRoot, absolutePath)
	if err != nil || !filepath.IsLocal(relative) {
		return Target{}, workspaceOpenError(ErrorTargetUnavailable)
	}
	info, err := os.Stat(absolutePath)
	if err != nil || !info.Mode().IsRegular() {
		return Target{}, workspaceOpenError(ErrorTargetUnavailable)
	}
	if err := contextError(ctx); err != nil {
		return Target{}, err
	}

	changed, err := sourceChanged(ctx, absolutePath, source.ContentSHA256, hashLimit)
	if err != nil {
		return Target{}, err
	}
	return Target{
		Path:          source.Path,
		AbsolutePath:  absolutePath,
		SourceChanged: changed,
	}, nil
}

func normalizeHashLimit(value int64) (int64, bool) {
	if value == 0 {
		return MaxHashBytes, true
	}
	return value, value > 0 && value <= MaxHashBytes
}

func validPath(value string) bool {
	if value == "" || value == "." || !utf8.ValidString(value) ||
		!fs.ValidPath(value) || path.Clean(value) != value ||
		strings.ContainsRune(value, '\\') {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return workspaceOpenError(ErrorInvalidRequest)
	}
	if ctx.Err() != nil {
		return workspaceOpenError(ErrorCanceled)
	}
	return nil
}

func sourceChanged(
	ctx context.Context,
	absolutePath, capturedSHA256 string,
	maxBytes int64,
) (bool, error) {
	if capturedSHA256 == "" {
		return false, nil
	}
	if err := contextError(ctx); err != nil {
		return false, err
	}
	file, err := os.Open(absolutePath)
	if err != nil {
		return false, nil
	}
	defer file.Close()
	return hashChanged(ctx, file, capturedSHA256, maxBytes)
}

func hashChanged(
	ctx context.Context,
	reader io.Reader,
	capturedSHA256 string,
	maxBytes int64,
) (bool, error) {
	if capturedSHA256 == "" {
		return false, nil
	}
	hash := sha256.New()
	buffer := make([]byte, hashBufferSize)
	var total int64
	for total <= maxBytes {
		if err := contextError(ctx); err != nil {
			return false, err
		}
		readSize := int64(len(buffer))
		if remaining := maxBytes + 1 - total; remaining < readSize {
			readSize = remaining
		}
		read, readErr := reader.Read(buffer[:int(readSize)])
		if read > 0 {
			total += int64(read)
			if total > maxBytes {
				return false, nil
			}
			_, _ = hash.Write(buffer[:read])
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return false, nil
		}
		if read == 0 {
			return false, nil
		}
	}
	if err := contextError(ctx); err != nil {
		return false, err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)) != capturedSHA256, nil
}
