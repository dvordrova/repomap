package report

import (
	"fmt"
	"io/fs"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	ProgramViewVersion = 5

	// These values are advisory scale thresholds retained only for aggregate
	// diagnostics. ProgramView always projects every indexed row and witness;
	// crossing a threshold never authorizes truncation or rejection.
	MaxProgramViewSeeds                = 1_024
	MaxProgramViewObjects              = 2_048
	MaxProgramViewRelations            = 4_096
	MaxProgramViewWitnessesPerRelation = 4

	maxProgramViewTextBytes = 8 << 20
)

// ProgramView is a complete, provider-free presentation projection of one
// validated ProgramIndex. It retains exact local identities and structural
// facts; it does not assign product semantics to objects or relations.
type ProgramView struct {
	Version     int    `json:"version"`
	TargetID    string `json:"target_id"`
	IndexSHA256 string `json:"index_sha256"`

	Seeds     []ProgramViewSeed     `json:"seeds"`
	Objects   []ProgramViewObject   `json:"objects"`
	Relations []ProgramViewRelation `json:"relations"`

	IndexCoverage programindex.Coverage       `json:"index_coverage"`
	Projection    ProgramViewProjectionCounts `json:"projection"`
}

// ProgramViewSeed resolves a target seed to its exact ProgramIndex object.
// LaunchLocation is the adapter's launch fact; DeclarationLocation is the
// object's declaration fact. They remain separate even when they coincide.
type ProgramViewSeed struct {
	Kind                programindex.SeedKind   `json:"kind"`
	ObjectID            string                  `json:"object_id"`
	Name                string                  `json:"name"`
	ObjectKind          programindex.ObjectKind `json:"object_kind"`
	Signature           string                  `json:"signature,omitempty"`
	Visibility          programindex.Visibility `json:"visibility"`
	OwnerID             string                  `json:"owner_id,omitempty"`
	ContainerID         string                  `json:"container_id,omitempty"`
	LaunchLocation      *programindex.Location  `json:"launch_location"`
	DeclarationLocation *programindex.Location  `json:"declaration_location"`
}

// ProgramViewObject is an exact, identity-preserving ProgramIndex object row.
type ProgramViewObject struct {
	ID          string                       `json:"id"`
	SourceRef   string                       `json:"source_ref"`
	Kind        programindex.ObjectKind      `json:"kind"`
	Name        string                       `json:"name"`
	Visibility  programindex.Visibility      `json:"visibility"`
	Signature   string                       `json:"signature,omitempty"`
	OwnerID     string                       `json:"owner_id,omitempty"`
	ContainerID string                       `json:"container_id,omitempty"`
	Location    *programindex.Location       `json:"location,omitempty"`
	External    *programindex.ExternalSymbol `json:"external,omitempty"`
}

// ProgramViewRelation preserves one complete ProgramIndex relation with every
// indexed endpoint and witness.
type ProgramViewRelation struct {
	ID         string                    `json:"id"`
	SourceRef  string                    `json:"source_ref"`
	Kind       programindex.RelationKind `json:"kind"`
	Resolution programindex.Resolution   `json:"resolution"`
	FromID     string                    `json:"from_id"`
	ToIDs      []string                  `json:"to_ids"`
	Invocation string                    `json:"invocation,omitempty"`
	Location   *programindex.Location    `json:"location,omitempty"`

	TargetsObserved            int                    `json:"targets_observed"`
	TargetsIndexed             int                    `json:"targets_indexed"`
	TargetsOmitted             int                    `json:"targets_omitted"`
	WitnessesObserved          int                    `json:"witnesses_observed"`
	WitnessesIndexed           int                    `json:"witnesses_indexed"`
	WitnessesOmitted           int                    `json:"witnesses_omitted"`
	Witnesses                  []programindex.Witness `json:"witnesses"`
	WitnessesProjectionOmitted int                    `json:"witnesses_projection_omitted"`
}

// ProgramViewProjectionCounts distinguishes upstream ProgramIndex coverage
// from presentation coverage. New projections always have Eligible == Shown
// and Omitted == 0; the explicit fields keep the handoff self-describing.
type ProgramViewProjectionCounts struct {
	Seeds     ProgramViewCollectionCounts `json:"seeds"`
	Objects   ProgramViewCollectionCounts `json:"objects"`
	Relations ProgramViewCollectionCounts `json:"relations"`
}

type ProgramViewCollectionCounts struct {
	Eligible int `json:"eligible"`
	Shown    int `json:"shown"`
	Omitted  int `json:"omitted"`
}

// NewProgramView validates the complete input, projects it without name/path
// joins, then validates the closed presentation handoff before returning it.
func NewProgramView(index programindex.Index) (*ProgramView, error) {
	if err := index.Validate(); err != nil {
		return nil, fmt.Errorf("program view: invalid program index: %w", err)
	}

	objectsByID := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objectsByID[object.ID] = object
	}

	view := &ProgramView{
		Version: ProgramViewVersion, TargetID: index.Target.ID, IndexSHA256: index.SHA256,
		Seeds:         make([]ProgramViewSeed, 0, len(index.Target.Seeds)),
		Objects:       make([]ProgramViewObject, 0, len(index.Objects)),
		Relations:     make([]ProgramViewRelation, 0, len(index.Relations)),
		IndexCoverage: index.Coverage,
	}
	for _, seed := range index.Target.Seeds {
		object, ok := objectsByID[seed.ObjectID]
		if !ok {
			return nil, fmt.Errorf("program view: seed object %q is missing", seed.ObjectID)
		}
		view.Seeds = append(view.Seeds, programViewSeed(seed, object))
	}
	for _, object := range index.Objects {
		view.Objects = append(view.Objects, programViewObject(object))
	}
	for _, relation := range index.Relations {
		view.Relations = append(view.Relations, programViewRelation(relation))
	}

	view.Projection = ProgramViewProjectionCounts{
		Seeds:     newProgramViewCollectionCounts(len(index.Target.Seeds), len(view.Seeds)),
		Objects:   newProgramViewCollectionCounts(len(index.Objects), len(view.Objects)),
		Relations: newProgramViewCollectionCounts(len(index.Relations), len(view.Relations)),
	}
	if err := view.Validate(); err != nil {
		return nil, fmt.Errorf("program view: invalid projection: %w", err)
	}
	return view, nil
}

// Validate checks the complete projection independently of the in-memory
// ProgramIndex used to construct it.
func (view ProgramView) Validate() error {
	if view.Version != ProgramViewVersion || !validProgramViewText(view.TargetID) ||
		!strings.HasPrefix(view.TargetID, "program-target-") || !validProgramViewSHA256(view.IndexSHA256) {
		return fmt.Errorf("invalid identity")
	}
	if view.Seeds == nil || view.Objects == nil || view.Relations == nil {
		return fmt.Errorf("missing collection")
	}

	objectsByID := make(map[string]ProgramViewObject, len(view.Objects))
	previousObjectID := ""
	for _, object := range view.Objects {
		if err := validateProgramViewObject(object); err != nil {
			return err
		}
		if previousObjectID != "" && previousObjectID >= object.ID {
			return fmt.Errorf("objects are not canonical")
		}
		previousObjectID = object.ID
		objectsByID[object.ID] = object
	}
	for _, object := range view.Objects {
		if object.OwnerID != "" {
			if _, exists := objectsByID[object.OwnerID]; !exists || object.OwnerID == object.ID {
				return fmt.Errorf("object %q has missing owner closure", object.ID)
			}
		}
		if object.ContainerID != "" {
			if _, exists := objectsByID[object.ContainerID]; !exists || object.ContainerID == object.ID {
				return fmt.Errorf("object %q has missing container closure", object.ID)
			}
		}
	}

	previousSeedKey := ""
	for _, seed := range view.Seeds {
		if err := validateProgramViewSeed(seed, objectsByID); err != nil {
			return err
		}
		key := programViewSeedKey(seed)
		if previousSeedKey != "" && previousSeedKey >= key {
			return fmt.Errorf("seeds are not canonical")
		}
		previousSeedKey = key
	}

	previousRelationID := ""
	for _, relation := range view.Relations {
		if err := validateProgramViewRelation(relation, objectsByID); err != nil {
			return err
		}
		if previousRelationID != "" && previousRelationID >= relation.ID {
			return fmt.Errorf("relations are not canonical")
		}
		previousRelationID = relation.ID
	}
	if err := validateProgramViewIndexCoverage(view.IndexCoverage); err != nil {
		return err
	}
	if err := validateProgramViewCollectionCounts(view.Projection.Seeds, len(view.Seeds), len(view.Seeds)); err != nil {
		return fmt.Errorf("seed projection: %w", err)
	}
	if err := validateProgramViewCollectionCounts(view.Projection.Objects, view.IndexCoverage.ObjectsIndexed, len(view.Objects)); err != nil {
		return fmt.Errorf("object projection: %w", err)
	}
	if err := validateProgramViewCollectionCounts(view.Projection.Relations, view.IndexCoverage.RelationsIndexed, len(view.Relations)); err != nil {
		return fmt.Errorf("relation projection: %w", err)
	}
	return nil
}

func programViewSeed(seed programindex.TargetSeed, object programindex.Object) ProgramViewSeed {
	return ProgramViewSeed{
		Kind: seed.Kind, ObjectID: object.ID, Name: object.Name, ObjectKind: object.Kind,
		Signature: object.Signature, Visibility: object.Visibility, OwnerID: object.OwnerID,
		ContainerID: object.ContainerID, LaunchLocation: cloneProgramViewLocation(seed.Location),
		DeclarationLocation: cloneProgramViewLocation(object.Location),
	}
}

func programViewObject(object programindex.Object) ProgramViewObject {
	return ProgramViewObject{
		ID: object.ID, SourceRef: object.SourceRef, Kind: object.Kind, Name: object.Name,
		Visibility: object.Visibility, Signature: object.Signature, OwnerID: object.OwnerID,
		ContainerID: object.ContainerID, Location: cloneProgramViewLocation(object.Location),
		External: cloneProgramViewExternal(object.External),
	}
}

func programViewRelation(relation programindex.Relation) ProgramViewRelation {
	// An unresolved relation intentionally has a present-but-empty target
	// collection. Preserve that distinction across the presentation handoff;
	// nil is invalid rather than an omission signal.
	toIDs := append([]string{}, relation.ToIDs...)
	witnesses := cloneProgramViewWitnesses(relation.Witnesses)
	return ProgramViewRelation{
		ID: relation.ID, SourceRef: relation.SourceRef, Kind: relation.Kind, Resolution: relation.Resolution,
		FromID: relation.FromID, ToIDs: toIDs, Invocation: relation.Invocation,
		Location: cloneProgramViewLocation(relation.Location), TargetsObserved: relation.TargetsObserved,
		TargetsIndexed: len(relation.ToIDs), TargetsOmitted: relation.TargetsOmitted,
		WitnessesObserved: relation.WitnessesObserved, WitnessesIndexed: len(relation.Witnesses),
		WitnessesOmitted: relation.WitnessesOmitted, Witnesses: witnesses,
		WitnessesProjectionOmitted: 0,
	}
}

func newProgramViewCollectionCounts(eligible, shown int) ProgramViewCollectionCounts {
	return ProgramViewCollectionCounts{Eligible: eligible, Shown: shown, Omitted: eligible - shown}
}

func validateProgramViewObject(object ProgramViewObject) error {
	if !validProgramViewText(object.ID) || !strings.HasPrefix(object.ID, "program-object-") ||
		!validProgramViewText(object.SourceRef) || !object.Kind.Valid() || !validProgramViewText(object.Name) ||
		!object.Visibility.Valid() || !validOptionalProgramViewText(object.Signature) ||
		!validOptionalProgramViewText(object.OwnerID) || !validOptionalProgramViewText(object.ContainerID) ||
		!validProgramViewLocation(object.Location) {
		return fmt.Errorf("invalid object %q", object.ID)
	}
	if object.External != nil &&
		(object.Kind != programindex.ObjectExternalSymbol ||
			!object.External.AuthorityKind.Valid() ||
			!validProgramViewText(object.External.PackagePath) ||
			!validOptionalProgramViewText(object.External.Receiver) ||
			!validProgramViewText(object.External.Name)) {
		return fmt.Errorf("invalid external object authority %q", object.ID)
	}
	return nil
}

func validateProgramViewSeed(seed ProgramViewSeed, objectsByID map[string]ProgramViewObject) error {
	if !seed.Kind.Valid() || !validProgramViewLocation(seed.LaunchLocation) || seed.LaunchLocation == nil ||
		!validProgramViewLocation(seed.DeclarationLocation) || seed.DeclarationLocation == nil {
		return fmt.Errorf("invalid seed %q", seed.ObjectID)
	}
	object, exists := objectsByID[seed.ObjectID]
	if !exists || seed.Name != object.Name || seed.ObjectKind != object.Kind || seed.Signature != object.Signature ||
		seed.Visibility != object.Visibility || seed.OwnerID != object.OwnerID || seed.ContainerID != object.ContainerID ||
		!equalProgramViewLocations(seed.DeclarationLocation, object.Location) {
		return fmt.Errorf("seed %q does not resolve to its projected object", seed.ObjectID)
	}
	return nil
}

func validateProgramViewRelation(relation ProgramViewRelation, objectsByID map[string]ProgramViewObject) error {
	if !validProgramViewText(relation.ID) || !strings.HasPrefix(relation.ID, "program-relation-") ||
		!validProgramViewText(relation.SourceRef) || !relation.Kind.Valid() || !relation.Resolution.Valid() ||
		!validProgramViewText(relation.FromID) || !validOptionalProgramViewText(relation.Invocation) ||
		!validProgramViewLocation(relation.Location) || relation.ToIDs == nil || relation.Witnesses == nil {
		return fmt.Errorf("invalid relation %q", relation.ID)
	}
	if _, exists := objectsByID[relation.FromID]; !exists {
		return fmt.Errorf("relation %q has unprojected source", relation.ID)
	}
	previousID := ""
	for _, id := range relation.ToIDs {
		if !validProgramViewText(id) || previousID != "" && previousID >= id {
			return fmt.Errorf("relation %q targets are not canonical", relation.ID)
		}
		if _, exists := objectsByID[id]; !exists {
			return fmt.Errorf("relation %q has unprojected target", relation.ID)
		}
		previousID = id
	}
	if relation.TargetsIndexed != len(relation.ToIDs) || relation.TargetsObserved < relation.TargetsIndexed ||
		relation.TargetsOmitted != relation.TargetsObserved-relation.TargetsIndexed ||
		relation.WitnessesIndexed < 0 || relation.WitnessesObserved < relation.WitnessesIndexed ||
		relation.WitnessesOmitted != relation.WitnessesObserved-relation.WitnessesIndexed ||
		relation.WitnessesIndexed != len(relation.Witnesses) || relation.WitnessesProjectionOmitted != 0 {
		return fmt.Errorf("relation %q has invalid counts", relation.ID)
	}
	previousWitness := ""
	for _, witness := range relation.Witnesses {
		if !validProgramViewText(witness.Kind) || !validOptionalProgramViewText(witness.Detail) ||
			!validOptionalProgramViewText(witness.SourceExpression) ||
			!validProgramViewLocation(witness.Location) {
			return fmt.Errorf("relation %q has invalid witness", relation.ID)
		}
		key := programViewWitnessKey(witness)
		if previousWitness != "" && previousWitness >= key {
			return fmt.Errorf("relation %q witnesses are not canonical", relation.ID)
		}
		previousWitness = key
	}
	switch relation.Resolution {
	case programindex.ResolutionExact:
		if len(relation.ToIDs) != 1 || relation.TargetsOmitted != 0 || relation.WitnessesIndexed == 0 {
			return fmt.Errorf("relation %q has invalid exact resolution", relation.ID)
		}
	case programindex.ResolutionAlternatives:
		if len(relation.ToIDs) < 1 || relation.WitnessesIndexed == 0 {
			return fmt.Errorf("relation %q has invalid alternatives resolution", relation.ID)
		}
	case programindex.ResolutionUnresolved:
		if len(relation.ToIDs) != 0 {
			return fmt.Errorf("relation %q has invalid unresolved resolution", relation.ID)
		}
	}
	return nil
}

func validateProgramViewIndexCoverage(coverage programindex.Coverage) error {
	counts := []int{
		coverage.ObjectsObserved, coverage.ObjectsIndexed, coverage.ObjectsOmitted,
		coverage.RelationsObserved, coverage.RelationsIndexed, coverage.RelationsOmitted,
		coverage.ExactRelations, coverage.AlternativeRelations, coverage.UnresolvedRelations,
		coverage.TargetsObserved, coverage.TargetsIndexed, coverage.TargetsOmitted,
		coverage.WitnessesObserved, coverage.WitnessesIndexed, coverage.WitnessesOmitted,
	}
	for _, count := range counts {
		if count < 0 {
			return fmt.Errorf("invalid index coverage count")
		}
	}
	if coverage.ObjectsOmitted != coverage.ObjectsObserved-coverage.ObjectsIndexed ||
		coverage.RelationsOmitted != coverage.RelationsObserved-coverage.RelationsIndexed ||
		coverage.ExactRelations+coverage.AlternativeRelations+coverage.UnresolvedRelations != coverage.RelationsIndexed ||
		coverage.TargetsOmitted != coverage.TargetsObserved-coverage.TargetsIndexed ||
		coverage.WitnessesOmitted != coverage.WitnessesObserved-coverage.WitnessesIndexed {
		return fmt.Errorf("invalid index coverage")
	}
	return nil
}

func validateProgramViewCollectionCounts(counts ProgramViewCollectionCounts, eligible, shown int) error {
	if eligible != shown || counts.Eligible != eligible || counts.Shown != shown || counts.Omitted != 0 {
		return fmt.Errorf("invalid shown/eligible/omitted counts")
	}
	return nil
}

func cloneProgramViewLocation(location *programindex.Location) *programindex.Location {
	if location == nil {
		return nil
	}
	result := *location
	return &result
}

func equalProgramViewLocations(left, right *programindex.Location) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func programViewSeedKey(seed ProgramViewSeed) string {
	return seed.ObjectID + "\x00" + string(seed.Kind) + "\x00" + programViewLocationKey(seed.LaunchLocation)
}

func programViewLocationKey(location *programindex.Location) string {
	if location == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d:%d", location.Path, location.Line, location.Column)
}

func programViewTextBytes(view ProgramView) int {
	total := len(view.TargetID) + len(view.IndexSHA256)
	for _, seed := range view.Seeds {
		total += programViewSeedTextBytes(seed)
	}
	for _, object := range view.Objects {
		total += programViewObjectTextBytes(object)
	}
	for _, relation := range view.Relations {
		total += programViewRelationTextBytes(relation)
	}
	return total
}

func programViewSeedTextBytes(seed ProgramViewSeed) int {
	return len(string(seed.Kind)) + len(seed.ObjectID) + len(seed.Name) + len(string(seed.ObjectKind)) +
		len(seed.Signature) + len(string(seed.Visibility)) + len(seed.OwnerID) + len(seed.ContainerID) +
		programViewLocationTextBytes(seed.LaunchLocation) + programViewLocationTextBytes(seed.DeclarationLocation)
}

func programViewObjectTextBytes(object ProgramViewObject) int {
	return len(object.ID) + len(object.SourceRef) + len(string(object.Kind)) + len(object.Name) +
		len(string(object.Visibility)) + len(object.Signature) + len(object.OwnerID) + len(object.ContainerID) +
		programViewLocationTextBytes(object.Location) + programViewExternalTextBytes(object.External)
}

func programViewExternalTextBytes(value *programindex.ExternalSymbol) int {
	if value == nil {
		return 0
	}
	return len(string(value.AuthorityKind)) + len(value.PackagePath) + len(value.Receiver) + len(value.Name)
}

func cloneProgramViewExternal(value *programindex.ExternalSymbol) *programindex.ExternalSymbol {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func programViewRelationTextBytes(relation ProgramViewRelation) int {
	total := len(relation.ID) + len(relation.SourceRef) + len(string(relation.Kind)) +
		len(string(relation.Resolution)) + len(relation.FromID) + len(relation.Invocation) +
		programViewLocationTextBytes(relation.Location)
	for _, id := range relation.ToIDs {
		total += len(id)
	}
	for _, witness := range relation.Witnesses {
		total += len(witness.Kind) + len(witness.Detail) + len(witness.SourceExpression) +
			programViewLocationTextBytes(witness.Location)
	}
	return total
}

func cloneProgramViewWitnesses(values []programindex.Witness) []programindex.Witness {
	result := make([]programindex.Witness, len(values))
	for position, value := range values {
		result[position] = programindex.Witness{
			Kind: value.Kind, Detail: value.Detail, SourceExpression: value.SourceExpression,
			Location: cloneProgramViewLocation(value.Location),
		}
	}
	return result
}

func programViewWitnessKey(value programindex.Witness) string {
	return programViewLocationKey(value.Location) + "\x00" + value.Kind + "\x00" + value.Detail +
		"\x00" + value.SourceExpression
}

func programViewLocationTextBytes(location *programindex.Location) int {
	if location == nil {
		return 0
	}
	return len(location.Path)
}

func validProgramViewText(value string) bool {
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

func validOptionalProgramViewText(value string) bool {
	return value == "" || validProgramViewText(value)
}

func validProgramViewLocation(location *programindex.Location) bool {
	if location == nil {
		return true
	}
	return validProgramViewPath(location.Path) && location.Line > 0 && location.Column > 0
}

func validProgramViewPath(value string) bool {
	if value == "" || value != strings.TrimSpace(value) ||
		strings.Contains(value, "\\") || strings.HasPrefix(value, "/") || !fs.ValidPath(value) ||
		value == "." || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validProgramViewSHA256(value string) bool {
	if len(value) != 64 {
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
