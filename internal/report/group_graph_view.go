package report

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// GroupGraphView is the direct report projection of the final graph. It keeps
// the sealed GroupsIndexes themselves, rather than asking the browser to
// rebuild groups or infer edges from older semantic views.
type GroupGraphView struct {
	SelectedTargetID string             `json:"selected_target_id"`
	Indexes          []groupindex.Index `json:"indexes"`
}

// NewGroupGraphView owns and canonicalizes a complete target graph set. The
// selected target controls page focus only; it does not alter graph authority.
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
		return owned[i].Target.ID < owned[j].Target.ID
	})
	view := &GroupGraphView{SelectedTargetID: selectedTargetID, Indexes: owned}
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
		Indexes:          make([]groupindex.Index, len(view.Indexes)),
	}
	for position := range view.Indexes {
		result.Indexes[position] = view.Indexes[position].Snapshot()
	}
	return result
}

// Validate proves the complete target set and exact selected target.
func (view *GroupGraphView) Validate() error {
	if view == nil || view.SelectedTargetID == "" || len(view.Indexes) == 0 {
		return fmt.Errorf("group graph view: incomplete graph authority")
	}
	if err := groupindex.ValidateSet(view.Indexes); err != nil {
		return fmt.Errorf("group graph view: %w", err)
	}
	selected := false
	for position, index := range view.Indexes {
		if position > 0 && view.Indexes[position-1].Target.ID >= index.Target.ID {
			return fmt.Errorf("group graph view: target indexes are not canonical")
		}
		if index.Target.ID == view.SelectedTargetID {
			selected = true
		}
	}
	if !selected {
		return fmt.Errorf("group graph view: selected target is absent")
	}
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
	for _, index := range view.Indexes {
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
	entry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return err
	}
	view, err := NewGroupGraphView(indexes, entry.Target.ID)
	if err != nil {
		return err
	}
	if err := validateSelectedGroupGraphBinding(view, entry.Target, entry.View.IndexSHA256); err != nil {
		return err
	}
	if data.localGroupsIndex != nil {
		var selected *groupindex.Index
		for index := range view.Indexes {
			if view.Indexes[index].Target.ID == entry.Target.ID {
				selected = &view.Indexes[index]
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
	for _, index := range view.Indexes {
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
