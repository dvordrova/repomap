// Package workspacesearch provides bounded presentation-neutral lexical search
// over one immutable authorized source catalog and optional exact source
// entities. It owns no report, HTTP, browser, provider, or editorial types.
package workspacesearch

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
)

const (
	maxCatalogEntries = 4096
	maxSymbolEntries  = 1024
	maxEntries        = maxCatalogEntries + maxSymbolEntries
	maxResults        = 20

	maxQueryBytes    = 256
	maxPathBytes     = 4096
	maxEntityIDBytes = 512
	maxNameBytes     = 256
	maxLanguageBytes = 32
)

// Kind is an exact repository result class.
type Kind string

const (
	KindFile     Kind = "file"
	KindDocument Kind = "document"
	KindSymbol   Kind = "symbol"
)

// Entry is one authorized exact repository fact. Entity is populated only for
// KindSymbol and is suitable for direct use by exact-inspection consumers.
type Entry struct {
	Kind     Kind
	Name     string
	Path     string
	Location *evidence.Location
	Entity   *evidence.Entity
}

// Input binds construction to exactly one source catalog. Symbols are optional
// analyzer/local-fact projections; malformed or outside-catalog symbols are
// omitted rather than widening catalog authority.
type Input struct {
	Catalog sourcecatalog.Catalog
	Symbols []evidence.Entity
}

// Index is an immutable bounded lexical index.
type Index struct {
	entries []Entry
	records []searchRecord
}

// Query is one bounded exact lexical lookup. Zero MaxResults selects the
// retained browser limit of 20; positive values may only narrow it.
type Query struct {
	Text       string
	MaxResults int
}

// MatchKind records the strongest exact lexical relationship to the query.
type MatchKind string

const (
	MatchExactPath MatchKind = "exact_path"
	MatchExactName MatchKind = "exact_name"
	MatchPrefix    MatchKind = "prefix"
	MatchSubstring MatchKind = "substring"
)

// Match is a defensively copied search selection.
type Match struct {
	Entry Entry
	Match MatchKind
}

type searchRecord struct {
	entry Entry
	name  string
	path  string
}

// New constructs a bounded index without reading repository files.
func New(input Input) (Index, error) {
	if input.Catalog.AnalysisRoot() == "" {
		return Index{}, fmt.Errorf("workspace search: source catalog is required")
	}
	if input.Catalog.Len() > maxCatalogEntries {
		return Index{}, fmt.Errorf("workspace search: source catalog exceeds %d entries", maxCatalogEntries)
	}
	if len(input.Symbols) > maxSymbolEntries {
		return Index{}, fmt.Errorf("workspace search: symbols exceed %d entries", maxSymbolEntries)
	}

	paths := input.Catalog.Paths()
	for index, sourcePath := range paths {
		if len(sourcePath) > maxPathBytes {
			return Index{}, fmt.Errorf("workspace search: source path %d exceeds %d bytes", index, maxPathBytes)
		}
	}
	for index, entity := range input.Symbols {
		if symbolScalarOversized(entity) {
			return Index{}, fmt.Errorf("workspace search: symbol %d contains an oversized scalar", index)
		}
	}
	validSymbols := make([]evidence.Entity, 0, min(len(input.Symbols), maxSymbolEntries))
	for _, entity := range input.Symbols {
		if validSymbol(entity, input.Catalog) {
			validSymbols = append(validSymbols, cloneEntity(entity))
		}
	}
	sort.Slice(validSymbols, func(i, j int) bool {
		return entityLess(validSymbols[i], validSymbols[j])
	})

	capacity := len(paths) + len(validSymbols)
	if capacity > maxEntries {
		capacity = maxEntries
	}
	entries := make([]Entry, 0, capacity)
	seen := make(map[string]struct{}, capacity)
	for _, sourcePath := range paths {
		source, ok := input.Catalog.Lookup(sourcePath)
		if !ok || source.Path != sourcePath || !safeScalar(sourcePath, maxPathBytes, true) {
			return Index{}, fmt.Errorf("workspace search: source catalog contains an invalid path")
		}
		kind := KindFile
		if documentPath(sourcePath) {
			kind = KindDocument
		}
		entry := Entry{
			Kind: kind,
			Name: path.Base(sourcePath),
			Path: sourcePath,
			Location: &evidence.Location{
				Path: sourcePath,
			},
		}
		key := entryKey(entry)
		seen[key] = struct{}{}
		entries = append(entries, entry)
	}

	for _, entity := range validSymbols {
		copy := cloneEntity(entity)
		entry := Entry{
			Kind:     KindSymbol,
			Name:     copy.Name,
			Path:     copy.Location.Path,
			Location: cloneLocation(copy.Location),
			Entity:   &copy,
		}
		key := entryKey(entry)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return entityID(left) < entityID(right)
	})

	records := make([]searchRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, searchRecord{
			entry: cloneEntry(entry),
			name:  strings.ToLower(entry.Name),
			path:  strings.ToLower(entry.Path),
		})
	}
	return Index{entries: cloneEntries(entries), records: records}, nil
}

// Entries returns every authorized entry in deterministic order.
func (index Index) Entries() []Entry {
	return cloneEntries(index.entries)
}

// Search returns at most 20 deterministic exact lexical matches.
func (index Index) Search(query Query) ([]Match, error) {
	if len(query.Text) > maxQueryBytes {
		return nil, fmt.Errorf("workspace search: query exceeds %d bytes", maxQueryBytes)
	}
	if !utf8.ValidString(query.Text) || containsControl(query.Text) {
		return nil, fmt.Errorf("workspace search: query is invalid")
	}
	text := strings.TrimSpace(query.Text)
	if text == "" {
		return nil, fmt.Errorf("workspace search: query is empty")
	}
	limit := query.MaxResults
	if limit == 0 {
		limit = maxResults
	}
	if limit < 1 || limit > maxResults {
		return nil, fmt.Errorf("workspace search: result limit must be between 1 and %d", maxResults)
	}

	wanted := strings.ToLower(text)
	ranked := make([]rankedMatch, 0, maxResults)
	for _, record := range index.records {
		match, ok := matchRecord(record, wanted)
		if !ok {
			continue
		}
		ranked = append(ranked, rankedMatch{record: record, match: match})
	}
	sort.Slice(ranked, func(i, j int) bool {
		left, right := ranked[i], ranked[j]
		leftPriority := resultPriority(left.match, left.record.entry.Kind)
		rightPriority := resultPriority(right.match, right.record.entry.Kind)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if left.record.path != right.record.path {
			return left.record.path < right.record.path
		}
		if left.record.name != right.record.name {
			return left.record.name < right.record.name
		}
		if left.record.entry.Path != right.record.entry.Path {
			return left.record.entry.Path < right.record.entry.Path
		}
		if left.record.entry.Name != right.record.entry.Name {
			return left.record.entry.Name < right.record.entry.Name
		}
		return entityID(left.record.entry) < entityID(right.record.entry)
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	result := make([]Match, 0, limit)
	for _, candidate := range ranked {
		result = append(result, Match{
			Entry: cloneEntry(candidate.record.entry),
			Match: candidate.match,
		})
	}
	return result, nil
}

type rankedMatch struct {
	record searchRecord
	match  MatchKind
}

func matchRecord(record searchRecord, query string) (MatchKind, bool) {
	if record.path == query {
		return MatchExactPath, true
	}
	if record.name == query {
		return MatchExactName, true
	}
	if strings.HasPrefix(record.name, query) || strings.HasPrefix(record.path, query) {
		return MatchPrefix, true
	}
	if strings.Contains(record.name, query) || strings.Contains(record.path, query) {
		return MatchSubstring, true
	}
	return "", false
}

func resultPriority(match MatchKind, kind Kind) int {
	switch match {
	case MatchExactPath:
		if kind == KindSymbol {
			return 1
		}
		return 0
	case MatchExactName:
		if kind == KindSymbol {
			return 2
		}
		return 3
	case MatchPrefix:
		return 4 + kindPriority(kind)
	default:
		return 8 + kindPriority(kind)
	}
}

func kindPriority(kind Kind) int {
	switch kind {
	case KindSymbol:
		return 0
	case KindDocument:
		return 1
	default:
		return 2
	}
}

func symbolScalarOversized(entity evidence.Entity) bool {
	return len(entity.ID) > maxEntityIDBytes ||
		len(entity.Name) > maxNameBytes ||
		len(entity.Language) > maxLanguageBytes ||
		(entity.Location != nil && len(entity.Location.Path) > maxPathBytes)
}

func validSymbol(entity evidence.Entity, catalog sourcecatalog.Catalog) bool {
	root := catalog.AnalysisRoot()
	if entity.ID == "" || !safePublicScalar(entity.ID, root, maxEntityIDBytes, true) ||
		entity.Name == "" || !safePublicScalar(entity.Name, root, maxNameBytes, true) ||
		(entity.Language != "" && !safePublicScalar(entity.Language, root, maxLanguageBytes, true)) ||
		!searchableEntityKind(entity.Kind) ||
		(entity.Scope != "" && entity.Scope != evidence.SourceScopeRepository) ||
		entity.Location == nil || !validExactLocation(*entity.Location) {
		return false
	}
	source, ok := catalog.Lookup(entity.Location.Path)
	return ok && source.Path == entity.Location.Path
}

func searchableEntityKind(kind evidence.EntityKind) bool {
	switch kind {
	case evidence.EntityFunction,
		evidence.EntityMethod,
		evidence.EntityType,
		evidence.EntityInterface,
		evidence.EntityField,
		evidence.EntityVariable,
		evidence.EntityConstant,
		evidence.EntityTest,
		evidence.EntityEntrypoint,
		evidence.EntityReference:
		return true
	default:
		return false
	}
}

func validExactLocation(location evidence.Location) bool {
	if !safeScalar(location.Path, maxPathBytes, true) ||
		location.Line <= 0 || location.Column < 0 ||
		location.EndLine < 0 || location.EndColumn < 0 {
		return false
	}
	if location.EndLine > 0 && location.EndLine < location.Line {
		return false
	}
	return location.EndLine != 0 || location.EndColumn == 0
}

func entityLess(left, right evidence.Entity) bool {
	if left.ID != right.ID {
		return left.ID < right.ID
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	if left.Language != right.Language {
		return left.Language < right.Language
	}
	if left.Scope != right.Scope {
		return left.Scope < right.Scope
	}
	leftLocation, rightLocation := left.Location, right.Location
	if leftLocation.Path != rightLocation.Path {
		return leftLocation.Path < rightLocation.Path
	}
	if leftLocation.Line != rightLocation.Line {
		return leftLocation.Line < rightLocation.Line
	}
	if leftLocation.Column != rightLocation.Column {
		return leftLocation.Column < rightLocation.Column
	}
	if leftLocation.EndLine != rightLocation.EndLine {
		return leftLocation.EndLine < rightLocation.EndLine
	}
	return leftLocation.EndColumn < rightLocation.EndColumn
}

func safeScalar(value string, limit int, required bool) bool {
	if len(value) > limit {
		return false
	}
	if !utf8.ValidString(value) || containsControl(value) {
		return false
	}
	if required && value == "" {
		return false
	}
	return strings.TrimSpace(value) == value
}

func safePublicScalar(value, root string, limit int, required bool) bool {
	if !safeScalar(value, limit, required) {
		return false
	}
	if strings.Contains(value, root) || strings.Contains(value, "file://") {
		return false
	}
	for _, field := range strings.Fields(value) {
		trimmed := strings.Trim(field, `"'(),:;[]`)
		if filepath.IsAbs(trimmed) {
			return false
		}
		if index := strings.Index(trimmed, "="); index >= 0 {
			suffix := strings.Trim(trimmed[index+1:], `"'(),:;[]`)
			if filepath.IsAbs(suffix) {
				return false
			}
		}
	}
	return true
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func documentPath(value string) bool {
	switch strings.ToLower(path.Ext(value)) {
	case ".md", ".markdown", ".rst", ".adoc", ".txt":
		return true
	default:
		return false
	}
}

func entryKey(entry Entry) string {
	return string(entry.Kind) + "\x00" + entry.Path + "\x00" + entry.Name + "\x00" + entityID(entry)
}

func entityID(entry Entry) string {
	if entry.Entity == nil {
		return ""
	}
	return entry.Entity.ID
}

func cloneEntries(values []Entry) []Entry {
	result := make([]Entry, 0, len(values))
	for _, value := range values {
		result = append(result, cloneEntry(value))
	}
	return result
}

func cloneEntry(entry Entry) Entry {
	entry.Location = cloneLocation(entry.Location)
	if entry.Entity != nil {
		entity := cloneEntity(*entry.Entity)
		entry.Entity = &entity
	}
	return entry
}

func cloneEntity(entity evidence.Entity) evidence.Entity {
	entity.Location = cloneLocation(entity.Location)
	return entity
}

func cloneLocation(location *evidence.Location) *evidence.Location {
	if location == nil {
		return nil
	}
	copy := *location
	return &copy
}
