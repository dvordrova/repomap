package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

// The stages of a target's map of parts besides atlas_zones, the parts
// request itself: the placement follow-up, the part descriptions and the
// areas with their lines. Each is its own stage, so the journal and the
// timings say which one a request belonged to.
const (
	StagePlacement = "atlas_placement"
	StageDescribe  = "atlas_describe"
	StageAreas     = "atlas_areas"
)

//go:embed prompts/placement.md
var placementPrompt string

// Placement places the units (whole files and boxes of split files) a parts
// answer left out or listed in two parts. A row offers only the parts that
// unit may take: every drawn part for a unit left out, the parts it was
// listed in for a conflict.
func Placement() table.Definition {
	return table.Definition{
		Stage: StagePlacement, Contract: "repomap.atlas.placement.v2", System: placementPrompt,
		Columns: []table.Column{
			{Name: "part", Kind: table.Choice, OptionsFrom: "part_options", Note: "the p* ref of the one listed part this unit belongs to"},
		},
	}
}
