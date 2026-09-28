package reading

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/programindex"
)

// packageSymbol is one symbol of an outside package the program calls and
// the call of it a reader would look at: the first outside tests that is
// given a literal, else the first outside tests, else the first.
type packageSymbol struct {
	site             sourceSite
	literals, inTest bool
}

// readSystems asks which outside system each of the packages reaches
// (lines.Systems), once per package for the whole run, and returns the
// name of each package that reaches one. The item is the package, the
// dependency its targets' manifests record for it and every symbol of it
// the program calls, with one call of each as written; each package is
// remembered on its own. A package answered none, or not decided, has no
// name, and no catalogue offers anything for it.
func (r *reader) readSystems(ctx context.Context, packages []string) (map[string]string, error) {
	names := make(map[string]string)
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
			seen := symbols[call.API.Package][name]
			found := &packageSymbol{site: sourceSite{place.Path, call.Line, call.Column}, literals: len(call.Values) > 0, inTest: inTest}
			if seen == nil || seen.inTest && !inTest || seen.inTest == inTest && !seen.literals && found.literals {
				symbols[call.API.Package][name] = found
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
	sorted := slices.Clone(packages)
	sort.Strings(sorted)
	sorted = slices.Compact(sorted)
	files := make(map[string]*lines.CallFile)
	subjects := make(map[string]rowSubject, len(sorted))
	rows := make([]table.Row, 0, len(sorted))
	for i, pkg := range sorted {
		byName := symbols[pkg]
		called := make([]string, 0, len(byName))
		for name := range byName {
			called = append(called, name)
		}
		sort.Strings(called)
		calls := make([]lines.PackageCall, 0, len(called))
		var first sourceSite
		for _, name := range called {
			site := byName[name].site
			if first.path == "" || site.compare(first) < 0 {
				first = site
			}
			calls = append(calls, lines.PackageCall{Symbol: name, Call: r.sourceText(files, site.path, site.line, site.column)})
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
	for i, pkg := range sorted {
		name := "not decided"
		if answer := answers[i].answer; answer != nil {
			name = answer[lines.SystemColumn]
			if system := lines.SystemName(name); system != "" {
				names[pkg] = system
			}
		}
		fmt.Fprintf(&r.tables, "%s: %s · %s\n", lines.StageSystems, pkg, name)
	}
	fmt.Fprintln(&r.tables)
	r.reportStage(lines.StageSystems)
	return names, nil
}
