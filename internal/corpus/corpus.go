// Package corpus owns the run-local, tracked-file namespace shared by
// repository analysis cubes.
package corpus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/reporead"
)

const (
	Version          = 1
	MaxFiles         = 1_000_000
	MaxSnapshotBytes = 64 << 20
	MaxReadBytes     = int64(64 << 20)
	refPrefix        = "rc-"
)

// FileID is the compact run-local identity used by cubes. IDs are assigned by
// canonical repository-relative path order and never come from model output.
type FileID string

// Entry is one stage-0 tracked regular file. Path is repository-relative and
// slash-separated. Executable is exact Git index mode authority.
//
// Working-tree size is deliberately not part of this sealed identity: files
// may change during a run, and every cube binds the actual bounded bytes it
// reads instead of relying on a freshness gate.
type Entry struct {
	ID         FileID `json:"id"`
	Path       string `json:"path"`
	Executable bool   `json:"executable"`
}

// Gitlink is one exact stage-0 submodule entry captured by the corpus's sole
// Git-index read. Gitlinks remain outside the readable FileID namespace but
// are retained for repository-state authority.
type Gitlink struct {
	Path             string `json:"path"`
	RecordedObjectID string `json:"recorded_object_id"`
}

// Snapshot is the independently owned, serializable corpus identity. SHA256
// binds Version and Entries; Ref is its compact display/reference form.
type Snapshot struct {
	Version int     `json:"version"`
	Ref     string  `json:"ref"`
	SHA256  string  `json:"sha256"`
	Entries []Entry `json:"entries"`
}

// FileInfo is one resolved sealed corpus entry.
type FileInfo struct {
	Entry Entry
}

// Content is one current bounded read tied to its exact corpus entry.
type Content struct {
	Entry     Entry
	Bytes     []byte
	Truncated bool
}

// Corpus is an immutable file namespace plus a confined live reader. Metadata
// maps never change; the mutex coordinates concurrent reads with Close.
type Corpus struct {
	snapshot     Snapshot
	visiblePaths []string
	gitlinks     []Gitlink
	byID         map[FileID]FileInfo
	byPath       map[string]FileID
	reader       *reporead.Reader

	mu     sync.RWMutex
	closed bool
}

// Open inventories one repository through the exact stage-0 Git listing and
// opens its confined reader.
func Open(ctx context.Context, repoPath string) (*Corpus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	listing, err := gitfiles.ListWithModesContext(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("repository corpus: tracked files: %w", err)
	}
	return New(ctx, repoPath, listing)
}

// New builds a corpus from an already captured Git listing. Only
// Listing.RegularPaths enter the corpus; stage conflicts, symlinks, gitlinks,
// and other index modes are absent. Every retained working-tree path must
// currently be a non-symlink regular file.
func New(ctx context.Context, repoPath string, listing gitfiles.Listing) (*Corpus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(repoPath) == "" {
		return nil, fmt.Errorf("repository corpus: repository path is required")
	}

	regular, err := canonicalPaths("regular", listing.RegularPaths, MaxFiles)
	if err != nil {
		return nil, err
	}
	executable, err := canonicalPathSet("executable", listing.ExecutablePaths, len(regular))
	if err != nil {
		return nil, err
	}
	gitlinks, err := canonicalGitlinks(listing.Gitlinks)
	if err != nil {
		return nil, err
	}
	regularSet := make(map[string]struct{}, len(regular))
	for _, filePath := range regular {
		regularSet[filePath] = struct{}{}
	}
	for _, gitlink := range gitlinks {
		if _, duplicate := regularSet[gitlink.Path]; duplicate {
			return nil, fmt.Errorf(
				"repository corpus: path %q is both a regular file and a gitlink",
				gitlink.Path,
			)
		}
	}
	for filePath := range executable {
		if _, ok := regularSet[filePath]; !ok {
			return nil, fmt.Errorf(
				"repository corpus: executable path %q is not a stage-0 regular path",
				filePath,
			)
		}
	}
	visiblePaths := append([]string(nil), regular...)
	if listing.Paths != nil {
		if len(listing.Paths) > MaxFiles*4 {
			return nil, fmt.Errorf("repository corpus: visible path limit %d exceeded", MaxFiles*4)
		}
		// Non-regular index rows are deliberately opaque to the corpus. Keep
		// their exact strings only long enough to prove that every retained
		// regular row belongs to the listing; do not reject an otherwise ignored
		// symlink, gitlink, or conflict path for its spelling.
		visible := make(map[string]struct{}, len(listing.Paths))
		for _, filePath := range listing.Paths {
			visible[filePath] = struct{}{}
		}
		for _, filePath := range regular {
			if _, ok := visible[filePath]; !ok {
				return nil, fmt.Errorf(
					"repository corpus: regular path %q is absent from the tracked listing",
					filePath,
				)
			}
		}
		for _, gitlink := range gitlinks {
			if _, ok := visible[gitlink.Path]; !ok {
				return nil, fmt.Errorf(
					"repository corpus: gitlink path %q is absent from the tracked listing",
					gitlink.Path,
				)
			}
		}
		visiblePaths = append([]string(nil), listing.Paths...)
	}
	sort.Strings(visiblePaths)
	visiblePaths = compactStrings(visiblePaths)

	absoluteRoot, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("repository corpus: resolve repository path: %w", err)
	}
	entries := make([]Entry, len(regular))
	byID := make(map[FileID]FileInfo, len(regular))
	byPath := make(map[string]FileID, len(regular))
	for index, filePath := range regular {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := FileID("f" + strconv.Itoa(index+1))
		_, isExecutable := executable[filePath]
		entry := Entry{ID: id, Path: filePath, Executable: isExecutable}
		entries[index] = entry
		byID[id] = FileInfo{Entry: entry}
		byPath[filePath] = id
	}

	snapshot, err := seal(entries)
	if err != nil {
		return nil, err
	}
	reader, err := reporead.New(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("repository corpus: open confined reader: %w", err)
	}
	return &Corpus{
		snapshot:     snapshot,
		visiblePaths: visiblePaths,
		gitlinks:     append([]Gitlink(nil), gitlinks...),
		byID:         byID,
		byPath:       byPath,
		reader:       reader,
	}, nil
}

// Snapshot returns an independently owned corpus identity.
func (corpus *Corpus) Snapshot() Snapshot {
	if corpus == nil {
		return Snapshot{}
	}
	return cloneSnapshot(corpus.snapshot)
}

// Ref returns the compact sealed corpus reference.
func (corpus *Corpus) Ref() string {
	if corpus == nil {
		return ""
	}
	return corpus.snapshot.Ref
}

// SHA256 returns the complete corpus identity digest.
func (corpus *Corpus) SHA256() string {
	if corpus == nil {
		return ""
	}
	return corpus.snapshot.SHA256
}

// Entries returns independently owned entries in FileID order.
func (corpus *Corpus) Entries() []Entry {
	if corpus == nil {
		return nil
	}
	return append([]Entry(nil), corpus.snapshot.Entries...)
}

// Gitlinks returns the exact stage-0 submodule rows captured with Entries.
// They have no FileID and cannot be read as repository source.
func (corpus *Corpus) Gitlinks() []Gitlink {
	if corpus == nil {
		return nil
	}
	return append([]Gitlink(nil), corpus.gitlinks...)
}

// VisiblePaths returns every tracked path reported by Git, including
// symlinks, gitlinks, and unresolved conflict paths that deliberately have no
// FileID and cannot be read by analysis cubes.
func (corpus *Corpus) VisiblePaths() []string {
	if corpus == nil {
		return nil
	}
	return append([]string(nil), corpus.visiblePaths...)
}

// Info resolves one exact FileID and returns its sealed identity.
func (corpus *Corpus) Info(id FileID) (FileInfo, bool) {
	if corpus == nil || !validFileID(string(id)) {
		return FileInfo{}, false
	}
	info, ok := corpus.byID[id]
	return info, ok
}

// ID resolves one already canonical repository-relative path.
func (corpus *Corpus) ID(filePath string) (FileID, bool) {
	if corpus == nil || validatePath(filePath) != nil {
		return "", false
	}
	id, ok := corpus.byPath[filePath]
	return id, ok
}

// ReadFile reads current bytes only through the confined no-symlink reader.
// Repository changes are accepted; deletion, non-regular replacement, and
// symlink replacement fail at the read that observes them.
func (corpus *Corpus) ReadFile(id FileID, maxBytes int64) (Content, error) {
	if corpus == nil {
		return Content{}, fmt.Errorf("repository corpus: corpus is not initialized")
	}
	if maxBytes < 0 || maxBytes > MaxReadBytes {
		return Content{}, fmt.Errorf(
			"repository corpus: invalid byte limit %d (maximum %d)",
			maxBytes, MaxReadBytes,
		)
	}
	if !validFileID(string(id)) {
		return Content{}, fmt.Errorf("repository corpus: unknown file ID %q", id)
	}

	corpus.mu.RLock()
	defer corpus.mu.RUnlock()
	if corpus.closed || corpus.reader == nil {
		return Content{}, fmt.Errorf("repository corpus: corpus is closed")
	}
	info, ok := corpus.byID[id]
	if !ok {
		return Content{}, fmt.Errorf("repository corpus: unknown file ID %q", id)
	}
	content, err := corpus.reader.ReadFileNoSymlinks(info.Entry.Path, maxBytes)
	if err != nil {
		return Content{}, fmt.Errorf("repository corpus: read %s: %w", id, err)
	}
	return Content{
		Entry:     info.Entry,
		Bytes:     content.Bytes,
		Truncated: content.Truncated,
	}, nil
}

// Close prevents new reads and releases the confined repository root. It is
// idempotent and waits for already running reads.
func (corpus *Corpus) Close() error {
	if corpus == nil {
		return nil
	}
	corpus.mu.Lock()
	defer corpus.mu.Unlock()
	if corpus.closed {
		return nil
	}
	corpus.closed = true
	reader := corpus.reader
	corpus.reader = nil
	if reader == nil {
		return nil
	}
	if err := reader.Close(); err != nil {
		return fmt.Errorf("repository corpus: close reader: %w", err)
	}
	return nil
}

// Validate rejects shape, order, identity, and seal drift.
func (snapshot Snapshot) Validate() error {
	if snapshot.Version != Version || snapshot.Entries == nil || len(snapshot.Entries) > MaxFiles {
		return fmt.Errorf("repository corpus snapshot: invalid identity")
	}
	for index, entry := range snapshot.Entries {
		wantID := FileID("f" + strconv.Itoa(index+1))
		if entry.ID != wantID {
			return fmt.Errorf("repository corpus snapshot: entry %d has invalid file ID", index)
		}
		if err := validatePath(entry.Path); err != nil {
			return fmt.Errorf("repository corpus snapshot: entry %d: %w", index, err)
		}
		if index > 0 && snapshot.Entries[index-1].Path >= entry.Path {
			return fmt.Errorf("repository corpus snapshot: entries are not in canonical path order")
		}
	}
	wantSHA, err := identitySHA(snapshot.Version, snapshot.Entries)
	if err != nil {
		return err
	}
	wantRef := compactRef(wantSHA)
	if snapshot.SHA256 != wantSHA || snapshot.Ref != wantRef {
		return fmt.Errorf("repository corpus snapshot: seal binding mismatch")
	}
	return nil
}

// Owned validates and returns an independently owned Snapshot.
func (snapshot Snapshot) Owned() (Snapshot, error) {
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	return cloneSnapshot(snapshot), nil
}

// CanonicalJSON returns deterministic validated snapshot bytes.
func (snapshot Snapshot) CanonicalJSON() ([]byte, error) {
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	wire, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("repository corpus snapshot: encode: %w", err)
	}
	if len(wire) > MaxSnapshotBytes {
		return nil, fmt.Errorf("repository corpus snapshot: exceeds %d bytes", MaxSnapshotBytes)
	}
	return wire, nil
}

type identity struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

func seal(entries []Entry) (Snapshot, error) {
	owned := make([]Entry, len(entries))
	copy(owned, entries)
	sha, err := identitySHA(Version, owned)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{
		Version: Version,
		Ref:     compactRef(sha),
		SHA256:  sha,
		Entries: owned,
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func identitySHA(version int, entries []Entry) (string, error) {
	wire, err := json.Marshal(identity{Version: version, Entries: entries})
	if err != nil {
		return "", fmt.Errorf("repository corpus snapshot: encode identity: %w", err)
	}
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:]), nil
}

func compactRef(sha string) string {
	if len(sha) < 24 {
		return ""
	}
	return refPrefix + sha[:24]
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	result := snapshot
	if snapshot.Entries == nil {
		result.Entries = nil
	} else {
		result.Entries = make([]Entry, len(snapshot.Entries))
		copy(result.Entries, snapshot.Entries)
	}
	return result
}

func canonicalGitlinks(values []gitfiles.Gitlink) ([]Gitlink, error) {
	if len(values) > MaxFiles {
		return nil, fmt.Errorf("repository corpus: gitlink limit %d exceeded", MaxFiles)
	}
	result := make([]Gitlink, len(values))
	for index, value := range values {
		if err := validatePath(value.Path); err != nil {
			return nil, fmt.Errorf("repository corpus: gitlink %d: %w", index, err)
		}
		if !validObjectID(value.ObjectID) {
			return nil, fmt.Errorf("repository corpus: gitlink %d has invalid recorded object ID", index)
		}
		result[index] = Gitlink{Path: value.Path, RecordedObjectID: value.ObjectID}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	for index := 1; index < len(result); index++ {
		if result[index-1].Path == result[index].Path {
			return nil, fmt.Errorf("repository corpus: duplicate gitlink path %q", result[index].Path)
		}
	}
	return result, nil
}

func canonicalPaths(label string, values []string, limit int) ([]string, error) {
	if len(values) > limit {
		return nil, fmt.Errorf("repository corpus: %s path limit %d exceeded", label, limit)
	}
	result := append([]string(nil), values...)
	for index, value := range result {
		if err := validatePath(value); err != nil {
			return nil, fmt.Errorf("repository corpus: %s path %d: %w", label, index, err)
		}
	}
	sort.Strings(result)
	for index := 1; index < len(result); index++ {
		if result[index-1] == result[index] {
			return nil, fmt.Errorf("repository corpus: duplicate %s path %q", label, result[index])
		}
	}
	return result, nil
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func canonicalPathSet(label string, values []string, limit int) (map[string]struct{}, error) {
	canonical, err := canonicalPaths(label, values, limit)
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(canonical))
	for _, value := range canonical {
		result[value] = struct{}{}
	}
	return result, nil
}

func validatePath(value string) error {
	if value == "" || value == "." || !fs.ValidPath(value) || path.Clean(value) != value ||
		strings.ContainsRune(value, '\\') {
		return fmt.Errorf("path %q is not a canonical repository-relative slash path", value)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("path %q contains control characters", value)
		}
	}
	return nil
}

func validFileID(value string) bool {
	if len(value) < 2 || value[0] != 'f' || value[1] == '0' {
		return false
	}
	for _, character := range value[1:] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}
	return true
}
