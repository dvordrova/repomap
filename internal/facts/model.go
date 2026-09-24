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
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	Version          = 3
	ArtifactFilename = "facts.json"

	digestDomain = "repomap-facts-v3\x00"
	idDomain     = "repomap-fact-id-v1\x00"
	idHexWidth   = 16
)

// Kind is the closed vocabulary of fact rows.
type Kind string

const (
	// KindEntrypoint is a real execution root proved by a language adapter
	// (main guard, bound module object, callable seed, manifest script).
	KindEntrypoint Kind = "entrypoint"
	// KindRegistration is a call the repository does not own that hands over a
	// repository callable, an address-like literal, or a literal to a value
	// holding callables: a route, a consumer, a timer, a client request, a
	// server start. Its shape is a fact; what it is, the reading stage decides.
	KindRegistration Kind = "registration"
	// KindSQLQuery is an SQL statement literal handed to a call the repository
	// does not own, with the tables it names.
	KindSQLQuery Kind = "sql_query"
	// KindConfigRead is an environment/config key read.
	KindConfigRead Kind = "config_read"
	// KindDynamicExecution marks a place where the program runs code it was
	// given rather than code you can read: exec, eval, subprocess, a
	// deserializer that can construct objects. It is an orientation fact, not
	// a security finding: it tells a reader where static reading stops.
	KindDynamicExecution Kind = "dynamic_execution"
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
	// Extension entities and relationships preserve a producer's observations;
	// their human-readable labels are not architecture classifications.
	KindEntity   Kind = "entity"
	KindRelation Kind = "relation"
)

func (kind Kind) Valid() bool {
	switch kind {
	case KindEntrypoint, KindRegistration, KindSQLQuery, KindConfigRead,
		KindDynamicExecution, KindManifest, KindTODO,
		KindDeadModule, KindNegative, KindDependency, KindImport, KindEntity, KindRelation:
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
	NegativeNoTests      = "no_tests"
	NegativeNoDockerfile = "no_dockerfile"
	NegativeNoCI         = "no_ci"
	// The next four are what a person handing a repository over is asked
	// about first, and what a first-day reader looks for before the code:
	// under which terms, how to contribute, what changed, and what keeps
	// the code in one style.
	NegativeNoLicense      = "no_license"
	NegativeNoContributing = "no_contributing"
	NegativeNoChangelog    = "no_changelog"
	NegativeNoLinter       = "no_linter_config"
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
	ID       string `json:"id"`
	Language string `json:"language"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Root     string `json:"root"`
	Manifest string `json:"manifest,omitempty"`
	Anchor   Anchor `json:"anchor"`
}

// Fact is one anchored row. Only the fields meaningful for its Kind are set:
//
//	entrypoint  Symbol, ObjectID, Key (seed kind)
//	registration Key (call word), Values (literals), Path (address literal, mount
//	            prefixes applied), Method (verb the word or literal states),
//	            Symbol/ObjectID (callable handed over), Text (external symbol
//	            behind the call), Evidence (prefix/value sites)
//	sql_query   Value (statement), Key (tables), Symbol/ObjectID (caller)
//	config_read Key (env key), Value (literal default), Symbol
//	dynamic_execution Key (what runs the code), Symbol, Text (source line)
//	manifest    Key (dotted manifest key), Value
//	todo        Text
//	dead_module Path (file)
//	negative    Key (negative name), Text (detail)
//	dependency  Key (package), Value (declared version)
//	import      Path (imported file)
//	entity      Key (local id), Symbol (name), Path, Value (corpus presence)
//	relation    Key (label), Refs [from entity id, to entity id], Anchor
type Fact struct {
	Data         *DataObject `json:"data,omitempty"`
	ID           string      `json:"id"`
	Kind         Kind        `json:"kind"`
	TargetID     string      `json:"target_id,omitempty"`
	PeerTargetID string      `json:"peer_target_id,omitempty"`
	Anchor       *Anchor     `json:"anchor,omitempty"`
	// Holder is the value a registration acts on, at the call that produced
	// it: the router a route is put into, the server that is started.
	Holder   *Anchor `json:"holder,omitempty"`
	Symbol   string  `json:"symbol,omitempty"`
	ObjectID string  `json:"object_id,omitempty"`
	// OwnerID is the declaration that makes the call a registration records,
	// as distinct from the callable it hands over.
	OwnerID string `json:"owner_id,omitempty"`
	// Handed marks a registration that hands over a value of the
	// repository's own (an instance, a module) rather than a callable.
	Handed     bool       `json:"handed,omitempty"`
	Method     string     `json:"method,omitempty"`
	Path       string     `json:"path,omitempty"`
	Key        string     `json:"key,omitempty"`
	Value      string     `json:"value,omitempty"`
	Text       string     `json:"text,omitempty"`
	Values     []string   `json:"values,omitempty"`
	Resolution Resolution `json:"resolution,omitempty"`
	Refs       []string   `json:"refs,omitempty"`
	Evidence   []Anchor   `json:"evidence,omitempty"`
	// Extractor names the producer of an extension fact, including built-ins.
	Extractor string `json:"extractor,omitempty"`
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

// NewFactID derives an extraction-local, target-scoped key from its root,
// kind, anchor path, the trimmed content of the anchored line, and the
// principal literal (method+path, key, symbol...). Seal replaces it with the
// artifact-owned compact aN identity after canonical ordering. Callers that
// see a collision append an ordinal through WithOrdinal.
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
	factIDs := make(map[string]string, len(owned.Facts))
	for position := range owned.Facts {
		oldID := owned.Facts[position].ID
		if _, duplicate := factIDs[oldID]; duplicate {
			return Result{}, fmt.Errorf("facts: duplicate extraction key %q", oldID)
		}
		factIDs[oldID] = fmt.Sprintf("a%d", position+1)
		owned.Facts[position].ID = factIDs[oldID]
	}
	for position := range owned.Facts {
		for refPosition, oldRef := range owned.Facts[position].Refs {
			newRef, known := factIDs[oldRef]
			if !known {
				return Result{}, fmt.Errorf("facts: fact references unknown extraction key %q", oldRef)
			}
			owned.Facts[position].Refs[refPosition] = newRef
		}
	}
	sortDiagnostics(owned.Diagnostics)
	// Identical diagnostics say nothing more twice.
	owned.Diagnostics = slices.Compact(owned.Diagnostics)
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
		if fact.ID != fmt.Sprintf("a%d", position+1) {
			return fmt.Errorf("facts: fact IDs are not canonical")
		}
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
	if !programindex.ValidTargetID(target.ID) {
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
	return nil
}

func (fact Fact) validate(targets map[string]struct{}) error {
	if fact.ID == "" {
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
	for _, text := range []string{fact.Symbol, fact.ObjectID, fact.Method, fact.Key, fact.Extractor} {
		if text != "" && !validText(text) {
			return fmt.Errorf("invalid text field")
		}
	}
	// A path, a value and a symbol text are what the source says, newlines
	// included; only what no text may hold is refused.
	for _, text := range []string{fact.Path, fact.Value, fact.Text} {
		if text != "" && (!utf8.ValidString(text) || strings.ContainsRune(text, 0)) {
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
	case KindRegistration:
		if fact.Key == "" || fact.Anchor == nil {
			return fmt.Errorf("registration requires its call word and anchor")
		}
		for _, value := range fact.Values {
			if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
				return fmt.Errorf("invalid registration value")
			}
		}
	case KindSQLQuery:
		if fact.Value == "" || fact.Anchor == nil {
			return fmt.Errorf("sql_query requires statement and anchor")
		}
	case KindConfigRead, KindDynamicExecution, KindManifest, KindNegative, KindDependency:
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
	case KindEntity:
		if err := fact.Data.Validate(); err != nil {
			return err
		}
		if fact.Key == "" || fact.Extractor == "" || fact.Path == "" && fact.Symbol == "" {
			return fmt.Errorf("entity requires local identity, producer and path or name")
		}
		if fact.Path != "" && fact.Path != "." {
			if err := validateRepositoryPath(fact.Path); err != nil {
				return err
			}
		}
	case KindRelation:
		if fact.Key == "" || len(fact.Refs) != 2 || fact.Anchor == nil || fact.Extractor == "" {
			return fmt.Errorf("relation requires a label, two entities, producer and source anchor")
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
	return programindex.TargetIDLess(a.ID, b.ID)
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
	return compactFactIDLess(a.ID, b.ID)
}

func compactFactIDLess(left, right string) bool {
	if strings.HasPrefix(left, "a") && strings.HasPrefix(right, "a") {
		leftOrdinal, leftErr := strconv.Atoi(strings.TrimPrefix(left, "a"))
		rightOrdinal, rightErr := strconv.Atoi(strings.TrimPrefix(right, "a"))
		if leftErr == nil && rightErr == nil {
			return leftOrdinal < rightOrdinal
		}
	}
	return left < right
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
		copied.Data = CloneData(fact.Data)
		if fact.Anchor != nil {
			anchor := *fact.Anchor
			copied.Anchor = &anchor
		}
		copied.Refs = cloneSlice(fact.Refs)
		copied.Values = cloneSlice(fact.Values)
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
