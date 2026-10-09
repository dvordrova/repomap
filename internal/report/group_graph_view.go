package report

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// GroupGraphView is the complete set of thin group overlays stored in the
// report. The bound ProgramIndexes expand native subjects and structural edges
// in memory; report.json never repeats them.
type GroupGraphView struct {
	SelectedTargetID string               `json:"selected_target_id"`
	Indexes          []groupindex.Overlay `json:"indexes"`
	hydrated         []groupindex.Index
}

// NewGroupGraphView owns and canonicalizes a complete target graph set, then
// retains only its semantic overlays for persistence. The selected target
// controls page focus only; it does not alter graph authority.
func NewGroupGraphView(
	indexes []groupindex.Index,
	selectedTargetID string,
) (*GroupGraphView, error) {
	if len(indexes) == 0 {
		return nil, fmt.Errorf("group graph view: GroupsIndex set is empty")
	}
	owned := make([]groupindex.Index, len(indexes))
	for position := range indexes {
		owned[position] = indexes[position].Snapshot()
	}
	sort.Slice(owned, func(i, j int) bool {
		return programindex.TargetIDLess(owned[i].Target.ID, owned[j].Target.ID)
	})
	overlays := make([]groupindex.Overlay, len(owned))
	for position := range owned {
		overlays[position] = groupindex.OverlayFromIndex(owned[position])
	}
	view := &GroupGraphView{SelectedTargetID: selectedTargetID, Indexes: overlays, hydrated: owned}
	if err := view.Validate(); err != nil {
		return nil, err
	}
	return view, nil
}

// Snapshot returns a consumer-owned projection.
func (view *GroupGraphView) Snapshot() *GroupGraphView {
	if view == nil {
		return nil
	}
	result := &GroupGraphView{
		SelectedTargetID: view.SelectedTargetID,
		Indexes:          make([]groupindex.Overlay, len(view.Indexes)),
		hydrated:         make([]groupindex.Index, len(view.hydrated)),
	}
	for position := range view.hydrated {
		result.hydrated[position] = view.hydrated[position].Snapshot()
		result.Indexes[position] = groupindex.OverlayFromIndex(result.hydrated[position])
	}
	return result
}

// Validate proves the complete target set and exact selected target.
func (view *GroupGraphView) Validate() error {
	if view == nil || view.SelectedTargetID == "" || len(view.Indexes) == 0 {
		return fmt.Errorf("group graph view: incomplete graph authority")
	}
	selected := false
	for position, index := range view.Indexes {
		if err := index.Validate(); err != nil {
			return fmt.Errorf("group graph view: %w", err)
		}
		if position > 0 && !programindex.TargetIDLess(view.Indexes[position-1].TargetID, index.TargetID) {
			return fmt.Errorf("group graph view: target indexes are not canonical")
		}
		if index.TargetID == view.SelectedTargetID {
			selected = true
		}
	}
	if !selected {
		return fmt.Errorf("group graph view: selected target is absent")
	}
	if len(view.hydrated) != len(view.Indexes) {
		return fmt.Errorf("group graph view: ProgramIndex bindings are unavailable")
	}
	if err := groupindex.ValidateSet(view.hydrated); err != nil {
		return fmt.Errorf("group graph view: %w", err)
	}
	return nil
}

// Hydrate binds each thin semantic overlay to the one ProgramIndex it names.
// The expanded native graph exists only in memory for page construction.
func (view *GroupGraphView) Hydrate(programs []programindex.Index) error {
	if view == nil {
		return fmt.Errorf("group graph view: missing view")
	}
	if len(programs) != len(view.Indexes) {
		return fmt.Errorf("group graph view: ProgramIndex and overlay sets differ")
	}
	byTarget := make(map[string]programindex.Index, len(programs))
	for _, program := range programs {
		if _, duplicate := byTarget[program.Target.ID]; duplicate {
			return fmt.Errorf("group graph view: duplicate ProgramIndex %q", program.Target.ID)
		}
		byTarget[program.Target.ID] = program
	}
	view.hydrated = make([]groupindex.Index, 0, len(view.Indexes))
	for _, overlay := range view.Indexes {
		program, ok := byTarget[overlay.TargetID]
		if !ok {
			return fmt.Errorf("group graph view: ProgramIndex %q is missing", overlay.TargetID)
		}
		index, err := overlay.Hydrate(program)
		if err != nil {
			return fmt.Errorf("group graph view: hydrate %q: %w", overlay.TargetID, err)
		}
		view.hydrated = append(view.hydrated, index)
	}
	return view.Validate()
}

// hydratePortfolio reads each complete bound native body only while hydrating
// its original overlay. The complete native portfolio is not accumulated.
func (view *GroupGraphView) hydratePortfolio(portfolio *ProgramPortfolio) error {
	if view == nil || portfolio == nil || portfolio.Len() != len(view.Indexes) {
		return fmt.Errorf("group graph view: ProgramIndex and overlay sets differ")
	}
	if err := portfolio.validateBindings(); err != nil {
		return err
	}
	hydrated := make([]groupindex.Index, 0, len(view.Indexes))
	position := 0
	for program, err := range portfolio.indexes() {
		if err != nil {
			return err
		}
		index, err := view.Indexes[position].Hydrate(program)
		if err != nil {
			return fmt.Errorf("group graph view: hydrate %q: %w", program.Target.ID, err)
		}
		hydrated = append(hydrated, index)
		position++
	}
	bound := *view
	bound.hydrated = hydrated
	if err := bound.Validate(); err != nil {
		return err
	}
	view.hydrated = hydrated
	return nil
}

// SourcePaths returns every repository-relative location carried by the final
// graph. Multi-target finalization uses it to extend each page's source
// authority before embedding the shared graph.
func (view *GroupGraphView) SourcePaths() ([]string, error) {
	if err := view.Validate(); err != nil {
		return nil, err
	}
	paths := make([]string, 0)
	for _, index := range view.hydrated {
		for _, operation := range index.Operations {
			paths = append(paths, operation.Location.Path)
		}
		for _, call := range index.Outbound {
			paths = append(paths, call.Location.Path)
		}
		for _, connection := range index.Connections {
			if connection.FromLocation != nil {
				paths = append(paths, connection.FromLocation.Path)
			}
			if connection.ToLocation != nil {
				paths = append(paths, connection.ToLocation.Path)
			}
		}
		for _, source := range index.Target.Sources {
			paths = append(paths, source.Path)
		}
		for _, subject := range index.Subjects {
			switch {
			case subject.Object != nil && subject.Object.Location != nil:
				paths = append(paths, subject.Object.Location.Path)
			case subject.Pattern != nil && subject.Pattern.Location != nil:
				paths = append(paths, subject.Pattern.Location.Path)
			}
		}
	}
	sort.Strings(paths)
	write := 0
	for _, sourcePath := range paths {
		if sourcePath == "" || write > 0 && paths[write-1] == sourcePath {
			continue
		}
		paths[write] = sourcePath
		write++
	}
	return paths[:write], nil
}

// BindGroupGraphView installs a complete transaction-local graph projection
// into report data. The selected target must remain the page's ProgramTarget.
func BindGroupGraphView(data *ReportData, indexes []groupindex.Index) error {
	if data == nil || data.ProgramPortfolio == nil {
		return fmt.Errorf("group graph view: report ProgramPortfolio is unavailable")
	}
	entry, err := data.ProgramPortfolio.defaultBinding()
	if err != nil {
		return err
	}
	view, err := NewGroupGraphView(indexes, entry.Target.ID)
	if err != nil {
		return err
	}
	if err := validateSelectedGroupGraphBinding(view, entry.Target, entry.SHA256); err != nil {
		return err
	}
	if data.localGroupsIndex != nil {
		var selected *groupindex.Index
		for index := range view.hydrated {
			if view.hydrated[index].Target.ID == entry.Target.ID {
				selected = &view.hydrated[index]
				break
			}
		}
		if selected == nil {
			return fmt.Errorf("group graph view: selected target is absent")
		}
		if err := validateLocalGroupIndexExtension(*data.localGroupsIndex, *selected); err != nil {
			return err
		}
	}
	data.GroupGraph = view
	return nil
}

func validateLocalGroupIndexExtension(local, selected groupindex.Index) error {
	if selected.Version != local.Version ||
		selected.ProgramIndexSHA256 != local.ProgramIndexSHA256 ||
		!reflect.DeepEqual(selected.Target, local.Target) ||
		!reflect.DeepEqual(selected.Subjects, local.Subjects) ||
		!reflect.DeepEqual(selected.Groups, local.Groups) ||
		!reflect.DeepEqual(selected.Containers, local.Containers) ||
		!reflect.DeepEqual(selected.StructuralEdges, local.StructuralEdges) {
		return fmt.Errorf("group graph view: matched set does not preserve local graph authority")
	}
	selectedConnections := make(map[string]groupindex.Connection, len(selected.Connections))
	for _, connection := range selected.Connections {
		selectedConnections[connection.ID] = connection
	}
	localConnections := make(map[string]struct{}, len(local.Connections))
	for _, connection := range local.Connections {
		localConnections[connection.ID] = struct{}{}
		if restored, ok := selectedConnections[connection.ID]; !ok || !reflect.DeepEqual(restored, connection) {
			return fmt.Errorf("group graph view: matched set changes a local connection")
		}
	}
	for _, connection := range selected.Connections {
		if _, ok := localConnections[connection.ID]; ok {
			continue
		}
		if connection.From.TargetID == connection.To.TargetID {
			return fmt.Errorf("group graph view: matched set invents a local connection")
		}
	}
	return nil
}

func validateSelectedGroupGraphBinding(
	view *GroupGraphView,
	target programindex.Target,
	programIndexSHA256 string,
) error {
	if err := view.Validate(); err != nil {
		return err
	}
	for _, index := range view.hydrated {
		if index.Target.ID != view.SelectedTargetID {
			continue
		}
		if index.ProgramIndexSHA256 != programIndexSHA256 || !reflect.DeepEqual(index.Target, target) {
			return fmt.Errorf("group graph view: selected graph does not bind the default ProgramIndex")
		}
		return nil
	}
	return fmt.Errorf("group graph view: selected target is absent")
}
