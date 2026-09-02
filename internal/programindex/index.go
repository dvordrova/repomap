// Package programindex owns the sealed, language-neutral handoff from a
// language adapter to the semantic domain cubes.
//
// IDs in this package are stable local identities. Provider-facing code must
// still replace them with bounded request-local refs before model execution.
package programindex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	Version          = 11
	ArtifactFilename = "program-index.json"

	// These exported values are advisory scale thresholds. ProgramIndex does
	// not use them as local collection caps; valid repository scale is retained
	// completely and crossing these values produces diagnostics only.
	MaxTargetSources        = 4_096
	MaxTargetSeeds          = 4_096
	MaxObjects              = 131_072
	MaxRelations            = 262_144
	MaxTargetsPerRelation   = 64
	MaxWitnessesPerRelation = 64
	MaxPatternsPerRelation  = 524_288
	MaxArgumentsPerPattern  = 128
	MaxPatternParts         = 64
	MaxObjectsPerPatternRef = 64
	// These former alias/re-export bounds are advisory scale thresholds. Exact
	// compiler-owned identities and all of their parts remain lossless; the
	// aggregate ProgramIndex envelopes are the only size authority.
	MaxSymbolLinkIdentitiesPerObject = 16
	MaxSymbolLinkIdentityParts       = 16
	MaxWitnesses                     = 524_288
	MaxPatterns                      = 524_288
	MaxPatternArguments              = 2_097_152
	// MaxTextBytes is advisory. Individual semantic strings remain lossless.
	MaxTextBytes = 16 * 1024
	// AdvisoryAggregateTextBytes and AdvisoryIndexBytes are the former local
	// artifact thresholds. They drive warnings only. The Max* names remain as
	// zero compatibility sentinels for readers that interpret zero as unbounded.
	AdvisoryAggregateTextBytes = 64 * 1024 * 1024
	AdvisoryIndexBytes         = 128 * 1024 * 1024
	MaxAggregateTextBytes      = 0
	MaxIndexBytes              = 0
	// MaxObservedCount is the former portable-count ceiling. It is retained as
	// an advisory warning threshold only; int is the representation authority.
	MaxObservedCount = 1<<31 - 1
)

// ObjectKind is deliberately small and language-neutral. Language adapters
// retain richer declaration kinds in their private indexes.
type ObjectKind string

const (
	ObjectModule         ObjectKind = "module"
	ObjectPackage        ObjectKind = "package"
	ObjectType           ObjectKind = "type"
	ObjectFunction       ObjectKind = "function"
	ObjectMethod         ObjectKind = "method"
	ObjectLambda         ObjectKind = "lambda"
	ObjectVariable       ObjectKind = "variable"
	ObjectExternalSymbol ObjectKind = "external_symbol"
)

func (kind ObjectKind) Valid() bool {
	switch kind {
	case ObjectModule, ObjectPackage, ObjectType, ObjectFunction, ObjectMethod,
		ObjectLambda, ObjectVariable, ObjectExternalSymbol:
		return true
	default:
		return false
	}
}

// Visibility is the language-neutral reachability fact needed to distinguish
// public target APIs from implementation objects. Unknown is explicit when an
// adapter cannot establish that boundary.
type Visibility string

const (
	VisibilityPublic   Visibility = "public"
	VisibilityInternal Visibility = "internal"
	VisibilityUnknown  Visibility = "unknown"
)

func (visibility Visibility) Valid() bool {
	return visibility == VisibilityPublic || visibility == VisibilityInternal || visibility == VisibilityUnknown
}

// RelationKind describes structural program facts, not product semantics.
type RelationKind string

const (
	RelationCalls           RelationKind = "calls"
	RelationContains        RelationKind = "contains"
	RelationImports         RelationKind = "imports"
	RelationImplements      RelationKind = "implements"
	RelationDecorates       RelationKind = "decorates"
	RelationPassesCallback  RelationKind = "passes_callback"
	RelationSources         RelationKind = "sources"
	RelationExecutes        RelationKind = "executes"
	RelationReads           RelationKind = "reads"
	RelationWrites          RelationKind = "writes"
	RelationInvokesExternal RelationKind = "invokes_external"
)

func (kind RelationKind) Valid() bool {
	switch kind {
	case RelationCalls, RelationContains, RelationImports, RelationImplements,
		RelationDecorates, RelationPassesCallback, RelationSources,
		RelationExecutes, RelationReads, RelationWrites, RelationInvokesExternal:
		return true
	default:
		return false
	}
}

// Resolution is the closed amount of authority a language adapter has for a
// relation's retained target set.
type Resolution string

const (
	ResolutionExact Resolution = "exact"
	// ResolutionAlternatives retains one or more locally observed possible
	// targets without claiming that runtime dispatch is exact. The retained
	// set can contain a single syntactic candidate in a dynamic language; any
	// adapter-known omissions remain explicit in TargetsOmitted.
	ResolutionAlternatives Resolution = "alternatives"
	ResolutionUnresolved   Resolution = "unresolved"
)

func (resolution Resolution) Valid() bool {
	return resolution == ResolutionExact || resolution == ResolutionAlternatives || resolution == ResolutionUnresolved
}

// Location is an exact repository-relative source position.
type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// TargetSource binds one repository-corpus file identity to its exact
// repository-relative path. Keeping the pair together prevents consumers from
// guessing the ref/path association by array position or lexical order.
type TargetSource struct {
	FileRef string `json:"file_ref"`
	Path    string `json:"path"`
}

// SeedKind states the exact structural reason an object can begin execution
// for the selected target. It does not claim that the object runs in every
// process invocation.
type SeedKind string

const (
	SeedCallable    SeedKind = "callable"
	SeedModule      SeedKind = "module"
	SeedMainGuard   SeedKind = "main_guard"
	SeedScript      SeedKind = "script"
	SeedBoundObject SeedKind = "bound_object"
)

func (kind SeedKind) Valid() bool {
	switch kind {
	case SeedCallable, SeedModule, SeedMainGuard, SeedScript, SeedBoundObject:
		return true
	default:
		return false
	}
}

// TargetSeedInput binds an adapter-local object ref to one exact launch fact.
type TargetSeedInput struct {
	ObjectRef string
	Kind      SeedKind
	Location  *Location
}

// TargetSeed is the sealed language-neutral launch handoff consumed by later
// semantic cubes and presentation projections.
type TargetSeed struct {
	ObjectID string    `json:"object_id"`
	Kind     SeedKind  `json:"kind"`
	Location *Location `json:"location"`
}

// TargetInput is the adapter-owned target scope before local identity sealing.
// Sources are exact source/root/manifest evidence. AnchorFileRef is one member
// selected only as the stable display and identity anchor. Selector is the
// adapter-owned declaration key that distinguishes otherwise identical target
// views, such as Python console_scripts and gui_scripts aliases.
type TargetInput struct {
	Language      string
	Kind          string
	Name          string
	Selector      string
	Sources       []TargetSource
	AnchorFileRef string
	// Seeds establish where execution can begin for this selected target.
	// Libraries may leave the set empty; a later semantic cube can then choose
	// from public objects without pretending they are launch roots.
	Seeds []TargetSeedInput
}

// Target is one exact selected program scope. It remains independent of a
// provider request and can cover several executable roots or library sources.
type Target struct {
	ID            string         `json:"id"`
	Language      string         `json:"language"`
	Kind          string         `json:"kind"`
	Name          string         `json:"name"`
	Selector      string         `json:"selector"`
	Sources       []TargetSource `json:"sources"`
	AnchorFileRef string         `json:"anchor_file_ref"`
	Seeds         []TargetSeed   `json:"seeds"`
}

// Snapshot returns a consumer-owned copy of the selected target boundary.
func (target Target) Snapshot() Target {
	result := target
	result.Sources = cloneTargetSources(target.Sources)
	result.Seeds = cloneTargetSeeds(target.Seeds)
	return result
}

// Validate checks the standalone target shape and its stable identity. This
// lets manifests and report adapters bind the same language-neutral target
// without retaining an entire Index in memory.
func (target Target) Validate() error {
	if err := validateTargetShape(target); err != nil {
		return err
	}
	if target.ID != targetIdentity(target) {
		return fmt.Errorf("program index: target identity mismatch")
	}
	return nil
}

// ObjectInput is one adapter fact before SourceRef relationships are resolved
// to stable program-index IDs.
type ObjectInput struct {
	SourceRef string
	Kind      ObjectKind
	// Name is presentation text, never identity. Adapters keep repository
	// source paths in Location instead of concatenating them into this field.
	// Logical module/package names may retain their language-native spelling.
	Name         string
	Visibility   Visibility
	Signature    string
	OwnerRef     string
	ContainerRef string
	Location     *Location
	// SymbolLinkIdentities are adapter-normalized, exact identities that may
	// join this object to the same symbol in another sealed ProgramIndex shard.
	// The common builder owns opaque keys, canonical order and deduplication.
	SymbolLinkIdentities []SymbolLinkIdentityInput
	// External is adapter-owned origin and symbol authority. It is valid only
	// for ObjectExternalSymbol and avoids forcing consumers to recover package
	// or platform boundaries from presentation text or raw identity syntax.
	External *ExternalSymbol
}

// SymbolLinkIdentityInput is the only adapter-facing cross-shard symbol
// identity constructor. Domain namespaces one adapter/language identity
// scheme. Parts are already normalized by that adapter; ProgramIndex treats
// them as opaque ordered bytes and never parses their meaning. Display is
// optional presentation text and carries no matching authority.
type SymbolLinkIdentityInput struct {
	Domain  string
	Parts   []string
	Display string
}

// SymbolLinkIdentity is the sealed portable identity retained on an object.
// Consumers compare only the exact (Domain, Key) pair. Key is constructed by
// the common builder from the ordered input parts, so consumers never need to
// reproduce or understand adapter normalization.
type SymbolLinkIdentity struct {
	Domain  string `json:"domain"`
	Key     string `json:"key"`
	Display string `json:"display,omitempty"`
	// PartCount preserves only tuple size for warning-only scale diagnostics.
	// Key remains the matching authority; raw parts are never persisted.
	PartCount int `json:"part_count"`
}

// ExternalAuthorityKind is the adapter-established origin class for an
// external symbol. PackagePath remains the exact raw language-tool identity;
// consumers use this closed kind rather than parsing that identity.
type ExternalAuthorityKind string

const (
	ExternalAuthorityPackage  ExternalAuthorityKind = "package"
	ExternalAuthorityPlatform ExternalAuthorityKind = "platform"
)

func (kind ExternalAuthorityKind) Valid() bool {
	return kind == ExternalAuthorityPackage || kind == ExternalAuthorityPlatform
}

// ExternalSymbol is the exact language-tool identity of an external program
// object. PackagePath is the raw import/package identity used to join package
// authorities to dependencies.Catalog. Receiver is optional because free
// functions and package variables do not have one.
type ExternalSymbol struct {
	AuthorityKind ExternalAuthorityKind `json:"authority_kind"`
	PackagePath   string                `json:"package_path"`
	Receiver      string                `json:"receiver,omitempty"`
	Name          string                `json:"name"`
}

// IsExternalPackageAuthority reports exact adapter-owned package authority.
func IsExternalPackageAuthority(value *ExternalSymbol) bool {
	return value != nil && value.AuthorityKind == ExternalAuthorityPackage
}

// IsExternalPlatformAuthority reports exact adapter-owned standard-runtime
// platform authority.
func IsExternalPlatformAuthority(value *ExternalSymbol) bool {
	return value != nil && value.AuthorityKind == ExternalAuthorityPlatform
}

// Object is one language-neutral program declaration or external symbol.
// Signature, ownership, containment and location are optional because not all
// adapters can establish them with exact local authority.
type Object struct {
	ID                   string               `json:"id"`
	SourceRef            string               `json:"source_ref"`
	Kind                 ObjectKind           `json:"kind"`
	Name                 string               `json:"name"`
	Visibility           Visibility           `json:"visibility"`
	Signature            string               `json:"signature,omitempty"`
	OwnerID              string               `json:"owner_id,omitempty"`
	ContainerID          string               `json:"container_id,omitempty"`
	Location             *Location            `json:"location,omitempty"`
	SymbolLinkIdentities []SymbolLinkIdentity `json:"symbol_link_identities,omitempty"`
	External             *ExternalSymbol      `json:"external,omitempty"`
}

// Witness preserves one bounded local fact supporting a relation. Kind and
// Detail remain adapter facts; they are not model-authored semantics.
// SourceExpression is an optional exact, adapter-observed expression from the
// repository source. Consumers may interpret it according to the relation and
// witness kinds without parsing human-oriented Detail text.
type Witness struct {
	Kind             string    `json:"kind"`
	Detail           string    `json:"detail,omitempty"`
	SourceExpression string    `json:"source_expression,omitempty"`
	Location         *Location `json:"location,omitempty"`
}

// PatternForm is the closed syntactic shape retained for adapter-neutral
// pattern classification. It describes source syntax only, never framework or
// protocol semantics.
type PatternForm string

const (
	PatternCall          PatternForm = "call"
	PatternDecoratorCall PatternForm = "decorator_call"
)

func (form PatternForm) Valid() bool {
	return form == PatternCall || form == PatternDecoratorCall
}

// PatternValueKind states how much exact string structure the adapter retained
// for one call argument.
type PatternValueKind string

const (
	PatternLiteralString  PatternValueKind = "literal_string"
	PatternStringTemplate PatternValueKind = "string_template"
	PatternDynamic        PatternValueKind = "dynamic"
)

func (kind PatternValueKind) Valid() bool {
	return kind == PatternLiteralString || kind == PatternStringTemplate || kind == PatternDynamic
}

// PatternPartKind is one closed component of a string template. Hole names are
// deliberately not retained: only literal text carries matching authority.
type PatternPartKind string

const (
	PatternPartLiteral PatternPartKind = "literal"
	PatternPartHole    PatternPartKind = "hole"
)

func (kind PatternPartKind) Valid() bool {
	return kind == PatternPartLiteral || kind == PatternPartHole
}

type PatternPartInput struct {
	Kind PatternPartKind
	Text string
}

type PatternPart struct {
	Kind PatternPartKind `json:"kind"`
	Text string          `json:"text,omitempty"`
}

// PatternValueResolution states whether a locally reconstructed argument
// value is exact for the use or remains one possible runtime value. This is
// deliberately separate from Relation.Resolution: a mutable language binding
// may name one exact source object while its initializer is still only a
// possible value at the later use.
type PatternValueResolution string

const (
	PatternValueExact    PatternValueResolution = "exact"
	PatternValuePossible PatternValueResolution = "possible"
)

func (resolution PatternValueResolution) Valid() bool {
	return resolution == PatternValueExact || resolution == PatternValuePossible
}

// PatternValueSourceKind is the adapter-neutral structural joint used to
// recover a value without asking a model to copy it. New source kinds belong
// here only when a language adapter can retain their complete local evidence.
type PatternValueSourceKind string

const (
	PatternValueSourceInitializer    PatternValueSourceKind = "initializer"
	PatternValueSourceActualArgument PatternValueSourceKind = "actual_argument"
)

func (kind PatternValueSourceKind) Valid() bool {
	return kind == PatternValueSourceInitializer || kind == PatternValueSourceActualArgument
}

// PatternValueCandidateInput is one adapter-observed value reconstruction for
// a dynamic argument. SourceObjectRefs and SourceArgumentRefs bind the
// reconstruction to canonical ProgramIndex identities during sealing.
type PatternValueCandidateInput struct {
	Kind                    PatternValueKind
	Value                   string
	Parts                   []PatternPartInput
	Resolution              PatternValueResolution
	SourceKind              PatternValueSourceKind
	SourceObjectRefs        []string
	SourceObjectsObserved   int
	SourceArgumentRefs      []PatternArgumentRefInput
	SourceArgumentsObserved int
}

// PatternValueCandidate is one sealed, identity-bound value reconstruction.
// ID includes its owning argument, value shape, authority, provenance kind,
// and canonical source objects or arguments, so it cannot be moved between
// uses.
type PatternValueCandidate struct {
	ID                      string                 `json:"id"`
	Kind                    PatternValueKind       `json:"kind"`
	Value                   string                 `json:"value,omitempty"`
	Parts                   []PatternPart          `json:"parts"`
	Resolution              PatternValueResolution `json:"resolution"`
	SourceKind              PatternValueSourceKind `json:"source_kind"`
	SourceObjectIDs         []string               `json:"source_object_ids"`
	SourceObjectsObserved   int                    `json:"source_objects_observed"`
	SourceObjectsOmitted    int                    `json:"source_objects_omitted"`
	SourceArgumentIDs       []string               `json:"source_argument_ids"`
	SourceArgumentsObserved int                    `json:"source_arguments_observed"`
	SourceArgumentsOmitted  int                    `json:"source_arguments_omitted"`
}

// PatternArgumentInput is one adapter-observed positional or keyword
// argument. Exactly one of Position (one-based) and Keyword is set.
type PatternArgumentInput struct {
	Position                int
	Keyword                 string
	Kind                    PatternValueKind
	Value                   string
	Parts                   []PatternPartInput
	ObjectRefs              []string
	Resolution              Resolution
	ObjectsObserved         int
	ValueCandidates         []PatternValueCandidateInput
	ValueCandidatesObserved int
}

// PatternArgumentRefInput identifies one exact nested argument without asking
// an adapter to predict canonical ProgramIndex IDs. RelationSourceRef selects
// the owning relation, PatternSourceRef selects its nested pattern, and the
// positional/keyword key selects the argument. Resolution is fail-closed when
// that adapter-local tuple is absent or ambiguous.
type PatternArgumentRefInput struct {
	RelationSourceRef string
	PatternSourceRef  string
	Position          int
	Keyword           string
}

// PatternArgument is one sealed argument. ID is stable under input ordering
// and is derived from its owning pattern plus its positional or keyword key.
type PatternArgument struct {
	ID                      string                  `json:"id"`
	Position                int                     `json:"position,omitempty"`
	Keyword                 string                  `json:"keyword,omitempty"`
	Kind                    PatternValueKind        `json:"kind"`
	Value                   string                  `json:"value,omitempty"`
	Parts                   []PatternPart           `json:"parts"`
	ObjectIDs               []string                `json:"object_ids"`
	Resolution              Resolution              `json:"resolution,omitempty"`
	ObjectsObserved         int                     `json:"objects_observed"`
	ObjectsOmitted          int                     `json:"objects_omitted"`
	ValueCandidates         []PatternValueCandidate `json:"value_candidates"`
	ValueCandidatesObserved int                     `json:"value_candidates_observed"`
	ValueCandidatesOmitted  int                     `json:"value_candidates_omitted"`
}

// RelationPatternInput retains one bounded syntactic candidate nested in its
// owning relation. Object refs are temporary joins within the same Input.
type RelationPatternInput struct {
	SourceRef                string
	Form                     PatternForm
	Selector                 string
	Location                 *Location
	ResultRef                string
	ReceiverRef              string
	ReceiverOriginRefs       []string
	ReceiverOriginResolution Resolution
	ReceiverOriginsObserved  int
	Arguments                []PatternArgumentInput
	ArgumentsObserved        int
}

// RelationPattern is a sealed source-syntax candidate. Its identity is local
// to the owning relation; SourceRef therefore needs to be unique only there.
type RelationPattern struct {
	ID                       string            `json:"id"`
	SourceRef                string            `json:"source_ref"`
	Form                     PatternForm       `json:"form"`
	Selector                 string            `json:"selector"`
	Location                 *Location         `json:"location,omitempty"`
	ResultID                 string            `json:"result_id,omitempty"`
	ReceiverID               string            `json:"receiver_id,omitempty"`
	ReceiverOriginIDs        []string          `json:"receiver_origin_ids"`
	ReceiverOriginResolution Resolution        `json:"receiver_origin_resolution,omitempty"`
	ReceiverOriginsObserved  int               `json:"receiver_origins_observed"`
	ReceiverOriginsOmitted   int               `json:"receiver_origins_omitted"`
	Arguments                []PatternArgument `json:"arguments"`
	ArgumentsObserved        int               `json:"arguments_observed"`
	ArgumentsOmitted         int               `json:"arguments_omitted"`
}

// RelationInput cites ObjectInput.SourceRef values. Invocation is optional,
// advisory text (for example a language's synchronous/deferred distinction),
// never a cross-language enum or stronger call authority. TargetsObserved and
// WitnessesObserved are mandatory adapter measurements; the core never derives
// them from retained rows.
type RelationInput struct {
	SourceRef         string
	Kind              RelationKind
	FromRef           string
	ToRefs            []string
	Resolution        Resolution
	Invocation        string
	Location          *Location
	TargetsObserved   int
	Witnesses         []Witness
	WitnessesObserved int
	Patterns          []RelationPatternInput
	PatternsObserved  int
	SourceArgument    *PatternArgumentRefInput
}

// Relation is one typed, locally resolved edge or uncertainty joint.
type Relation struct {
	ID                string            `json:"id"`
	SourceRef         string            `json:"source_ref"`
	Kind              RelationKind      `json:"kind"`
	FromID            string            `json:"from_id"`
	ToIDs             []string          `json:"to_ids"`
	Resolution        Resolution        `json:"resolution"`
	Invocation        string            `json:"invocation,omitempty"`
	Location          *Location         `json:"location,omitempty"`
	TargetsObserved   int               `json:"targets_observed"`
	TargetsOmitted    int               `json:"targets_omitted"`
	Witnesses         []Witness         `json:"witnesses"`
	WitnessesObserved int               `json:"witnesses_observed"`
	WitnessesOmitted  int               `json:"witnesses_omitted"`
	Patterns          []RelationPattern `json:"patterns"`
	PatternsObserved  int               `json:"patterns_observed"`
	PatternsOmitted   int               `json:"patterns_omitted"`
	SourceArgumentID  string            `json:"source_argument_id,omitempty"`
}

// CoverageInput retains adapter observations that could not all be represented
// by bounded Object and Relation rows. Measured is mandatory: the core never
// invents a complete ledger from the rows that happened to survive an adapter.
type CoverageInput struct {
	Measured          bool
	ObjectsObserved   int
	RelationsObserved int
}

// Coverage makes both index contents and the adapter's omission frontier
// explicit. Target, witness and nested-pattern omissions are aggregated from
// Relation rows.
type Coverage struct {
	ObjectsObserved              int `json:"objects_observed"`
	ObjectsIndexed               int `json:"objects_indexed"`
	ObjectsOmitted               int `json:"objects_omitted"`
	RelationsObserved            int `json:"relations_observed"`
	RelationsIndexed             int `json:"relations_indexed"`
	RelationsOmitted             int `json:"relations_omitted"`
	ExactRelations               int `json:"exact_relations"`
	AlternativeRelations         int `json:"alternative_relations"`
	UnresolvedRelations          int `json:"unresolved_relations"`
	TargetsObserved              int `json:"targets_observed"`
	TargetsIndexed               int `json:"targets_indexed"`
	TargetsOmitted               int `json:"targets_omitted"`
	WitnessesObserved            int `json:"witnesses_observed"`
	WitnessesIndexed             int `json:"witnesses_indexed"`
	WitnessesOmitted             int `json:"witnesses_omitted"`
	PatternsObserved             int `json:"patterns_observed"`
	PatternsIndexed              int `json:"patterns_indexed"`
	PatternsOmitted              int `json:"patterns_omitted"`
	ArgumentsObserved            int `json:"arguments_observed"`
	ArgumentsIndexed             int `json:"arguments_indexed"`
	ArgumentsOmitted             int `json:"arguments_omitted"`
	ReceiverOriginsObserved      int `json:"receiver_origins_observed"`
	ReceiverOriginsIndexed       int `json:"receiver_origins_indexed"`
	ReceiverOriginsOmitted       int `json:"receiver_origins_omitted"`
	ArgumentObjectsObserved      int `json:"argument_objects_observed"`
	ArgumentObjectsIndexed       int `json:"argument_objects_indexed"`
	ArgumentObjectsOmitted       int `json:"argument_objects_omitted"`
	ArgumentValuesObserved       int `json:"argument_values_observed"`
	ArgumentValuesIndexed        int `json:"argument_values_indexed"`
	ArgumentValuesOmitted        int `json:"argument_values_omitted"`
	ValueSourcesObserved         int `json:"value_sources_observed"`
	ValueSourcesIndexed          int `json:"value_sources_indexed"`
	ValueSourcesOmitted          int `json:"value_sources_omitted"`
	ValueArgumentSourcesObserved int `json:"value_argument_sources_observed"`
	ValueArgumentSourcesIndexed  int `json:"value_argument_sources_indexed"`
	ValueArgumentSourcesOmitted  int `json:"value_argument_sources_omitted"`
}

type Input struct {
	ScenarioSHA256 string
	SourceSHA256   string
	Target         TargetInput
	Objects        []ObjectInput
	Relations      []RelationInput
	Coverage       CoverageInput
}

// Index is the canonical, bounded and SHA-sealed language-neutral handoff.
type Index struct {
	Version        int             `json:"version"`
	ScenarioSHA256 string          `json:"scenario_sha256"`
	SourceSHA256   string          `json:"source_sha256"`
	Target         Target          `json:"target"`
	Objects        []Object        `json:"objects"`
	Relations      []Relation      `json:"relations"`
	Coverage       Coverage        `json:"coverage"`
	Categorization *Categorization `json:"categorization,omitempty"`
	SHA256         string          `json:"sha256"`
}

// Encode validates and returns the canonical JSON artifact bytes.
func Encode(index Index) ([]byte, error) {
	if err := index.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(index)
	if err != nil {
		return nil, fmt.Errorf("program index: encode artifact: %w", err)
	}
	return encoded, nil
}

// Decode strictly decodes one JSON artifact, rejects unknown fields and
// trailing JSON values, then validates identities, references and the seal.
func Decode(encoded []byte) (Index, error) {
	if len(encoded) == 0 {
		return Index{}, fmt.Errorf("program index: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var index Index
	if err := decoder.Decode(&index); err != nil {
		return Index{}, fmt.Errorf("program index: decode artifact: %w", err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Index{}, fmt.Errorf("program index: trailing JSON value")
		}
		return Index{}, fmt.Errorf("program index: trailing data: %w", err)
	}
	if err := index.Validate(); err != nil {
		return Index{}, err
	}
	return index, nil
}

// New resolves adapter-local source refs, assigns stable local identities,
// canonicalizes all collections, derives coverage, validates and seals the
// result. It never truncates or rejects an input at an advisory collection
// threshold.
func New(input Input) (Index, error) {
	targetSources, err := canonicalizeTargetSources(input.Target.Sources)
	if err != nil {
		return Index{}, err
	}
	index := Index{
		Version:        Version,
		ScenarioSHA256: input.ScenarioSHA256,
		SourceSHA256:   input.SourceSHA256,
		Target: Target{
			Language: input.Target.Language, Kind: input.Target.Kind, Name: input.Target.Name,
			Selector: input.Target.Selector, Sources: targetSources, AnchorFileRef: input.Target.AnchorFileRef,
			Seeds: []TargetSeed{},
		},
		Objects:   make([]Object, 0, len(input.Objects)),
		Relations: make([]Relation, 0, len(input.Relations)),
	}
	if err := validateTargetShape(index.Target); err != nil {
		return Index{}, err
	}
	seedInputs, err := canonicalizeTargetSeedInputs(input.Target.Seeds)
	if err != nil {
		return Index{}, err
	}
	objectScopeID := targetObjectScopeIdentity(index.Target)

	bindings := make([]objectBinding, 0, len(input.Objects))
	for _, value := range input.Objects {
		if err := validateObjectInput(value); err != nil {
			return Index{}, err
		}
		linkIdentities, err := canonicalizeSymbolLinkIdentityInputs(value.SymbolLinkIdentities)
		if err != nil {
			return Index{}, fmt.Errorf("program index: object %q symbol link identities: %w", value.SourceRef, err)
		}
		object := Object{
			ID: stableID("program-object", objectScopeID, value.SourceRef), SourceRef: value.SourceRef,
			Kind: value.Kind, Name: value.Name, Visibility: value.Visibility,
			Signature: value.Signature, Location: cloneLocation(value.Location),
			SymbolLinkIdentities: linkIdentities, External: cloneExternalSymbol(value.External),
		}
		index.Objects = append(index.Objects, object)
		bindings = append(bindings, objectBinding{SourceRef: value.SourceRef, ID: object.ID})
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].SourceRef < bindings[j].SourceRef })
	for position := 1; position < len(bindings); position++ {
		if bindings[position-1].SourceRef == bindings[position].SourceRef {
			return Index{}, fmt.Errorf("program index: duplicate object source ref %q", bindings[position].SourceRef)
		}
	}
	for position, value := range input.Objects {
		ownerID, err := resolveObjectRef(bindings, value.OwnerRef)
		if err != nil {
			return Index{}, fmt.Errorf("program index: object %q owner: %w", value.SourceRef, err)
		}
		containerID, err := resolveObjectRef(bindings, value.ContainerRef)
		if err != nil {
			return Index{}, fmt.Errorf("program index: object %q container: %w", value.SourceRef, err)
		}
		if ownerID == index.Objects[position].ID || containerID == index.Objects[position].ID {
			return Index{}, fmt.Errorf("program index: object %q owns or contains itself", value.SourceRef)
		}
		index.Objects[position].OwnerID = ownerID
		index.Objects[position].ContainerID = containerID
	}
	sort.Slice(index.Objects, func(i, j int) bool { return index.Objects[i].ID < index.Objects[j].ID })
	for position := 1; position < len(index.Objects); position++ {
		if index.Objects[position-1].ID == index.Objects[position].ID {
			return Index{}, fmt.Errorf("program index: duplicate object identity %q", index.Objects[position].ID)
		}
	}
	index.Target.Seeds = make([]TargetSeed, 0, len(seedInputs))
	for _, seedInput := range seedInputs {
		id, err := resolveObjectRef(bindings, seedInput.ObjectRef)
		if err != nil {
			return Index{}, fmt.Errorf("program index: target seed: %w", err)
		}
		object, ok := objectWithID(index.Objects, id)
		if !ok || object.Kind == ObjectExternalSymbol {
			return Index{}, fmt.Errorf("program index: target seed %q is not a local program object", seedInput.ObjectRef)
		}
		index.Target.Seeds = append(index.Target.Seeds, TargetSeed{
			ObjectID: id,
			Kind:     seedInput.Kind,
			Location: cloneLocation(seedInput.Location),
		})
	}
	sort.Slice(index.Target.Seeds, func(i, j int) bool {
		return compareTargetSeeds(index.Target.Seeds[i], index.Target.Seeds[j]) < 0
	})
	index.Target.ID = targetIdentity(index.Target)

	pendingSourceArguments := make(map[string]PatternArgumentRefInput)
	pendingValueSourceArguments := make(map[string]pendingPatternValueSourceArguments)
	for _, value := range input.Relations {
		if !validText(value.SourceRef) || !value.Kind.Valid() || !value.Resolution.Valid() || !validText(value.FromRef) ||
			!validOptionalText(value.Invocation) || !validOptionalLocation(value.Location) {
			return Index{}, fmt.Errorf("program index: invalid relation input")
		}
		fromID, err := resolveObjectRef(bindings, value.FromRef)
		if err != nil {
			return Index{}, fmt.Errorf("program index: relation %q source: %w", value.SourceRef, err)
		}
		toRefs := cloneStrings(value.ToRefs)
		for _, ref := range toRefs {
			if !validText(ref) {
				return Index{}, fmt.Errorf("program index: relation %q has invalid target ref", value.SourceRef)
			}
		}
		sort.Strings(toRefs)
		toRefs = compactStrings(toRefs)
		toIDs := make([]string, 0, len(toRefs))
		for _, ref := range toRefs {
			id, resolveErr := resolveObjectRef(bindings, ref)
			if resolveErr != nil {
				return Index{}, fmt.Errorf("program index: relation %q target: %w", value.SourceRef, resolveErr)
			}
			toIDs = append(toIDs, id)
		}
		sort.Strings(toIDs)
		toIDs = compactStrings(toIDs)

		witnesses, err := canonicalWitnesses(value.Witnesses)
		if err != nil {
			return Index{}, fmt.Errorf("program index: relation %q: %w", value.SourceRef, err)
		}
		relationID := stableID("program-relation", index.Target.ID, value.SourceRef, string(value.Kind), fromID)
		patterns, err := canonicalizeRelationPatterns(
			value.Patterns, relationID, bindings, pendingValueSourceArguments,
		)
		if err != nil {
			return Index{}, fmt.Errorf("program index: relation %q patterns: %w", value.SourceRef, err)
		}
		relation := Relation{
			ID:        relationID,
			SourceRef: value.SourceRef, Kind: value.Kind, FromID: fromID, ToIDs: toIDs,
			Resolution: value.Resolution, Invocation: value.Invocation, Location: cloneLocation(value.Location),
			TargetsObserved: value.TargetsObserved, TargetsOmitted: value.TargetsObserved - len(toIDs),
			Witnesses: witnesses, WitnessesObserved: value.WitnessesObserved,
			WitnessesOmitted: value.WitnessesObserved - len(witnesses),
			Patterns:         patterns, PatternsObserved: value.PatternsObserved,
			PatternsOmitted: value.PatternsObserved - len(patterns),
		}
		if value.SourceArgument != nil {
			if value.Kind != RelationPassesCallback || !validPatternArgumentRefInput(*value.SourceArgument) {
				return Index{}, fmt.Errorf("program index: relation %q has invalid source argument input", value.SourceRef)
			}
			pendingSourceArguments[relationID] = *value.SourceArgument
		}
		index.Relations = append(index.Relations, relation)
	}
	if err := resolvePatternValueSourceArgumentReferences(index.Relations, pendingValueSourceArguments); err != nil {
		return Index{}, err
	}
	for position := range index.Relations {
		reference, ok := pendingSourceArguments[index.Relations[position].ID]
		if !ok {
			continue
		}
		argumentID, err := resolvePatternArgumentReference(index.Relations, reference)
		if err != nil {
			return Index{}, fmt.Errorf("program index: relation %q source argument: %w", index.Relations[position].SourceRef, err)
		}
		index.Relations[position].SourceArgumentID = argumentID
	}
	for _, relation := range index.Relations {
		if err := validateRelationShape(relation); err != nil {
			return Index{}, err
		}
	}
	sort.Slice(index.Relations, func(i, j int) bool { return index.Relations[i].ID < index.Relations[j].ID })
	for position := 1; position < len(index.Relations); position++ {
		if index.Relations[position-1].ID == index.Relations[position].ID {
			return Index{}, fmt.Errorf("program index: duplicate relation identity %q", index.Relations[position].ID)
		}
	}

	if !input.Coverage.Measured {
		return Index{}, fmt.Errorf("program index: adapter coverage was not measured")
	}
	objectsObserved := input.Coverage.ObjectsObserved
	relationsObserved := input.Coverage.RelationsObserved
	index.Coverage = compileCoverage(index.Objects, index.Relations, objectsObserved, relationsObserved)
	if err := validateCoverage(index.Coverage, len(index.Objects), len(index.Relations)); err != nil {
		return Index{}, err
	}
	digest, err := indexDigest(index)
	if err != nil {
		return Index{}, err
	}
	index.SHA256 = digest
	if err := index.Validate(); err != nil {
		return Index{}, err
	}
	return index, nil
}

// Snapshot returns a deep copy suitable for a consumer-owned handoff.
func (index Index) Snapshot() Index {
	result := index
	result.Target = index.Target.Snapshot()
	result.Objects = make([]Object, len(index.Objects))
	copy(result.Objects, index.Objects)
	for position := range result.Objects {
		result.Objects[position].Location = cloneLocation(index.Objects[position].Location)
		result.Objects[position].SymbolLinkIdentities = cloneSymbolLinkIdentities(index.Objects[position].SymbolLinkIdentities)
		result.Objects[position].External = cloneExternalSymbol(index.Objects[position].External)
	}
	result.Relations = make([]Relation, len(index.Relations))
	copy(result.Relations, index.Relations)
	for position := range result.Relations {
		result.Relations[position].ToIDs = cloneStrings(index.Relations[position].ToIDs)
		result.Relations[position].Location = cloneLocation(index.Relations[position].Location)
		result.Relations[position].Witnesses = cloneWitnesses(index.Relations[position].Witnesses)
		result.Relations[position].Patterns = cloneRelationPatterns(index.Relations[position].Patterns)
	}
	result.Categorization = cloneCategorization(index.Categorization)
	return result
}

// Validate checks identity bindings, canonical order, references, bounds,
// coverage and the complete-index SHA seal.
func (index Index) Validate() error {
	if index.Version != Version || !validSHA256(index.ScenarioSHA256) || !validSHA256(index.SourceSHA256) {
		return fmt.Errorf("program index: invalid producer identity")
	}
	if index.Objects == nil || index.Relations == nil {
		return fmt.Errorf("program index: missing collections")
	}
	if err := index.Target.Validate(); err != nil {
		return err
	}
	for position, object := range index.Objects {
		if err := validateObject(object, targetObjectScopeIdentity(index.Target)); err != nil {
			return err
		}
		if position > 0 && index.Objects[position-1].ID >= object.ID {
			return fmt.Errorf("program index: objects are not canonical")
		}
	}
	for _, object := range index.Objects {
		if object.OwnerID != "" {
			owner, ok := objectWithID(index.Objects, object.OwnerID)
			if object.OwnerID == object.ID || !ok {
				return fmt.Errorf("program index: object %q has invalid owner", object.ID)
			}
			if object.Kind == ObjectMethod && owner.Kind != ObjectType {
				return fmt.Errorf("program index: method %q owner is not a type", object.ID)
			}
		}
		if object.ContainerID != "" {
			if object.ContainerID == object.ID || !hasObjectID(index.Objects, object.ContainerID) {
				return fmt.Errorf("program index: object %q has invalid container", object.ID)
			}
		}
	}
	for _, seed := range index.Target.Seeds {
		object, ok := objectWithID(index.Objects, seed.ObjectID)
		if !ok || object.Kind == ObjectExternalSymbol {
			return fmt.Errorf("program index: target has invalid seed object %q", seed.ObjectID)
		}
		if err := validateTargetSeedBinding(seed, object); err != nil {
			return err
		}
	}
	type patternArgumentAuthority struct {
		fromID         string
		resolution     Resolution
		targetCount    int
		targetsOmitted int
		argument       PatternArgument
	}
	argumentsByID := make(map[string]patternArgumentAuthority)
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				if _, exists := argumentsByID[argument.ID]; exists {
					return fmt.Errorf("program index: duplicate pattern argument identity %q", argument.ID)
				}
				argumentsByID[argument.ID] = patternArgumentAuthority{
					fromID: relation.FromID, resolution: relation.Resolution,
					targetCount: len(relation.ToIDs), targetsOmitted: relation.TargetsOmitted,
					argument: argument,
				}
			}
		}
	}
	for position, relation := range index.Relations {
		if err := validateRelationShape(relation); err != nil {
			return err
		}
		if relation.ID != stableID("program-relation", index.Target.ID, relation.SourceRef, string(relation.Kind), relation.FromID) {
			return fmt.Errorf("program index: relation identity mismatch")
		}
		if !hasObjectID(index.Objects, relation.FromID) {
			return fmt.Errorf("program index: relation %q has unknown source", relation.ID)
		}
		for _, id := range relation.ToIDs {
			if !hasObjectID(index.Objects, id) {
				return fmt.Errorf("program index: relation %q has unknown target", relation.ID)
			}
		}
		for _, pattern := range relation.Patterns {
			if pattern.ResultID != "" && !hasObjectID(index.Objects, pattern.ResultID) {
				return fmt.Errorf("program index: pattern %q has unknown result", pattern.ID)
			}
			if pattern.ReceiverID != "" && !hasObjectID(index.Objects, pattern.ReceiverID) {
				return fmt.Errorf("program index: pattern %q has unknown receiver", pattern.ID)
			}
			for _, id := range pattern.ReceiverOriginIDs {
				if !hasObjectID(index.Objects, id) {
					return fmt.Errorf("program index: pattern %q has unknown receiver origin", pattern.ID)
				}
			}
			for _, argument := range pattern.Arguments {
				for _, id := range argument.ObjectIDs {
					if !hasObjectID(index.Objects, id) {
						return fmt.Errorf("program index: pattern argument %q has unknown object", argument.ID)
					}
				}
				for _, candidate := range argument.ValueCandidates {
					for _, id := range candidate.SourceObjectIDs {
						if !hasObjectID(index.Objects, id) {
							return fmt.Errorf("program index: pattern value candidate %q has unknown source object", candidate.ID)
						}
					}
					for _, id := range candidate.SourceArgumentIDs {
						if id == argument.ID {
							return fmt.Errorf("program index: pattern value candidate %q cites its owning argument", candidate.ID)
						}
						authority, ok := argumentsByID[id]
						if !ok {
							return fmt.Errorf("program index: pattern value candidate %q has unknown source argument", candidate.ID)
						}
						if candidate.SourceKind == PatternValueSourceActualArgument &&
							(authority.resolution != ResolutionExact || authority.targetCount != 1 || authority.targetsOmitted != 0 ||
								!samePatternValue(candidate.Kind, candidate.Value, candidate.Parts, authority.argument)) {
							return fmt.Errorf("program index: pattern value candidate %q has incompatible actual source argument", candidate.ID)
						}
					}
				}
			}
		}
		if relation.SourceArgumentID != "" {
			authority, ok := argumentsByID[relation.SourceArgumentID]
			if !ok {
				return fmt.Errorf("program index: relation %q has unknown source argument", relation.ID)
			}
			argument := authority.argument
			targetsWithinArgument := true
			for _, targetID := range relation.ToIDs {
				if !slices.Contains(argument.ObjectIDs, targetID) {
					targetsWithinArgument = false
					break
				}
			}
			// A neutral argument may resolve to callable and non-callable
			// declarations for the same language symbol. The callback relation
			// retains only callable targets, so it is authoritative when those
			// targets are a measured subset of the source argument authority.
			if authority.fromID != relation.FromID || argument.Resolution != relation.Resolution ||
				!targetsWithinArgument || argument.ObjectsObserved != relation.TargetsObserved {
				return fmt.Errorf("program index: relation %q source argument authority mismatch", relation.ID)
			}
		}
		if position > 0 && index.Relations[position-1].ID >= relation.ID {
			return fmt.Errorf("program index: relations are not canonical")
		}
	}
	if err := validateCoverage(index.Coverage, len(index.Objects), len(index.Relations)); err != nil {
		return err
	}
	wantCoverage := compileCoverage(index.Objects, index.Relations, index.Coverage.ObjectsObserved, index.Coverage.RelationsObserved)
	if index.Coverage != wantCoverage {
		return fmt.Errorf("program index: coverage mismatch")
	}
	if err := validateCategorization(index); err != nil {
		return err
	}
	want, err := indexDigest(index)
	if err != nil {
		return err
	}
	if !validSHA256(index.SHA256) || index.SHA256 != want {
		return fmt.Errorf("program index: sha256 mismatch")
	}
	return nil
}

type objectBinding struct {
	SourceRef string
	ID        string
}

type pendingPatternValueSourceArguments struct {
	Refs     []PatternArgumentRefInput
	Observed int
}

func canonicalizeTargetSources(values []TargetSource) ([]TargetSource, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("program index: target has no source evidence")
	}
	result := cloneTargetSources(values)
	pathsByRef := make(map[string]string, len(result))
	refsByPath := make(map[string]string, len(result))
	for _, source := range result {
		if !validText(source.FileRef) || !validPath(source.Path) {
			return nil, fmt.Errorf("program index: invalid target source")
		}
		if previous, exists := pathsByRef[source.FileRef]; exists && previous != source.Path {
			return nil, fmt.Errorf("program index: target file ref %q has conflicting paths", source.FileRef)
		}
		if previous, exists := refsByPath[source.Path]; exists && previous != source.FileRef {
			return nil, fmt.Errorf("program index: target source path %q has conflicting file refs", source.Path)
		}
		pathsByRef[source.FileRef] = source.Path
		refsByPath[source.Path] = source.FileRef
	}
	sort.Slice(result, func(i, j int) bool { return compareTargetSources(result[i], result[j]) < 0 })
	compacted := result[:0]
	for _, source := range result {
		if len(compacted) == 0 || compareTargetSources(compacted[len(compacted)-1], source) != 0 {
			compacted = append(compacted, source)
		}
	}
	return compacted, nil
}

func canonicalizeTargetSeedInputs(values []TargetSeedInput) ([]TargetSeedInput, error) {
	result := make([]TargetSeedInput, len(values))
	copy(result, values)
	for position := range result {
		result[position].Location = cloneLocation(values[position].Location)
		seed := result[position]
		if !validText(seed.ObjectRef) || !seed.Kind.Valid() || seed.Location == nil || !validLocation(*seed.Location) {
			return nil, fmt.Errorf("program index: invalid target seed input")
		}
	}
	sort.Slice(result, func(i, j int) bool { return compareTargetSeedInputs(result[i], result[j]) < 0 })
	compacted := result[:0]
	for _, seed := range result {
		if len(compacted) == 0 || compareTargetSeedInputs(compacted[len(compacted)-1], seed) != 0 {
			compacted = append(compacted, seed)
		}
	}
	return compacted, nil
}

func compareTargetSources(left, right TargetSource) int {
	if order := strings.Compare(left.FileRef, right.FileRef); order != 0 {
		return order
	}
	return strings.Compare(left.Path, right.Path)
}

func compareTargetSeedInputs(left, right TargetSeedInput) int {
	if order := strings.Compare(left.ObjectRef, right.ObjectRef); order != 0 {
		return order
	}
	if order := strings.Compare(string(left.Kind), string(right.Kind)); order != 0 {
		return order
	}
	return strings.Compare(locationKey(left.Location), locationKey(right.Location))
}

func compareTargetSeeds(left, right TargetSeed) int {
	if order := strings.Compare(left.ObjectID, right.ObjectID); order != 0 {
		return order
	}
	if order := strings.Compare(string(left.Kind), string(right.Kind)); order != 0 {
		return order
	}
	return strings.Compare(locationKey(left.Location), locationKey(right.Location))
}

func hasTargetSourceRef(sources []TargetSource, wanted string) bool {
	position := sort.Search(len(sources), func(position int) bool {
		return sources[position].FileRef >= wanted
	})
	return position < len(sources) && sources[position].FileRef == wanted
}

func hasTargetSourcePath(sources []TargetSource, wanted string) bool {
	for _, source := range sources {
		if source.Path == wanted {
			return true
		}
	}
	return false
}

func resolveObjectRef(bindings []objectBinding, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	position := sort.Search(len(bindings), func(position int) bool { return bindings[position].SourceRef >= ref })
	if position == len(bindings) || bindings[position].SourceRef != ref {
		return "", fmt.Errorf("unknown object ref %q", ref)
	}
	return bindings[position].ID, nil
}

func hasObjectID(objects []Object, id string) bool {
	position := sort.Search(len(objects), func(position int) bool { return objects[position].ID >= id })
	return position < len(objects) && objects[position].ID == id
}

func objectWithID(objects []Object, id string) (Object, bool) {
	position := sort.Search(len(objects), func(position int) bool { return objects[position].ID >= id })
	if position == len(objects) || objects[position].ID != id {
		return Object{}, false
	}
	return objects[position], true
}

func validateTargetShape(target Target) error {
	if !validText(target.Language) || !validText(target.Kind) || !validText(target.Name) ||
		!validText(target.Selector) || len(target.Sources) == 0 ||
		!validText(target.AnchorFileRef) || !hasTargetSourceRef(target.Sources, target.AnchorFileRef) ||
		target.Seeds == nil {
		return fmt.Errorf("program index: invalid target")
	}
	pathsByRef := make(map[string]string, len(target.Sources))
	refsByPath := make(map[string]string, len(target.Sources))
	for position, source := range target.Sources {
		if !validText(source.FileRef) || !validPath(source.Path) {
			return fmt.Errorf("program index: invalid target source")
		}
		if position > 0 && compareTargetSources(target.Sources[position-1], source) >= 0 {
			return fmt.Errorf("program index: target sources are not canonical")
		}
		if previous, exists := pathsByRef[source.FileRef]; exists && previous != source.Path {
			return fmt.Errorf("program index: target file ref %q has conflicting paths", source.FileRef)
		}
		if previous, exists := refsByPath[source.Path]; exists && previous != source.FileRef {
			return fmt.Errorf("program index: target source path %q has conflicting file refs", source.Path)
		}
		pathsByRef[source.FileRef] = source.Path
		refsByPath[source.Path] = source.FileRef
	}
	for position, seed := range target.Seeds {
		if !validText(seed.ObjectID) || !seed.Kind.Valid() || seed.Location == nil || !validLocation(*seed.Location) ||
			!hasTargetSourcePath(target.Sources, seed.Location.Path) {
			return fmt.Errorf("program index: invalid target seed")
		}
		if position > 0 && compareTargetSeeds(target.Seeds[position-1], seed) >= 0 {
			return fmt.Errorf("program index: target seeds are not canonical")
		}
	}
	return nil
}

func validateObjectInput(value ObjectInput) error {
	if !value.Visibility.Valid() {
		return fmt.Errorf("program index: invalid object visibility")
	}
	if !validText(value.SourceRef) || !value.Kind.Valid() || !validText(value.Name) ||
		!validOptionalText(value.Signature) || !validOptionalText(value.OwnerRef) ||
		!validOptionalText(value.ContainerRef) || !validOptionalLocation(value.Location) {
		return fmt.Errorf("program index: invalid object input")
	}
	if err := validateExternalSymbolBinding(value.Kind, value.External); err != nil {
		return err
	}
	return nil
}

func validateObject(value Object, objectScopeID string) error {
	if !canonicalSymbolLinkIdentities(value.SymbolLinkIdentities) {
		return fmt.Errorf("program index: invalid object symbol link identities")
	}
	if !validText(value.SourceRef) || !value.Kind.Valid() || !validText(value.Name) || !value.Visibility.Valid() ||
		!validOptionalText(value.Signature) || !validOptionalText(value.OwnerID) ||
		!validOptionalText(value.ContainerID) || !validOptionalLocation(value.Location) {
		return fmt.Errorf("program index: invalid object")
	}
	if err := validateExternalSymbolBinding(value.Kind, value.External); err != nil {
		return err
	}
	if value.ID != stableID("program-object", objectScopeID, value.SourceRef) {
		return fmt.Errorf("program index: object identity mismatch")
	}
	return nil
}

func validateTargetSeedBinding(seed TargetSeed, object Object) error {
	compatible := false
	sameLine := false
	switch seed.Kind {
	case SeedCallable:
		compatible = object.Kind == ObjectFunction || object.Kind == ObjectMethod || object.Kind == ObjectLambda
		sameLine = true
	case SeedModule:
		compatible = object.Kind == ObjectModule || object.Kind == ObjectPackage
		sameLine = true
	case SeedMainGuard, SeedScript:
		compatible = object.Kind == ObjectModule || object.Kind == ObjectPackage
	case SeedBoundObject:
		compatible = object.Kind == ObjectVariable || object.Kind == ObjectType
		sameLine = true
	}
	if !compatible || object.Location == nil || seed.Location == nil || object.Location.Path != seed.Location.Path ||
		sameLine && object.Location.Line != seed.Location.Line {
		return fmt.Errorf("program index: target seed is incompatible with object %q", object.ID)
	}
	return nil
}

func validateRelationShape(value Relation) error {
	if !validText(value.ID) || !validText(value.SourceRef) || !value.Kind.Valid() || !value.Resolution.Valid() ||
		!validText(value.FromID) || !validOptionalText(value.Invocation) || !validOptionalLocation(value.Location) ||
		!validOptionalText(value.SourceArgumentID) ||
		value.ToIDs == nil || value.Witnesses == nil || value.Patterns == nil ||
		!canonicalStrings(value.ToIDs) {
		return fmt.Errorf("program index: invalid relation")
	}
	if value.SourceArgumentID != "" && value.Kind != RelationPassesCallback {
		return fmt.Errorf("program index: source argument is only valid for callback transfer")
	}
	if value.TargetsObserved <= 0 || value.TargetsObserved < len(value.ToIDs) ||
		value.TargetsOmitted != value.TargetsObserved-len(value.ToIDs) ||
		value.WitnessesObserved <= 0 || value.WitnessesObserved < len(value.Witnesses) ||
		value.WitnessesOmitted != value.WitnessesObserved-len(value.Witnesses) ||
		value.PatternsObserved < len(value.Patterns) ||
		value.PatternsOmitted != value.PatternsObserved-len(value.Patterns) {
		return fmt.Errorf("program index: invalid relation coverage")
	}
	previousWitness := ""
	for _, witness := range value.Witnesses {
		if err := validateWitness(witness); err != nil {
			return err
		}
		key := witnessKey(witness)
		if previousWitness != "" && previousWitness >= key {
			return fmt.Errorf("program index: witnesses are not canonical")
		}
		previousWitness = key
	}
	for position, pattern := range value.Patterns {
		if err := validateRelationPatternShape(pattern, value.ID); err != nil {
			return err
		}
		if position > 0 && value.Patterns[position-1].ID >= pattern.ID {
			return fmt.Errorf("program index: relation patterns are not canonical")
		}
	}
	switch value.Resolution {
	case ResolutionExact:
		if len(value.ToIDs) != 1 || value.TargetsOmitted != 0 || len(value.Witnesses) == 0 {
			return fmt.Errorf("program index: invalid exact relation")
		}
	case ResolutionAlternatives:
		if len(value.ToIDs) < 1 || len(value.Witnesses) == 0 {
			return fmt.Errorf("program index: invalid alternatives relation")
		}
	case ResolutionUnresolved:
		if len(value.ToIDs) != 0 {
			return fmt.Errorf("program index: invalid unresolved relation")
		}
	}
	return nil
}

func validateWitness(value Witness) error {
	if !validText(value.Kind) || !validOptionalText(value.Detail) ||
		!validOptionalText(value.SourceExpression) || !validOptionalLocation(value.Location) {
		return fmt.Errorf("program index: invalid witness")
	}
	return nil
}

func canonicalWitnesses(values []Witness) ([]Witness, error) {
	result := cloneWitnesses(values)
	for _, witness := range result {
		if err := validateWitness(witness); err != nil {
			return nil, err
		}
	}
	sort.Slice(result, func(i, j int) bool { return witnessKey(result[i]) < witnessKey(result[j]) })
	if len(result) < 2 {
		return result, nil
	}
	compacted := result[:1]
	for _, witness := range result[1:] {
		if witnessKey(compacted[len(compacted)-1]) != witnessKey(witness) {
			compacted = append(compacted, witness)
		}
	}
	return compacted, nil
}

func canonicalizeRelationPatterns(
	values []RelationPatternInput,
	relationID string,
	bindings []objectBinding,
	pendingValueSources map[string]pendingPatternValueSourceArguments,
) ([]RelationPattern, error) {
	result := make([]RelationPattern, 0, len(values))
	for _, value := range values {
		if !validText(value.SourceRef) || !value.Form.Valid() || !validText(value.Selector) ||
			!validOptionalLocation(value.Location) ||
			!validOptionalText(value.ResultRef) || !validOptionalText(value.ReceiverRef) {
			return nil, fmt.Errorf("invalid pattern input")
		}
		resultID, err := resolveObjectRef(bindings, value.ResultRef)
		if err != nil {
			return nil, fmt.Errorf("pattern %q result: %w", value.SourceRef, err)
		}
		receiverID, err := resolveObjectRef(bindings, value.ReceiverRef)
		if err != nil {
			return nil, fmt.Errorf("pattern %q receiver: %w", value.SourceRef, err)
		}
		receiverOriginIDs, receiverOriginsOmitted, err := resolvePatternObjectRefs(
			bindings, value.ReceiverOriginRefs, value.ReceiverOriginResolution, value.ReceiverOriginsObserved,
		)
		if err != nil {
			return nil, fmt.Errorf("pattern %q receiver origins: %w", value.SourceRef, err)
		}
		id := stableID("program-pattern", relationID, value.SourceRef)
		arguments, err := canonicalizePatternArguments(value.Arguments, id, bindings, pendingValueSources)
		if err != nil {
			return nil, fmt.Errorf("pattern %q arguments: %w", value.SourceRef, err)
		}
		pattern := RelationPattern{
			ID: id, SourceRef: value.SourceRef, Form: value.Form, Selector: value.Selector,
			Location: cloneLocation(value.Location),
			ResultID: resultID, ReceiverID: receiverID, ReceiverOriginIDs: receiverOriginIDs,
			ReceiverOriginResolution: value.ReceiverOriginResolution,
			ReceiverOriginsObserved:  value.ReceiverOriginsObserved, ReceiverOriginsOmitted: receiverOriginsOmitted,
			Arguments: arguments, ArgumentsObserved: value.ArgumentsObserved,
			ArgumentsOmitted: value.ArgumentsObserved - len(arguments),
		}
		result = append(result, pattern)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	for position := 1; position < len(result); position++ {
		if result[position-1].ID == result[position].ID {
			return nil, fmt.Errorf("duplicate pattern source ref %q", result[position].SourceRef)
		}
	}
	return result, nil
}

func canonicalizePatternArguments(
	values []PatternArgumentInput,
	patternID string,
	bindings []objectBinding,
	pendingValueSources map[string]pendingPatternValueSourceArguments,
) ([]PatternArgument, error) {
	result := make([]PatternArgument, 0, len(values))
	for _, value := range values {
		if !validPatternArgumentKey(value.Position, value.Keyword) || !value.Kind.Valid() {
			return nil, fmt.Errorf("invalid argument input")
		}
		switch value.Kind {
		case PatternLiteralString:
			if len(value.Parts) != 0 || !validPatternString(value.Value) {
				return nil, fmt.Errorf("invalid literal argument input")
			}
		case PatternStringTemplate:
			if value.Value != "" {
				return nil, fmt.Errorf("invalid template argument input")
			}
		case PatternDynamic:
			if value.Value != "" || len(value.Parts) != 0 {
				return nil, fmt.Errorf("invalid dynamic argument input")
			}
		}
		parts, err := canonicalizePatternParts(value.Parts)
		if err != nil {
			return nil, err
		}
		objectIDs, objectsOmitted, err := resolvePatternObjectRefs(bindings, value.ObjectRefs, value.Resolution, value.ObjectsObserved)
		if err != nil {
			return nil, fmt.Errorf("argument %q objects: %w", patternArgumentKey(value.Position, value.Keyword), err)
		}
		argumentID := stableID("program-pattern-argument", patternID, patternArgumentKey(value.Position, value.Keyword))
		valueCandidates, valueCandidatesOmitted, err := canonicalizePatternValueCandidates(
			value.ValueCandidates, value.ValueCandidatesObserved, argumentID, bindings, pendingValueSources,
		)
		if err != nil {
			return nil, fmt.Errorf("argument %q value candidates: %w", patternArgumentKey(value.Position, value.Keyword), err)
		}
		argument := PatternArgument{
			ID:       argumentID,
			Position: value.Position, Keyword: value.Keyword, Kind: value.Kind, Value: value.Value, Parts: parts,
			ObjectIDs: objectIDs, Resolution: value.Resolution,
			ObjectsObserved: value.ObjectsObserved, ObjectsOmitted: objectsOmitted,
			ValueCandidates: valueCandidates, ValueCandidatesObserved: value.ValueCandidatesObserved,
			ValueCandidatesOmitted: valueCandidatesOmitted,
		}
		result = append(result, argument)
	}
	sort.Slice(result, func(i, j int) bool { return comparePatternArguments(result[i], result[j]) < 0 })
	for position := 1; position < len(result); position++ {
		if comparePatternArguments(result[position-1], result[position]) == 0 {
			return nil, fmt.Errorf("duplicate argument key %q", patternArgumentKey(result[position].Position, result[position].Keyword))
		}
	}
	return result, nil
}

func canonicalizePatternValueCandidates(
	values []PatternValueCandidateInput,
	observed int,
	argumentID string,
	bindings []objectBinding,
	pendingValueSources map[string]pendingPatternValueSourceArguments,
) ([]PatternValueCandidate, int, error) {
	if observed != len(values) {
		return nil, 0, fmt.Errorf("incomplete candidate coverage")
	}
	result := make([]PatternValueCandidate, 0, len(values))
	for _, value := range values {
		if !value.Resolution.Valid() || !value.SourceKind.Valid() ||
			value.Kind != PatternLiteralString && value.Kind != PatternStringTemplate {
			return nil, 0, fmt.Errorf("invalid candidate input")
		}
		switch value.Kind {
		case PatternLiteralString:
			if len(value.Parts) != 0 || !validPatternString(value.Value) {
				return nil, 0, fmt.Errorf("invalid literal candidate input")
			}
		case PatternStringTemplate:
			if value.Value != "" {
				return nil, 0, fmt.Errorf("invalid template candidate input")
			}
		}
		parts, err := canonicalizePatternParts(value.Parts)
		if err != nil {
			return nil, 0, err
		}
		sourceIDs, sourceOmitted, err := resolvePatternValueSourceRefs(
			bindings, value.SourceObjectRefs, value.SourceObjectsObserved,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("source objects: %w", err)
		}
		sourceArgumentRefs, err := canonicalizePatternArgumentReferences(
			value.SourceArgumentRefs, value.SourceArgumentsObserved,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("source arguments: %w", err)
		}
		candidate := PatternValueCandidate{
			Kind: value.Kind, Value: value.Value, Parts: parts, Resolution: value.Resolution,
			SourceKind: value.SourceKind, SourceObjectIDs: sourceIDs,
			SourceObjectsObserved: value.SourceObjectsObserved, SourceObjectsOmitted: sourceOmitted,
			SourceArgumentIDs: []string{}, SourceArgumentsObserved: value.SourceArgumentsObserved,
		}
		switch value.SourceKind {
		case PatternValueSourceInitializer:
			if len(sourceIDs) == 0 || len(sourceArgumentRefs) != 0 {
				return nil, 0, fmt.Errorf("initializer candidate has invalid sources")
			}
			candidate.ID = patternValueCandidateIdentity(argumentID, candidate)
			if err := validatePatternValueCandidateShape(candidate, argumentID); err != nil {
				return nil, 0, err
			}
		case PatternValueSourceActualArgument:
			if len(sourceIDs) != 0 || len(sourceArgumentRefs) == 0 || value.Resolution != PatternValuePossible {
				return nil, 0, fmt.Errorf("actual-argument candidate has invalid sources or authority")
			}
			candidate.ID = pendingPatternValueCandidateIdentity(argumentID, candidate, sourceArgumentRefs)
			if _, duplicate := pendingValueSources[candidate.ID]; duplicate {
				return nil, 0, fmt.Errorf("duplicate pending candidate identity %q", candidate.ID)
			}
			pendingValueSources[candidate.ID] = pendingPatternValueSourceArguments{
				Refs: sourceArgumentRefs, Observed: value.SourceArgumentsObserved,
			}
		}
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	for position := 1; position < len(result); position++ {
		if result[position-1].ID == result[position].ID {
			return nil, 0, fmt.Errorf("duplicate candidate identity %q", result[position].ID)
		}
	}
	return result, observed - len(result), nil
}

func resolvePatternValueSourceRefs(
	bindings []objectBinding,
	refs []string,
	observed int,
) ([]string, int, error) {
	canonicalRefs := cloneStrings(refs)
	for _, ref := range canonicalRefs {
		if !validText(ref) {
			return nil, 0, fmt.Errorf("invalid source object ref")
		}
	}
	sort.Strings(canonicalRefs)
	for position := 1; position < len(canonicalRefs); position++ {
		if canonicalRefs[position-1] == canonicalRefs[position] {
			return nil, 0, fmt.Errorf("duplicate source object ref %q", canonicalRefs[position])
		}
	}
	ids := make([]string, 0, len(canonicalRefs))
	for _, ref := range canonicalRefs {
		id, err := resolveObjectRef(bindings, ref)
		if err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if observed != len(ids) {
		return nil, 0, fmt.Errorf("incomplete source object coverage")
	}
	return ids, 0, nil
}

func canonicalizePatternArgumentReferences(
	refs []PatternArgumentRefInput,
	observed int,
) ([]PatternArgumentRefInput, error) {
	result := make([]PatternArgumentRefInput, len(refs))
	copy(result, refs)
	for _, ref := range result {
		if !validPatternArgumentRefInput(ref) {
			return nil, fmt.Errorf("invalid source argument ref")
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return patternArgumentReferenceKey(result[i]) < patternArgumentReferenceKey(result[j])
	})
	for position := 1; position < len(result); position++ {
		if patternArgumentReferenceKey(result[position-1]) == patternArgumentReferenceKey(result[position]) {
			return nil, fmt.Errorf("duplicate source argument ref")
		}
	}
	if observed != len(result) {
		return nil, fmt.Errorf("incomplete source argument coverage")
	}
	return result, nil
}

func canonicalizePatternParts(values []PatternPartInput) ([]PatternPart, error) {
	result := make([]PatternPart, 0, len(values))
	for _, value := range values {
		if !value.Kind.Valid() || value.Kind == PatternPartHole && value.Text != "" ||
			value.Kind == PatternPartLiteral && !validPatternString(value.Text) {
			return nil, fmt.Errorf("invalid template part")
		}
		if value.Kind == PatternPartLiteral && value.Text == "" {
			continue
		}
		if value.Kind == PatternPartLiteral && len(result) > 0 && result[len(result)-1].Kind == PatternPartLiteral {
			combined := result[len(result)-1].Text + value.Text
			if !validPatternString(combined) {
				return nil, fmt.Errorf("template literal bound exceeded")
			}
			result[len(result)-1].Text = combined
			continue
		}
		result = append(result, PatternPart{Kind: value.Kind, Text: value.Text})
	}
	return result, nil
}

func resolvePatternObjectRefs(bindings []objectBinding, refs []string, resolution Resolution, observed int) ([]string, int, error) {
	canonicalRefs := cloneStrings(refs)
	for _, ref := range canonicalRefs {
		if !validText(ref) {
			return nil, 0, fmt.Errorf("invalid object ref")
		}
	}
	sort.Strings(canonicalRefs)
	canonicalRefs = compactStrings(canonicalRefs)
	ids := make([]string, 0, len(canonicalRefs))
	for _, ref := range canonicalRefs {
		id, err := resolveObjectRef(bindings, ref)
		if err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ids = compactStrings(ids)
	omitted := observed - len(ids)
	if err := validateOptionalObjectAuthority(ids, resolution, observed, omitted); err != nil {
		return nil, 0, err
	}
	return ids, omitted, nil
}

func validateRelationPatternShape(value RelationPattern, relationID string) error {
	if !validText(value.ID) || !validText(value.SourceRef) || !value.Form.Valid() || !validText(value.Selector) ||
		!validOptionalLocation(value.Location) ||
		!validOptionalText(value.ResultID) || !validOptionalText(value.ReceiverID) ||
		value.ReceiverOriginIDs == nil || value.Arguments == nil ||
		!canonicalStringsAllowEmpty(value.ReceiverOriginIDs) {
		return fmt.Errorf("program index: invalid relation pattern")
	}
	if value.ID != stableID("program-pattern", relationID, value.SourceRef) {
		return fmt.Errorf("program index: pattern identity mismatch")
	}
	if err := validateOptionalObjectAuthority(value.ReceiverOriginIDs, value.ReceiverOriginResolution,
		value.ReceiverOriginsObserved, value.ReceiverOriginsOmitted); err != nil {
		return fmt.Errorf("program index: invalid pattern receiver origin coverage")
	}
	if value.ArgumentsObserved < len(value.Arguments) ||
		value.ArgumentsOmitted != value.ArgumentsObserved-len(value.Arguments) {
		return fmt.Errorf("program index: invalid pattern argument coverage")
	}
	for position, argument := range value.Arguments {
		if err := validatePatternArgumentShape(argument, value.ID); err != nil {
			return err
		}
		if position > 0 && comparePatternArguments(value.Arguments[position-1], argument) >= 0 {
			return fmt.Errorf("program index: pattern arguments are not canonical")
		}
	}
	return nil
}

func validatePatternArgumentShape(value PatternArgument, patternID string) error {
	if !validText(value.ID) || !validPatternArgumentKey(value.Position, value.Keyword) || !value.Kind.Valid() ||
		value.Parts == nil || value.ObjectIDs == nil || value.ValueCandidates == nil ||
		!canonicalStringsAllowEmpty(value.ObjectIDs) {
		return fmt.Errorf("program index: invalid pattern argument")
	}
	if value.ID != stableID("program-pattern-argument", patternID, patternArgumentKey(value.Position, value.Keyword)) {
		return fmt.Errorf("program index: pattern argument identity mismatch")
	}
	if err := validateOptionalObjectAuthority(value.ObjectIDs, value.Resolution, value.ObjectsObserved, value.ObjectsOmitted); err != nil {
		return fmt.Errorf("program index: invalid pattern argument object coverage")
	}
	if value.ValueCandidatesObserved != len(value.ValueCandidates) || value.ValueCandidatesOmitted != 0 {
		return fmt.Errorf("program index: invalid pattern value candidate coverage")
	}
	if value.Kind != PatternDynamic && len(value.ValueCandidates) != 0 {
		return fmt.Errorf("program index: resolved values require a dynamic pattern argument")
	}
	for position, candidate := range value.ValueCandidates {
		if err := validatePatternValueCandidateShape(candidate, value.ID); err != nil {
			return err
		}
		if position > 0 && value.ValueCandidates[position-1].ID >= candidate.ID {
			return fmt.Errorf("program index: pattern value candidates are not canonical")
		}
	}
	switch value.Kind {
	case PatternLiteralString:
		if !validPatternString(value.Value) || len(value.Parts) != 0 {
			return fmt.Errorf("program index: invalid literal pattern argument")
		}
	case PatternStringTemplate:
		if value.Value != "" || len(value.Parts) == 0 {
			return fmt.Errorf("program index: invalid template pattern argument")
		}
		hasHole := false
		previousLiteral := false
		for _, part := range value.Parts {
			if !part.Kind.Valid() || part.Kind == PatternPartHole && part.Text != "" ||
				part.Kind == PatternPartLiteral && (part.Text == "" || !validPatternString(part.Text)) ||
				part.Kind == PatternPartLiteral && previousLiteral {
				return fmt.Errorf("program index: invalid template pattern part")
			}
			hasHole = hasHole || part.Kind == PatternPartHole
			previousLiteral = part.Kind == PatternPartLiteral
		}
		if !hasHole {
			return fmt.Errorf("program index: template pattern has no hole")
		}
	case PatternDynamic:
		if value.Value != "" || len(value.Parts) != 0 {
			return fmt.Errorf("program index: invalid dynamic pattern argument")
		}
	}
	return nil
}

func validatePatternValueCandidateShape(value PatternValueCandidate, argumentID string) error {
	if !validText(value.ID) || !value.Resolution.Valid() || !value.SourceKind.Valid() ||
		value.Parts == nil || value.SourceObjectIDs == nil || value.SourceArgumentIDs == nil ||
		!canonicalStringsAllowEmpty(value.SourceObjectIDs) || !canonicalStringsAllowEmpty(value.SourceArgumentIDs) ||
		value.SourceObjectsObserved != len(value.SourceObjectIDs) || value.SourceObjectsOmitted != 0 ||
		value.SourceArgumentsObserved != len(value.SourceArgumentIDs) || value.SourceArgumentsOmitted != 0 {
		return fmt.Errorf("program index: invalid pattern value candidate")
	}
	switch value.SourceKind {
	case PatternValueSourceInitializer:
		if len(value.SourceObjectIDs) == 0 || len(value.SourceArgumentIDs) != 0 || value.SourceArgumentsObserved != 0 {
			return fmt.Errorf("program index: invalid initializer value candidate sources")
		}
	case PatternValueSourceActualArgument:
		if len(value.SourceObjectIDs) != 0 || value.SourceObjectsObserved != 0 ||
			len(value.SourceArgumentIDs) != 1 || value.SourceArgumentsObserved != 1 ||
			value.Resolution != PatternValuePossible {
			return fmt.Errorf("program index: invalid actual-argument value candidate sources")
		}
	}
	if value.ID != patternValueCandidateIdentity(argumentID, value) {
		return fmt.Errorf("program index: pattern value candidate identity mismatch")
	}
	switch value.Kind {
	case PatternLiteralString:
		if !validPatternString(value.Value) || len(value.Parts) != 0 {
			return fmt.Errorf("program index: invalid literal pattern value candidate")
		}
	case PatternStringTemplate:
		if value.Value != "" || len(value.Parts) == 0 {
			return fmt.Errorf("program index: invalid template pattern value candidate")
		}
		hasHole := false
		previousLiteral := false
		for _, part := range value.Parts {
			if !part.Kind.Valid() || part.Kind == PatternPartHole && part.Text != "" ||
				part.Kind == PatternPartLiteral && (part.Text == "" || !validPatternString(part.Text)) ||
				part.Kind == PatternPartLiteral && previousLiteral {
				return fmt.Errorf("program index: invalid template pattern value candidate part")
			}
			hasHole = hasHole || part.Kind == PatternPartHole
			previousLiteral = part.Kind == PatternPartLiteral
		}
		if !hasHole {
			return fmt.Errorf("program index: template pattern value candidate has no hole")
		}
	default:
		return fmt.Errorf("program index: invalid pattern value candidate kind")
	}
	return nil
}

func patternValueCandidateIdentity(argumentID string, value PatternValueCandidate) string {
	fields := []string{
		argumentID, string(value.Kind), value.Value, string(value.Resolution), string(value.SourceKind),
	}
	for _, part := range value.Parts {
		fields = append(fields, string(part.Kind), part.Text)
	}
	fields = append(fields, "source-objects")
	fields = append(fields, value.SourceObjectIDs...)
	fields = append(fields, "source-arguments")
	fields = append(fields, value.SourceArgumentIDs...)
	return stableID("program-pattern-value", fields...)
}

func pendingPatternValueCandidateIdentity(
	argumentID string,
	value PatternValueCandidate,
	refs []PatternArgumentRefInput,
) string {
	fields := []string{
		argumentID, string(value.Kind), value.Value, string(value.Resolution), string(value.SourceKind),
	}
	for _, part := range value.Parts {
		fields = append(fields, string(part.Kind), part.Text)
	}
	for _, ref := range refs {
		fields = append(fields, patternArgumentReferenceKey(ref))
	}
	return stableID("program-pattern-value-pending", fields...)
}

func validateOptionalObjectAuthority(ids []string, resolution Resolution, observed, omitted int) error {
	if observed < 0 || observed < len(ids) || omitted != observed-len(ids) {
		return fmt.Errorf("invalid object coverage")
	}
	if resolution == "" {
		if observed != 0 || len(ids) != 0 || omitted != 0 {
			return fmt.Errorf("object authority is missing resolution")
		}
		return nil
	}
	if !resolution.Valid() || observed == 0 {
		return fmt.Errorf("invalid object resolution")
	}
	switch resolution {
	case ResolutionExact:
		if len(ids) != 1 || omitted != 0 {
			return fmt.Errorf("invalid exact object authority")
		}
	case ResolutionAlternatives:
		if len(ids) == 0 {
			return fmt.Errorf("invalid alternative object authority")
		}
	case ResolutionUnresolved:
		if len(ids) != 0 {
			return fmt.Errorf("invalid unresolved object authority")
		}
	}
	return nil
}

func validPatternArgumentRefInput(value PatternArgumentRefInput) bool {
	return validText(value.RelationSourceRef) && validText(value.PatternSourceRef) &&
		validPatternArgumentKey(value.Position, value.Keyword)
}

func patternArgumentReferenceKey(value PatternArgumentRefInput) string {
	return strings.Join([]string{
		value.RelationSourceRef, value.PatternSourceRef,
		patternArgumentKey(value.Position, value.Keyword),
	}, "\x00")
}

func resolvePatternArgumentReference(
	relations []Relation,
	reference PatternArgumentRefInput,
) (string, error) {
	if !validPatternArgumentRefInput(reference) {
		return "", fmt.Errorf("invalid reference")
	}
	argumentID := ""
	for _, relation := range relations {
		if relation.SourceRef != reference.RelationSourceRef {
			continue
		}
		for _, pattern := range relation.Patterns {
			if pattern.SourceRef != reference.PatternSourceRef {
				continue
			}
			for _, argument := range pattern.Arguments {
				if argument.Position != reference.Position || argument.Keyword != reference.Keyword {
					continue
				}
				if argumentID != "" && argumentID != argument.ID {
					return "", fmt.Errorf("ambiguous reference")
				}
				argumentID = argument.ID
			}
		}
	}
	if argumentID == "" {
		return "", fmt.Errorf("unknown reference")
	}
	return argumentID, nil
}

func resolvePatternValueSourceArgumentReferences(
	relations []Relation,
	pending map[string]pendingPatternValueSourceArguments,
) error {
	resolvedPending := make(map[string]struct{}, len(pending))
	for relationPosition := range relations {
		for patternPosition := range relations[relationPosition].Patterns {
			pattern := &relations[relationPosition].Patterns[patternPosition]
			for argumentPosition := range pattern.Arguments {
				argument := &pattern.Arguments[argumentPosition]
				for candidatePosition := range argument.ValueCandidates {
					candidate := &argument.ValueCandidates[candidatePosition]
					value, ok := pending[candidate.ID]
					if !ok {
						continue
					}
					ids := make([]string, 0, len(value.Refs))
					for _, ref := range value.Refs {
						id, err := resolvePatternArgumentReference(relations, ref)
						if err != nil {
							return fmt.Errorf("program index: pattern value source argument: %w", err)
						}
						if id == argument.ID {
							return fmt.Errorf("program index: pattern value candidate cites its owning argument")
						}
						sourceRelation, sourceArgument, ok := patternArgumentAuthorityWithID(relations, id)
						if !ok || sourceRelation.Resolution != ResolutionExact || len(sourceRelation.ToIDs) != 1 ||
							sourceRelation.TargetsOmitted != 0 || !samePatternValue(candidate.Kind, candidate.Value, candidate.Parts, sourceArgument) {
							return fmt.Errorf("program index: actual value source argument has incompatible authority")
						}
						ids = append(ids, id)
					}
					sort.Strings(ids)
					for position := 1; position < len(ids); position++ {
						if ids[position-1] == ids[position] {
							return fmt.Errorf("program index: duplicate resolved value source argument")
						}
					}
					if value.Observed != len(ids) {
						return fmt.Errorf("program index: incomplete resolved value source arguments")
					}
					pendingID := candidate.ID
					candidate.SourceArgumentIDs = ids
					candidate.SourceArgumentsObserved = value.Observed
					candidate.SourceArgumentsOmitted = 0
					candidate.ID = patternValueCandidateIdentity(argument.ID, *candidate)
					resolvedPending[pendingID] = struct{}{}
				}
				sort.Slice(argument.ValueCandidates, func(i, j int) bool {
					return argument.ValueCandidates[i].ID < argument.ValueCandidates[j].ID
				})
				for position := 1; position < len(argument.ValueCandidates); position++ {
					if argument.ValueCandidates[position-1].ID == argument.ValueCandidates[position].ID {
						return fmt.Errorf("program index: duplicate resolved pattern value candidate")
					}
				}
			}
		}
	}
	if len(resolvedPending) != len(pending) {
		return fmt.Errorf("program index: unresolved pending pattern value candidate")
	}
	return nil
}

func patternArgumentAuthorityWithID(relations []Relation, id string) (Relation, PatternArgument, bool) {
	for _, relation := range relations {
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				if argument.ID == id {
					return relation, argument, true
				}
			}
		}
	}
	return Relation{}, PatternArgument{}, false
}

func samePatternValue(kind PatternValueKind, value string, parts []PatternPart, source PatternArgument) bool {
	if source.Kind != kind || source.Value != value || len(source.Parts) != len(parts) {
		return false
	}
	for position := range parts {
		if source.Parts[position] != parts[position] {
			return false
		}
	}
	return kind == PatternLiteralString || kind == PatternStringTemplate
}

func validPatternArgumentKey(position int, keyword string) bool {
	return (position > 0 && keyword == "") ||
		(position == 0 && validText(keyword))
}

func patternArgumentKey(position int, keyword string) string {
	if position > 0 {
		return "position:" + strconv.Itoa(position)
	}
	return "keyword:" + keyword
}

func comparePatternArguments(left, right PatternArgument) int {
	if left.Position > 0 && right.Position == 0 {
		return -1
	}
	if left.Position == 0 && right.Position > 0 {
		return 1
	}
	if left.Position > 0 {
		if left.Position < right.Position {
			return -1
		}
		if left.Position > right.Position {
			return 1
		}
		return 0
	}
	return strings.Compare(left.Keyword, right.Keyword)
}

func compileCoverage(objects []Object, relations []Relation, objectsObserved, relationsObserved int) Coverage {
	coverage := Coverage{
		ObjectsObserved: objectsObserved, ObjectsIndexed: len(objects), ObjectsOmitted: objectsObserved - len(objects),
		RelationsObserved: relationsObserved, RelationsIndexed: len(relations), RelationsOmitted: relationsObserved - len(relations),
	}
	for _, relation := range relations {
		switch relation.Resolution {
		case ResolutionExact:
			coverage.ExactRelations++
		case ResolutionAlternatives:
			coverage.AlternativeRelations++
		case ResolutionUnresolved:
			coverage.UnresolvedRelations++
		}
		coverage.TargetsObserved += relation.TargetsObserved
		coverage.TargetsIndexed += len(relation.ToIDs)
		coverage.TargetsOmitted += relation.TargetsOmitted
		coverage.WitnessesObserved += relation.WitnessesObserved
		coverage.WitnessesIndexed += len(relation.Witnesses)
		coverage.WitnessesOmitted += relation.WitnessesOmitted
		coverage.PatternsObserved += relation.PatternsObserved
		coverage.PatternsIndexed += len(relation.Patterns)
		coverage.PatternsOmitted += relation.PatternsOmitted
		for _, pattern := range relation.Patterns {
			coverage.ArgumentsObserved += pattern.ArgumentsObserved
			coverage.ArgumentsIndexed += len(pattern.Arguments)
			coverage.ArgumentsOmitted += pattern.ArgumentsOmitted
			coverage.ReceiverOriginsObserved += pattern.ReceiverOriginsObserved
			coverage.ReceiverOriginsIndexed += len(pattern.ReceiverOriginIDs)
			coverage.ReceiverOriginsOmitted += pattern.ReceiverOriginsOmitted
			for _, argument := range pattern.Arguments {
				coverage.ArgumentObjectsObserved += argument.ObjectsObserved
				coverage.ArgumentObjectsIndexed += len(argument.ObjectIDs)
				coverage.ArgumentObjectsOmitted += argument.ObjectsOmitted
				coverage.ArgumentValuesObserved += argument.ValueCandidatesObserved
				coverage.ArgumentValuesIndexed += len(argument.ValueCandidates)
				coverage.ArgumentValuesOmitted += argument.ValueCandidatesOmitted
				for _, candidate := range argument.ValueCandidates {
					coverage.ValueSourcesObserved += candidate.SourceObjectsObserved
					coverage.ValueSourcesIndexed += len(candidate.SourceObjectIDs)
					coverage.ValueSourcesOmitted += candidate.SourceObjectsOmitted
					coverage.ValueArgumentSourcesObserved += candidate.SourceArgumentsObserved
					coverage.ValueArgumentSourcesIndexed += len(candidate.SourceArgumentIDs)
					coverage.ValueArgumentSourcesOmitted += candidate.SourceArgumentsOmitted
				}
			}
		}
	}
	return coverage
}

func validateCoverage(value Coverage, objectsIndexed, relationsIndexed int) error {
	counts := []int{
		value.ObjectsObserved, value.ObjectsIndexed, value.ObjectsOmitted,
		value.RelationsObserved, value.RelationsIndexed, value.RelationsOmitted,
		value.ExactRelations, value.AlternativeRelations, value.UnresolvedRelations,
		value.TargetsObserved, value.TargetsIndexed, value.TargetsOmitted,
		value.WitnessesObserved, value.WitnessesIndexed, value.WitnessesOmitted,
		value.PatternsObserved, value.PatternsIndexed, value.PatternsOmitted,
		value.ArgumentsObserved, value.ArgumentsIndexed, value.ArgumentsOmitted,
		value.ReceiverOriginsObserved, value.ReceiverOriginsIndexed, value.ReceiverOriginsOmitted,
		value.ArgumentObjectsObserved, value.ArgumentObjectsIndexed, value.ArgumentObjectsOmitted,
		value.ArgumentValuesObserved, value.ArgumentValuesIndexed, value.ArgumentValuesOmitted,
		value.ValueSourcesObserved, value.ValueSourcesIndexed, value.ValueSourcesOmitted,
		value.ValueArgumentSourcesObserved, value.ValueArgumentSourcesIndexed, value.ValueArgumentSourcesOmitted,
	}
	for _, count := range counts {
		if count < 0 {
			return fmt.Errorf("program index: invalid coverage count")
		}
	}
	if value.ObjectsIndexed != objectsIndexed || value.ObjectsObserved < objectsIndexed ||
		value.ObjectsOmitted != value.ObjectsObserved-objectsIndexed ||
		value.RelationsIndexed != relationsIndexed || value.RelationsObserved < relationsIndexed ||
		value.RelationsOmitted != value.RelationsObserved-relationsIndexed ||
		value.ExactRelations+value.AlternativeRelations+value.UnresolvedRelations != relationsIndexed ||
		value.TargetsOmitted != value.TargetsObserved-value.TargetsIndexed ||
		value.WitnessesOmitted != value.WitnessesObserved-value.WitnessesIndexed ||
		value.PatternsOmitted != value.PatternsObserved-value.PatternsIndexed ||
		value.ArgumentsOmitted != value.ArgumentsObserved-value.ArgumentsIndexed ||
		value.ReceiverOriginsOmitted != value.ReceiverOriginsObserved-value.ReceiverOriginsIndexed ||
		value.ArgumentObjectsOmitted != value.ArgumentObjectsObserved-value.ArgumentObjectsIndexed ||
		value.ArgumentValuesOmitted != value.ArgumentValuesObserved-value.ArgumentValuesIndexed ||
		value.ValueSourcesOmitted != value.ValueSourcesObserved-value.ValueSourcesIndexed ||
		value.ValueArgumentSourcesOmitted != value.ValueArgumentSourcesObserved-value.ValueArgumentSourcesIndexed {
		return fmt.Errorf("program index: invalid coverage")
	}
	return nil
}

func indexDigest(index Index) (string, error) {
	payload := index.Snapshot()
	payload.SHA256 = ""
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("program index: encode digest material: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func targetObjectScopeIdentity(target Target) string {
	fields := make([]string, 0, 4+2*len(target.Sources))
	fields = append(fields, target.Language, target.Kind, target.Selector, target.AnchorFileRef)
	for _, source := range target.Sources {
		fields = append(fields, source.FileRef, source.Path)
	}
	return stableID("program-target-scope", fields...)
}

func targetIdentity(target Target) string {
	fields := make([]string, 0, 1+3*len(target.Seeds))
	fields = append(fields, targetObjectScopeIdentity(target))
	for _, seed := range target.Seeds {
		fields = append(fields, seed.ObjectID, string(seed.Kind), locationKey(seed.Location))
	}
	return stableID("program-target", fields...)
}

func stableID(prefix string, fields ...string) string {
	digest := sha256.New()
	for _, field := range append([]string{prefix}, fields...) {
		digest.Write([]byte(strconv.Itoa(len(field))))
		digest.Write([]byte{0})
		digest.Write([]byte(field))
	}
	return prefix + "-" + hex.EncodeToString(digest.Sum(nil))
}

func witnessKey(value Witness) string {
	return strings.Join([]string{
		locationKey(value.Location), value.Kind, value.Detail, value.SourceExpression,
	}, "\x00")
}

func locationKey(value *Location) string {
	if value == nil {
		return ""
	}
	return value.Path + ":" + strconv.Itoa(value.Line) + ":" + strconv.Itoa(value.Column)
}

func validOptionalLocation(value *Location) bool {
	return value == nil || validLocation(*value)
}

func validLocation(value Location) bool {
	return validPath(value.Path) && value.Line > 0 && value.Column > 0
}

func validPath(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") ||
		!fs.ValidPath(value) || value == "." || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validOptionalText(value string) bool {
	return value == "" || validText(value)
}

func validPatternString(value string) bool {
	return utf8.ValidString(value)
}

func validateExternalSymbolBinding(kind ObjectKind, value *ExternalSymbol) error {
	if value == nil {
		return nil
	}
	if kind != ObjectExternalSymbol || !value.AuthorityKind.Valid() || !validText(value.PackagePath) ||
		!validOptionalText(value.Receiver) || !validText(value.Name) {
		return fmt.Errorf("program index: invalid external symbol authority")
	}
	return nil
}

func cloneExternalSymbol(value *ExternalSymbol) *ExternalSymbol {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func canonicalizeSymbolLinkIdentityInputs(values []SymbolLinkIdentityInput) ([]SymbolLinkIdentity, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make([]SymbolLinkIdentity, 0, len(values))
	for _, value := range values {
		if !validText(value.Domain) || !validOptionalText(value.Display) ||
			len(value.Parts) == 0 {
			return nil, fmt.Errorf("invalid identity")
		}
		parts := cloneStrings(value.Parts)
		for _, part := range parts {
			if !validText(part) {
				return nil, fmt.Errorf("invalid identity part")
			}
		}
		result = append(result, SymbolLinkIdentity{
			Domain:    value.Domain,
			Key:       stableID("symbol-link", append([]string{value.Domain}, parts...)...),
			Display:   value.Display,
			PartCount: len(parts),
		})
	}
	sort.Slice(result, func(i, j int) bool { return symbolLinkIdentityKey(result[i]) < symbolLinkIdentityKey(result[j]) })
	compacted := result[:0]
	for _, identity := range result {
		if len(compacted) > 0 && compacted[len(compacted)-1].Domain == identity.Domain &&
			compacted[len(compacted)-1].Key == identity.Key {
			if compacted[len(compacted)-1].Display != identity.Display {
				return nil, fmt.Errorf("conflicting display for one exact identity")
			}
			continue
		}
		compacted = append(compacted, identity)
	}
	return compacted, nil
}

func canonicalSymbolLinkIdentities(values []SymbolLinkIdentity) bool {
	previous := ""
	for _, value := range values {
		if !validText(value.Domain) || !validSymbolLinkKey(value.Key) || !validOptionalText(value.Display) ||
			value.PartCount <= 0 {
			return false
		}
		key := symbolLinkIdentityKey(value)
		if previous != "" && previous >= key {
			return false
		}
		previous = key
	}
	return true
}

func symbolLinkIdentityKey(value SymbolLinkIdentity) string {
	return value.Domain + "\x00" + value.Key
}

func validSymbolLinkKey(value string) bool {
	const prefix = "symbol-link-"
	return strings.HasPrefix(value, prefix) && validSHA256(strings.TrimPrefix(value, prefix))
}

func cloneSymbolLinkIdentities(values []SymbolLinkIdentity) []SymbolLinkIdentity {
	if values == nil {
		return nil
	}
	result := make([]SymbolLinkIdentity, len(values))
	copy(result, values)
	return result
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func canonicalStrings(values []string) bool {
	if values == nil || !sort.StringsAreSorted(values) {
		return false
	}
	for position, value := range values {
		if !validText(value) || position > 0 && values[position-1] == value {
			return false
		}
	}
	return true
}

func canonicalStringsAllowEmpty(values []string) bool {
	if values == nil {
		return false
	}
	if len(values) == 0 {
		return true
	}
	return canonicalStrings(values)
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func cloneStrings(values []string) []string {
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func cloneTargetSources(values []TargetSource) []TargetSource {
	result := make([]TargetSource, len(values))
	copy(result, values)
	return result
}

func cloneTargetSeeds(values []TargetSeed) []TargetSeed {
	result := make([]TargetSeed, len(values))
	copy(result, values)
	for position := range result {
		result[position].Location = cloneLocation(values[position].Location)
	}
	return result
}

func cloneLocation(value *Location) *Location {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func cloneWitnesses(values []Witness) []Witness {
	result := make([]Witness, len(values))
	copy(result, values)
	for position := range result {
		result[position].Location = cloneLocation(values[position].Location)
	}
	return result
}

func cloneRelationPatterns(values []RelationPattern) []RelationPattern {
	result := make([]RelationPattern, len(values))
	copy(result, values)
	for position := range result {
		result[position].Location = cloneLocation(values[position].Location)
		result[position].ReceiverOriginIDs = cloneStrings(values[position].ReceiverOriginIDs)
		result[position].Arguments = clonePatternArguments(values[position].Arguments)
	}
	return result
}

func clonePatternArguments(values []PatternArgument) []PatternArgument {
	result := make([]PatternArgument, len(values))
	copy(result, values)
	for position := range result {
		result[position].Parts = clonePatternParts(values[position].Parts)
		result[position].ObjectIDs = cloneStrings(values[position].ObjectIDs)
		result[position].ValueCandidates = clonePatternValueCandidates(values[position].ValueCandidates)
	}
	return result
}

func clonePatternValueCandidates(values []PatternValueCandidate) []PatternValueCandidate {
	result := make([]PatternValueCandidate, len(values))
	copy(result, values)
	for position := range result {
		result[position].Parts = clonePatternParts(values[position].Parts)
		result[position].SourceObjectIDs = cloneStrings(values[position].SourceObjectIDs)
		result[position].SourceArgumentIDs = cloneStrings(values[position].SourceArgumentIDs)
	}
	return result
}

func clonePatternParts(values []PatternPart) []PatternPart {
	result := make([]PatternPart, len(values))
	copy(result, values)
	return result
}
