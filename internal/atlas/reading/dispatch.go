package reading

import (
	"context"
	"fmt"
	"slices"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// Values the repository's own code compares with several words (ProgramIndex
// Comparison, places SymbolFacts.Comparisons): a switch on a program's first
// argument, an if/elif chain on it, a match or a case form. A comparison is
// no call, so readCalls never asks it, and no outside symbol's role decides
// it; like a table of names it is an input the repository's own code
// declares. Each comparison outside tests is asked once, for all its cases
// (lines.APIDispatch), in the inputs step; its decided kind is kept beside
// the tables' in tableKinds under comparisonKey, which holds a NUL no place
// ID holds. An entry answer makes one entry per case whose words can name
// one, declared by the comparing declaration, whose handler is not
// established, at the case's first word (bindComparisons): a case listing
// several words is one entry. None and an undecided answer make nothing;
// the classifier's rejected rows and a tables.md line record them.

// comparisonKey names one comparison by its declaration's place and its
// first word.
func comparisonKey(placeID string, comparison atlas.Comparison) string {
	return fmt.Sprintf("comparison\x00%s\x00%d\x00%d", placeID, comparison.LineNo, comparison.Column)
}

// originOwners names a parameter's or receiver's owner, by the
// declaration's anchor, for originText.
func (r *reader) originOwners() map[sourcevalue.Anchor]string {
	owners := map[sourcevalue.Anchor]string{}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		owners[sourcevalue.Anchor{Path: place.Path, Line: place.LineNo, Column: place.Symbol.Decl.Column}] = place.Symbol.Decl.Name
		if line := (sourcevalue.Anchor{Path: place.Path, Line: place.LineNo}); owners[line] == "" {
			owners[line] = place.Symbol.Decl.Name
		}
	}
	return owners
}

// readComparisons asks what the words of each comparison outside tests
// become. A comparison none of whose words can name an entry is not asked.
func (r *reader) readComparisons(ctx context.Context) error {
	var owners map[sourcevalue.Anchor]string
	var rows []table.Row
	var keys []string
	subjects := map[string]rowSubject{}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil || len(place.Symbol.Comparisons) == 0 || r.testFile(place.Parent) || len(runningTargets(place)) == 0 {
			continue
		}
		if owners == nil {
			owners = r.originOwners()
		}
		decl := place.Symbol.Decl
		in := decl.Name
		if decl.Signature != "" {
			in += " " + decl.Signature
		}
		for _, comparison := range place.Symbol.Comparisons {
			cases := make([][]string, 0, len(comparison.Cases))
			var words []string
			for _, item := range comparison.Cases {
				cases = append(cases, item.Words)
				words = append(words, item.Words...)
			}
			if len(lines.NameableWords(words)) == 0 {
				continue
			}
			fields := []table.Field{{Name: "compares", Value: comparison.Value}}
			if comparison.Origin != nil {
				fields = append(fields, table.Field{Name: "from", Value: originText(comparison.Origin, owners)})
			}
			fields = append(fields, table.Field{Name: "in", Value: in}, table.Field{Name: "cases", Value: cases})
			id := fmt.Sprintf("cmp%d", len(rows)+1)
			subjects[id] = rowSubject{id: fmt.Sprintf("comparison:%s@%d:%d", place.ID, comparison.LineNo, comparison.Column), path: place.Path, line: comparison.LineNo}
			rows = append(rows, table.Row{ID: id, Fields: fields})
			keys = append(keys, comparisonKey(place.ID, comparison))
		}
	}
	if len(rows) == 0 {
		return nil
	}
	r.opts.Stage(lines.StageInputs, fmt.Sprintf("asking what the words of %d values the repository's own code compares with several words become", len(rows)))
	previous := r.rowSubjects
	r.rowSubjects = subjects
	defer func() { r.rowSubjects = previous }()
	answers, err := r.runTable(ctx, lines.APIDispatch(), 4, rows)
	if err != nil {
		return err
	}
	decided := 0
	for i, key := range keys {
		if answer := answers[i].answer; answer != nil && answer["enters"] != "" {
			r.tableKinds[key] = answer["enters"]
			decided++
		}
	}
	fmt.Fprintf(&r.tables, "atlas_inputs comparisons: %d of %d comparisons decided\n\n", decided, len(rows))
	return nil
}

// bindComparisons makes each case of a comparison answered an entry kind an
// entry at its first word, declared by the comparing declaration (its
// catalogue), named from the case's words. A case whose lines call into the
// program's own code is handled there: its handler is the comparing
// declaration within the case's lines (atlas Boundary BranchLine), as
// litestream's case "replicate" runs NewReplicateCommand and the command's
// ParseFlags, Run and Close; any other case's handler is not established
// (a case returning a word, printing usage). A case none of whose words can
// name an entry makes none (entry_unnamed).
func (r *reader) bindComparisons() {
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil || len(place.Symbol.Comparisons) == 0 || r.testFile(place.Parent) {
			continue
		}
		targets := runningTargets(place)
		decl := place.Symbol.Decl
		for _, comparison := range place.Symbol.Comparisons {
			kind := r.tableKinds[comparisonKey(place.ID, comparison)]
			if kind == "" || kind == lines.APINone || len(targets) == 0 {
				continue
			}
			for _, item := range comparison.Cases {
				if len(lines.NameableWords(item.Words)) == 0 {
					r.noEntryWithoutWords(atlas.Place{ID: place.ID, Path: place.Path, LineNo: item.LineNo}, kind)
					continue
				}
				source := fmt.Sprintf("%s\x00comparison\x00%s\x00%d\x00%d", atlas.DirectionIn, place.ID, item.LineNo, item.Column)
				id := r.boundaryIDs[source]
				if id == "" {
					id = r.compactID("b", &r.nextBoundary)
					r.boundaryIDs[source] = id
				}
				state := &boundaryState{kind: kind, handlerUnknown: true, place: atlas.Place{
					ID: id, Kind: atlas.PlaceBoundary, Path: place.Path, LineNo: item.LineNo, Column: item.Column,
					Parent: place.Parent, TargetIDs: slices.Clone(targets), Boundary: &atlas.BoundaryFacts{
						Source: "model", ObjectID: decl.ObjectID, Caller: decl.Name,
						Values: slices.Clone(item.Words), Words: slices.Clone(item.Words), Direction: atlas.DirectionIn, GivenKind: kind}}}
				if caseCallsProgram(place, item) {
					state.handlerUnknown, state.branch = false, [2]int{item.BranchLine, item.BranchEnd}
				}
				r.boundaries[id] = state
			}
		}
	}
}

// caseCallsProgram reports a case whose lines, when the adapter knows them,
// hold a call into the program's own code the comparing declaration makes.
func caseCallsProgram(place atlas.Place, item atlas.ComparisonCase) bool {
	if item.BranchLine < 1 || item.BranchEnd < item.BranchLine {
		return false
	}
	for _, call := range place.Symbol.Calls {
		if call.Kind == "calls" && len(call.CalleeIDs) > 0 && call.Line >= item.BranchLine && call.Line <= item.BranchEnd {
			return true
		}
	}
	return false
}
