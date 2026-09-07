package lines

import (
	_ "embed"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const StageRoute = "atlas_route"
const RouteSteps = 6

//go:embed prompts/route.md
var routePrompt string

func Route() table.Definition {
	return table.Definition{Stage: StageRoute, Contract: "repomap.atlas.route.v7", Window: 1, System: routePrompt,
		Columns: []table.Column{
			{Name: "order", Kind: table.Sequence, OptionsFrom: "candidate_options"},
			{Name: "open_question", Kind: table.Text, MaxRunes: 240},
		}}
}

// AnchorEvidence preserves the selected original evidence for later reading
// decisions. A model's reason is stored separately and never becomes a fact.
func AnchorEvidence(chunk QuestionChunk, ref string) map[string]any {
	result := make(map[string]any)
	context := map[string]any{"path": chunk.Place.Path}
	for _, field := range chunk.Row.Fields {
		switch field.Name {
		case "anchor_options", "chunk", "chunks", "place_kind", "path":
			continue
		case "evidence":
			var facts []map[string]any
			for _, item := range field.Value.([]map[string]any) {
				if ref != "file" && item["ref"] != ref {
					continue
				}
				copy := make(map[string]any)
				for key, value := range item {
					if key != "ref" {
						copy[key] = value
					}
				}
				facts = append(facts, copy)
			}
			result[field.Name] = facts
		default:
			context[field.Name] = field.Value
		}
	}
	result["context"] = context
	return result
}
