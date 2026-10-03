package reading

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// packageSymbol is one symbol of an outside package the program calls and
// every call of it outside tests, or every call in tests when the program
// calls it nowhere else.
type packageSymbol struct {
	calls, testCalls []packageCall
}

// packageCall is one call of an outside symbol where it is written.
type packageCall struct {
	site sourceSite
	call atlas.SymbolCall
}

// callEvidence is what the code knows a call is given, the evidence that may
// choose a system: the literals written at it and, by position, what its
// arguments' source values hold: literals (a driver name a config default
// supplies, a URL a constant concatenates), and the calls and fields that
// supply a value (casdoor's DSN from refineDataSourceNameForPostgres beside
// one handed in). Calls of one symbol with equal evidence are one
// observation; a variable's or a parameter's name chooses nothing, so
// session.Find(&users) and session.Find(&orgs) are one.
func callEvidence(call atlas.SymbolCall) string {
	parts := []string{strings.Join(call.Values, "\x00")}
	var literals func(value *sourcevalue.Value, into *[]string)
	literals = func(value *sourcevalue.Value, into *[]string) {
		if value == nil {
			return
		}
		switch value.Kind {
		case "literal", "call_result", "field":
			*into = append(*into, value.Kind+":"+value.Text)
		}
		literals(value.Initializer, into)
		for i := range value.Parts {
			literals(&value.Parts[i], into)
		}
	}
	for _, argument := range call.SourceArguments {
		var held []string
		literals(argument.Origin, &held)
		if len(held) > 0 {
			parts = append(parts, fmt.Sprintf("%d=%s", argument.Position, strings.Join(held, "\x00")))
		}
	}
	return strings.Join(parts, "\x01")
}

// readSystems asks which outside systems each of the packages reaches
// (lines.Systems), once per package for the whole run, and returns the
// names of each package that reaches some. The item is the package, the
// dependency its targets' manifests record for it and every symbol of it
// the program calls, with each of its calls given different values as
// written: calls given the same values are one (callEvidence), and a call
// given another driver or URL is another, so a package whose calls choose
// several systems is named by each, never by the first it calls (control
// review B7: casdoor's three xorm.NewEngine and two sql.Open calls had
// reached the question as one of each). Each package is remembered on its
// own. A package answered none, or not
// decided, has no name, and no catalogue offers anything for it. packages
// are sorted, each once (targetPackages.all).
func (r *reader) readSystems(ctx context.Context, packages []string) (map[string][]string, error) {
	names := make(map[string][]string)
	if len(packages) == 0 {
		return names, nil
	}
	wanted := make(map[string]bool, len(packages))
	for _, pkg := range packages {
		wanted[pkg] = true
	}
	symbols := make(map[string]map[string]*packageSymbol)
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		inTest := r.testFile(place.Parent)
		for _, call := range place.Symbol.Calls {
			if call.Kind != string(programindex.RelationInvokesExternal) || call.API == nil || !wanted[call.API.Package] || call.Line < 1 {
				continue
			}
			name := call.API.Name
			if call.API.Receiver != "" {
				name = strings.TrimPrefix(call.API.Receiver, "*") + "." + name
			}
			if symbols[call.API.Package] == nil {
				symbols[call.API.Package] = make(map[string]*packageSymbol)
			}
			symbol := symbols[call.API.Package][name]
			if symbol == nil {
				symbol = &packageSymbol{}
				symbols[call.API.Package][name] = symbol
			}
			found := packageCall{site: sourceSite{place.Path, call.Line, call.Column}, call: call}
			if inTest {
				symbol.testCalls = append(symbol.testCalls, found)
			} else {
				symbol.calls = append(symbol.calls, found)
			}
		}
	}
	dependencies := make(map[string][]string)
	for _, target := range r.opts.Targets {
		for _, dependency := range target.Dependencies {
			if record := strings.TrimSpace(dependency.Module + " " + dependency.Version); wanted[dependency.Package] && record != "" && !slices.Contains(dependencies[dependency.Package], record) {
				dependencies[dependency.Package] = append(dependencies[dependency.Package], record)
			}
		}
	}
	files := make(map[string]*lines.CallFile)
	subjects := make(map[string]rowSubject, len(packages))
	rows := make([]table.Row, 0, len(packages))
	for i, pkg := range packages {
		byName := symbols[pkg]
		called := make([]string, 0, len(byName))
		for name := range byName {
			called = append(called, name)
		}
		sort.Strings(called)
		calls := make([]lines.PackageCall, 0, len(called))
		var first sourceSite
		for _, name := range called {
			found := byName[name].calls
			if len(found) == 0 {
				found = byName[name].testCalls
			}
			found = slices.Clone(found)
			slices.SortFunc(found, func(a, b packageCall) int { return a.site.compare(b.site) })
			// The first call of each evidence stands for the calls given
			// the same, as it is written; written alike, two are one.
			call := lines.PackageCall{Symbol: name}
			evidence := make(map[string]bool)
			for _, each := range found {
				if first.path == "" || each.site.compare(first) < 0 {
					first = each.site
				}
				key := callEvidence(each.call)
				if evidence[key] {
					continue
				}
				evidence[key] = true
				if text := r.sourceText(files, each.site.path, each.site.line, each.site.column); !slices.Contains(call.Calls, text) {
					call.Calls = append(call.Calls, text)
				}
			}
			calls = append(calls, call)
		}
		records := dependencies[pkg]
		sort.Strings(records)
		id := fmt.Sprintf("pkg%d", i+1)
		subjects[id] = rowSubject{id: "package:" + pkg, path: first.path, line: first.line}
		rows = append(rows, lines.SystemRow(id, pkg, records, calls))
	}
	r.opts.Stage(lines.StageSystems, fmt.Sprintf("naming the outside systems %d packages reach", len(rows)))
	previous := r.rowSubjects
	r.rowSubjects = subjects
	defer func() { r.rowSubjects = previous }()
	answers, err := r.runTable(ctx, lines.Systems(), 1, rows)
	if err != nil {
		return nil, err
	}
	for i, pkg := range packages {
		name := "not decided"
		if answer := answers[i].answer; answer != nil {
			name = answer[lines.SystemColumn]
			if systems := lines.SystemNames(name); len(systems) > 0 {
				names[pkg] = systems
			}
		}
		fmt.Fprintf(&r.tables, "%s: %s · %s\n", lines.StageSystems, pkg, name)
	}
	fmt.Fprintln(&r.tables)
	r.reportStage(lines.StageSystems)
	return names, nil
}
