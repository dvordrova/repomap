package lines

import (
	_ "embed"
	"fmt"
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
// through, which outside system its calls reach (repomap.atlas.systems.v1).
// A name is text, so the text model answers; each package is its own row,
// remembered on its own. The boundaries table then chooses among these
// names; no list of known systems is kept in code.
func Systems() table.Definition {
	return table.Definition{Stage: StageSystems, Contract: "repomap.atlas.systems.v1", System: systemsPrompt, Memoize: true,
		Columns: []table.Column{{Name: SystemColumn, Kind: table.Text, MaxRunes: LabelRunes,
			Note: "the outside system calls through this package reach, as a newcomer would name it, or none"}}}
}

// PackageCall is one symbol of an outside package the program calls, with
// one call of it as the repository wrote it.
type PackageCall struct {
	Symbol string `json:"symbol"`
	Call   string `json:"call,omitempty"`
}

// SystemRow is one outside package: its path as the code names it, the
// dependency lines the manifest records for it ("module version"), and
// every symbol of it the program calls, one call of each.
func SystemRow(id, pkg string, dependency []string, calls []PackageCall) table.Row {
	fields := []table.Field{{Name: "package", Value: pkg}}
	if len(dependency) > 0 {
		fields = append(fields, table.Field{Name: "dependency", Value: dependency})
	}
	fields = append(fields, table.Field{Name: "calls", Value: calls})
	return table.Row{ID: id, Fields: fields}
}

// SystemName reads an accepted system cell: the name as written, or empty
// when the package reaches no outside system.
func SystemName(cell string) string {
	name := strings.TrimSpace(cell)
	if strings.EqualFold(strings.TrimSuffix(name, "."), SystemNone) {
		return ""
	}
	return name
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
// name: its ref, or its name written after the free prefix (name, case
// aside), which chooses the offered program as its ref does: freqtrade's
// run drew "other: Freqtrade" where "freqtrade" was offered. A program
// offered by its import path is named so by the path's last element too
// (pathName), when that names one offered program alone: casdoor's web
// drew "other: Casdoor" for 91 calls where github.com/casdoor/casdoor was
// offered, and its Outside showed the repository's own program as an
// outside system. Empty for a system, an unknown ref or any other name.
func DestinationTarget(catalog []Destination, ref, name string) (string, string) {
	name = strings.TrimSpace(name)
	for _, entry := range catalog {
		if entry.Program && (entry.Ref == ref || name != "" && strings.EqualFold(entry.Value, name)) {
			return entry.Target, entry.Value
		}
	}
	if name == "" {
		return "", ""
	}
	var named *Destination
	for i := range catalog {
		if entry := &catalog[i]; entry.Program && strings.EqualFold(pathName(entry.Value), name) {
			if named != nil {
				return "", ""
			}
			named = entry
		}
	}
	if named == nil {
		return "", ""
	}
	return named.Target, named.Value
}

// pathName is the last element of a program's name written as an import
// path, a major version element aside (github.com/casdoor/casdoor is
// casdoor, go.etcd.io/etcd/server/v3 is server); empty for a name that is
// no path.
func pathName(value string) string {
	elements := strings.Split(strings.Trim(value, "/"), "/")
	if len(elements) < 2 {
		return ""
	}
	last := elements[len(elements)-1]
	if len(elements) > 2 && len(last) > 1 && last[0] == 'v' && strings.Trim(last[1:], "0123456789") == "" {
		last = elements[len(elements)-2]
	}
	return last
}

// Destinations is the closed catalogue of the named packages: one entry per
// system, names equal but for case being one, in name order, refs d1, d2,
// ... It depends only on the names given, so rows of the same programs are
// offered the same catalogue in every window.
func Destinations(names map[string]string) []Destination {
	byName := make(map[string]*Destination)
	for pkg, name := range names {
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		entry := byName[key]
		if entry == nil {
			entry = &Destination{Value: name}
			byName[key] = entry
		}
		// The spelling kept is the least in byte order, whatever map order
		// visits the packages in.
		if name < entry.Value {
			entry.Value = name
		}
		entry.Packages = append(entry.Packages, pkg)
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
