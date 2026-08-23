package report

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/dvordrova/repomap/internal/activityentrypoint"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	ActivityEntrypointViewVersion  = 2
	MaxActivityEntrypointViewBytes = 16 << 20
)

// ActivityEntrypointView is the report-owned projection of model-selected
// activity starts for one exact ProgramIndex. It carries only selected
// activity objects and exact local facts needed to understand and open them;
// the browser never promotes target seeds or unselected objects as substitutes.
type ActivityEntrypointView struct {
	Version            int                            `json:"version"`
	ProgramTargetID    string                         `json:"program_target_id"`
	ProgramIndexSHA256 string                         `json:"program_index_sha256"`
	Entrypoints        []ActivityEntrypointViewObject `json:"entrypoints"`
	Coverage           activityentrypoint.Coverage    `json:"coverage"`
}

// ActivityEntrypointViewObject is one exact selected ProgramIndex activity
// object: a callable or an exact seeded module/package launch anchor.
// Owner and container names, when present, are exact declaration facts rather
// than browser-side joins. SeedKinds remain structural launch evidence and do
// not independently grant activity-entrypoint authority.
type ActivityEntrypointViewObject struct {
	ObjectID      string                  `json:"object_id"`
	Kind          programindex.ObjectKind `json:"kind"`
	Name          string                  `json:"name"`
	Signature     string                  `json:"signature,omitempty"`
	Visibility    programindex.Visibility `json:"visibility"`
	OwnerName     string                  `json:"owner_name,omitempty"`
	ContainerName string                  `json:"container_name,omitempty"`
	Location      programindex.Location   `json:"location"`
	SeedKinds     []programindex.SeedKind `json:"seed_kinds"`
}

// NewActivityEntrypointView revalidates the producer result against its exact
// sealed ProgramIndex, then derives a bounded presentation handoff without
// name/path matching or semantic repair.
func NewActivityEntrypointView(
	result activityentrypoint.Result,
	index programindex.Index,
) (*ActivityEntrypointView, error) {
	if err := result.ValidateAgainst(index); err != nil {
		return nil, fmt.Errorf("activity entrypoint view: producer authority: %w", err)
	}
	objectsByID := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objectsByID[object.ID] = object
	}
	seedKindsByObjectID := make(map[string][]programindex.SeedKind)
	for _, seed := range index.Target.Seeds {
		seedKindsByObjectID[seed.ObjectID] = append(
			seedKindsByObjectID[seed.ObjectID], seed.Kind,
		)
	}
	for objectID := range seedKindsByObjectID {
		sort.Slice(seedKindsByObjectID[objectID], func(left, right int) bool {
			return seedKindsByObjectID[objectID][left] < seedKindsByObjectID[objectID][right]
		})
		seedKindsByObjectID[objectID] = compactActivitySeedKinds(seedKindsByObjectID[objectID])
	}

	view := &ActivityEntrypointView{
		Version: ActivityEntrypointViewVersion, ProgramTargetID: index.Target.ID,
		ProgramIndexSHA256: index.SHA256,
		Entrypoints:        make([]ActivityEntrypointViewObject, 0, len(result.Objects)),
		Coverage:           result.Coverage,
	}
	for _, object := range result.Objects {
		if object.Location == nil {
			return nil, fmt.Errorf("activity entrypoint view: selected object %q has no exact location", object.ID)
		}
		entrypoint := ActivityEntrypointViewObject{
			ObjectID: object.ID, Kind: object.Kind, Name: object.Name,
			Signature: object.Signature, Visibility: object.Visibility,
			Location:  *object.Location,
			SeedKinds: append([]programindex.SeedKind{}, seedKindsByObjectID[object.ID]...),
		}
		if owner, ok := objectsByID[object.OwnerID]; ok {
			entrypoint.OwnerName = owner.Name
		}
		if container, ok := objectsByID[object.ContainerID]; ok {
			entrypoint.ContainerName = container.Name
		}
		view.Entrypoints = append(view.Entrypoints, entrypoint)
	}
	if err := view.Validate(); err != nil {
		return nil, fmt.Errorf("activity entrypoint view: invalid projection: %w", err)
	}
	return view, nil
}

func compactActivitySeedKinds(values []programindex.SeedKind) []programindex.SeedKind {
	result := make([]programindex.SeedKind, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

// ValidateAgainst re-derives the complete projection from the exact producer
// inputs and rejects any changed selection, source fact, or context join.
func (view ActivityEntrypointView) ValidateAgainst(
	result activityentrypoint.Result,
	index programindex.Index,
) error {
	if err := view.Validate(); err != nil {
		return err
	}
	expected, err := NewActivityEntrypointView(result, index)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(view, *expected) {
		return fmt.Errorf("activity entrypoint view: projection does not match exact producer authority")
	}
	return nil
}

// Validate checks the standalone report/browser handoff. Exact equality with
// the producer artifact is separately enforced by ValidateAgainst and the run
// manifest verifier.
func (view ActivityEntrypointView) Validate() error {
	if view.Version != ActivityEntrypointViewVersion ||
		!validCubeMapViewText(view.ProgramTargetID, false) ||
		!validCubeMapViewSHA256(view.ProgramIndexSHA256) || view.Entrypoints == nil ||
		len(view.Entrypoints) > activityentrypoint.MaxSelectedEntrypoints {
		return fmt.Errorf("activity entrypoint view: invalid identity or entry bound")
	}
	if err := validateActivityEntrypointViewCoverage(view.Coverage, len(view.Entrypoints)); err != nil {
		return err
	}
	for position, entrypoint := range view.Entrypoints {
		if err := validateActivityEntrypointViewObject(entrypoint); err != nil {
			return fmt.Errorf("activity entrypoint view: entrypoint %d: %w", position, err)
		}
		if position > 0 && !activityEntrypointViewCallableLess(view.Entrypoints[position-1], entrypoint) {
			return fmt.Errorf("activity entrypoint view: entrypoints are not canonical")
		}
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		return fmt.Errorf("activity entrypoint view: encode bound check: %w", err)
	}
	if len(encoded) > MaxActivityEntrypointViewBytes {
		return fmt.Errorf(
			"activity entrypoint view: JSON size %d exceeds projection limit %d",
			len(encoded), MaxActivityEntrypointViewBytes,
		)
	}
	return nil
}

func validateActivityEntrypointViewCoverage(
	coverage activityentrypoint.Coverage,
	entrypoints int,
) error {
	if coverage.ObjectsIndexed < 0 || coverage.ObjectsIndexed > programindex.MaxObjects ||
		coverage.ProgramObjectsOmitted < 0 || coverage.ProgramRelationsOmitted < 0 ||
		coverage.ProgramTargetsOmitted < 0 || coverage.ProgramWitnessesOmitted < 0 ||
		coverage.CallablesIndexed < 0 || coverage.CallablesIndexed > coverage.ObjectsIndexed ||
		coverage.CallablesWithoutLocation < 0 ||
		coverage.CallablesWithoutLocation > coverage.CallablesIndexed ||
		coverage.CallablesIneligible < 0 ||
		coverage.CallablesIneligible > coverage.CallablesIndexed-coverage.CallablesWithoutLocation ||
		coverage.SeededModulesIndexed < 0 ||
		coverage.SeededModulesIndexed > coverage.ObjectsIndexed-coverage.CallablesIndexed ||
		coverage.SeededModulesWithoutLocation < 0 ||
		coverage.SeededModulesWithoutLocation > coverage.SeededModulesIndexed ||
		coverage.CandidatesObserved < 0 ||
		coverage.CandidatesObserved+coverage.CallablesWithoutLocation+coverage.CallablesIneligible+
			coverage.SeededModulesWithoutLocation != coverage.CallablesIndexed+coverage.SeededModulesIndexed ||
		coverage.CandidatesAdvertised < 0 || coverage.CandidatesOmitted != 0 ||
		coverage.CandidatesAdvertised+coverage.CandidatesOmitted != coverage.CandidatesObserved ||
		coverage.CandidatesAdvertised > activityentrypoint.MaxAdvertisedCandidates ||
		coverage.Selected != entrypoints || coverage.Selected < 0 ||
		coverage.Selected > coverage.CandidatesAdvertised ||
		coverage.Selected > activityentrypoint.MaxSelectedEntrypoints {
		return fmt.Errorf("activity entrypoint view: invalid producer coverage")
	}
	if coverage.CandidatesAdvertised == 0 {
		if coverage.Batches != 0 || coverage.ModelCalled {
			return fmt.Errorf("activity entrypoint view: empty coverage called the model")
		}
		return nil
	}
	if coverage.Batches <= 0 || coverage.Batches > activityentrypoint.MaxCandidateBatches ||
		!coverage.ModelCalled {
		return fmt.Errorf("activity entrypoint view: non-empty coverage has invalid batch execution")
	}
	return nil
}

func validateActivityEntrypointViewObject(value ActivityEntrypointViewObject) error {
	if !validCubeMapViewText(value.ObjectID, false) ||
		(value.Kind != programindex.ObjectFunction && value.Kind != programindex.ObjectMethod &&
			value.Kind != programindex.ObjectLambda && value.Kind != programindex.ObjectModule &&
			value.Kind != programindex.ObjectPackage) ||
		!validCubeMapViewText(value.Name, false) || !validCubeMapViewText(value.Signature, true) ||
		!value.Visibility.Valid() || !validCubeMapViewText(value.OwnerName, true) ||
		!validCubeMapViewText(value.ContainerName, true) ||
		!validCubeMapViewLocation(CubeMapViewLocation{
			Path: value.Location.Path, Line: value.Location.Line, Column: value.Location.Column,
		}, true) || value.SeedKinds == nil {
		return fmt.Errorf("invalid selected activity object")
	}
	previous := programindex.SeedKind("")
	for _, kind := range value.SeedKinds {
		if !kind.Valid() || previous != "" && previous >= kind {
			return fmt.Errorf("seed kinds are invalid or non-canonical")
		}
		previous = kind
	}
	return nil
}

func activityEntrypointViewCallableLess(
	left ActivityEntrypointViewObject,
	right ActivityEntrypointViewObject,
) bool {
	if left.Location.Path != right.Location.Path {
		return left.Location.Path < right.Location.Path
	}
	if left.Location.Line != right.Location.Line {
		return left.Location.Line < right.Location.Line
	}
	if left.Location.Column != right.Location.Column {
		return left.Location.Column < right.Location.Column
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	return left.ObjectID < right.ObjectID
}
