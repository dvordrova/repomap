package reading

import (
	"context"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// readPrograms asks, of every call that starts another program, which of
// the words it is given names that program (lines.Program). A symbol such
// as exec.Command starts git at one site and make at another, so each call
// is its own row: the symbol, the call as written and its words. The rows
// of one symbol share their windows, and each exact row is remembered on
// its own, so a warm reading asks nothing. A call given no word that can
// name a program is not asked, and its program stays not established: the
// words an index records are its literal arguments, and a program named
// inside a list (subprocess.run(["git", …])) is no such word, so no word
// is not evidence that none names it. An undecided answer leaves the
// program undecided too.
func (r *reader) readPrograms(ctx context.Context) error {
	var ids []string
	for id, state := range r.boundaries {
		if state.kind == atlas.BoundaryRunsProgram {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Slice(ids, func(i, j int) bool { return compactIDLess(ids[i], ids[j]) })
	type asked struct {
		state *boundaryState
		words map[string]string
	}
	bySymbol := make(map[string][]table.Row)
	var symbols []string
	var order []asked
	files := make(map[string]*lines.CallFile)
	unnamed := 0
	for _, id := range ids {
		state := r.boundaries[id]
		place := state.place
		usage := r.sourceText(files, place.Path, place.LineNo, place.Column)
		row, words := lines.ProgramRow(id, state.apiSymbol, usage, place.Boundary.Values)
		if len(words) == 0 {
			unnamed++
			continue
		}
		r.places[id] = place
		if bySymbol[state.apiSymbol] == nil {
			symbols = append(symbols, state.apiSymbol)
		}
		bySymbol[state.apiSymbol] = append(bySymbol[state.apiSymbol], row)
		order = append(order, asked{state: state, words: words})
	}
	sort.Strings(symbols)
	var groups rowGroups
	byRow := make(map[string]asked, len(order))
	for _, item := range order {
		byRow[item.state.place.ID] = item
	}
	var rows []table.Row
	for _, symbol := range symbols {
		groups = append(groups, rowGroup{rows: bySymbol[symbol]})
		rows = append(rows, bySymbol[symbol]...)
	}
	r.opts.Stage(lines.StageProgram, fmt.Sprintf("naming the programs %d calls start (%d symbols); %d calls give no word to choose among", len(rows), len(symbols), unnamed))
	if len(rows) == 0 {
		return nil
	}
	answers, err := r.runTableGroups(ctx, lines.Program(), 1, groups, nil)
	if err != nil {
		return err
	}
	for i, row := range rows {
		answer := answers[i].answer
		if answer == nil {
			continue
		}
		item := byRow[row.ID]
		switch choice := answer["program"]; choice {
		case lines.ProgramNotNamed:
			item.state.programNotNamed = true
		default:
			if word, ok := item.words[choice]; ok {
				item.state.destination = word
			}
		}
	}
	r.reportStage(lines.StageProgram)
	return nil
}
