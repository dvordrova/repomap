// Package workspaceopen resolves one snapshot-authorized local file target
// without owning HTTP, report, browser, or editor-launch behavior.
package workspaceopen

import (
	"context"
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

const maxPathBytes = 4096

// errorKind is the private closed failure taxonomy for target resolution.
// The report server deliberately exposes one source_unavailable response.
type errorKind string

const (
	errorInvalidRequest    errorKind = "invalid_request"
	errorUnauthorized      errorKind = "unauthorized"
	errorRootUnavailable   errorKind = "root_unavailable"
	errorTargetUnavailable errorKind = "target_unavailable"
	errorCanceled          errorKind = "canceled"
)

type openError struct {
	kind errorKind
}

func (err *openError) Error() string {
	if err == nil {
		return "workspace open: target_unavailable"
	}
	return "workspace open: " + string(err.kind)
}

func workspaceOpenError(kind errorKind) error {
	return &openError{kind: kind}
}

// Request selects one exact catalog-authorized analysis-relative path.
type Request struct {
	Path string
}

// Target contains no source bytes. AbsolutePath is intended only for a local
// adapter that has separately authorized its presentation-level action.
type Target struct {
	AbsolutePath string
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
		return nil, workspaceOpenError(errorInvalidRequest)
	}
	return &Service{analysisRoot: analysisRoot, catalog: catalog}, nil
}

// Resolve verifies one authorized current local file. Repository changes do
// not alter this action: the editor opens the current regular file at the
// exact manifest-authorized path.
func (service *Service) Resolve(ctx context.Context, request Request) (Target, error) {
	// Path is caller-controlled. Bound it before trimming, Unicode scanning,
	// canonicalization, filesystem calls, or path-derived allocations.
	if len(request.Path) > maxPathBytes {
		return Target{}, workspaceOpenError(errorInvalidRequest)
	}
	if service == nil || service.analysisRoot == "" || ctx == nil {
		return Target{}, workspaceOpenError(errorInvalidRequest)
	}
	if !validPath(request.Path) {
		return Target{}, workspaceOpenError(errorInvalidRequest)
	}
	if err := contextError(ctx); err != nil {
		return Target{}, err
	}
	source, ok := service.catalog.Lookup(request.Path)
	if !ok || source.Path != request.Path {
		return Target{}, workspaceOpenError(errorUnauthorized)
	}

	liveRoot, err := filepath.EvalSymlinks(service.analysisRoot)
	if err != nil || liveRoot != service.analysisRoot {
		return Target{}, workspaceOpenError(errorRootUnavailable)
	}
	if err := contextError(ctx); err != nil {
		return Target{}, err
	}

	// Resolve the exact path authorized by the catalog.
	localPath := filepath.FromSlash(request.Path)
	if !filepath.IsLocal(localPath) || localPath == "." {
		return Target{}, workspaceOpenError(errorTargetUnavailable)
	}
	unresolved := filepath.Join(liveRoot, localPath)
	entryInfo, err := os.Lstat(unresolved)
	if err != nil || entryInfo.Mode()&os.ModeSymlink != 0 {
		return Target{}, workspaceOpenError(errorTargetUnavailable)
	}
	absolutePath, err := filepath.EvalSymlinks(unresolved)
	if err != nil {
		return Target{}, workspaceOpenError(errorTargetUnavailable)
	}
	relative, err := filepath.Rel(liveRoot, absolutePath)
	if err != nil || !filepath.IsLocal(relative) {
		return Target{}, workspaceOpenError(errorTargetUnavailable)
	}
	info, err := os.Stat(absolutePath)
	if err != nil || !info.Mode().IsRegular() {
		return Target{}, workspaceOpenError(errorTargetUnavailable)
	}
	if err := contextError(ctx); err != nil {
		return Target{}, err
	}

	return Target{AbsolutePath: absolutePath}, nil
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
		return workspaceOpenError(errorInvalidRequest)
	}
	if ctx.Err() != nil {
		return workspaceOpenError(errorCanceled)
	}
	return nil
}
