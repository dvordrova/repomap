package reading

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/atlas/lines"
)

// zoneState is one area of a target and the parts it holds.
type zoneState struct {
	id     string
	parent string
	title  string
	line   string
	boxes  []string
}

// readAreas draws the areas of every target from its grouping tree: each box
// the grouping divided, the target's own aside, is an area holding the parts
// that sit in it directly. No model is asked; the areas take their compact
// IDs in target order, each target's in depth-first order. A part kept off
// the canvas sits in no area.
func (r *reader) readAreas(context.Context) error {
	r.zones = map[string][]*zoneState{}
	r.started[lines.StageAreas] = time.Now()
	for _, target := range r.opts.Targets {
		drafts := r.treeZones[target.ID]
		ids := make([]string, len(drafts))
		for i, draft := range drafts {
			ids[i] = r.compactID("z", &r.nextZone)
			zone := &zoneState{id: ids[i], title: draft.title, line: draft.line}
			if draft.parent >= 0 {
				zone.parent = ids[draft.parent]
			}
			for _, id := range draft.parts {
				if box := r.boxes[id]; box != nil && !box.offCanvas() {
					zone.boxes = append(zone.boxes, id)
				}
			}
			slices.SortFunc(zone.boxes, compactIDLess3)
			r.zones[target.ID] = append(r.zones[target.ID], zone)
			fmt.Fprintf(&r.tables, "%s · area %s: %s (%s)\n", target.Name, zone.id, zone.title, strings.Join(zone.boxes, " "))
		}
	}
	r.tables.WriteString("\n")
	r.reportStage(lines.StageAreas)
	return nil
}
