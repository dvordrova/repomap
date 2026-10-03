package lines

import (
	_ "embed"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

// StageSystems names the outside system each outside package reaches.
const StageSystems = "atlas_systems"

// SystemColumn is the one cell of the systems table; SystemNone its answer
// that calls through the package reach no outside system.
const (
	SystemColumn = "system"
	SystemNone   = "none"
)

//go:embed prompts/systems.md
var systemsPrompt string

// Systems asks, of each outside package the outgoing boundaries call
// through, which outside systems its calls reach (repomap.atlas.systems.v2).
// A name is text, so the text model answers; each package is its own row,
// remembered on its own, with every distinct call of it, so a package whose
// calls choose several systems (a driver name or a URL given to them) is
// named by each, never by its first call. The boundaries table then chooses
// among these names; no list of known systems is kept in code.
func Systems() table.Definition {
	return table.Definition{Stage: StageSystems, Contract: "repomap.atlas.systems.v2", System: systemsPrompt, Memoize: true,
		Columns: []table.Column{{Name: SystemColumn, Kind: table.Text, MaxRunes: SystemsRunes,
			Note: "every outside system calls through this package reach, each as a newcomer would name it, separated by \"; \", or none"}}}
}

// SystemsRunes bounds the systems cell: a few systems' names.
const SystemsRunes = 3 * LabelRunes

// PackageCall is one symbol of an outside package the program calls, with
// each distinct call of it as the repository wrote it: calls written alike
// are one, any difference in what they are given is another.
type PackageCall struct {
	Symbol string   `json:"symbol"`
	Calls  []string `json:"calls,omitempty"`
}

// SystemRow is one outside package: its path as the code names it, the
// dependency lines the manifest records for it ("module version"), and
// every symbol of it the program calls, with every distinct call of each.
func SystemRow(id, pkg string, dependency []string, calls []PackageCall) table.Row {
	fields := []table.Field{{Name: "package", Value: pkg}}
	if len(dependency) > 0 {
		fields = append(fields, table.Field{Name: "dependency", Value: dependency})
	}
	fields = append(fields, table.Field{Name: "calls", Value: calls})
	return table.Row{ID: id, Fields: fields}
}

// SystemNames reads an accepted system cell: each name as written, once
// whatever its case, in the order written; none when the package reaches no
// outside system. A "none" beside names adds nothing.
func SystemNames(cell string) []string {
	var names []string
	for _, part := range strings.Split(cell, ";") {
		name := strings.TrimSpace(part)
		if name == "" || strings.EqualFold(strings.TrimSuffix(name, "."), SystemNone) {
			continue
		}
		if !slices.ContainsFunc(names, func(known string) bool { return strings.EqualFold(known, name) }) {
			names = append(names, name)
		}
	}
	return names
}

// Destination is one entry of a boundary row's closed catalogue: a system
// the outside packages of the row's programs reach, and those packages, or
// one of this repository's other programs (Program) with what it takes in
// while it runs (Takes). Target is that program's target, never sent.
type Destination struct {
	Ref      string   `json:"ref"`
	Value    string   `json:"value"`
	Packages []string `json:"packages,omitempty"`
	Program  bool     `json:"program,omitempty"`
	Takes    []string `json:"takes,omitempty"`
	Target   string   `json:"-"`
}

// WithPrograms appends this repository's other programs to a catalogue of
// systems, their refs going on from the catalogue's last.
func WithPrograms(catalog, programs []Destination) []Destination {
	offered := append([]Destination(nil), catalog...)
	for _, program := range programs {
		program.Ref = fmt.Sprintf("d%d", len(offered)+1)
		offered = append(offered, program)
	}
	return offered
}

// DestinationTarget is the program a chosen cell names, with the program's
// name. A ref the cell chose stands as chosen: a program's ref is that
// program, a system's ref that system, never a program sharing its name.
// A free name (a cell naming no known ref) chooses the offered program it
// writes as offered, case aside (freqtrade's run drew "other: Freqtrade"
// where "freqtrade" was offered: a form difference, not a second choice),
// unless a system of the catalogue has that name too. A free name is never
// read through a program's import path: a path's last element proves no
// runtime endpoint (review 2026-10-02). A program is offered by the name
// the toolchain gives its executable (reading's programNames), so casdoor's
// "other: Casdoor" writes the offered "casdoor". Empty for a system, an
// unknown ref or any other name.
func DestinationTarget(catalog []Destination, ref, name string) (string, string) {
	for _, entry := range catalog {
		if entry.Ref == ref {
			if entry.Program {
				return entry.Target, entry.Value
			}
			return "", ""
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ""
	}
	var named *Destination
	for i := range catalog {
		entry := &catalog[i]
		if !strings.EqualFold(entry.Value, name) {
			continue
		}
		if !entry.Program || named != nil {
			return "", ""
		}
		named = entry
	}
	if named == nil {
		return "", ""
	}
	return named.Target, named.Value
}

// Destinations is the closed catalogue of the named packages: one entry per
// system, names equal but for case being one, in name order, refs d1, d2,
// ...; a package reaching several systems is listed under each. It depends
// only on the names given, so rows of the same programs are offered the same
// catalogue in every window.
func Destinations(names map[string][]string) []Destination {
	byName := make(map[string]*Destination)
	for pkg, systems := range names {
		for _, name := range systems {
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			entry := byName[key]
			if entry == nil {
				entry = &Destination{Value: name}
				byName[key] = entry
			}
			// The spelling kept is the least in byte order, whatever map
			// order visits the packages in.
			if name < entry.Value {
				entry.Value = name
			}
			if !slices.Contains(entry.Packages, pkg) {
				entry.Packages = append(entry.Packages, pkg)
			}
		}
	}
	keys := make([]string, 0, len(byName))
	for key := range byName {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	catalog := make([]Destination, 0, len(keys))
	for i, key := range keys {
		entry := byName[key]
		sort.Strings(entry.Packages)
		entry.Ref = fmt.Sprintf("d%d", i+1)
		catalog = append(catalog, *entry)
	}
	return catalog
}

// DestinationFields are a window's closed catalogue and its refs, the
// options of the destination column.
func DestinationFields(catalog []Destination) []table.Field {
	refs := make([]string, 0, len(catalog))
	for _, entry := range catalog {
		refs = append(refs, entry.Ref)
	}
	return []table.Field{{Name: "destination_catalog", Value: catalog}, {Name: "destination_options", Value: refs}}
}

// DestinationProgram is the program whose calls a window of destinations
// names, by its name: a destination is one program's, and the calls a
// server makes to its master are named in its own window, never beside
// its clients' identical calls to the server.
func DestinationProgram(name string) table.Field {
	return table.Field{Name: "program", Value: name}
}

// DestinationValue is the system a chosen ref names; empty when the
// catalogue has no such ref.
func DestinationValue(catalog []Destination, ref string) string {
	for _, entry := range catalog {
		if entry.Ref == ref {
			return entry.Value
		}
	}
	return ""
}
