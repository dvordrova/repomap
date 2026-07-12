// Package memory persists one investigation as separately versioned facts,
// model claims, and session state. The in-memory reducer remains unaware of
// storage and receives a fully hydrated, validated Session after loading.
package memory

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/investigation"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

const (
	Version              = 1
	FactDocumentVersion  = 1
	ClaimDocumentVersion = 1
	SessionFileName      = "investigation_session.json"
	maxSessionBytes      = 4 << 20
	maxFactBytes         = 32 << 20
	maxClaimBytes        = 16 << 20
)

var contentPathPattern = regexp.MustCompile(`^(facts|claims)/[0-9a-f]{64}\.json$`)

type Input struct {
	Session    investigation.Session
	Repository freshness.RepositoryState
	Facts      *freshness.FactContext
	Claims     *freshness.ClaimContext
}

type Record struct {
	Session    investigation.Session
	Repository freshness.RepositoryState
	Facts      *freshness.FactContext
	Claims     *freshness.ClaimContext
	Changes    []freshness.Difference
}

// Current contains the local inputs against which a saved session may be
// reused. Facts and Claims are required only when the saved record contains
// their respective layer. An empty claim fact digest is bound to the saved
// fact document; empty provider/model values preserve their saved provenance.
type Current struct {
	Repository freshness.RepositoryState
	Facts      *freshness.FactContext
	Claims     *freshness.ClaimContext
}

type reference struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type sessionDocument struct {
	investigation.Session
	MemoryVersion   int                       `json:"memory_version"`
	RepositoryState freshness.RepositoryState `json:"repository_state"`
	FactsRef        *reference                `json:"facts_ref,omitempty"`
	ClaimsRef       *reference                `json:"claims_ref,omitempty"`
}

type factDocument struct {
	Version    int                   `json:"version"`
	Context    freshness.FactContext `json:"context"`
	Symbol     *symbol.Bundle        `json:"symbol,omitempty"`
	Source     *sourcecard.Card      `json:"source,omitempty"`
	Assessment *sourceexplain.Bundle `json:"assessment,omitempty"`
	Tests      *testevidence.Bundle  `json:"tests,omitempty"`
}

type claimDocument struct {
	Version      int                    `json:"version"`
	Context      freshness.ClaimContext `json:"context"`
	SourceReport *sourceexplain.Report  `json:"source_report"`
}

// Save writes content-addressed fact and claim documents before atomically
// replacing the small session document that references them.
func Save(dir string, input Input) (string, error) {
	if err := validateInput(input); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("memory: create directory: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", fmt.Errorf("memory: open directory: %w", err)
	}
	defer root.Close()
	if err := SaveRoot(root, input); err != nil {
		return "", err
	}
	return filepath.Join(dir, SessionFileName), nil
}

// SaveRoot writes one checkpoint below an already-open, caller-confined store
// root. Every artifact name is fixed internally; no caller-controlled relative
// path crosses this boundary.
func SaveRoot(root *os.Root, input Input) error {
	if root == nil {
		return fmt.Errorf("memory: store root is required")
	}
	if err := validateInput(input); err != nil {
		return err
	}
	for _, child := range []string{"facts", "claims"} {
		if err := ensureRootDirectory(root, child); err != nil {
			return err
		}
	}

	core := cloneSessionWithoutArtifacts(input.Session)
	document := sessionDocument{
		Session:         core,
		MemoryVersion:   Version,
		RepositoryState: cloneRepository(input.Repository),
	}

	var factPayload []byte
	if hasFacts(input.Session) {
		encoded, err := encodeDocument(factDocument{
			Version:    FactDocumentVersion,
			Context:    cloneFactContext(*input.Facts),
			Symbol:     input.Session.Symbol,
			Source:     input.Session.Source,
			Assessment: input.Session.Assessment,
			Tests:      input.Session.Tests,
		})
		if err != nil {
			return fmt.Errorf("memory: encode facts: %w", err)
		}
		factPayload = encoded
		factRef := contentReference("facts", encoded)
		if err := writeContentFile(root, factRef, encoded); err != nil {
			return err
		}
		document.FactsRef = &factRef
	}

	if input.Session.SourceReport != nil {
		context := *input.Claims
		factDigest := sha256Hex(factPayload)
		if context.FactDigest == "" {
			context.FactDigest = factDigest
		}
		if context.FactDigest != factDigest {
			return fmt.Errorf("memory: claim fact digest does not match facts document")
		}
		if err := context.Validate(); err != nil {
			return fmt.Errorf("memory: invalid claim context: %w", err)
		}
		encoded, err := encodeDocument(claimDocument{
			Version:      ClaimDocumentVersion,
			Context:      context,
			SourceReport: input.Session.SourceReport,
		})
		if err != nil {
			return fmt.Errorf("memory: encode claims: %w", err)
		}
		claimRef := contentReference("claims", encoded)
		if err := writeContentFile(root, claimRef, encoded); err != nil {
			return err
		}
		document.ClaimsRef = &claimRef
	}

	encoded, err := encodeDocument(document)
	if err != nil {
		return fmt.Errorf("memory: encode session: %w", err)
	}
	if err := writeAtomic(root, SessionFileName, encoded); err != nil {
		return err
	}
	return nil
}

// Load verifies storage integrity and reconciles freshness before returning an
// executable pending action. Stale facts or claims are invalidated through the
// investigation reducer rather than exposed to the caller.
func Load(path string, current Current) (Record, error) {
	dir := filepath.Dir(path)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Record{}, fmt.Errorf("memory: open session directory: %w", err)
	}
	defer root.Close()
	record, err := loadStoredRoot(root, filepath.Base(path))
	if err != nil {
		return Record{}, err
	}
	return reconcileCurrent(record, current)
}

// LoadRoot loads the fixed latest checkpoint from an already-open,
// caller-confined store root.
func LoadRoot(root *os.Root, current Current) (Record, error) {
	if root == nil {
		return Record{}, fmt.Errorf("memory: store root is required")
	}
	record, err := loadStoredRoot(root, SessionFileName)
	if err != nil {
		return Record{}, err
	}
	return reconcileCurrent(record, current)
}

func reconcileCurrent(record Record, current Current) (Record, error) {
	if err := current.Repository.Validate(); err != nil {
		return Record{}, fmt.Errorf("memory: invalid current repository state: %w", err)
	}
	repositoryDifferences := freshness.CompareRepository(record.Repository, current.Repository)
	if hasReason(repositoryDifferences, freshness.ReasonRepositoryIdentity) {
		return Record{}, fmt.Errorf("memory: saved repository identity does not match current repository")
	}
	if len(repositoryDifferences) > 0 {
		revision, err := current.Repository.Digest()
		if err != nil {
			return Record{}, err
		}
		next, _, err := investigation.Reduce(record.Session, investigation.Event{
			Kind:     investigation.EventRepositoryChanged,
			Revision: revision,
		})
		if err != nil {
			return Record{}, fmt.Errorf("memory: invalidate changed repository: %w", err)
		}
		record.Session = next
		record.Repository = cloneRepository(current.Repository)
		record.Facts = nil
		record.Claims = nil
		record.Changes = cloneDifferences(repositoryDifferences)
		return record, nil
	}

	if record.Facts != nil {
		if current.Facts == nil {
			return Record{}, fmt.Errorf("memory: current fact context is required")
		}
		if err := current.Facts.Validate(); err != nil {
			return Record{}, fmt.Errorf("memory: invalid current fact context: %w", err)
		}
		factDifferences := freshness.CompareFactContext(*record.Facts, *current.Facts)
		if len(factDifferences) > 0 {
			next, _, err := investigation.Reduce(record.Session, investigation.Event{
				Kind:    investigation.EventFactContextChanged,
				Message: formatDifferences(factDifferences),
			})
			if err != nil {
				return Record{}, fmt.Errorf("memory: invalidate stale facts: %w", err)
			}
			record.Session = next
			record.Facts = nil
			record.Claims = nil
			record.Changes = cloneDifferences(factDifferences)
			return record, nil
		}
	}

	if record.Claims != nil {
		if current.Claims == nil {
			return Record{}, fmt.Errorf("memory: current claim context is required")
		}
		expected := *current.Claims
		if expected.FactDigest == "" {
			expected.FactDigest = record.Claims.FactDigest
		}
		if expected.Provider == "" {
			expected.Provider = record.Claims.Provider
		}
		if expected.Model == "" {
			expected.Model = record.Claims.Model
		}
		if err := expected.Validate(); err != nil {
			return Record{}, fmt.Errorf("memory: invalid current claim context: %w", err)
		}
		claimDifferences := freshness.CompareClaimContext(*record.Claims, expected)
		if len(claimDifferences) > 0 {
			next, _, err := investigation.Reduce(record.Session, investigation.Event{
				Kind:    investigation.EventClaimContextChanged,
				Message: formatDifferences(claimDifferences),
			})
			if err != nil {
				return Record{}, fmt.Errorf("memory: invalidate stale claims: %w", err)
			}
			record.Session = next
			context := cloneFactContext(*current.Facts)
			record.Facts = &context
			record.Claims = nil
			record.Changes = cloneDifferences(claimDifferences)
			return record, nil
		}
	}
	return record, nil
}

func loadStored(path string) (Record, error) {
	dir := filepath.Dir(path)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Record{}, fmt.Errorf("memory: open session directory: %w", err)
	}
	defer root.Close()
	return loadStoredRoot(root, filepath.Base(path))
}

func loadStoredRoot(root *os.Root, name string) (Record, error) {
	data, err := readBounded(root, name, maxSessionBytes)
	if err != nil {
		return Record{}, fmt.Errorf("memory: read session: %w", err)
	}
	var document sessionDocument
	if err := decodeStrict(data, &document); err != nil {
		return Record{}, fmt.Errorf("memory: decode session: %w", err)
	}
	if document.MemoryVersion != Version {
		return Record{}, fmt.Errorf("memory: unsupported session-document version %d", document.MemoryVersion)
	}
	if hasFacts(document.Session) || document.Session.SourceReport != nil || hasEmbeddedActionArtifacts(document.Session) {
		return Record{}, fmt.Errorf("memory: session document embeds facts or claims")
	}
	if err := document.RepositoryState.Validate(); err != nil {
		return Record{}, fmt.Errorf("memory: invalid repository state: %w", err)
	}
	if !repositoryContainsScope(document.RepositoryState.Identity, document.Session.Repository.Path) {
		return Record{}, fmt.Errorf("memory: session repository path is not a canonical repository scope")
	}
	revision, err := document.RepositoryState.Digest()
	if err != nil {
		return Record{}, err
	}
	if document.Session.Repository.Revision != revision {
		return Record{}, fmt.Errorf("memory: session revision does not match repository state")
	}

	record := Record{
		Session:    document.Session,
		Repository: cloneRepository(document.RepositoryState),
	}
	var factSHA string
	if document.FactsRef != nil {
		data, err := readReference(root, *document.FactsRef, "facts", maxFactBytes)
		if err != nil {
			return Record{}, err
		}
		var facts factDocument
		if err := decodeStrict(data, &facts); err != nil {
			return Record{}, fmt.Errorf("memory: decode facts: %w", err)
		}
		if facts.Version != FactDocumentVersion {
			return Record{}, fmt.Errorf("memory: unsupported fact-document version %d", facts.Version)
		}
		if err := facts.Context.Validate(); err != nil {
			return Record{}, fmt.Errorf("memory: invalid fact context: %w", err)
		}
		if differences := freshness.CompareRepository(facts.Context.Repository, document.RepositoryState); len(differences) > 0 {
			return Record{}, fmt.Errorf("memory: fact repository state does not match session: %s", formatDifferences(differences))
		}
		record.Session.Symbol = facts.Symbol
		record.Session.Source = facts.Source
		record.Session.Assessment = facts.Assessment
		record.Session.Tests = facts.Tests
		context := cloneFactContext(facts.Context)
		record.Facts = &context
		factSHA = document.FactsRef.SHA256
	}
	if document.ClaimsRef != nil {
		if document.FactsRef == nil {
			return Record{}, fmt.Errorf("memory: claims reference exists without facts")
		}
		data, err := readReference(root, *document.ClaimsRef, "claims", maxClaimBytes)
		if err != nil {
			return Record{}, err
		}
		var claims claimDocument
		if err := decodeStrict(data, &claims); err != nil {
			return Record{}, fmt.Errorf("memory: decode claims: %w", err)
		}
		if claims.Version != ClaimDocumentVersion {
			return Record{}, fmt.Errorf("memory: unsupported claim-document version %d", claims.Version)
		}
		if err := claims.Context.Validate(); err != nil {
			return Record{}, fmt.Errorf("memory: invalid claim context: %w", err)
		}
		if claims.Context.FactDigest != factSHA {
			return Record{}, fmt.Errorf("memory: claims reference a different fact document")
		}
		if claims.SourceReport == nil {
			return Record{}, fmt.Errorf("memory: claim document has no source report")
		}
		record.Session.SourceReport = claims.SourceReport
		context := claims.Context
		record.Claims = &context
	}
	if err := hydratePendingAction(&record.Session); err != nil {
		return Record{}, err
	}
	if err := record.Session.Validate(); err != nil {
		return Record{}, fmt.Errorf("memory: invalid hydrated session: %w", err)
	}
	return record, nil
}

func validateInput(input Input) error {
	if err := input.Session.Validate(); err != nil {
		return fmt.Errorf("memory: invalid session: %w", err)
	}
	if err := input.Repository.Validate(); err != nil {
		return fmt.Errorf("memory: invalid repository state: %w", err)
	}
	revision, err := input.Repository.Digest()
	if err != nil {
		return err
	}
	if !repositoryContainsScope(input.Repository.Identity, input.Session.Repository.Path) || input.Session.Repository.Revision != revision {
		return fmt.Errorf("memory: session repository does not match captured state")
	}
	if hasFacts(input.Session) {
		if input.Facts == nil {
			return fmt.Errorf("memory: fact context is required when facts exist")
		}
		if err := input.Facts.Validate(); err != nil {
			return fmt.Errorf("memory: invalid fact context: %w", err)
		}
		if differences := freshness.CompareRepository(input.Facts.Repository, input.Repository); len(differences) > 0 {
			return fmt.Errorf("memory: fact repository state does not match session: %s", formatDifferences(differences))
		}
	} else if input.Facts != nil {
		return fmt.Errorf("memory: fact context exists without facts")
	}
	if input.Session.SourceReport != nil {
		if input.Claims == nil {
			return fmt.Errorf("memory: claim context is required when model claims exist")
		}
	} else if input.Claims != nil {
		return fmt.Errorf("memory: claim context exists without model claims")
	}
	return nil
}

func cloneSessionWithoutArtifacts(session investigation.Session) investigation.Session {
	result := session
	result.Symbol = nil
	result.Source = nil
	result.Assessment = nil
	result.SourceReport = nil
	result.Tests = nil
	result.Next = append([]investigation.Action(nil), session.Next...)
	for index := range result.Next {
		switch result.Next[index].Kind {
		case investigation.ActionReadSource:
			result.Next[index].ReadSource = nil
		case investigation.ActionAssessSource:
			result.Next[index].AssessSource = nil
		case investigation.ActionFindTests:
			result.Next[index].FindTests = nil
		case investigation.ActionFindTestReferences:
			result.Next[index].FindTestReferences = nil
		}
	}
	return result
}

func hasEmbeddedActionArtifacts(session investigation.Session) bool {
	for _, action := range session.Next {
		switch action.Kind {
		case investigation.ActionReadSource:
			if action.ReadSource != nil {
				return true
			}
		case investigation.ActionAssessSource:
			if action.AssessSource != nil {
				return true
			}
		case investigation.ActionFindTests:
			if action.FindTests != nil {
				return true
			}
		case investigation.ActionFindTestReferences:
			if action.FindTestReferences != nil {
				return true
			}
		}
	}
	return false
}

func hydratePendingAction(session *investigation.Session) error {
	if len(session.Next) != 1 {
		return nil
	}
	action := &session.Next[0]
	switch action.Kind {
	case investigation.ActionReadSource:
		if session.Symbol == nil {
			return fmt.Errorf("memory: read-source action has no stored symbol fact")
		}
		action.ReadSource = &investigation.ReadSourceInput{
			RepoPath:         session.Repository.Path,
			TargetEvidenceID: session.Symbol.Target.EvidenceID,
			Target:           session.Symbol.Target.Entity,
		}
	case investigation.ActionAssessSource:
		if session.Assessment == nil {
			return fmt.Errorf("memory: assess-source action has no stored assessment fact")
		}
		action.AssessSource = session.Assessment
	case investigation.ActionFindTests:
		if session.Symbol == nil || session.Assessment == nil || session.SourceReport == nil {
			return fmt.Errorf("memory: find-tests action has incomplete stored evidence")
		}
		action.FindTests = &investigation.FindTestsInput{
			RepoPath:   session.Repository.Path,
			Structural: *session.Symbol,
			Assessment: *session.Assessment,
			Report:     *session.SourceReport,
		}
	case investigation.ActionFindTestReferences:
		if session.Symbol == nil {
			return fmt.Errorf("memory: find-test-references action has no stored symbol fact")
		}
		action.FindTestReferences = &investigation.FindTestReferencesInput{
			RepoPath:   session.Repository.Path,
			Structural: *session.Symbol,
		}
	}
	return nil
}

func hasFacts(session investigation.Session) bool {
	return session.Symbol != nil || session.Source != nil || session.Assessment != nil || session.Tests != nil
}

func contentReference(kind string, data []byte) reference {
	digest := sha256Hex(data)
	return reference{Path: kind + "/" + digest + ".json", SHA256: digest}
}

func readReference(root *os.Root, ref reference, kind string, limit int64) ([]byte, error) {
	wantPath := kind + "/" + ref.SHA256 + ".json"
	if !contentPathPattern.MatchString(ref.Path) || ref.Path != wantPath || !validSHA256(ref.SHA256) ||
		filepath.ToSlash(filepath.Clean(filepath.FromSlash(ref.Path))) != ref.Path {
		return nil, fmt.Errorf("memory: invalid %s reference", kind)
	}
	data, err := readBounded(root, filepath.FromSlash(ref.Path), limit)
	if err != nil {
		return nil, fmt.Errorf("memory: read %s: %w", kind, err)
	}
	if sha256Hex(data) != ref.SHA256 {
		return nil, fmt.Errorf("memory: %s content hash mismatch", kind)
	}
	return data, nil
}

func writeContentFile(root *os.Root, ref reference, data []byte) error {
	path := filepath.FromSlash(ref.Path)
	if info, err := root.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("memory: existing content-addressed file %q is not regular", ref.Path)
		}
		if info.Size() != int64(len(data)) {
			return fmt.Errorf("memory: existing content-addressed file %q has the wrong size", ref.Path)
		}
		existing, readErr := readBounded(root, path, int64(len(data)))
		if readErr != nil {
			return fmt.Errorf("memory: inspect %q: %w", ref.Path, readErr)
		}
		if sha256Hex(existing) != ref.SHA256 {
			return fmt.Errorf("memory: existing content-addressed file %q has the wrong hash", ref.Path)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("memory: inspect %q: %w", ref.Path, err)
	}
	return writeAtomic(root, path, data)
}

func ensureRootDirectory(root *os.Root, name string) error {
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		if err := root.Mkdir(name, 0o700); err != nil && !os.IsExist(err) {
			return fmt.Errorf("memory: create %s directory: %w", name, err)
		}
		info, err = root.Lstat(name)
	}
	if err != nil {
		return fmt.Errorf("memory: inspect %s directory: %w", name, err)
	}
	if !info.Mode().IsDir() {
		return fmt.Errorf("memory: %s is not a directory", name)
	}
	child, err := root.OpenRoot(name)
	if err != nil {
		return fmt.Errorf("memory: open %s directory: %w", name, err)
	}
	return child.Close()
}

func writeAtomic(root *os.Root, path string, data []byte) error {
	dir := filepath.Dir(path)
	var (
		temporary     *os.File
		temporaryPath string
	)
	for attempt := 0; attempt < 10; attempt++ {
		name, err := randomTemporaryName()
		if err != nil {
			return err
		}
		temporaryPath = filepath.Join(dir, name)
		temporary, err = root.OpenFile(temporaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			break
		}
		if !os.IsExist(err) {
			return fmt.Errorf("memory: create temporary artifact: %w", err)
		}
	}
	if temporary == nil {
		return fmt.Errorf("memory: could not allocate a temporary artifact name")
	}
	defer root.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("memory: write temporary artifact: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("memory: sync temporary artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("memory: close temporary artifact: %w", err)
	}
	if err := os.Rename(filepath.Join(root.Name(), temporaryPath), filepath.Join(root.Name(), path)); err != nil {
		return fmt.Errorf("memory: replace artifact: %w", err)
	}
	return nil
}

func encodeDocument(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func readBounded(root *os.Root, path string, limit int64) ([]byte, error) {
	info, err := root.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("file is not regular")
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	file, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return data, nil
}

func randomTemporaryName() (string, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("memory: generate temporary artifact name: %w", err)
	}
	return ".repomap-memory-" + hex.EncodeToString(random[:]), nil
}

func decodeStrict(data []byte, target any) error {
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

func cloneRepository(state freshness.RepositoryState) freshness.RepositoryState {
	if state.Dirty != nil {
		state.Dirty = append([]freshness.DirtyFile{}, state.Dirty...)
	}
	return state
}

func cloneFactContext(context freshness.FactContext) freshness.FactContext {
	context.Repository = cloneRepository(context.Repository)
	context.Build.BuildTags = append([]string{}, context.Build.BuildTags...)
	return context
}

func formatDifferences(differences []freshness.Difference) string {
	values := make([]string, 0, len(differences))
	for _, difference := range differences {
		values = append(values, difference.String())
	}
	return strings.Join(values, "; ")
}

func hasReason(differences []freshness.Difference, reason freshness.Reason) bool {
	for _, difference := range differences {
		if difference.Reason == reason {
			return true
		}
	}
	return false
}

func cloneDifferences(differences []freshness.Difference) []freshness.Difference {
	result := append([]freshness.Difference(nil), differences...)
	for index := range result {
		result[index].Paths = append([]string(nil), result[index].Paths...)
	}
	return result
}

func repositoryContainsScope(repositoryRoot, scope string) bool {
	if !filepath.IsAbs(repositoryRoot) || filepath.Clean(repositoryRoot) != repositoryRoot ||
		!filepath.IsAbs(scope) || filepath.Clean(scope) != scope {
		return false
	}
	resolvedRoot, err := filepath.EvalSymlinks(repositoryRoot)
	if err != nil {
		return false
	}
	resolvedScope, err := filepath.EvalSymlinks(scope)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedScope)
	return err == nil && filepath.IsLocal(relative)
}

func validSHA256(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}

func sha256Hex(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
