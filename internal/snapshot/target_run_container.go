package snapshot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
)

const (
	TargetRunContainerVersion          = 2
	TargetRunContainerArtifactFilename = "target_run_container.v2.json"
	MaxTargetRunContainerBytes         = 4 << 20
)

// TargetRunSelection is the closed backend handoff from portfolio selection.
// TargetRefs is a set: provider order has no authority. DefaultTargetRef must
// name exactly one member of that set.
type TargetRunSelection struct {
	DefaultTargetRef string
	TargetRefs       []string
}

// TargetRunProjection binds one selected catalog entry to the exact scoped
// snapshot and analysis-file inputs that can be reproduced from the one
// complete deferred snapshot. CatalogIndex preserves the catalog's canonical
// order without turning display text into identity.
type TargetRunProjection struct {
	CatalogIndex        int                   `json:"catalog_index"`
	Target              analysistarget.Target `json:"target"`
	DisplayPath         string                `json:"display_path"`
	SnapshotSHA256      string                `json:"snapshot_sha256"`
	AnalysisFilesSHA256 string                `json:"analysis_files_sha256"`
}

// TargetRunContainer is the sealed selected-target authority for one run.
// Its persisted form contains only exact target identities and projection
// digests. The complete unscoped snapshot remains live-run-only and enables
// ScopedSnapshot to materialize every target without another repository scan
// or go list invocation.
type TargetRunContainer struct {
	Version          int                   `json:"version"`
	CatalogRef       string                `json:"catalog_ref"`
	DefaultTargetRef string                `json:"default_target_ref"`
	Targets          []TargetRunProjection `json:"targets"`
	SHA256           string                `json:"sha256"`

	deferred *Snapshot
}

// BuildTargetRunContainer validates an unordered selected set against the
// complete exact catalog, restores catalog order, and seals every reproducible
// target projection. It performs no filesystem access or provider call.
func BuildTargetRunContainer(
	source Snapshot,
	selection TargetRunSelection,
) (TargetRunContainer, error) {
	ownedSource, err := cloneTargetRunSnapshot(source)
	if err != nil {
		return TargetRunContainer{}, fmt.Errorf("target run container: own deferred snapshot: %w", err)
	}
	if err := validateTargetRunSource(ownedSource); err != nil {
		return TargetRunContainer{}, err
	}
	if selection.DefaultTargetRef == "" ||
		selection.DefaultTargetRef != strings.TrimSpace(selection.DefaultTargetRef) {
		return TargetRunContainer{}, fmt.Errorf("target run container: default target ref must be exact and non-empty")
	}
	if len(selection.TargetRefs) == 0 {
		return TargetRunContainer{}, fmt.Errorf("target run container: selected target set is empty")
	}

	selected := make(map[string]struct{}, len(selection.TargetRefs))
	for _, targetRef := range selection.TargetRefs {
		if targetRef == "" || targetRef != strings.TrimSpace(targetRef) {
			return TargetRunContainer{}, fmt.Errorf("target run container: selected target ref must be exact and non-empty")
		}
		if _, duplicate := selected[targetRef]; duplicate {
			return TargetRunContainer{}, fmt.Errorf("target run container: duplicate selected target ref")
		}
		selected[targetRef] = struct{}{}
	}
	if _, included := selected[selection.DefaultTargetRef]; !included {
		return TargetRunContainer{}, fmt.Errorf("target run container: default target is absent from selected targets")
	}

	container := TargetRunContainer{
		Version:          TargetRunContainerVersion,
		CatalogRef:       ownedSource.TargetCatalog.Ref,
		DefaultTargetRef: selection.DefaultTargetRef,
		Targets:          make([]TargetRunProjection, 0, len(selected)),
		deferred:         &ownedSource,
	}
	for catalogIndex, entry := range ownedSource.TargetCatalog.Entries {
		targetRef := entry.Candidate.Target.Ref
		if _, keep := selected[targetRef]; !keep {
			continue
		}
		scoped, scopeErr := ScopeAnalysisTarget(ownedSource, targetRef)
		if scopeErr != nil {
			return TargetRunContainer{}, fmt.Errorf("target run container: project catalog entry %d: %w", catalogIndex, scopeErr)
		}
		snapshotDigest, digestErr := targetRunSnapshotDigest(scoped)
		if digestErr != nil {
			return TargetRunContainer{}, fmt.Errorf("target run container: hash catalog entry %d snapshot: %w", catalogIndex, digestErr)
		}
		analysisFilesDigest, digestErr := targetRunAnalysisFilesDigest(scoped.FilteredFiles)
		if digestErr != nil {
			return TargetRunContainer{}, fmt.Errorf("target run container: hash catalog entry %d files: %w", catalogIndex, digestErr)
		}
		container.Targets = append(container.Targets, TargetRunProjection{
			CatalogIndex:        catalogIndex,
			Target:              entry.Candidate.Target.Snapshot(),
			DisplayPath:         entry.DisplayPath,
			SnapshotSHA256:      snapshotDigest,
			AnalysisFilesSHA256: analysisFilesDigest,
		})
	}
	if len(container.Targets) != len(selected) {
		return TargetRunContainer{}, fmt.Errorf("target run container: selection cites unknown target ref")
	}
	container.SHA256, err = targetRunContainerDigest(container)
	if err != nil {
		return TargetRunContainer{}, err
	}
	if err := container.ValidateAgainst(ownedSource); err != nil {
		return TargetRunContainer{}, err
	}
	return container, nil
}

// Validate verifies the standalone container shape, target identities,
// canonical selected order, and self-seal. ValidateAgainst additionally binds
// every projection to the complete deferred snapshot and catalog.
func (container TargetRunContainer) Validate() error {
	if container.Version != TargetRunContainerVersion ||
		container.CatalogRef == "" || container.CatalogRef != strings.TrimSpace(container.CatalogRef) ||
		container.DefaultTargetRef == "" || container.DefaultTargetRef != strings.TrimSpace(container.DefaultTargetRef) ||
		len(container.Targets) == 0 {
		return fmt.Errorf("target run container: invalid identity")
	}
	seenRefs := make(map[string]struct{}, len(container.Targets))
	defaultFound := false
	previousCatalogIndex := -1
	for index, projection := range container.Targets {
		if projection.CatalogIndex < 0 || projection.CatalogIndex <= previousCatalogIndex {
			return fmt.Errorf("target run container: targets are not in canonical catalog order")
		}
		previousCatalogIndex = projection.CatalogIndex
		if err := projection.Target.Validate(); err != nil {
			return fmt.Errorf("target run container: target %d: %w", index, err)
		}
		if projection.DisplayPath != projection.Target.DisplayPath() {
			return fmt.Errorf("target run container: target %d display path mismatch", index)
		}
		if _, duplicate := seenRefs[projection.Target.Ref]; duplicate {
			return fmt.Errorf("target run container: duplicate target ref")
		}
		seenRefs[projection.Target.Ref] = struct{}{}
		if projection.Target.Ref == container.DefaultTargetRef {
			defaultFound = true
		}
		if !validTargetRunDigest(projection.SnapshotSHA256) ||
			!validTargetRunDigest(projection.AnalysisFilesSHA256) {
			return fmt.Errorf("target run container: target %d has invalid projection digest", index)
		}
	}
	if !defaultFound {
		return fmt.Errorf("target run container: default target is absent from targets")
	}
	if !validTargetRunDigest(container.SHA256) {
		return fmt.Errorf("target run container: invalid seal")
	}
	want, err := targetRunContainerDigest(container)
	if err != nil {
		return err
	}
	if container.SHA256 != want {
		return fmt.Errorf("target run container: seal binding mismatch")
	}
	return nil
}

// ValidateAgainst proves that every selected target and digest is an exact
// projection of source. Source must be the one complete pre-scope snapshot.
func (container TargetRunContainer) ValidateAgainst(source Snapshot) error {
	if err := container.Validate(); err != nil {
		return err
	}
	if err := validateTargetRunSource(source); err != nil {
		return err
	}
	if container.CatalogRef != source.TargetCatalog.Ref {
		return fmt.Errorf("target run container: catalog binding mismatch")
	}
	for index, projection := range container.Targets {
		if projection.CatalogIndex >= len(source.TargetCatalog.Entries) {
			return fmt.Errorf("target run container: target %d catalog index is unavailable", index)
		}
		entry := source.TargetCatalog.Entries[projection.CatalogIndex]
		if !reflect.DeepEqual(projection.Target, entry.Candidate.Target) ||
			projection.DisplayPath != entry.DisplayPath {
			return fmt.Errorf("target run container: target %d catalog projection mismatch", index)
		}
		scoped, err := ScopeAnalysisTarget(source, projection.Target.Ref)
		if err != nil {
			return fmt.Errorf("target run container: restore target %d: %w", index, err)
		}
		snapshotDigest, err := targetRunSnapshotDigest(scoped)
		if err != nil {
			return fmt.Errorf("target run container: hash restored target %d snapshot: %w", index, err)
		}
		analysisFilesDigest, err := targetRunAnalysisFilesDigest(scoped.FilteredFiles)
		if err != nil {
			return fmt.Errorf("target run container: hash restored target %d files: %w", index, err)
		}
		if projection.SnapshotSHA256 != snapshotDigest ||
			projection.AnalysisFilesSHA256 != analysisFilesDigest {
			return fmt.Errorf("target run container: target %d projection binding mismatch", index)
		}
	}
	return nil
}

// ScopedSnapshot returns an independently owned ordinary one-target snapshot
// for an exact selected ref. It only projects the bound in-memory facts.
func (container TargetRunContainer) ScopedSnapshot(targetRef string) (Snapshot, error) {
	if container.deferred == nil {
		return Snapshot{}, fmt.Errorf("target run container: complete deferred snapshot is unavailable")
	}
	if err := container.ValidateAgainst(*container.deferred); err != nil {
		return Snapshot{}, err
	}
	selected := false
	for _, projection := range container.Targets {
		if projection.Target.Ref == targetRef {
			selected = true
			break
		}
	}
	if !selected {
		return Snapshot{}, fmt.Errorf("target run container: target ref is not selected")
	}
	ownedSource, err := cloneTargetRunSnapshot(*container.deferred)
	if err != nil {
		return Snapshot{}, fmt.Errorf("target run container: own deferred snapshot: %w", err)
	}
	scoped, err := ScopeAnalysisTarget(ownedSource, targetRef)
	if err != nil {
		return Snapshot{}, err
	}
	return scoped, nil
}

// Snapshot returns an independently owned live container handoff.
func (container TargetRunContainer) Snapshot() TargetRunContainer {
	result := container
	result.Targets = make([]TargetRunProjection, len(container.Targets))
	for index, projection := range container.Targets {
		result.Targets[index] = projection
		result.Targets[index].Target = projection.Target.Snapshot()
	}
	result.deferred = nil
	if container.deferred != nil {
		if source, err := cloneTargetRunSnapshot(*container.deferred); err == nil {
			result.deferred = &source
		}
	}
	return result
}

// BindTargetRunContainer attaches persisted public authority to its complete
// deferred snapshot after recomputing every target projection.
func BindTargetRunContainer(container TargetRunContainer, source Snapshot) (TargetRunContainer, error) {
	ownedSource, err := cloneTargetRunSnapshot(source)
	if err != nil {
		return TargetRunContainer{}, fmt.Errorf("target run container: own deferred snapshot: %w", err)
	}
	container = container.Snapshot()
	container.deferred = nil
	if err := container.ValidateAgainst(ownedSource); err != nil {
		return TargetRunContainer{}, err
	}
	container.deferred = &ownedSource
	return container, nil
}

func (container TargetRunContainer) CanonicalJSON() ([]byte, error) {
	if err := container.Validate(); err != nil {
		return nil, err
	}
	wire, err := json.Marshal(container)
	if err != nil {
		return nil, fmt.Errorf("target run container: encode artifact: %w", err)
	}
	if len(wire) > MaxTargetRunContainerBytes {
		return nil, fmt.Errorf("target run container: artifact exceeds bounded envelope")
	}
	return wire, nil
}

// DecodeTargetRunContainer accepts only the exact canonical artifact bytes.
// The returned public container remains unbound until paired with the complete
// deferred snapshot through BindTargetRunContainer.
func DecodeTargetRunContainer(data []byte) (TargetRunContainer, error) {
	if len(data) == 0 || len(data) > MaxTargetRunContainerBytes {
		return TargetRunContainer{}, fmt.Errorf("target run container: artifact exceeds bounded envelope")
	}
	var container TargetRunContainer
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&container); err != nil {
		return TargetRunContainer{}, fmt.Errorf("target run container: invalid artifact JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return TargetRunContainer{}, fmt.Errorf("target run container: invalid trailing artifact data")
	}
	if err := container.Validate(); err != nil {
		return TargetRunContainer{}, err
	}
	canonical, err := container.CanonicalJSON()
	if err != nil {
		return TargetRunContainer{}, err
	}
	if !bytes.Equal(data, canonical) {
		return TargetRunContainer{}, fmt.Errorf("target run container: artifact is not canonical")
	}
	return container, nil
}

// ValidateArtifact verifies exact persisted bytes and, for a live container,
// recomputes their projection bindings against the owned deferred snapshot.
func (container TargetRunContainer) ValidateArtifact(data []byte) error {
	decoded, err := DecodeTargetRunContainer(data)
	if err != nil {
		return err
	}
	if container.deferred == nil {
		return fmt.Errorf("target run container: complete deferred snapshot is unavailable")
	}
	_, err = BindTargetRunContainer(decoded, *container.deferred)
	return err
}

func validateTargetRunSource(source Snapshot) error {
	if source.TargetCatalog == nil {
		return fmt.Errorf("target run container: complete target catalog is unavailable")
	}
	if err := source.TargetCatalog.Validate(); err != nil {
		return fmt.Errorf("target run container: validate complete target catalog: %w", err)
	}
	if source.GoFacts == nil {
		return fmt.Errorf("target run container: complete Go facts are unavailable")
	}
	if source.AnalysisTarget != nil {
		return fmt.Errorf("target run container: source snapshot is already target-scoped")
	}
	return nil
}

func targetRunContainerDigest(container TargetRunContainer) (string, error) {
	identity := container
	identity.SHA256 = ""
	identity.deferred = nil
	wire, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("target run container: encode identity: %w", err)
	}
	digest := sha256.Sum256(append([]byte("target-run-container-v1\x00"), wire...))
	return hex.EncodeToString(digest[:]), nil
}

func targetRunSnapshotDigest(scoped Snapshot) (string, error) {
	wire, err := scoped.JSON()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:]), nil
}

func targetRunAnalysisFilesDigest(files []string) (string, error) {
	for index, file := range files {
		if file == "" || file != strings.TrimSpace(file) {
			return "", fmt.Errorf("analysis files contain a non-canonical path")
		}
		if index > 0 && files[index-1] >= file {
			return "", fmt.Errorf("analysis files are not in canonical order")
		}
	}
	wire, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(append([]byte("target-run-analysis-files-v1\x00"), wire...))
	return hex.EncodeToString(digest[:]), nil
}

func validTargetRunDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func cloneTargetRunSnapshot(source Snapshot) (Snapshot, error) {
	wire, err := json.Marshal(source)
	if err != nil {
		return Snapshot{}, err
	}
	var result Snapshot
	if err := json.Unmarshal(wire, &result); err != nil {
		return Snapshot{}, err
	}
	result.FilteredFiles = append([]string(nil), source.FilteredFiles...)
	if source.GoTargetAdvisory != nil {
		advisory := *source.GoTargetAdvisory
		advisory.Examples = append([]string(nil), source.GoTargetAdvisory.Examples...)
		result.GoTargetAdvisory = &advisory
	}
	if source.GoTargetSelection != nil {
		selection := *source.GoTargetSelection
		selection.Examples = append([]string(nil), source.GoTargetSelection.Examples...)
		result.GoTargetSelection = &selection
	}
	if source.TargetCatalog != nil {
		catalog := source.TargetCatalog.Snapshot()
		result.TargetCatalog = &catalog
	}
	return result, nil
}

// OwnSnapshot returns an independently owned copy of a deterministic snapshot.
// It is the handoff used when an already-bound target projection enters the
// ordinary orientation pipeline without rebuilding repository facts.
func OwnSnapshot(source Snapshot) (Snapshot, error) {
	return cloneTargetRunSnapshot(source)
}
