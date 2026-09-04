package lines

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const (
	StageSymbols    = "atlas_symbols"
	StageBoundaries = "atlas_boundaries"
	StageZones      = "atlas_zones"
	StageArrows     = "atlas_arrows"
	StageTargets    = "atlas_targets"
	StageJoints     = "atlas_joints"

	symbolsContract    = "repomap.atlas.symbols.v1"
	boundariesContract = "repomap.atlas.boundaries.v1"
	zonesContract      = "repomap.atlas.zones.v1"
	arrowsContract     = "repomap.atlas.arrows.v1"
	targetsContract    = "repomap.atlas.targets.v1"
	jointsContract     = "repomap.atlas.joints.v1"

	// ShortLineRunes bounds the lines of boundaries and symbols; LabelRunes
	// the label of a joint.
	ShortLineRunes = 120
	LabelRunes     = 40

	// PeerNone is the peer choice that says no listed boundary is the
	// counterpart.
	PeerNone = "none"

	maxWitnesses = 3
	maxZoneBoxes = 8
	maxValues    = 8
)

//go:embed prompts/symbols.md
var symbolsPrompt string

//go:embed prompts/boundaries.md
var boundariesPrompt string

//go:embed prompts/zones.md
var zonesPrompt string

//go:embed prompts/arrows.md
var arrowsPrompt string

//go:embed prompts/targets.md
var targetsPrompt string

//go:embed prompts/joints.md
var jointsPrompt string

// SymbolWindowRows is the symbol table's window: the rows are short.
const SymbolWindowRows = 50

// MaxKeysPerFile bounds how many symbols the model may mark as key in one
// file; the code keeps the first by rank.
const MaxKeysPerFile = 5

// Symbols is the symbol table.
func Symbols() table.Definition {
	return table.Definition{
		Stage: StageSymbols, Contract: symbolsContract, Window: SymbolWindowRows,
		System: symbolsPrompt, MaxOutputTokens: 8192,
		Columns: []table.Column{
			{Name: "line", Kind: table.Text, MaxRunes: ShortLineRunes, Note: "one sentence, what this declaration does or is"},
			{Name: "key_symbol", Kind: table.Choice, Options: []string{"yes", "no"}, Note: "yes for the declarations a reader looks at first"},
		},
	}
}

// SymbolRow builds the row of one candidate symbol.
func SymbolRow(place atlas.Place, fileLine string) table.Row {
	decl := place.Symbol.Decl
	fields := []table.Field{
		{Name: "name", Value: decl.Name},
		{Name: "kind", Value: decl.Kind},
	}
	if decl.Signature != "" {
		fields = append(fields, table.Field{Name: "signature", Value: cut(decl.Signature, maxSignature)})
	}
	if decl.Doc != "" {
		fields = append(fields, table.Field{Name: "doc", Value: cut(decl.Doc, maxDoc)})
	}
	if fileLine != "" {
		fields = append(fields, table.Field{Name: "file", Value: fileLine})
	}
	fields = append(fields, table.Field{Name: "callers", Value: decl.FanIn})
	return table.Row{ID: place.ID, Fields: fields}
}

// Boundaries is the boundary table.
func Boundaries() table.Definition {
	return table.Definition{
		Stage: StageBoundaries, Contract: boundariesContract, Window: WindowRows,
		System: boundariesPrompt, MaxOutputTokens: 8192,
		Columns: []table.Column{
			{Name: "line", Kind: table.Text, MaxRunes: ShortLineRunes, Note: "one sentence, what crosses this boundary"},
			{Name: "kind", Kind: table.Choice, Options: atlas.BoundaryKinds(), Note: "repeat kind_given when present"},
		},
	}
}

// BoundaryRow builds the row of one boundary place.
func BoundaryRow(place atlas.Place, fileLine string) table.Row {
	facts := place.Boundary
	fields := []table.Field{
		{Name: "path", Value: place.Path},
		{Name: "caller", Value: facts.Caller},
	}
	if facts.CallerDoc != "" {
		fields = append(fields, table.Field{Name: "caller_doc", Value: facts.CallerDoc})
	}
	if fileLine != "" {
		fields = append(fields, table.Field{Name: "file", Value: fileLine})
	}
	if facts.External != "" {
		fields = append(fields, table.Field{Name: "external", Value: facts.External})
	}
	if facts.Method != "" {
		fields = append(fields, table.Field{Name: "method", Value: facts.Method})
	}
	fields = append(fields,
		table.Field{Name: "values", Value: bounded(facts.Values, maxValues)},
		table.Field{Name: "direction", Value: facts.Direction},
	)
	if facts.GivenKind != "" {
		fields = append(fields, table.Field{Name: "kind_given", Value: facts.GivenKind})
	}
	return table.Row{ID: place.ID, Fields: fields}
}

// ZoneNames asks for exactly want part names of one target in one row: a
// cell per part, so the answer cannot be one name per box. ZoneAssign then
// places every top box against that closed list; ZoneLines gives each part
// its sentence. All three share one prompt and one stage.
func ZoneNames(want int) table.Definition {
	columns := make([]table.Column, 0, want)
	for i := 1; i <= want; i++ {
		columns = append(columns, table.Column{
			Name: fmt.Sprintf("part_%d", i), Kind: table.Text, MaxRunes: TitleRunes,
			Note: "the name of one part, two to four words, distinct from the others",
		})
	}
	return table.Definition{
		Stage: StageZones, Contract: fmt.Sprintf("%s.names.%d", zonesContract, want), Window: 1,
		System: zonesPrompt, MaxOutputTokens: 2048, Columns: columns,
	}
}

// ZoneNamesRow is the one row of the names question: the target's largest
// boxes, each with its title, line and size.
func ZoneNamesRow(targetID string, boxes []BoxSummary) table.Row {
	entries := make([]string, 0, len(boxes))
	for _, box := range boxes {
		entries = append(entries, fmt.Sprintf("%s: %s (%d files)", box.Title, box.Line, box.Files))
	}
	return table.Row{ID: targetID, Fields: []table.Field{{Name: "boxes", Value: entries}}}
}

// PartName reads the i-th part cell of a names answer.
func PartName(answer map[string]string, i int) string {
	return answer[fmt.Sprintf("part_%d", i+1)]
}

func ZoneAssign(parts []string) table.Definition {
	return table.Definition{
		Stage: StageZones, Contract: zonesContract + ".assign", Window: WindowRows,
		System: zonesPrompt, MaxOutputTokens: 4096,
		Columns: []table.Column{{Name: "part", Kind: table.Choice, Options: parts, Note: "one of context.parts"}},
	}
}

func ZoneLines() table.Definition {
	return table.Definition{
		Stage: StageZones, Contract: zonesContract + ".lines", Window: WindowRows,
		System: zonesPrompt, MaxOutputTokens: 4096,
		Columns: []table.Column{{Name: "line", Kind: table.Text, MaxRunes: LineRunes, Note: "one sentence, what this part does"}},
	}
}

// BoxSummary is what a zone row says about a box.
type BoxSummary struct {
	ID    string
	Title string
	Line  string
	Files int
}

// ZoneBoxRow is a row of the names or assign question.
func ZoneBoxRow(box BoxSummary) table.Row {
	return table.Row{ID: box.ID, Fields: []table.Field{
		{Name: "title", Value: box.Title},
		{Name: "line", Value: box.Line},
		{Name: "files", Value: box.Files},
	}}
}

// ZoneLineRow is a row of the lines question.
func ZoneLineRow(zoneID, title string, boxes []BoxSummary) table.Row {
	titles := make([]string, 0, min(len(boxes), maxZoneBoxes))
	files := 0
	for _, box := range boxes {
		files += box.Files
		if len(titles) < maxZoneBoxes {
			titles = append(titles, box.Title)
		}
	}
	return table.Row{ID: zoneID, Fields: []table.Field{
		{Name: "part", Value: title},
		{Name: "boxes", Value: titles},
		{Name: "files", Value: files},
	}}
}

// WantZones says how many parts a target of n top boxes should have: the
// square root, held between four and eight.
func WantZones(n int) int {
	want := 0
	for want*want < n {
		want++
	}
	if want < 4 {
		want = 4
	}
	if want > 8 {
		want = 8
	}
	if want > n {
		want = n
	}
	return want
}

// Arrows is the arrow table.
func Arrows() table.Definition {
	return table.Definition{
		Stage: StageArrows, Contract: arrowsContract, Window: WindowRows,
		System: arrowsPrompt, MaxOutputTokens: 8192,
		Columns: []table.Column{{Name: "sentence", Kind: table.Text, MaxRunes: LineRunes, Note: "what the first box does with the second"}},
	}
}

// ArrowRow builds the row of one drawn arrow.
func ArrowRow(id string, from, to BoxSummary, witnesses []atlas.Witness, calls int) table.Row {
	pairs := make([]string, 0, min(len(witnesses), maxWitnesses))
	for _, witness := range witnesses {
		pairs = append(pairs, witness.Caller+" calls "+witness.Callee)
		if len(pairs) == maxWitnesses {
			break
		}
	}
	return table.Row{ID: id, Fields: []table.Field{
		{Name: "from", Value: from.Title + ": " + from.Line},
		{Name: "to", Value: to.Title + ": " + to.Line},
		{Name: "witnesses", Value: pairs},
		{Name: "calls", Value: calls},
	}}
}

// FallbackSentence is the arrow's sentence when the model has not spoken.
func FallbackSentence(from, to BoxSummary, witnesses []atlas.Witness) string {
	names := make([]string, 0, maxWitnesses)
	for _, witness := range witnesses {
		names = append(names, witness.Callee)
		if len(names) == maxWitnesses {
			break
		}
	}
	if len(names) == 0 {
		return from.Title + " uses " + to.Title + "."
	}
	return from.Title + " calls " + to.Title + ": " + strings.Join(names, ", ") + "."
}

// Targets is the portfolio table.
func Targets() table.Definition {
	return table.Definition{
		Stage: StageTargets, Contract: targetsContract, Window: WindowRows,
		System: targetsPrompt, MaxOutputTokens: 8192,
		Columns: []table.Column{
			{Name: "line", Kind: table.Text, MaxRunes: LineRunes, Note: "one sentence, what this target is and does"},
			{Name: "role", Kind: table.Choice, Options: atlas.Roles()},
		},
	}
}

// TargetSummary is what a portfolio row says about a target.
type TargetSummary struct {
	ID         string
	Name       string
	Root       string
	Language   string
	Kind       string
	Readme     string
	Entrypoint string
	Files      int
	Dirs       int
	Boundaries map[string]int
}

// TargetRow builds the row of one target.
func TargetRow(target TargetSummary) table.Row {
	fields := []table.Field{
		{Name: "name", Value: target.Name},
		{Name: "root", Value: target.Root},
		{Name: "language", Value: target.Language},
		{Name: "kind", Value: target.Kind},
	}
	if target.Readme != "" {
		fields = append(fields, table.Field{Name: "readme", Value: target.Readme})
	}
	if target.Entrypoint != "" {
		fields = append(fields, table.Field{Name: "entrypoint", Value: target.Entrypoint})
	}
	fields = append(fields,
		table.Field{Name: "files", Value: target.Files},
		table.Field{Name: "dirs", Value: target.Dirs},
	)
	if len(target.Boundaries) > 0 {
		kinds := make([]string, 0, len(target.Boundaries))
		for kind := range target.Boundaries {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		counts := make([]string, 0, len(kinds))
		for _, kind := range kinds {
			counts = append(counts, fmt.Sprintf("%s %d", kind, target.Boundaries[kind]))
		}
		fields = append(fields, table.Field{Name: "boundaries", Value: counts})
	}
	return table.Row{ID: target.ID, Fields: fields}
}

// FallbackRole guesses a target's role from its path and kind when the
// model has not spoken.
func FallbackRole(root, kind string) string {
	lower := strings.ToLower(root)
	for _, marker := range []string{"testdata", "fixture", "fixtures", "test", "tests"} {
		if strings.HasPrefix(lower, marker+"/") || strings.Contains(lower, "/"+marker+"/") || lower == marker {
			return atlas.RoleFixture
		}
	}
	for _, marker := range []string{"example", "examples", "samples", "demo"} {
		if strings.HasPrefix(lower, marker+"/") || strings.Contains(lower, "/"+marker+"/") || lower == marker {
			return atlas.RoleExample
		}
	}
	if strings.Contains(kind, "library") {
		return atlas.RoleLibrary
	}
	return atlas.RoleProduct
}

// Joints is the joint table; Peers the blind form of it.
func Joints() table.Definition {
	return table.Definition{
		Stage: StageJoints, Contract: jointsContract + ".joints", Window: WindowRows,
		System: jointsPrompt, MaxOutputTokens: 4096,
		Columns: []table.Column{
			{Name: "same", Kind: table.Choice, Options: []string{"yes", "no"}},
			{Name: "label", Kind: table.Text, MaxRunes: LabelRunes, Note: "at most six words, or - when same is no"},
		},
	}
}

func Peers() table.Definition {
	return table.Definition{
		Stage: StageJoints, Contract: jointsContract + ".peers", Window: WindowRows,
		System: jointsPrompt, MaxOutputTokens: 4096,
		Columns: []table.Column{
			{Name: "peer", Kind: table.Choice, OptionsFrom: "peer_options", Note: "a ref from context.peers, or none"},
			{Name: "label", Kind: table.Text, MaxRunes: LabelRunes, Note: "at most six words, or - when peer is none"},
		},
	}
}

// BoundarySide is one side of a joint as the model sees it.
type BoundarySide struct {
	Target   string
	Line     string
	External string
	Method   string
	Values   []string
}

func sideValue(side BoundarySide) map[string]any {
	value := map[string]any{"target": side.Target, "line": side.Line, "values": bounded(side.Values, maxValues)}
	if side.External != "" {
		value["external"] = side.External
	}
	if side.Method != "" {
		value["method"] = side.Method
	}
	return value
}

// JointRow builds the row of one candidate joint.
func JointRow(id, value string, a, b BoundarySide) table.Row {
	return table.Row{ID: id, Fields: []table.Field{
		{Name: "value", Value: value},
		{Name: "a", Value: sideValue(a)},
		{Name: "b", Value: sideValue(b)},
	}}
}

// PeerRow builds the row of one blind outgoing boundary; refs are the
// window's peer refs.
func PeerRow(id string, side BoundarySide, refs []string) table.Row {
	options := append([]string{PeerNone}, refs...)
	return table.Row{ID: id, Fields: []table.Field{
		{Name: "a", Value: sideValue(side)},
		{Name: "peer_options", Value: options},
	}}
}

// PeerContext is the closed list of incoming boundaries a peers window
// chooses from, as the context field.
func PeerContext(refs []string, sides []BoundarySide) table.Field {
	peers := make([]map[string]any, 0, len(refs))
	for i, ref := range refs {
		peer := sideValue(sides[i])
		peer["ref"] = ref
		peers = append(peers, peer)
	}
	return table.Field{Name: "peers", Value: peers}
}
