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
		Stage: StageZoneParts, Contract: "repomap.atlas.zone_parts.v2", System: zonePartsPrompt, Independent: true, Classifier: true,
		// Measured on cmd/repomap's saved windows: a direct question with no
		// text-model prompt in the state left 800 of 4,238 units uncertain
		// instead of 1,046, and a blind judge preferred its differing
		// assignments 34 to 21.
		ClassifierOmitTask: true,
		Columns: []table.Column{
			{Name: "part", Kind: table.Choice, OptionsFrom: "part_options", Note: "a ref from context.parts, or none",
				Ask: `Which part of this program's architecture map does this function or type belong to? Choose the part in ` + "`context.parts`" + ` whose purpose it serves; choose "none" only if no listed part fits.`},
		},
	}
}

// ZoneAreas assigns each drawn part to one proposed area.
func ZoneAreas() table.Definition {
	return table.Definition{
		Stage: StageZoneAreas, Contract: "repomap.atlas.zone_areas.v2", System: zoneAreasPrompt, Independent: true, Classifier: true,
		ClassifierOmitTask: true,
		Columns: []table.Column{
			{Name: "area", Kind: table.Choice, OptionsFrom: "area_options", Note: "a ref from context.areas, or none",
				Ask: `Which area of this program's architecture map does this part belong to? Choose the area in ` + "`context.areas`" + ` whose purpose covers it; choose "none" only if no listed area fits.`},
		},
	}
}
