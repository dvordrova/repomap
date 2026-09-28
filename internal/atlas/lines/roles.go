package lines

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

// The stages of the role split of a file on the map of parts: the helper
// question asks whether a declaration is a helper, the gate asks whether a
// file's code goes in one box of our map or in several, the naming names the
// boxes a file's code goes in, and the assignment puts each of its
// declarations that is no helper in one of them. Each is its own stage, so
// the journal and the timings say which one a request belonged to.
const (
	StageRoleHelper = "atlas_role_helper"
	StageRoleGate   = "atlas_role_gate"
	StageRoleBoxes  = "atlas_role_boxes"
	StageRoleAssign = "atlas_role_assign"
)

// RoleMap is what we want, the state every request of the role split
// shares: our own architecture map of a program for a newcomer, whose box is
// a responsibility and whose unit is not the author's file.
//
//go:embed prompts/role_map.md
var RoleMap string

//go:embed prompts/role_gate.md
var roleGatePrompt string

//go:embed prompts/role_gate_options.md
var roleGateOptionsText string

//go:embed prompts/role_assign.md
var roleAssignPrompt string

//go:embed prompts/role_helper.md
var roleHelperPrompt string

//go:embed prompts/role_helper_options.md
var roleHelperOptionsText string

// The gate's options.
const (
	RoleOneBox       = "one box"
	RoleSeveralBoxes = "several boxes"
)

// roleGateOptions are the gate's options with their criteria, read once
// from their embedded Markdown.
var roleGateOptions = mustOptions("prompts/role_gate_options.md", roleGateOptionsText, []string{RoleOneBox, RoleSeveralBoxes})

// The helper question's options. "none of these" is listed with its own
// criteria, as the probe asked it, rather than the table's optional cell.
const (
	RoleHelperHelper = "helper"
	RoleHelperOwnJob = "responsibility"
	RoleHelperNone   = "none of these"
)

// roleHelperOptions are the helper question's options with their criteria.
var roleHelperOptions = mustOptions("prompts/role_helper_options.md", roleHelperOptionsText, []string{RoleHelperHelper, RoleHelperOwnJob, RoleHelperNone})

// RoleHelper asks, for each declaration of one file per request, whether it
// is a helper on our map: code that serves the work of other declarations,
// which code then places with its users instead of naming or assigning it.
// The item carries its name, kind, file, signature, code lines, methods, the
// declarations of the program it calls and that call it, read it or hand it
// over ("path:name", test and generated code left out) and the words of its
// registrations; no documentation.
//
// Measured before adoption (steps 0b and 0c, 2026-09-28) with these texts:
// redis-server's main, processCommand (0.98-1.00), call (0.56-0.66),
// rdbSave, syncWithMaster, serverCron and the 94 registered handlers (lowest
// lead 0.72) are no helpers; symsTable is a helper at 0.85-0.91 once its
// item carries its reader; the library files keep their helper marks. On
// pykrx's library target 49-50 of 188 asked units are helpers, 78.9% of the
// leads are 0.40 or more and no unit flips between helper and a decided
// other option across 8 draws. A declaration of a kind with use facts and
// no user in the program is not asked (reading), so an uncalled API
// function is no helper by code.
func RoleHelper() table.Definition {
	criteria := make(map[string]llm.Criteria, len(roleHelperOptions))
	for name, value := range roleHelperOptions {
		criteria[name] = value
	}
	return table.Definition{
		Stage: StageRoleHelper, Contract: "repomap.atlas.role_helper.v1", System: RoleMap + roleHelperPrompt,
		Classifier: true,
		Columns: []table.Column{{
			Name: "helper", Kind: table.Choice, Options: []string{RoleHelperHelper, RoleHelperOwnJob, RoleHelperNone},
			Criteria: criteria, Item: "declaration",
			Ask: "What is `declaration` on our map: a helper, the code of a responsibility, or none of these?",
		}},
	}
}

// RoleGate asks, for one file per request, whether its code goes in one box
// of our map or in several. The file and its declarations are the question's
// item; each option carries its criteria in the map's own terms.
func RoleGate() table.Definition {
	criteria := make(map[string]llm.Criteria, len(roleGateOptions))
	for name, value := range roleGateOptions {
		criteria[name] = value
	}
	return table.Definition{
		Stage: StageRoleGate, Contract: "repomap.atlas.role_gate.v1", System: RoleMap + roleGatePrompt,
		// Measured over every candidate of redis, pykrx, litestream and
		// repomap (377 files, 2 draws, then 5 draws of the 13 nearest the
		// cut): redis.c scores 0.97-0.98 for several boxes, pykrx's
		// stock_api.py 0.74-0.79, litestream's main.go 0.69-0.76;
		// redis-cli.c 0.15-0.21, redis-check-dump.c 0.22-0.30. Criteria that let "their own
		// vocabulary of names" mean several boxes split redis-cli.c,
		// redis-benchmark.c and repomap's render.go and table.go every draw
		// and flipped redis-check-dump.c.
		Classifier: true,
		Columns: []table.Column{{
			Name: "boxes", Kind: table.Choice, Options: []string{RoleOneBox, RoleSeveralBoxes},
			Criteria: criteria, Item: "file",
			Ask: "Does the code of `file` go in one box of our map, or in several boxes?",
		}},
	}
}

// RoleAssign asks, for each declaration of one file whose code goes in
// several boxes, which of the boxes named for that file it goes in. The
// boxes are a catalogue of refs; each option is a box with what it holds as
// its criteria, and the catalogue reaches the model only that way. The
// declaration carries its calls, its callers and the words of every
// registration that hands it over.
//
// Measured on the saved requests of redis.c (339 declarations, 20 boxes)
// and pykrx's 7 split files (187 declarations), 3 draws each: the sentence
// on the box that runs every command and the registration's words together
// put getCommand (Command dispatch at 0.94-0.96 before) and appendCommand in
// String commands, setCommand there in every draw, and pingCommand in
// Server administration commands; either one alone left getCommand in
// Command dispatch. A declaration it leaves open is placed by code where
// its file's users are (reading), never asked again.
func RoleAssign() table.Definition {
	return table.Definition{
		Stage: StageRoleAssign, Contract: "repomap.atlas.role_assign.v2", System: RoleMap + roleAssignPrompt,
		Classifier: true,
		Columns: []table.Column{{
			Name: "box", Kind: table.Choice, OptionsFrom: "boxes", CriteriaFrom: "holds", Item: "declaration",
			Ask: "Which box of our map does `declaration` go in?",
		}},
	}
}

// parseOptionCriteria reads "## option" sections, each with What, Includes,
// Not for and a list of Examples; "none of these" may give only What and Not
// for, since the other options' criteria say what it is not.
func parseOptionCriteria(text string) (map[string]llm.Criteria, error) {
	options := map[string]llm.Criteria{}
	sections := strings.Split(text, "\n## ")
	for _, section := range sections[1:] {
		lines := strings.Split(section, "\n")
		name := strings.TrimSpace(lines[0])
		var criteria llm.Criteria
		examples := false
		for _, line := range lines[1:] {
			line = strings.TrimSpace(line)
			switch {
			case line == "":
			case strings.HasPrefix(line, "What: "):
				criteria.What, examples = strings.TrimPrefix(line, "What: "), false
			case strings.HasPrefix(line, "Includes: "):
				criteria.Includes, examples = strings.TrimPrefix(line, "Includes: "), false
			case strings.HasPrefix(line, "Not for: "):
				criteria.NotFor, examples = strings.TrimPrefix(line, "Not for: "), false
			case line == "Examples:":
				examples = true
			case examples && strings.HasPrefix(line, "- "):
				criteria.Examples = append(criteria.Examples, strings.TrimPrefix(line, "- "))
			default:
				return nil, fmt.Errorf("option %q: unreadable line %q", name, line)
			}
		}
		if name == "" || criteria.What == "" || criteria.NotFor == "" || name != RoleHelperNone && (criteria.Includes == "" || len(criteria.Examples) == 0) {
			return nil, fmt.Errorf("option %q lacks what, includes, not for or examples", name)
		}
		if _, repeated := options[name]; repeated {
			return nil, fmt.Errorf("option %q is given twice", name)
		}
		options[name] = criteria
	}
	return options, nil
}
