package groupindex

import (
	"github.com/dvordrova/repomap/internal/programindex"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestOwnUseReaderPreservesTheOriginalSelectionForEveryQuery(t *testing.T) {
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "app.ts", Line: line, Column: 2}
	}
	object := func(id, name string, kind programindex.ObjectKind, line int) programindex.Object {
		return programindex.Object{ID: id, Name: name, Kind: kind, Location: at(line)}
	}
	program := programindex.Index{Objects: []programindex.Object{
		object("a", "handler", programindex.ObjectFunction, 1), object("b", "handler", programindex.ObjectFunction, 2), object("c", "handler", programindex.ObjectFunction, 3),
		object("owner", "OldOwner", programindex.ObjectType, 4), object("owner", "*FinalOwner", programindex.ObjectType, 4),
		{ID: "method", Name: "wrong", Kind: programindex.ObjectMethod, OwnerID: "owner", Location: at(5)},
		{ID: "method", Name: "Run", Kind: programindex.ObjectMethod, OwnerID: "owner", Location: at(5)},
		object("go", "goToLink", programindex.ObjectVariable, 6),
		literal(object("inline", "inline", programindex.ObjectFunction, 7)),
		{ID: "no-location", Name: "unlocated", Kind: programindex.ObjectFunction},
		{ID: "platform", External: &programindex.ExternalSymbol{Name: "URL", AuthorityKind: programindex.ExternalAuthorityPlatform}},
		{ID: "receiver", External: &programindex.ExternalSymbol{Name: "GetCertificate", Receiver: "*Config"}},
		{ID: "package", External: &programindex.ExternalSymbol{Name: "Error", PackagePath: "google.golang.org/grpc/status"}},
		{ID: "invalid", External: &programindex.ExternalSymbol{Name: "invalid word"}},
	}}
	relation := func(from string, kind programindex.RelationKind, line int, to ...string) programindex.Relation {
		return programindex.Relation{FromID: from, Kind: kind, ToIDs: to, Location: at(line)}
	}
	program.Relations = []programindex.Relation{
		relation("a", programindex.RelationInvokesExternal, 1, "platform"),
		relation("b", programindex.RelationCalls, 1, "platform"),
		relation("a", programindex.RelationCalls, 15, "package", "receiver"),
		relation("a", programindex.RelationReads, 9, "inline", "no-location", "missing", "invalid", "a", "method", "method"),
		relation("a", programindex.RelationCalls, 9, "go"),
		relation("b", programindex.RelationReads, 12, "go"),
		relation("c", programindex.RelationCalls, 1, "platform"),
		relation("c", programindex.RelationCalls, 15, "receiver"),
		relation("b", programindex.RelationWrites, 1, "method"),
		{FromID: "c", Kind: programindex.RelationCalls, ToIDs: []string{"method"}},
		relation("unrelated", programindex.RelationCalls, 1, "method"),
	}
	reader := NewOwnUseReader(program)
	want := []OwnUse{{Word: "FinalOwner.Run", Reads: true, At: *at(9)}, {}}
	if got := reader.OwnUses([]string{"a", "b"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("original source/word rules: got %+v, want %+v", got, want)
	}
	want = []OwnUse{{Word: "goToLink", Reads: true, At: *at(12)}, {Word: "Config.GetCertificate", At: *at(15)}}
	if got := reader.OwnUses([]string{"b", "c"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("query-local unique users changed: got %+v, want %+v", got, want)
	}
	queries := 0
	var visit func([]string)
	visit = func(ids []string) {
		queries++
		if got, want := reader.OwnUses(ids), originalOwnUses(program, ids); !reflect.DeepEqual(got, want) {
			t.Fatalf("query %v: got %+v, old %+v", ids, got, want)
		}
		if len(ids) < 4 {
			for _, id := range []string{"a", "b", "c", "missing"} {
				visit(append(append([]string(nil), ids...), id))
			}
		}
	}
	visit(nil)
	if queries != 341 {
		t.Fatalf("incomplete ordered query inventory: %d", queries)
	}
}

// Verbatim pre-optimization decision path, retained as an independent oracle.
func originalOwnUses(program programindex.Index, ids []string) []OwnUse {
	byID := make(map[string]programindex.Object, len(program.Objects))
	for _, object := range program.Objects {
		byID[object.ID] = object
	}
	named := func(object programindex.Object) string {
		if owner, ok := byID[object.OwnerID]; ok && object.Kind == programindex.ObjectMethod && owner.Kind == programindex.ObjectType && owner.Name != "" && !strings.Contains(object.Name, ".") {
			return strings.TrimPrefix(owner.Name, "*") + "." + object.Name
		}
		return object.Name
	}
	return originalOwnUsesDecision(program, ids, byID, named, writtenInline)
}

func originalOwnUsesDecision(program programindex.Index, ids []string, byID map[string]programindex.Object, named func(programindex.Object) string, inline func(programindex.Object) bool) []OwnUse {
	type use struct {
		to    string
		reads bool
		at    programindex.Location
	}
	position := make(map[string]int, len(ids))
	for i, id := range ids {
		position[id] = i
	}
	uses := make([][]use, len(ids))
	for _, relation := range program.Relations {
		at, ours := position[relation.FromID]
		if !ours || relation.Location == nil {
			continue
		}
		reads := relation.Kind == programindex.RelationReads
		if !reads && relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationInvokesExternal {
			continue
		}
		for _, to := range relation.ToIDs {
			if to != relation.FromID {
				uses[at] = append(uses[at], use{to: to, reads: reads, at: *relation.Location})
			}
		}
	}
	users := map[string]map[int]bool{}
	for at, list := range uses {
		for _, used := range list {
			if users[used.to] == nil {
				users[used.to] = map[int]bool{}
			}
			users[used.to][at] = true
		}
	}
	// An outside callee reads by its own name: a method with its type
	// (Config.GetCertificate), the platform's function alone (URL,
	// ListenAndServe), a package's function with the package's last element
	// (status.Error).
	word := func(object programindex.Object) string {
		if external := object.External; external != nil {
			if receiver := strings.TrimPrefix(external.Receiver, "*"); receiver != "" {
				return receiver + "." + external.Name
			}
			if external.PackagePath != "" && external.AuthorityKind != programindex.ExternalAuthorityPlatform {
				return path.Base(external.PackagePath) + "." + external.Name
			}
			return external.Name
		}
		return named(object)
	}
	result := make([]OwnUse, len(ids))
	for at, list := range uses {
		sort.SliceStable(list, func(i, j int) bool {
			a, b := list[i].at, list[j].at
			return a.Path < b.Path || a.Path == b.Path && (a.Line < b.Line || a.Line == b.Line && a.Column < b.Column)
		})
		for _, outside := range []bool{false, true} {
			for _, used := range list {
				object, known := byID[used.to]
				if !known || len(users[used.to]) != 1 || (object.External != nil) != outside || !outside && (object.Location == nil || inline(object)) {
					continue
				}
				if said := word(object); said != "" && !strings.ContainsAny(said, " \t\r\n()") && validText(said) {
					result[at] = OwnUse{Word: said, Reads: used.reads, At: used.at}
					break
				}
			}
			if result[at].Word != "" {
				break
			}
		}
	}
	return result
}
