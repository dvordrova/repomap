package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

// The stages of the grouping of a target's declarations into the boxes of
// its map (owner, 2026-10-08): Jev asks whether a box needs smaller boxes,
// DeepSeek proposes the smaller boxes (StageZones), and Jev puts each
// declaration in one of them. The grouping repeats inside every smaller box.
const (
	StageGroupEnough = "atlas_group_enough"
	StageGroupAssign = "atlas_group_assign"
	// StageGroupGather gathers a divided box's many smaller boxes into the
	// larger parts a newcomer opens first (owner, 2026-10-14).
	StageGroupGather = "atlas_group_gather"
	// StageGroupKeep asks Jev whether each larger part the gathering
	// proposed is one thing a newcomer names; only those are gathered.
	StageGroupKeep = "atlas_group_keep"
)

//go:embed prompts/group_enough.md
var groupEnoughPrompt string

//go:embed prompts/group_enough_options.md
var groupEnoughOptionsText string

//go:embed prompts/group_assign.md
var groupAssignPrompt string

//go:embed prompts/group_keep.md
var groupKeepPrompt string

//go:embed prompts/group_keep_options.md
var groupKeepOptionsText string

// The gathering question's options.
const (
	GroupOnePart      = "one part"
	GroupOnlyItsBoxes = "only its boxes"
)

var groupKeepOptions = mustOptions("prompts/group_keep_options.md", groupKeepOptionsText, []string{GroupOnePart, GroupOnlyItsBoxes})

// GroupKeepTable asks, for each larger part the gathering proposed, whether
// a newcomer names it as one thing or it only puts its smaller boxes side by
// side (owner, 2026-10-14: "Triggers and callbacks" over Triggers and
// Callbacks had been one more box to open, saying nothing of its own).
func GroupKeepTable() table.Definition {
	criteria := make(map[string]llm.Criteria, len(groupKeepOptions))
	for name, value := range groupKeepOptions {
		criteria[name] = value
	}
	return table.Definition{
		Stage: StageGroupKeep, Contract: "repomap.atlas.group_keep.v1", System: RoleMap + groupKeepPrompt,
		Classifier: true,
		Columns: []table.Column{{
			Name: "gathering", Kind: table.Choice, Options: []string{GroupOnePart, GroupOnlyItsBoxes},
			Criteria: criteria, Item: "group",
			Ask: "Is `group` one larger part a newcomer names as one thing, or only its smaller boxes side by side?",
		}},
	}
}

// The grouping question's options.
const (
	GroupEnough       = "grouped enough"
	GroupNeedsSmaller = "needs smaller boxes"
)

var groupEnoughOptions = mustOptions("prompts/group_enough_options.md", groupEnoughOptionsText, []string{GroupEnough, GroupNeedsSmaller})

// GroupEnoughTable asks, for one box per question, whether a newcomer reads it as
// it is or needs it divided into smaller boxes first.
func GroupEnoughTable() table.Definition {
	criteria := make(map[string]llm.Criteria, len(groupEnoughOptions))
	for name, value := range groupEnoughOptions {
		criteria[name] = value
	}
	return table.Definition{
		Stage: StageGroupEnough, Contract: "repomap.atlas.group_enough.v1", System: RoleMap + groupEnoughPrompt,
		Classifier: true,
		Columns: []table.Column{{
			Name: "grouping", Kind: table.Choice, Options: []string{GroupEnough, GroupNeedsSmaller},
			Criteria: criteria, Item: "box",
			Ask: "Does a newcomer read `box` as it is, or does it need smaller boxes inside?",
		}},
	}
}

// GroupAssignTable asks, for each declaration of a box being divided, which of
// the proposed smaller boxes it goes in. The smaller boxes are a catalogue
// of refs in the shared context; each option is a box with what it holds as
// its criteria.
func GroupAssignTable() table.Definition {
	return table.Definition{
		Stage: StageGroupAssign, Contract: "repomap.atlas.group_assign.v1", System: RoleMap + groupAssignPrompt,
		// Every element takes its leading box (owner, 2026-10-08): with ten
		// boxes a near-tie is common, and a one-declaration box per tie made
		// 6,183 of Metabase's 9,033 parts.
		Classifier: true, TopChoice: true,
		Columns: []table.Column{{
			Name: "box", Kind: table.Choice, OptionsFrom: "boxes", CriteriaFrom: "holds", Item: "declaration",
			Ask: "Which smaller box of our map does `declaration` go in?",
		}},
	}
}
