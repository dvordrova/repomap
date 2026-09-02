// Package facts owns the deterministic fact layer of a repomap run: the
// first-day facts a newcomer needs, each anchored to an exact source
// location. Every row is derived locally from the repository corpus, the
// sealed ProgramIndex set, dependency catalogs, and manifests. No model output
// enters this layer; model stages reference these rows by id.
package facts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	Version          = 1
	ArtifactFilename = "facts.json"

	digestDomain = "repomap-facts-v1\x00"
	idDomain     = "repomap-fact-id-v1\x00"
	idHexWidth   = 16
)

// Kind is the closed vocabulary of fact rows.
type Kind string

const (
	// KindEntrypoint is a real execution root proved by a language adapter
	// (main guard, bound module object, callable seed, manifest script).
	KindEntrypoint Kind = "entrypoint"
	// KindHTTPRoute is a server-side route registration with a method and a
	// path literal.
	KindHTTPRoute Kind = "http_route"
	// KindHTTPCall is a client-side HTTP call with a method and a path
	// literal or template.
	KindHTTPCall Kind = "http_call"
	// KindPortal joins one http_call to exactly one http_route of another
	// target on method and path literals.
	KindPortal Kind = "portal"
	// KindConfigRead is an environment/config key read.
	KindConfigRead Kind = "config_read"
	// KindRisk is a dangerous call pattern (exec, eval, subprocess, ...).
	KindRisk Kind = "risk"
	// KindManifest is a fact quoted from a manifest (package.json scripts,
	// proxy, engines, pinned versions, Pipfile packages, go.mod module...).
	KindManifest Kind = "manifest"
	// KindTODO is a TODO/FIXME/XXX/HACK marker.
	KindTODO Kind = "todo"
	// KindDeadModule is a target source file unreachable from every entrypoint
	// through imports, calls and containment.
	KindDeadModule Kind = "dead_module"
	// KindNegative states something the repository lacks (tests, README,
	// Dockerfile, CI).
	KindNegative Kind = "negative"
	// KindDependency is an external package the target imports.
	KindDependency Kind = "dependency"
	// KindImport is a file-level import edge inside a target.
	KindImport Kind = "import"
)

func (kind Kind) Valid() bool {
	switch kind {
	case KindEntrypoint, KindHTTPRoute, KindHTTPCall, KindPortal, KindConfigRead,
		KindRisk, KindManifest, KindTODO, KindDeadModule, KindNegative,
		KindDependency, KindImport:
		return true
	default:
		return false
	}
}

// Resolution says how strong a derived fact is. Exact rows come from one
// literal or one proved structure; possible rows involve a template hole or
// a parameter segment match.
type Resolution string

const (
	ResolutionExact    Resolution = "exact"
	ResolutionPossible Resolution = "possible"
)

// Negative names are closed so the report can phrase them.
const (
	NegativeNoReadme       = "no_readme"
	NegativeReadmeTooShort = "readme_too_short"
	NegativeNoTests        = "no_tests"
	NegativeNoDockerfile   = "no_dockerfile"
	NegativeNoCI           = "no_ci"
)

// Anchor is an exact repository-relative source location.
type Anchor struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

func (anchor Anchor) String() string {
	if anchor.Line <= 0 {
		return anchor.Path
	}
	return fmt.Sprintf("%s:%d", anchor.Path, anchor.Line)
}

func (anchor Anchor) validate() error {
	if err := validateRepositoryPath(anchor.Path); err != nil {
		return err
	}
	if anchor.Line < 0 || anchor.Column < 0 {
		return fmt.Errorf("facts: negative anchor position for %q", anchor.Path)
	}
	return nil
}

// Target is one analyzed target as the reader sees it: a language, a root
// directory, and the manifest that defines it.
type Target struct {
	ID              string `json:"id"`
	ProgramTargetID string `json:"program_target_id"`
	Language        string `json:"language"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Root            string `json:"root"`
	Manifest        string `json:"manifest,omitempty"`
	Anchor          Anchor `json:"anchor"`
	// RunID is the child run directory that holds this target's ProgramIndex
	// and GroupsIndex; it lets a later stage re-read exact inputs.
	RunID string `json:"run_id,omitempty"`
}

// Fact is one anchored row. Only the fields meaningful for its Kind are set:
//
//	entrypoint  Symbol, ObjectID, Key (seed kind)
//	http_route  Method, Path, Symbol (handler), ObjectID (handler object)
//	http_call   Method, Path, Symbol (caller), ObjectID (caller object)
//	portal      Method, Path, Refs [call id, route id], PeerTargetID, Evidence [route anchor]
//	config_read Key (env key), Value (literal default), Symbol
//	risk        Key (pattern), Symbol, Text (witness)
//	manifest    Key (dotted manifest key), Value
//	todo        Text
//	dead_module Path (file)
//	negative    Key (negative name), Text (detail)
//	dependency  Key (package), Value (declared version)
//	import      Path (imported file)
type Fact struct {
	ID           string     `json:"id"`
	Kind         Kind       `json:"kind"`
	TargetID     string     `json:"target_id,omitempty"`
	PeerTargetID string     `json:"peer_target_id,omitempty"`
	Anchor       *Anchor    `json:"anchor,omitempty"`
	Symbol       string     `json:"symbol,omitempty"`
	ObjectID     string     `json:"object_id,omitempty"`
	Method       string     `json:"method,omitempty"`
	Path         string     `json:"path,omitempty"`
	Key          string     `json:"key,omitempty"`
	Value        string     `json:"value,omitempty"`
	Text         string     `json:"text,omitempty"`
	Resolution   Resolution `json:"resolution,omitempty"`
	Refs         []string   `json:"refs,omitempty"`
	Evidence     []Anchor   `json:"evidence,omitempty"`
}

// Diagnostic records something the extractor saw but could not turn into a
// fact (an ambiguous portal, an unreadable manifest). It is never a fact.
type Diagnostic struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

// Result is the sealed facts artifact for one repository run.
type Result struct {
	Version     int          `json:"version"`
	Revision    string       `json:"revision"`
	Targets     []Target     `json:"targets"`
	Facts       []Fact       `json:"facts"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	SHA256      string       `json:"sha256"`
}

// NewFactID derives the stable, target-scoped id of one fact from its root,
// kind, anchor path, the trimmed content of the anchored line, and the
// principal literal (method+path, key, symbol...). Callers that see a
// collision append an ordinal through WithOrdinal.
func NewFactID(root string, kind Kind, anchorPath string, lineContent string, principal ...string) string {
	hasher := sha256.New()
	hasher.Write([]byte(idDomain))
	for _, part := range append([]string{root, string(kind), anchorPath, strings.TrimSpace(lineContent)}, principal...) {
		hasher.Write([]byte(part))
		hasher.Write([]byte{0})
	}
	return "f-" + hex.EncodeToString(hasher.Sum(nil))[:idHexWidth]
}

// WithOrdinal disambiguates an id that collided with an earlier row.
func WithOrdinal(id string, ordinal int) string {
	if ordinal <= 1 {
		return id
	}
	return fmt.Sprintf("%s-%d", id, ordinal)
}

// NewTargetID derives a stable target id from its language, root and manifest.
func NewTargetID(language, root, manifest string) string {
	hasher := sha256.New()
	hasher.Write([]byte(idDomain))
	for _, part := range []string{"target", language, root, manifest} {
		hasher.Write([]byte(part))
		hasher.Write([]byte{0})
	}
	return "t-" + hex.EncodeToString(hasher.Sum(nil))[:idHexWidth]
}

// Seal sorts the rows canonically, checks the shape, and computes the digest.
func Seal(result Result) (Result, error) {
	owned := clone(result)
	owned.Version = Version
	if owned.Targets == nil {
		owned.Targets = []Target{}
	}
	if owned.Facts == nil {
		owned.Facts = []Fact{}
	}
	if owned.Diagnostics == nil {
		owned.Diagnostics = []Diagnostic{}
	}
	sortTargets(owned.Targets)
	sortFacts(owned.Facts)
	sortDiagnostics(owned.Diagnostics)
	digest, err := resultDigest(owned)
	if err != nil {
		return Result{}, err
	}
	owned.SHA256 = digest
	if err := owned.Validate(); err != nil {
		return Result{}, err
	}
	return owned, nil
}

// Validate checks the closed shape, canonical order, and the seal.
func (result Result) Validate() error {
	if result.Version != Version {
		return fmt.Errorf("facts: unsupported version %d", result.Version)
	}
	if result.Targets == nil || result.Facts == nil || result.Diagnostics == nil {
		return fmt.Errorf("facts: collections are missing")
	}
	if result.Revision != "" && !validRevision(result.Revision) {
		return fmt.Errorf("facts: invalid revision %q", result.Revision)
	}
	targetIDs := make(map[string]struct{}, len(result.Targets))
	for position, target := range result.Targets {
		if err := target.validate(); err != nil {
			return fmt.Errorf("facts: target %d: %w", position, err)
		}
		if _, duplicate := targetIDs[target.ID]; duplicate {
			return fmt.Errorf("facts: duplicate target id %q", target.ID)
		}
		targetIDs[target.ID] = struct{}{}
		if position > 0 && !targetLess(result.Targets[position-1], target) {
			return fmt.Errorf("facts: targets are not canonical at %d", position)
		}
	}
	factIDs := make(map[string]struct{}, len(result.Facts))
	for position, fact := range result.Facts {
		if err := fact.validate(targetIDs); err != nil {
			return fmt.Errorf("facts: fact %d (%s): %w", position, fact.ID, err)
		}
		if _, duplicate := factIDs[fact.ID]; duplicate {
			return fmt.Errorf("facts: duplicate fact id %q", fact.ID)
		}
		factIDs[fact.ID] = struct{}{}
		if position > 0 && !factLess(result.Facts[position-1], fact) {
			return fmt.Errorf("facts: facts are not canonical at %d", position)
		}
	}
	for _, fact := range result.Facts {
		for _, ref := range fact.Refs {
			if _, ok := factIDs[ref]; !ok {
				return fmt.Errorf("facts: fact %s references unknown fact %q", fact.ID, ref)
			}
		}
	}
	for position, diagnostic := range result.Diagnostics {
		if !validText(diagnostic.Kind) || !validText(diagnostic.Detail) {
			return fmt.Errorf("facts: diagnostic %d is invalid", position)
		}
		if position > 0 && !diagnosticLess(result.Diagnostics[position-1], diagnostic) {
			return fmt.Errorf("facts: diagnostics are not canonical at %d", position)
		}
	}
	digest, err := resultDigest(result)
	if err != nil {
		return err
	}
	if digest != result.SHA256 {
		return fmt.Errorf("facts: digest mismatch")
	}
	return nil
}

// Snapshot validates and returns an independently owned copy.
func (result Result) Snapshot() (Result, error) {
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return clone(result), nil
}

// ByID indexes facts by id.
func (result Result) ByID() map[string]Fact {
	index := make(map[string]Fact, len(result.Facts))
	for _, fact := range result.Facts {
		index[fact.ID] = fact
	}
	return index
}

// OfKind returns the facts of one kind in canonical order.
func (result Result) OfKind(kind Kind) []Fact {
	var rows []Fact
	for _, fact := range result.Facts {
		if fact.Kind == kind {
			rows = append(rows, fact)
		}
	}
	return rows
}

// TargetByID returns one target.
func (result Result) TargetByID(id string) (Target, bool) {
	for _, target := range result.Targets {
		if target.ID == id {
			return target, true
		}
	}
	return Target{}, false
}

func (target Target) validate() error {
	if !validID(target.ID, "t-") {
		return fmt.Errorf("invalid target id %q", target.ID)
	}
	if !validText(target.Language) || !validText(target.Name) || !validText(target.Kind) {
		return fmt.Errorf("target text is invalid")
	}
	if target.Root != "" && target.Root != "." {
		if err := validateRepositoryPath(target.Root); err != nil {
			return fmt.Errorf("root: %w", err)
		}
	}
	if target.Manifest != "" {
		if err := validateRepositoryPath(target.Manifest); err != nil {
			return fmt.Errorf("manifest: %w", err)
		}
	}
	if err := target.Anchor.validate(); err != nil {
		return err
	}
	if target.ProgramTargetID != "" && !validText(target.ProgramTargetID) {
		return fmt.Errorf("invalid program target id")
	}
	if target.RunID != "" && !validText(target.RunID) {
		return fmt.Errorf("invalid run id")
	}
	return nil
}

func (fact Fact) validate(targets map[string]struct{}) error {
	if !validID(fact.ID, "f-") {
		return fmt.Errorf("invalid fact id")
	}
	if !fact.Kind.Valid() {
		return fmt.Errorf("invalid kind %q", fact.Kind)
	}
	if fact.TargetID != "" {
		if _, ok := targets[fact.TargetID]; !ok {
			return fmt.Errorf("unknown target %q", fact.TargetID)
		}
	}
	if fact.PeerTargetID != "" {
		if _, ok := targets[fact.PeerTargetID]; !ok {
			return fmt.Errorf("unknown peer target %q", fact.PeerTargetID)
		}
	}
	if fact.Anchor != nil {
		if err := fact.Anchor.validate(); err != nil {
			return err
		}
	}
	for _, text := range []string{fact.Symbol, fact.ObjectID, fact.Method, fact.Path, fact.Key, fact.Value, fact.Text} {
		if text != "" && !validText(text) {
			return fmt.Errorf("invalid text field")
		}
	}
	if fact.Resolution != "" && fact.Resolution != ResolutionExact && fact.Resolution != ResolutionPossible {
		return fmt.Errorf("invalid resolution %q", fact.Resolution)
	}
	for _, evidence := range fact.Evidence {
		if err := evidence.validate(); err != nil {
			return err
		}
	}
	switch fact.Kind {
	case KindHTTPRoute, KindHTTPCall, KindPortal:
		if fact.Method == "" || fact.Path == "" || fact.Anchor == nil {
			return fmt.Errorf("%s requires method, path and anchor", fact.Kind)
		}
		if fact.Kind == KindPortal && len(fact.Refs) != 2 {
			return fmt.Errorf("portal requires exactly two refs")
		}
	case KindConfigRead, KindRisk, KindManifest, KindNegative, KindDependency:
		if fact.Key == "" {
			return fmt.Errorf("%s requires key", fact.Kind)
		}
	case KindTODO:
		if fact.Text == "" || fact.Anchor == nil {
			return fmt.Errorf("todo requires text and anchor")
		}
	case KindDeadModule, KindImport:
		if fact.Path == "" {
			return fmt.Errorf("%s requires path", fact.Kind)
		}
	case KindEntrypoint:
		if fact.Anchor == nil {
			return fmt.Errorf("entrypoint requires anchor")
		}
	}
	return nil
}

func sortTargets(targets []Target) {
	sort.SliceStable(targets, func(i, j int) bool { return targetLess(targets[i], targets[j]) })
}

func targetLess(a, b Target) bool {
	if a.Root != b.Root {
		return a.Root < b.Root
	}
	if a.Language != b.Language {
		return a.Language < b.Language
	}
	return a.ID < b.ID
}

func sortFacts(facts []Fact) {
	sort.SliceStable(facts, func(i, j int) bool { return factLess(facts[i], facts[j]) })
}

func factLess(a, b Fact) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.TargetID != b.TargetID {
		return a.TargetID < b.TargetID
	}
	aPath, bPath := anchorPath(a), anchorPath(b)
	if aPath != bPath {
		return aPath < bPath
	}
	aLine, bLine := anchorLine(a), anchorLine(b)
	if aLine != bLine {
		return aLine < bLine
	}
	if a.Key != b.Key {
		return a.Key < b.Key
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	return a.ID < b.ID
}

func anchorPath(fact Fact) string {
	if fact.Anchor == nil {
		return ""
	}
	return fact.Anchor.Path
}

func anchorLine(fact Fact) int {
	if fact.Anchor == nil {
		return 0
	}
	return fact.Anchor.Line
}

func sortDiagnostics(rows []Diagnostic) {
	sort.SliceStable(rows, func(i, j int) bool { return diagnosticLess(rows[i], rows[j]) })
}

func diagnosticLess(a, b Diagnostic) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return a.Detail < b.Detail
}

func resultDigest(result Result) (string, error) {
	unsealed := clone(result)
	unsealed.SHA256 = ""
	encoded, err := json.Marshal(unsealed)
	if err != nil {
		return "", fmt.Errorf("facts: digest: %w", err)
	}
	hasher := sha256.New()
	hasher.Write([]byte(digestDomain))
	hasher.Write(encoded)
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// clone copies every collection. An empty slice must stay empty rather than
// becoming nil: a sealed result distinguishes "no rows" from "not built".
func clone(result Result) Result {
	owned := result
	owned.Targets = cloneSlice(result.Targets)
	owned.Facts = make([]Fact, len(result.Facts))
	for position, fact := range result.Facts {
		copied := fact
		if fact.Anchor != nil {
			anchor := *fact.Anchor
			copied.Anchor = &anchor
		}
		copied.Refs = cloneSlice(fact.Refs)
		copied.Evidence = cloneSlice(fact.Evidence)
		owned.Facts[position] = copied
	}
	owned.Diagnostics = cloneSlice(result.Diagnostics)
	return owned
}

// cloneSlice copies a slice and preserves the difference between an empty
// slice and a missing one.
func cloneSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	return append(make([]T, 0, len(values)), values...)
}

func validID(value string, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) < len(prefix)+idHexWidth {
		return false
	}
	body := value[len(prefix):]
	for i, r := range body {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f':
		case r == '-' && i >= idHexWidth:
		default:
			return false
		}
	}
	return true
}

func validRevision(value string) bool {
	if len(value) < 7 || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	if value == "" || !utf8.ValidString(value) {
		return false
	}
	if strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r == '\n' || r == '\r' || r == 0 {
			return false
		}
	}
	return true
}

func validateRepositoryPath(value string) error {
	if value == "" || !utf8.ValidString(value) {
		return fmt.Errorf("facts: empty path")
	}
	if strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || path.Clean(value) != value {
		return fmt.Errorf("facts: path %q is not canonical repository-relative", value)
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("facts: path %q has an invalid segment", value)
		}
	}
	return nil
}
