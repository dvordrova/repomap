package lines

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

// The stages of the role split of a file on the map of parts: the gate
// asks whether a file's code goes in one box of our map or in several, the
// naming names the boxes a file's code goes in, the assignment puts each of
// its declarations in one of them, and the neighbours' question asks again
// about a declaration the assignment left open, with the boxes its calls and
// callers went in. Each is its own stage, so the journal and the timings say
// which one a request belonged to.
const (
	StageRoleGate       = "atlas_role_gate"
	StageRoleBoxes      = "atlas_role_boxes"
	StageRoleAssign     = "atlas_role_assign"
	StageRoleNeighbours = "atlas_role_neighbours"
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

//go:embed prompts/role_neighbours.md
var roleNeighboursPrompt string

// The gate's options.
const (
	RoleOneBox       = "one box"
	RoleSeveralBoxes = "several boxes"
)

// roleGateOptions are the gate's options with their criteria, read once
// from their embedded Markdown.
var roleGateOptions = mustRoleGateOptions(roleGateOptionsText)

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
// Measured with the neighbours' question after it on the saved requests of
// redis.c (339 declarations, 20 boxes) and pykrx's 7 split files (187
// declarations), 3 draws each: redis.c left 3, 3 and 3 declarations
// undecided (4, 9 and 8 before) and none landed differently between draws
// (12 before); pykrx 2, 3 and 2 (3, 6 and 2). The sentence on the box that
// runs every command and the registration's words together put getCommand
// (Command dispatch at 0.94-0.96 before) and appendCommand in String
// commands, setCommand there in every draw, and pingCommand in Server
// administration commands; either one alone left getCommand in Command
// dispatch.
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

// RoleNeighbours asks the assignment's question again for a declaration it
// left open, one that has a call or a caller of its file whose box was
// chosen: each of its calls and callers then names the box it went in. The
// options, their criteria and the lead a choice needs are the assignment's.
//
// Measured the same way, it decided 3 of the 6 redis.c and both pykrx
// declarations it asked about; ttlCommand stayed a near-tie (0.49-0.52
// against 0.43-0.47). Written as "name: box" beside the calls instead of as
// each call's own box, it put setCommand, which calls setGenericCommand of
// String commands, in Set commands in 5 of 5 draws.
func RoleNeighbours() table.Definition {
	def := RoleAssign()
	def.Stage, def.Contract, def.System = StageRoleNeighbours, "repomap.atlas.role_neighbours.v1", RoleMap+roleAssignPrompt+roleNeighboursPrompt
	return def
}

// mustRoleGateOptions reads "## option" sections, each with What, Includes,
// Not for and a list of Examples. The text is embedded, so a malformed file
// is a defect every test of this package meets.
func mustRoleGateOptions(text string) map[string]llm.Criteria {
	options, err := parseOptionCriteria(text)
	if err != nil {
		panic(fmt.Sprintf("lines: prompts/role_gate_options.md: %v", err))
	}
	for _, name := range []string{RoleOneBox, RoleSeveralBoxes} {
		if _, ok := options[name]; !ok {
			panic(fmt.Sprintf("lines: prompts/role_gate_options.md has no option %q", name))
		}
	}
	if len(options) != 2 {
		panic("lines: prompts/role_gate_options.md lists options the gate does not ask")
	}
	return options
}

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
		if name == "" || criteria.What == "" || criteria.Includes == "" || criteria.NotFor == "" || len(criteria.Examples) == 0 {
			return nil, fmt.Errorf("option %q lacks what, includes, not for or examples", name)
		}
		if _, repeated := options[name]; repeated {
			return nil, fmt.Errorf("option %q is given twice", name)
		}
		options[name] = criteria
	}
	return options, nil
}
