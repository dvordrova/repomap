package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

const (
	StageZoneParts = "atlas_zone_parts"
	StageZoneAreas = "atlas_zone_areas"

	// ZoneNone is the choice that says no listed part or area fits.
	ZoneNone = "none"
)

//go:embed prompts/zone_parts.md
var zonePartsPrompt string

//go:embed prompts/zone_areas.md
var zoneAreasPrompt string

// ZoneParts assigns each function or type to one proposed part.
func ZoneParts() table.Definition {
	return table.Definition{
		Stage: StageZoneParts, Contract: "repomap.atlas.zone_parts.v1", System: zonePartsPrompt, Independent: true, Classifier: true,
		Columns: []table.Column{
			{Name: "part", Kind: table.Choice, OptionsFrom: "part_options", Note: "a ref from context.parts, or none"},
		},
	}
}

// ZoneAreas assigns each drawn part to one proposed area.
func ZoneAreas() table.Definition {
	return table.Definition{
		Stage: StageZoneAreas, Contract: "repomap.atlas.zone_areas.v1", System: zoneAreasPrompt, Independent: true, Classifier: true,
		Columns: []table.Column{
			{Name: "area", Kind: table.Choice, OptionsFrom: "area_options", Note: "a ref from context.areas, or none"},
		},
	}
}
