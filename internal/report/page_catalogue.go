package report

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pageCatalogue is one catalogue of inputs whose handler is not established
// (GroupsIndex Catalogue), as each member's reading shows it: where they are
// declared (Declarer, or On, the object they are declared on), its members'
// nodes by name, where the declaring code is called from, and the variables
// it also uses with every other declaration using them. It is one JSON
// string shared by its members. Code lists; where these inputs take effect
// is not established, and the reading says so once.
type pageCatalogue struct {
	Declarer int  `json:"declarer"`
	On       *int `json:"on,omitempty"`
	// OnInput is the input declared at the call that made the object, and
	// OnHandler its handler when established (the members' L2).
	OnInput   string              `json:"on_input,omitempty"`
	OnHandler *int                `json:"on_handler,omitempty"`
	Kind      string              `json:"kind"`
	Members   []string            `json:"members"`
	Calls     []pageCatalogueCall `json:"calls,omitempty"`
	Uses      []pageCatalogueUse  `json:"uses,omitempty"`
	// Table says the declarer is a table the inputs are rows of, and
	// Readers the functions that look them up in it.
	Table   bool                  `json:"table,omitempty"`
	Readers []pageCatalogueReader `json:"readers,omitempty"`
	Decls   []pageDecl            `json:"decls"`
}

// pageCatalogueCall is one caller of the declaring code, named once however
// many places it calls from (owner, 2026-09-29: no line numbers; the name
// reads its code), possible only when every one of its calls is.
type pageCatalogueCall struct {
	Caller   int  `json:"caller"`
	Possible bool `json:"possible,omitempty"`
}

// pageCatalogueReader is one function reading a table of inputs, with the
// calls into it.
type pageCatalogueReader struct {
	Reader int                 `json:"reader"`
	Calls  []pageCatalogueCall `json:"calls,omitempty"`
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
func (builder *pageBuilder) catalogueReadings(index *groupindex.Index, partOf func(string) string, inputNode func(string) string) (map[string]string, map[string]int, map[string]string, map[string][]string) {
	reading := map[string]string{}
	declares := map[string][]string{}
	order := map[string]int{}
	declaredBy := map[string]string{}
	at := 0
	names := map[string]string{}
	for _, operation := range index.Operations {
		names[operation.ID] = builder.operationDisplayName(index.Target.ID, operation)
	}
	for _, catalogue := range index.Catalogues {
		decls := builder.pathDecls(index.Target.ID, partOf)
		result := pageCatalogue{Kind: catalogue.Kind, Declarer: -1}
		name := ""
		if catalogue.DeclaredBy != "" {
			result.Declarer = decls.of(catalogue.DeclaredBy)
			name = decls.list[result.Declarer].Name
		}
		if on := catalogue.On; on != nil {
			text := on.Text
			if text == "" {
				text = fmt.Sprintf("%s:%d", on.Location.Path, on.Location.Line)
			}
			anchor := builder.links.anchor(on.Location.Path, on.Location.Line, on.Location.Column)
			at := decls.add("on "+operationLocationKey(on.Location), pageDecl{Name: text, Href: anchor.Href, Open: anchor.Open, Source: anchor.Text, NoSource: anchor.NoSource})
			result.On = &at
			if catalogue.OnOperationID != "" {
				result.OnInput = inputNode(catalogue.OnOperationID)
				for _, id := range catalogue.OperationIDs {
					declares[catalogue.OnOperationID] = append(declares[catalogue.OnOperationID], inputNode(id))
				}
				for _, operation := range index.Operations {
					if operation.ID == catalogue.OnOperationID && operation.SubjectID != "" {
						handler := decls.of(operation.SubjectID)
						result.OnHandler = &handler
					}
				}
			}
			if name == "" {
				name = text
			}
		}
		// Its members by name, whatever their case, as a reader looks for
		// one ("Человек нормально ищет по алфавиту", owner 2026-09-28).
		members := slices.Clone(catalogue.OperationIDs)
		slices.SortStableFunc(members, func(a, b string) int {
			return cmp.Or(strings.Compare(strings.ToLower(names[a]), strings.ToLower(names[b])), strings.Compare(names[a], names[b]))
		})
		for _, id := range members {
			result.Members = append(result.Members, inputNode(id))
		}
		callers := func(positions []int) []pageCatalogueCall {
			var calls []pageCatalogueCall
			for _, position := range positions {
				edge := index.StructuralEdges[position]
				call := pageCatalogueCall{Caller: decls.of(edge.FromSubjectID), Possible: edge.Resolution != programindex.ResolutionExact}
				if at := slices.IndexFunc(calls, func(listed pageCatalogueCall) bool { return listed.Caller == call.Caller }); at >= 0 {
					calls[at].Possible = calls[at].Possible && call.Possible
					continue
				}
				calls = append(calls, call)
			}
			return calls
		}
		result.Calls = callers(catalogue.Calls)
		for _, reader := range catalogue.Readers {
			result.Table = true
			result.Readers = append(result.Readers, pageCatalogueReader{Reader: decls.of(reader.SubjectID), Calls: callers(reader.Calls)})
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
	return reading, order, declaredBy, declares
}

// pageTakenIn is one place an input whose handler is not established is
// taken in: the part, the function there, and how ("declared in",
// "looked up in").
type pageTakenIn struct{ part, subject, label string }

// takenInPlaces are, by operation ID, the parts the code taking in each
// handler-less input stands in, from saved DeclaredBy and catalogue data:
// an option or word is taken in by the function declaring it, a table's row
// by every function reading the table (a table with no reader is taken in
// nowhere drawn). The arrow into such a part says "taken in here", never
// "implemented in": where the input takes effect stays not established, and
// neither the declaring function nor a reader becomes its handler, its reach
// or its phase. No part is chosen among several: each is drawn.
func (builder *pageBuilder) takenInPlaces(index *groupindex.Index, partOf func(string) string) map[string][]pageTakenIn {
	catalogueOf := map[string]int{}
	for position, catalogue := range index.Catalogues {
		for _, id := range catalogue.OperationIDs {
			catalogueOf[id] = position
		}
	}
	result := map[string][]pageTakenIn{}
	for _, operation := range index.Operations {
		if !operation.HandlerUnknown {
			continue
		}
		var places []pageTakenIn
		add := func(subject, label string) {
			if part := partOf(subject); part != "" {
				places = append(places, pageTakenIn{part: part, subject: subject, label: label})
			}
		}
		position, catalogued := catalogueOf[operation.ID]
		switch {
		case catalogued && len(index.Catalogues[position].Readers) > 0:
			for _, reader := range index.Catalogues[position].Readers {
				add(reader.SubjectID, "looked up in")
			}
		case operation.DeclaredBy != "":
			if ref, known := builder.subject(index.Target.ID, operation.DeclaredBy); known && ref.subject.Object != nil && ref.subject.Object.Kind == programindex.ObjectVariable {
				continue
			}
			add(operation.DeclaredBy, "declared in")
		}
		if len(places) > 0 {
			result[operation.ID] = places
		}
	}
	return result
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
	if catalogue.OnInput != "" {
		catalogue.OnInput = rename(catalogue.OnInput)
	}
	encoded, err := json.Marshal(catalogue)
	if err != nil {
		return raw
	}
	return string(encoded)
}
