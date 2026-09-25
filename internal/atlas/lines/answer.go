package lines

import (
	_ "embed"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const StageAnswer = "atlas_answer"

//go:embed prompts/answer.md
var answerPrompt string

func Answer() table.Definition {
	return table.Definition{Stage: StageAnswer, Contract: "repomap.atlas.answer.v9", System: answerPrompt, Reasoning: true,
		Columns: []table.Column{
			{Name: "answer", Kind: table.Prose, EmptyValue: "none"},
			{Name: "basis", Kind: table.Prose, EmptyValue: "none"},
			// An unanswered row has no sources; a substantive answer without
			// them is still refused by its owner (Optional is decoder-only).
			{Name: "sources", Kind: table.Sequence, OptionsFrom: "candidate_options", Optional: true},
			{Name: "remaining", Kind: table.Prose, EmptyValue: "none"},
			{Name: "state", Kind: table.Choice, Options: []string{"answered", "partial", "unanswered", "not_applicable"}},
		}}
}
