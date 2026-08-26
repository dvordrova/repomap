package report

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	CoreMapViewVersion = 6

	MaxCoreMapViewBytes = 8 << 20
)

// CoreMapView is the report-owned projection of the progressive CoreMap
// result produced for one exact ProgramIndex. Block names, purposes, and
// memberships in model-authority groups remain model-owned hypotheses. A
// provenance-marked local-unassigned group is only exact omission accounting,
// never a semantic membership. File, symbol, kind, and location fields are
// exact producer-restored evidence; the report never derives new groups from
// their spelling.
type CoreMapView struct {
	Version                int                 `json:"version"`
	ProgramTargetID        string              `json:"program_target_id"`
	ProgramIndexSHA256     string              `json:"program_index_sha256"`
	IntegrationUsageSHA256 string              `json:"integration_usage_sha256,omitempty"`
	BaselineCore           []CoreMapViewBlock  `json:"baseline_core"`
	RefinedCore            []CoreMapViewBlock  `json:"refined_core"`
	RefinedGroups          []CoreMapViewGroup  `json:"refined_groups"`
	Coverage               CoreMapViewCoverage `json:"coverage"`
}

type CoreMapViewGroup struct {
	ID           string                 `json:"id"`
	Authority    coremap.GroupAuthority `json:"authority"`
	Name         string                 `json:"name"`
	Purpose      string                 `json:"purpose"`
	CoreBlockIDs []string               `json:"core_block_ids"`
}

type CoreMapViewBlock struct {
	ID                    string                            `json:"id"`
	Name                  string                            `json:"name"`
	Purpose               string                            `json:"purpose"`
	Files                 []CoreMapViewFile                 `json:"files"`
	RepresentativeSymbols []CoreMapViewRepresentativeSymbol `json:"representative_symbols"`
	Children              []CoreMapViewBlock                `json:"children"`
}

type CoreMapViewFile struct {
	FileRef string `json:"file_ref"`
	Path    string `json:"path"`
}

type CoreMapViewRepresentativeSymbol struct {
	Kind               programindex.ObjectKind `json:"kind"`
	Symbol             CoreMapViewSymbol       `json:"symbol"`
	Visibility         programindex.Visibility `json:"visibility"`
	IncomingCalls      int                     `json:"incoming_calls"`
	OutgoingCalls      int                     `json:"outgoing_calls"`
	UnresolvedOutgoing int                     `json:"unresolved_outgoing"`
}

type CoreMapViewSymbol struct {
	NodeID   string              `json:"node_id"`
	Package  string              `json:"package"`
	Name     string              `json:"name"`
	Location CoreMapViewLocation `json:"location"`
}

type CoreMapViewLocation struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

// CoreMapViewCoverage keeps only language-neutral producer accounting. The
// legacy Go direct-call ledger in coremap.Coverage is deliberately rejected
// for this ProgramIndex-backed handoff and does not leak into report JSON.
type CoreMapViewCoverage struct {
	TrackedFiles            int `json:"tracked_files"`
	BaselineRoleFiles       int `json:"baseline_role_files"`
	SymbolsAvailable        int `json:"symbols_available"`
	BaselineBlocks          int `json:"baseline_blocks"`
	BaselineFilesSelected   int `json:"baseline_files_selected"`
	RefinedBlocks           int `json:"refined_blocks"`
	RefinedFilesSelected    int `json:"refined_files_selected"`
	RefinedSymbolsSelected  int `json:"refined_symbols_selected"`
	RefinedGroups           int `json:"refined_groups"`
	RefinedModelGroups      int `json:"refined_model_groups"`
	RefinedLocalGroups      int `json:"refined_local_groups"`
	RefinedUnassignedBlocks int `json:"refined_unassigned_blocks"`
	RefinedGroupCalls       int `json:"refined_group_calls"`
	ProgramObjectsOmitted   int `json:"program_objects_omitted"`
	ProgramRelationsOmitted int `json:"program_relations_omitted"`
}

// NewCoreMapView accepts only the ProgramIndex-backed CoreMap variant and
// binds it to the exact default ProgramTarget and semantic index seal already
// carried by ProgramPortfolio.
func NewCoreMapView(
	value coremap.Result,
	index programindex.Index,
	readmeFiles map[string]string,
) (*CoreMapView, error) {
	if err := value.Validate(); err != nil {
		return nil, fmt.Errorf("core map view: invalid core map: %w", err)
	}
	if err := index.Validate(); err != nil {
		return nil, fmt.Errorf("core map view: program index: %w", err)
	}
	target := index.Target
	if !programSemanticLanguage(target.Language) {
		return nil, fmt.Errorf("core map view: unsupported program language %q", target.Language)
	}
	if value.ProgramTarget == nil || !reflect.DeepEqual(*value.ProgramTarget, target.Snapshot()) {
		return nil, fmt.Errorf("core map view: program target authority mismatch")
	}
	if value.ProgramIndexSHA256 != index.SHA256 {
		return nil, fmt.Errorf("core map view: program index authority mismatch")
	}
	if value.Coverage.DirectCallState != "" || !reflect.ValueOf(value.Coverage.DirectCallCoverage).IsZero() {
		return nil, fmt.Errorf("core map view: ProgramIndex-backed core map carries foreign direct-call authority")
	}
	if err := validateCoreMapMembersAgainstProgramIndex(value, index); err != nil {
		return nil, err
	}
	if err := validateCoreMapFilesAgainstAuthority(value, index, readmeFiles); err != nil {
		return nil, err
	}
	view := &CoreMapView{
		Version: CoreMapViewVersion, ProgramTargetID: target.ID,
		ProgramIndexSHA256:     index.SHA256,
		IntegrationUsageSHA256: value.IntegrationUsageSHA256,
		BaselineCore:           make([]CoreMapViewBlock, 0, len(value.Baseline)),
		RefinedCore:            make([]CoreMapViewBlock, 0, len(value.Refined)),
		RefinedGroups:          make([]CoreMapViewGroup, 0, len(value.RefinedGroups)),
		Coverage: CoreMapViewCoverage{
			TrackedFiles: value.Coverage.TrackedFiles, BaselineRoleFiles: value.Coverage.BaselineRoleFiles,
			SymbolsAvailable: value.Coverage.SymbolsAvailable, BaselineBlocks: value.Coverage.BaselineBlocks,
			BaselineFilesSelected: value.Coverage.BaselineFilesSelected,
			RefinedBlocks:         value.Coverage.RefinedBlocks, RefinedFilesSelected: value.Coverage.RefinedFilesSelected,
			RefinedSymbolsSelected:  value.Coverage.RefinedSymbolsSelected,
			RefinedGroups:           value.Coverage.RefinedGroups,
			RefinedModelGroups:      value.Coverage.RefinedModelGroups,
			RefinedLocalGroups:      value.Coverage.RefinedLocalGroups,
			RefinedUnassignedBlocks: value.Coverage.RefinedUnassignedBlocks,
			RefinedGroupCalls:       value.Coverage.RefinedGroupCalls,
			ProgramObjectsOmitted:   index.Coverage.ObjectsOmitted,
			ProgramRelationsOmitted: index.Coverage.RelationsOmitted,
		},
	}
	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	for _, block := range value.Baseline {
		view.BaselineCore = append(view.BaselineCore, projectCoreMapViewBlock(block, objects))
	}
	for _, block := range value.Refined {
		view.RefinedCore = append(view.RefinedCore, projectCoreMapViewBlock(block, objects))
	}
	for _, group := range value.RefinedGroups {
		view.RefinedGroups = append(view.RefinedGroups, CoreMapViewGroup{
			ID: group.ID, Authority: group.Authority, Name: group.Name, Purpose: group.Purpose,
			CoreBlockIDs: append([]string(nil), group.BlockIDs...),
		})
	}
	if err := view.Validate(); err != nil {
		return nil, fmt.Errorf("core map view: invalid projection: %w", err)
	}
	return view, nil
}

func validateCoreMapFilesAgainstAuthority(
	value coremap.Result,
	index programindex.Index,
	readmeFiles map[string]string,
) error {
	if value.Coverage.BaselineRoleFiles != len(readmeFiles) {
		return fmt.Errorf("core map view: README file-role authority count differs from producer coverage")
	}
	readmeRefsByPath := make(map[string]string, len(readmeFiles))
	for ref, sourcePath := range readmeFiles {
		if !validCubeMapViewText(ref, false) || !validCubeMapViewPath(sourcePath) {
			return fmt.Errorf("core map view: invalid README file authority")
		}
		if previous, duplicate := readmeRefsByPath[sourcePath]; duplicate && previous != ref {
			return fmt.Errorf("core map view: README file authority is not injective")
		}
		readmeRefsByPath[sourcePath] = ref
	}
	indexObjects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		indexObjects[object.ID] = object
	}
	locationPaths := make(map[string]struct{})
	for _, object := range index.Objects {
		if coreMapProgramObject(object, indexObjects) && object.Location != nil {
			locationPaths[object.Location.Path] = struct{}{}
		}
	}
	targetPathsByRef := make(map[string]string, len(index.Target.Sources))
	targetRefsByPath := make(map[string]string, len(index.Target.Sources))
	for _, source := range index.Target.Sources {
		targetPathsByRef[source.FileRef] = source.Path
		targetRefsByPath[source.Path] = source.FileRef
	}
	var validateBaseline func([]coremap.Block) error
	validateBaseline = func(blocks []coremap.Block) error {
		for _, block := range blocks {
			for _, file := range block.Files {
				if readmeFiles[string(file.FileRef)] != file.Path {
					return fmt.Errorf("core map view: baseline file differs from README file-role authority")
				}
			}
			if err := validateBaseline(block.Children); err != nil {
				return err
			}
		}
		return nil
	}
	if err := validateBaseline(value.Baseline); err != nil {
		return err
	}
	for _, block := range value.Refined {
		for _, file := range block.Files {
			ref := string(file.FileRef)
			if readmeFiles[ref] == file.Path {
				continue
			}
			if knownPath, exists := readmeFiles[ref]; exists && knownPath != file.Path {
				return fmt.Errorf("core map view: refined file ref conflicts with README file-role authority")
			}
			if knownRef, exists := readmeRefsByPath[file.Path]; exists && knownRef != ref {
				return fmt.Errorf("core map view: refined file path conflicts with README file-role authority")
			}
			if _, exists := locationPaths[file.Path]; !exists {
				return fmt.Errorf("core map view: refined file has no ProgramIndex core-object location authority")
			}
			if knownPath, exists := targetPathsByRef[ref]; exists && knownPath != file.Path {
				return fmt.Errorf("core map view: refined file ref conflicts with ProgramTarget source authority")
			}
			if knownRef, exists := targetRefsByPath[file.Path]; exists && knownRef != ref {
				return fmt.Errorf("core map view: refined file path conflicts with ProgramTarget source authority")
			}
		}
	}
	return nil
}

func validateCoreMapMembersAgainstProgramIndex(value coremap.Result, index programindex.Index) error {
	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	incoming := make(map[string]int)
	outgoing := make(map[string]int)
	unresolved := make(map[string]int)
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationExecutes {
			continue
		}
		outgoing[relation.FromID]++
		if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) == 0 {
			unresolved[relation.FromID]++
			continue
		}
		for _, targetID := range relation.ToIDs {
			incoming[targetID]++
		}
	}
	eligible := 0
	for _, object := range index.Objects {
		if coreMapProgramObject(object, objects) {
			eligible++
		}
	}
	if value.Coverage.SymbolsAvailable != eligible {
		return fmt.Errorf("core map view: producer symbol coverage does not match ProgramIndex")
	}
	for _, block := range value.Refined {
		for _, member := range block.Symbols {
			object, ok := objects[member.NodeID]
			if !ok || !coreMapProgramObject(object, objects) || object.Location == nil {
				return fmt.Errorf("core map view: representative %q is not an exact ProgramIndex core object", member.NodeID)
			}
			packageName, err := coreMapProgramObjectPackage(object, objects)
			if err != nil {
				return err
			}
			locationMatches := member.Declaration.Path == object.Location.Path &&
				member.Declaration.Line == object.Location.Line && member.Declaration.Column == object.Location.Column
			symbolLocationMatches := member.Symbol.Location.Path == object.Location.Path &&
				member.Symbol.Location.Line == object.Location.Line && member.Symbol.Location.Column == object.Location.Column
			if member.Kind != object.Kind || member.Symbol.ID != object.ID || member.Symbol.Name != object.Name ||
				member.Symbol.Package != packageName || member.Package != packageName ||
				member.Exported != (object.Visibility == programindex.VisibilityPublic) ||
				!locationMatches || !symbolLocationMatches || member.IncomingCalls != incoming[object.ID] ||
				member.OutgoingCalls != outgoing[object.ID] || member.UnresolvedOutgoing != unresolved[object.ID] {
				return fmt.Errorf("core map view: representative %q differs from exact ProgramIndex evidence", member.NodeID)
			}
		}
	}
	return nil
}

func coreMapProgramObject(
	object programindex.Object,
	objects map[string]programindex.Object,
) bool {
	switch object.Kind {
	case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda, programindex.ObjectType:
		return true
	case programindex.ObjectVariable:
		if object.Visibility != programindex.VisibilityPublic {
			return false
		}
		parentID := object.ContainerID
		if parentID == "" {
			parentID = object.OwnerID
		}
		parent, ok := objects[parentID]
		return ok && (parent.Kind == programindex.ObjectModule || parent.Kind == programindex.ObjectPackage)
	default:
		return false
	}
}

func coreMapProgramObjectPackage(
	object programindex.Object,
	objects map[string]programindex.Object,
) (string, error) {
	seen := make(map[string]struct{})
	current := object
	for {
		if current.Kind == programindex.ObjectModule || current.Kind == programindex.ObjectPackage {
			return current.Name, nil
		}
		if _, duplicate := seen[current.ID]; duplicate {
			return "", fmt.Errorf("core map view: ProgramIndex containment contains a cycle")
		}
		seen[current.ID] = struct{}{}
		nextID := current.ContainerID
		if nextID == "" {
			nextID = current.OwnerID
		}
		next, ok := objects[nextID]
		if nextID == "" || !ok {
			return "", fmt.Errorf("core map view: object %q has no exact module context", object.ID)
		}
		current = next
	}
}

func projectCoreMapViewBlock(
	block coremap.Block,
	objects map[string]programindex.Object,
) CoreMapViewBlock {
	result := CoreMapViewBlock{
		ID: block.ID, Name: block.Name, Purpose: block.Purpose,
		Files:                 make([]CoreMapViewFile, 0, len(block.Files)),
		RepresentativeSymbols: make([]CoreMapViewRepresentativeSymbol, 0, len(block.Symbols)),
		Children:              make([]CoreMapViewBlock, 0, len(block.Children)),
	}
	for _, file := range block.Files {
		result.Files = append(result.Files, CoreMapViewFile{FileRef: string(file.FileRef), Path: file.Path})
	}
	for _, symbol := range block.Symbols {
		object := objects[symbol.NodeID]
		result.RepresentativeSymbols = append(result.RepresentativeSymbols, CoreMapViewRepresentativeSymbol{
			Kind: symbol.Kind,
			Symbol: CoreMapViewSymbol{
				NodeID: symbol.NodeID, Package: symbol.Package, Name: symbol.Symbol.Name,
				Location: CoreMapViewLocation{
					Path: symbol.Declaration.Path, Line: symbol.Declaration.Line, Column: symbol.Declaration.Column,
				},
			},
			Visibility: object.Visibility, IncomingCalls: symbol.IncomingCalls,
			OutgoingCalls: symbol.OutgoingCalls, UnresolvedOutgoing: symbol.UnresolvedOutgoing,
		})
	}
	for _, child := range block.Children {
		result.Children = append(result.Children, projectCoreMapViewBlock(child, objects))
	}
	return result
}

// Validate checks the standalone browser handoff without trusting the
// artifact that originally produced it. Artifact equality is checked again by
// RunManifest before a run becomes authoritative.
func (view CoreMapView) Validate() error {
	if view.Version != CoreMapViewVersion || !validCubeMapViewText(view.ProgramTargetID, false) ||
		!validCubeMapViewSHA256(view.ProgramIndexSHA256) ||
		!validCubeMapViewSHA256(view.IntegrationUsageSHA256) {
		return fmt.Errorf("core map view: invalid identity")
	}
	if view.BaselineCore == nil || view.RefinedCore == nil || view.RefinedGroups == nil || len(view.RefinedCore) == 0 {
		return fmt.Errorf("core map view: required block collection is missing")
	}
	counts := coreMapViewCounts{}
	filesByRef := make(map[string]string)
	refsByPath := make(map[string]string)
	symbolsByID := make(map[string]CoreMapViewRepresentativeSymbol)
	baselineIDs := make(map[string]struct{})
	if err := validateCoreMapViewBlocks(
		view.BaselineCore, true, 0, baselineIDs,
		filesByRef, refsByPath, symbolsByID, &counts,
	); err != nil {
		return err
	}
	refinedIDs := make(map[string]struct{})
	if err := validateCoreMapViewBlocks(
		view.RefinedCore, false, 0, refinedIDs,
		filesByRef, refsByPath, symbolsByID, &counts,
	); err != nil {
		return err
	}
	if err := validateCoreMapViewGroups(view.RefinedGroups, refinedIDs); err != nil {
		return err
	}
	if err := validateCoreMapViewCoverage(view, filesByRef, symbolsByID); err != nil {
		return err
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		return fmt.Errorf("core map view: encode bound check: %w", err)
	}
	if len(encoded) > MaxCoreMapViewBytes {
		return fmt.Errorf("core map view: JSON size %d exceeds projection limit %d", len(encoded), MaxCoreMapViewBytes)
	}
	return nil
}

func validateCoreMapViewGroups(groups []CoreMapViewGroup, refinedIDs map[string]struct{}) error {
	if groups == nil {
		return fmt.Errorf("core map view: refined groups must retain an exact array")
	}
	if len(groups) == 0 {
		return nil
	}
	seenGroups := make(map[string]struct{}, len(groups))
	seenNames := make(map[string]struct{}, len(groups))
	modelBlocks := make(map[string]struct{}, len(refinedIDs))
	localBlocks := make(map[string]struct{}, len(refinedIDs))
	modelGroups := 0
	localGroups := 0
	for position, group := range groups {
		if !validCubeMapViewText(group.ID, false) || !validCubeMapViewText(group.Name, false) ||
			!validCubeMapViewText(group.Purpose, false) || group.CoreBlockIDs == nil || len(group.CoreBlockIDs) == 0 {
			return fmt.Errorf("core map view: invalid refined group")
		}
		switch group.Authority {
		case coremap.GroupAuthorityModel:
			modelGroups++
		case coremap.GroupAuthorityLocalUnassigned:
			localGroups++
			if localGroups != 1 || position != len(groups)-1 ||
				group.Name != coremap.LocalUnassignedGroupName || group.Purpose != coremap.LocalUnassignedGroupPurpose {
				return fmt.Errorf("core map view: invalid local unassigned group")
			}
		default:
			return fmt.Errorf("core map view: invalid refined group authority")
		}
		if _, duplicate := seenGroups[group.ID]; duplicate {
			return fmt.Errorf("core map view: duplicate refined group identity")
		}
		seenGroups[group.ID] = struct{}{}
		if group.Authority == coremap.GroupAuthorityModel {
			if _, duplicate := seenNames[group.Name]; duplicate {
				return fmt.Errorf("core map view: duplicate refined group name")
			}
			seenNames[group.Name] = struct{}{}
		}
		inside := make(map[string]struct{}, len(group.CoreBlockIDs))
		for _, blockID := range group.CoreBlockIDs {
			if _, ok := refinedIDs[blockID]; !ok {
				return fmt.Errorf("core map view: refined group cites unknown block")
			}
			if _, duplicate := inside[blockID]; duplicate {
				return fmt.Errorf("core map view: refined group repeats a block")
			}
			inside[blockID] = struct{}{}
			if group.Authority == coremap.GroupAuthorityModel {
				modelBlocks[blockID] = struct{}{}
			} else {
				localBlocks[blockID] = struct{}{}
			}
		}
	}
	if modelGroups == 0 {
		return fmt.Errorf("core map view: local unassigned group has no model grouping")
	}
	for blockID := range localBlocks {
		if _, modeled := modelBlocks[blockID]; modeled {
			return fmt.Errorf("core map view: local unassigned group overlaps a model group")
		}
	}
	if len(modelBlocks)+len(localBlocks) != len(refinedIDs) {
		return fmt.Errorf("core map view: groups do not account for every refined block")
	}
	if len(localBlocks) == 0 && localGroups != 0 {
		return fmt.Errorf("core map view: empty local unassigned accounting")
	}
	if len(localBlocks) != 0 && localGroups != 1 {
		return fmt.Errorf("core map view: missing local unassigned accounting")
	}
	return nil
}

type coreMapViewCounts struct {
	blocks, fileRows, symbolRows int
}

func validateCoreMapViewBlocks(
	blocks []CoreMapViewBlock,
	baseline bool,
	depth int,
	blockIDs map[string]struct{},
	filesByRef map[string]string,
	refsByPath map[string]string,
	symbolsByID map[string]CoreMapViewRepresentativeSymbol,
	counts *coreMapViewCounts,
) error {
	if baseline && depth > coremap.MaxBlockDepth && len(blocks) != 0 {
		return fmt.Errorf("core map view: baseline hierarchy exceeds its producer depth")
	}
	for _, block := range blocks {
		counts.blocks++
		if !validCubeMapViewText(block.ID, false) || !validCubeMapViewText(block.Name, false) ||
			!validCubeMapViewText(block.Purpose, false) || block.Files == nil ||
			block.RepresentativeSymbols == nil || block.Children == nil {
			return fmt.Errorf("core map view: invalid block")
		}
		if _, duplicate := blockIDs[block.ID]; duplicate {
			return fmt.Errorf("core map view: duplicate block %q", block.ID)
		}
		blockIDs[block.ID] = struct{}{}
		if baseline && len(block.RepresentativeSymbols) != 0 || !baseline && len(block.Children) != 0 {
			return fmt.Errorf("core map view: invalid stage shape")
		}
		if len(block.Files)+len(block.RepresentativeSymbols)+len(block.Children) == 0 {
			return fmt.Errorf("core map view: block %q has no exact evidence", block.ID)
		}
		seenFiles := make(map[string]struct{}, len(block.Files))
		for _, file := range block.Files {
			counts.fileRows++
			if !validCubeMapViewText(file.FileRef, false) || !validCubeMapViewPath(file.Path) {
				return fmt.Errorf("core map view: invalid file evidence")
			}
			key := file.FileRef + "\x00" + file.Path
			if _, duplicate := seenFiles[key]; duplicate {
				return fmt.Errorf("core map view: duplicate file evidence")
			}
			seenFiles[key] = struct{}{}
			if path, exists := filesByRef[file.FileRef]; exists && path != file.Path {
				return fmt.Errorf("core map view: file ref maps to conflicting paths")
			}
			if ref, exists := refsByPath[file.Path]; exists && ref != file.FileRef {
				return fmt.Errorf("core map view: file path maps to conflicting refs")
			}
			filesByRef[file.FileRef] = file.Path
			refsByPath[file.Path] = file.FileRef
		}
		seenSymbols := make(map[string]struct{}, len(block.RepresentativeSymbols))
		for _, symbol := range block.RepresentativeSymbols {
			counts.symbolRows++
			if err := validateCoreMapViewRepresentative(symbol); err != nil {
				return err
			}
			if _, duplicate := seenSymbols[symbol.Symbol.NodeID]; duplicate {
				return fmt.Errorf("core map view: duplicate representative symbol")
			}
			seenSymbols[symbol.Symbol.NodeID] = struct{}{}
			if previous, exists := symbolsByID[symbol.Symbol.NodeID]; exists && !reflect.DeepEqual(previous, symbol) {
				return fmt.Errorf("core map view: symbol ID has conflicting exact facts")
			}
			symbolsByID[symbol.Symbol.NodeID] = symbol
		}
		if err := validateCoreMapViewBlocks(
			block.Children, baseline, depth+1, blockIDs,
			filesByRef, refsByPath, symbolsByID, counts,
		); err != nil {
			return err
		}
	}
	return nil
}

func validateCoreMapViewRepresentative(value CoreMapViewRepresentativeSymbol) error {
	switch value.Kind {
	case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda,
		programindex.ObjectType, programindex.ObjectVariable:
	default:
		return fmt.Errorf("core map view: invalid representative symbol kind")
	}
	if !value.Visibility.Valid() {
		return fmt.Errorf("core map view: invalid representative symbol visibility")
	}
	if !validCubeMapViewText(value.Symbol.NodeID, false) ||
		!validCubeMapViewText(value.Symbol.Package, false) ||
		!validCubeMapViewText(value.Symbol.Name, false) ||
		!validCubeMapViewLocation(CubeMapViewLocation{
			Path: value.Symbol.Location.Path, Line: value.Symbol.Location.Line, Column: value.Symbol.Location.Column,
		}, true) || value.IncomingCalls < 0 || value.OutgoingCalls < 0 || value.UnresolvedOutgoing < 0 {
		return fmt.Errorf("core map view: invalid representative symbol evidence")
	}
	return nil
}

func validateCoreMapViewCoverage(
	view CoreMapView,
	filesByRef map[string]string,
	symbolsByID map[string]CoreMapViewRepresentativeSymbol,
) error {
	coverage := view.Coverage
	baselineBlocks, baselineFiles := coreMapViewStageCounts(view.BaselineCore)
	refinedBlocks, refinedFiles := coreMapViewStageCounts(view.RefinedCore)
	modelGroups, localGroups, unassignedBlocks := coreMapViewGroupingCounts(view.RefinedGroups)
	if coverage.TrackedFiles < len(filesByRef) || coverage.BaselineRoleFiles < baselineFiles ||
		coverage.SymbolsAvailable < len(symbolsByID) || coverage.BaselineBlocks != baselineBlocks ||
		coverage.BaselineFilesSelected != baselineFiles || coverage.RefinedBlocks != refinedBlocks ||
		coverage.RefinedFilesSelected != refinedFiles ||
		coverage.RefinedSymbolsSelected != len(symbolsByID) ||
		coverage.RefinedGroups != len(view.RefinedGroups) ||
		coverage.RefinedModelGroups != modelGroups || coverage.RefinedLocalGroups != localGroups ||
		coverage.RefinedUnassignedBlocks != unassignedBlocks ||
		coverage.RefinedGroupCalls != coreMapViewBoolCount(len(view.RefinedCore) >= 2) ||
		coverage.ProgramObjectsOmitted < 0 || coverage.ProgramRelationsOmitted < 0 {
		return fmt.Errorf("core map view: coverage does not match exact projection")
	}
	return nil
}

func coreMapViewGroupingCounts(groups []CoreMapViewGroup) (modelGroups, localGroups, unassignedBlocks int) {
	for _, group := range groups {
		switch group.Authority {
		case coremap.GroupAuthorityModel:
			modelGroups++
		case coremap.GroupAuthorityLocalUnassigned:
			localGroups++
			unassignedBlocks += len(group.CoreBlockIDs)
		}
	}
	return modelGroups, localGroups, unassignedBlocks
}

func coreMapViewBoolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func coreMapViewStageCounts(blocks []CoreMapViewBlock) (int, int) {
	blockCount := 0
	files := make(map[string]struct{})
	var visit func([]CoreMapViewBlock)
	visit = func(values []CoreMapViewBlock) {
		for _, block := range values {
			blockCount++
			for _, file := range block.Files {
				files[file.FileRef+"\x00"+file.Path] = struct{}{}
			}
			visit(block.Children)
		}
	}
	visit(blocks)
	return blockCount, len(files)
}
