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
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	Version          = 7
	ArtifactFilename = "program-index.json"

	MaxTargetSources        = 4_096
	MaxTargetSeeds          = 4_096
	MaxObjects              = 131_072
	MaxRelations            = 262_144
	MaxTargetsPerRelation   = 64
	MaxWitnessesPerRelation = 64
	MaxWitnesses            = 524_288
	MaxTextBytes            = 16 * 1024
	// MaxAggregateTextBytes bounds semantic string payload independently of
	// JSON field names, punctuation, and numeric metadata. MaxIndexBytes is the
	// complete encoded envelope bound; using one value for both made a valid
	// bounded index impossible to persist once structural JSON overhead crossed
	// the semantic text ceiling.
	MaxAggregateTextBytes = 64 * 1024 * 1024
	MaxIndexBytes         = 128 * 1024 * 1024
	MaxObservedCount      = 1<<31 - 1
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
	SourceRef    string
	Kind         ObjectKind
	Name         string
	Visibility   Visibility
	Signature    string
	OwnerRef     string
	ContainerRef string
	Location     *Location
	// External is adapter-owned package and symbol authority. It is valid only
	// for ObjectExternalSymbol and avoids forcing consumers to recover package
	// boundaries by splitting the presentation-oriented Name field.
	External *ExternalSymbol
}

// ExternalSymbol is the exact language-tool identity of an external program
// object. PackagePath is the import/package authority used to join the object
// to dependencies.Catalog. Receiver is optional because free functions and
// package variables do not have one.
type ExternalSymbol struct {
	PackagePath string `json:"package_path"`
	Receiver    string `json:"receiver,omitempty"`
	Name        string `json:"name"`
}

// Object is one language-neutral program declaration or external symbol.
// Signature, ownership, containment and location are optional because not all
// adapters can establish them with exact local authority.
type Object struct {
	ID          string          `json:"id"`
	SourceRef   string          `json:"source_ref"`
	Kind        ObjectKind      `json:"kind"`
	Name        string          `json:"name"`
	Visibility  Visibility      `json:"visibility"`
	Signature   string          `json:"signature,omitempty"`
	OwnerID     string          `json:"owner_id,omitempty"`
	ContainerID string          `json:"container_id,omitempty"`
	Location    *Location       `json:"location,omitempty"`
	External    *ExternalSymbol `json:"external,omitempty"`
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
}

// Relation is one typed, locally resolved edge or uncertainty joint.
type Relation struct {
	ID                string       `json:"id"`
	SourceRef         string       `json:"source_ref"`
	Kind              RelationKind `json:"kind"`
	FromID            string       `json:"from_id"`
	ToIDs             []string     `json:"to_ids"`
	Resolution        Resolution   `json:"resolution"`
	Invocation        string       `json:"invocation,omitempty"`
	Location          *Location    `json:"location,omitempty"`
	TargetsObserved   int          `json:"targets_observed"`
	TargetsOmitted    int          `json:"targets_omitted"`
	Witnesses         []Witness    `json:"witnesses"`
	WitnessesObserved int          `json:"witnesses_observed"`
	WitnessesOmitted  int          `json:"witnesses_omitted"`
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
// explicit. Target and witness omissions are aggregated from Relation rows.
type Coverage struct {
	ObjectsObserved      int `json:"objects_observed"`
	ObjectsIndexed       int `json:"objects_indexed"`
	ObjectsOmitted       int `json:"objects_omitted"`
	RelationsObserved    int `json:"relations_observed"`
	RelationsIndexed     int `json:"relations_indexed"`
	RelationsOmitted     int `json:"relations_omitted"`
	ExactRelations       int `json:"exact_relations"`
	AlternativeRelations int `json:"alternative_relations"`
	UnresolvedRelations  int `json:"unresolved_relations"`
	TargetsObserved      int `json:"targets_observed"`
	TargetsIndexed       int `json:"targets_indexed"`
	TargetsOmitted       int `json:"targets_omitted"`
	WitnessesObserved    int `json:"witnesses_observed"`
	WitnessesIndexed     int `json:"witnesses_indexed"`
	WitnessesOmitted     int `json:"witnesses_omitted"`
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
	Version        int        `json:"version"`
	ScenarioSHA256 string     `json:"scenario_sha256"`
	SourceSHA256   string     `json:"source_sha256"`
	Target         Target     `json:"target"`
	Objects        []Object   `json:"objects"`
	Relations      []Relation `json:"relations"`
	Coverage       Coverage   `json:"coverage"`
	SHA256         string     `json:"sha256"`
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
	if len(encoded) > MaxIndexBytes {
		return nil, fmt.Errorf("program index: artifact is %d bytes, limit is %d", len(encoded), MaxIndexBytes)
	}
	return encoded, nil
}

// Decode strictly decodes one JSON artifact, rejects unknown fields and
// trailing JSON values, then validates identities, references and the seal.
func Decode(encoded []byte) (Index, error) {
	if len(encoded) == 0 || len(encoded) > MaxIndexBytes {
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
// result. It never truncates an over-bound input.
func New(input Input) (Index, error) {
	if len(input.Target.Sources) > MaxTargetSources || len(input.Target.Seeds) > MaxTargetSeeds ||
		len(input.Objects) > MaxObjects || len(input.Relations) > MaxRelations {
		return Index{}, fmt.Errorf("program index: collection bound exceeded")
	}
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
		object := Object{
			ID: stableID("program-object", objectScopeID, value.SourceRef), SourceRef: value.SourceRef,
			Kind: value.Kind, Name: value.Name, Visibility: value.Visibility,
			Signature: value.Signature, Location: cloneLocation(value.Location),
			External: cloneExternalSymbol(value.External),
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

	totalWitnesses := 0
	for _, value := range input.Relations {
		if len(value.ToRefs) > MaxTargetsPerRelation || len(value.Witnesses) > MaxWitnessesPerRelation {
			return Index{}, fmt.Errorf("program index: relation bound exceeded")
		}
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
		relation := Relation{
			ID:        stableID("program-relation", index.Target.ID, value.SourceRef, string(value.Kind), fromID),
			SourceRef: value.SourceRef, Kind: value.Kind, FromID: fromID, ToIDs: toIDs,
			Resolution: value.Resolution, Invocation: value.Invocation, Location: cloneLocation(value.Location),
			TargetsObserved: value.TargetsObserved, TargetsOmitted: value.TargetsObserved - len(toIDs),
			Witnesses: witnesses, WitnessesObserved: value.WitnessesObserved,
			WitnessesOmitted: value.WitnessesObserved - len(witnesses),
		}
		if err := validateRelationShape(relation); err != nil {
			return Index{}, err
		}
		if totalWitnesses > MaxWitnesses-len(relation.Witnesses) {
			return Index{}, fmt.Errorf("program index: witness bound exceeded")
		}
		totalWitnesses += len(relation.Witnesses)
		index.Relations = append(index.Relations, relation)
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
	if err := validateAggregateText(index); err != nil {
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
		result.Objects[position].External = cloneExternalSymbol(index.Objects[position].External)
	}
	result.Relations = make([]Relation, len(index.Relations))
	copy(result.Relations, index.Relations)
	for position := range result.Relations {
		result.Relations[position].ToIDs = cloneStrings(index.Relations[position].ToIDs)
		result.Relations[position].Location = cloneLocation(index.Relations[position].Location)
		result.Relations[position].Witnesses = cloneWitnesses(index.Relations[position].Witnesses)
	}
	return result
}

// Validate checks identity bindings, canonical order, references, bounds,
// coverage and the complete-index SHA seal.
func (index Index) Validate() error {
	if index.Version != Version || !validSHA256(index.ScenarioSHA256) || !validSHA256(index.SourceSHA256) {
		return fmt.Errorf("program index: invalid producer identity")
	}
	if index.Objects == nil || index.Relations == nil || len(index.Target.Sources) > MaxTargetSources ||
		len(index.Target.Seeds) > MaxTargetSeeds ||
		len(index.Objects) > MaxObjects || len(index.Relations) > MaxRelations {
		return fmt.Errorf("program index: collection bound exceeded")
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
	totalWitnesses := 0
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
		if position > 0 && index.Relations[position-1].ID >= relation.ID {
			return fmt.Errorf("program index: relations are not canonical")
		}
		if totalWitnesses > MaxWitnesses-len(relation.Witnesses) {
			return fmt.Errorf("program index: witness bound exceeded")
		}
		totalWitnesses += len(relation.Witnesses)
	}
	if err := validateCoverage(index.Coverage, len(index.Objects), len(index.Relations)); err != nil {
		return err
	}
	wantCoverage := compileCoverage(index.Objects, index.Relations, index.Coverage.ObjectsObserved, index.Coverage.RelationsObserved)
	if index.Coverage != wantCoverage {
		return fmt.Errorf("program index: coverage mismatch")
	}
	if err := validateAggregateText(index); err != nil {
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
		!validText(target.Selector) || len(target.Sources) == 0 || len(target.Sources) > MaxTargetSources ||
		!validText(target.AnchorFileRef) || !hasTargetSourceRef(target.Sources, target.AnchorFileRef) ||
		target.Seeds == nil || len(target.Seeds) > MaxTargetSeeds {
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
		value.ToIDs == nil || value.Witnesses == nil || len(value.ToIDs) > MaxTargetsPerRelation ||
		len(value.Witnesses) > MaxWitnessesPerRelation || !canonicalStrings(value.ToIDs) {
		return fmt.Errorf("program index: invalid relation")
	}
	if value.TargetsObserved <= 0 || value.TargetsObserved < len(value.ToIDs) || value.TargetsObserved > MaxObservedCount ||
		value.TargetsOmitted != value.TargetsObserved-len(value.ToIDs) ||
		value.WitnessesObserved <= 0 || value.WitnessesObserved < len(value.Witnesses) || value.WitnessesObserved > MaxObservedCount ||
		value.WitnessesOmitted != value.WitnessesObserved-len(value.Witnesses) {
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
	}
	for _, count := range counts {
		if count < 0 || count > MaxObservedCount {
			return fmt.Errorf("program index: invalid coverage count")
		}
	}
	if value.ObjectsIndexed != objectsIndexed || value.ObjectsObserved < objectsIndexed ||
		value.ObjectsOmitted != value.ObjectsObserved-objectsIndexed ||
		value.RelationsIndexed != relationsIndexed || value.RelationsObserved < relationsIndexed ||
		value.RelationsOmitted != value.RelationsObserved-relationsIndexed ||
		value.ExactRelations+value.AlternativeRelations+value.UnresolvedRelations != relationsIndexed ||
		value.TargetsOmitted != value.TargetsObserved-value.TargetsIndexed ||
		value.WitnessesOmitted != value.WitnessesObserved-value.WitnessesIndexed {
		return fmt.Errorf("program index: invalid coverage")
	}
	return nil
}

func validateAggregateText(index Index) error {
	total := 0
	add := func(values ...string) error {
		for _, value := range values {
			if len(value) > MaxTextBytes {
				return fmt.Errorf("program index: scalar bound exceeded")
			}
			if total > MaxAggregateTextBytes-len(value) {
				return fmt.Errorf("program index: aggregate text bound exceeded")
			}
			total += len(value)
		}
		return nil
	}
	if err := add(index.ScenarioSHA256, index.SourceSHA256, index.Target.ID, index.Target.Language,
		index.Target.Kind, index.Target.Name, index.Target.Selector, index.Target.AnchorFileRef); err != nil {
		return err
	}
	for _, source := range index.Target.Sources {
		if err := add(source.FileRef, source.Path); err != nil {
			return err
		}
	}
	for _, seed := range index.Target.Seeds {
		if err := add(seed.ObjectID, string(seed.Kind), seed.Location.Path); err != nil {
			return err
		}
	}
	for _, object := range index.Objects {
		if err := add(object.ID, object.SourceRef, string(object.Kind), object.Name, string(object.Visibility),
			object.Signature, object.OwnerID, object.ContainerID); err != nil {
			return err
		}
		if object.External != nil {
			if err := add(object.External.PackagePath, object.External.Receiver, object.External.Name); err != nil {
				return err
			}
		}
		if object.Location != nil {
			if err := add(object.Location.Path); err != nil {
				return err
			}
		}
	}
	for _, relation := range index.Relations {
		if err := add(relation.ID, relation.SourceRef, string(relation.Kind), relation.FromID,
			string(relation.Resolution), relation.Invocation); err != nil {
			return err
		}
		if err := add(relation.ToIDs...); err != nil {
			return err
		}
		if relation.Location != nil {
			if err := add(relation.Location.Path); err != nil {
				return err
			}
		}
		for _, witness := range relation.Witnesses {
			if err := add(witness.Kind, witness.Detail, witness.SourceExpression); err != nil {
				return err
			}
			if witness.Location != nil {
				if err := add(witness.Location.Path); err != nil {
					return err
				}
			}
		}
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
	if len(encoded) > MaxIndexBytes {
		return "", fmt.Errorf("program index: canonical substrate is %d bytes, limit is %d", len(encoded), MaxIndexBytes)
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
	if value == "" || value != strings.TrimSpace(value) || len(value) > MaxTextBytes || !utf8.ValidString(value) {
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

func validateExternalSymbolBinding(kind ObjectKind, value *ExternalSymbol) error {
	if value == nil {
		return nil
	}
	if kind != ObjectExternalSymbol || !validText(value.PackagePath) ||
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
