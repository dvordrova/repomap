package report

import (
	"encoding/json"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pageCatalogue is one catalogue of inputs whose handler is not established
// (GroupsIndex Catalogue), as each member's reading shows it: where they are
// declared (Declarer, or On, the object they are declared on), its members'
// nodes in source order, where the declaring code is called from, and the
// variables it also uses with every other declaration using them. It is one
// JSON string shared by its members. Code lists; where these inputs take
// effect is not established, and the reading says so once.
type pageCatalogue struct {
	Declarer int                 `json:"declarer"`
	On       *int                `json:"on,omitempty"`
	Kind     string              `json:"kind"`
	Members  []string            `json:"members"`
	Calls    []pageCatalogueCall `json:"calls,omitempty"`
	Uses     []pageCatalogueUse  `json:"uses,omitempty"`
	Decls    []pageDecl          `json:"decls"`
}

// pageCatalogueCall is one call into the declaring code: its caller and the
// call's own source line, a link to it.
type pageCatalogueCall struct {
	Caller   int    `json:"caller"`
	Line     int    `json:"line,omitempty"`
	Href     string `json:"href,omitempty"`
	Open     string `json:"open,omitempty"`
	Possible bool   `json:"possible,omitempty"`
}

// pageCatalogueUse is one variable the declaring code reads, with every
// other function reading it; Parts counts the parts they stand in.
type pageCatalogueUse struct {
	Decl  int   `json:"decl"`
	Users []int `json:"users,omitempty"`
	Parts int   `json:"parts,omitempty"`
}

// catalogueReadings builds each catalogue's shared JSON, by operation ID,
// and each operation's position in catalogue order.
func (builder *pageBuilder) catalogueReadings(index *groupindex.Index, partOf func(string) string, inputNode func(string) string) (map[string]string, map[string]int, map[string]string) {
	reading := map[string]string{}
	order := map[string]int{}
	declaredBy := map[string]string{}
	at := 0
	for _, catalogue := range index.Catalogues {
		decls := builder.pathDecls(index.Target.ID, partOf)
		result := pageCatalogue{Kind: catalogue.Kind, Declarer: -1}
		name := ""
		if catalogue.DeclaredBy != "" {
			result.Declarer = decls.of(catalogue.DeclaredBy)
			name = decls.list[result.Declarer].Name
		}
		if catalogue.DeclaredOn != "" {
			on := decls.of(catalogue.DeclaredOn)
			result.On = &on
			if name == "" {
				name = decls.list[on].Name
			}
		}
		for _, id := range catalogue.OperationIDs {
			result.Members = append(result.Members, inputNode(id))
		}
		for _, position := range catalogue.Calls {
			edge := index.StructuralEdges[position]
			call := pageCatalogueCall{Caller: decls.of(edge.FromSubjectID), Possible: edge.Resolution != programindex.ResolutionExact}
			if edge.Location != nil {
				anchor := builder.links.anchor(edge.Location.Path, edge.Location.Line, edge.Location.Column)
				call.Line, call.Href, call.Open = edge.Location.Line, anchor.Href, anchor.Open
			}
			result.Calls = append(result.Calls, call)
		}
		for _, use := range catalogue.Uses {
			row := pageCatalogueUse{Decl: decls.of(use.SubjectID)}
			parts := map[string]bool{}
			for _, user := range use.Users {
				position := decls.of(user)
				row.Users = append(row.Users, position)
				if part := decls.list[position].Part; part != "" {
					parts[part] = true
				}
			}
			row.Parts = len(parts)
			result.Uses = append(result.Uses, row)
		}
		result.Decls = decls.list
		raw, err := json.Marshal(result)
		if err != nil {
			continue
		}
		for _, id := range catalogue.OperationIDs {
			reading[id] = string(raw)
			order[id] = at
			declaredBy[id] = name
			at++
		}
	}
	return reading, order, declaredBy
}

// remapCatalogue renames the parts and input nodes a catalogue reading
// names, as remapInputPath does for an input's path.
func remapCatalogue(raw string, rename func(string) string) string {
	if raw == "" {
		return raw
	}
	var catalogue pageCatalogue
	if json.Unmarshal([]byte(raw), &catalogue) != nil {
		return raw
	}
	for i := range catalogue.Decls {
		if catalogue.Decls[i].Part != "" {
			catalogue.Decls[i].Part = rename(catalogue.Decls[i].Part)
		}
	}
	for i := range catalogue.Members {
		catalogue.Members[i] = rename(catalogue.Members[i])
	}
	encoded, err := json.Marshal(catalogue)
	if err != nil {
		return raw
	}
	return string(encoded)
}
