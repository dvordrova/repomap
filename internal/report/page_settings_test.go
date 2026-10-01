package report

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A setting names the fields the branch comparing its key writes: exact
// field writes of the declaring function on the lines the comparison guards
// (the adapter's pattern Branch), each once, in the order written, each
// reading the type declaring it. A write outside the branch, or a read in
// it, is not the setting's; without a branch it names none.
func TestASettingNamesTheFieldsItsBranchWrites(t *testing.T) {
	at := func(line, column int) *programindex.Location {
		return &programindex.Location{Path: "redis.c", Line: line, Column: column}
	}
	builder := &pageBuilder{subjects: map[string]subjectRef{}, links: pageLinks{repositoryURL: "https://github.com/o/r", blobPrefix: "/blob/", revision: "abc"},
		data: &ReportData{ProgramPortfolio: &ProgramPortfolio{}}}
	object := func(id, name string, kind programindex.ObjectKind, owner string, line int) {
		builder.subjects[subjectKey("t1", id)] = subjectRef{programTargetID: "t1", subject: groupindex.Subject{ID: id, Kind: groupindex.SubjectObject,
			Object: &groupindex.ObjectFacts{Name: name, Kind: kind, OwnerID: owner, Location: at(line, 1)}}}
	}
	object("load", "loadServerConfig", programindex.ObjectFunction, "", 1650)
	object("server", "redisServer", programindex.ObjectType, "", 300)
	object("masterhost", "masterhost", programindex.ObjectVariable, "server", 320)
	object("masterport", "masterport", programindex.ObjectVariable, "server", 321)
	object("port", "port", programindex.ObjectVariable, "server", 310)
	write := func(field, path string, line int, kind programindex.RelationKind) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: "load", ToSubjectID: field, Role: groupindex.EdgeRelationTarget, RelationKind: kind,
			Resolution: programindex.ResolutionExact, Location: at(line, 9), FieldPath: path}
	}
	index := groupindex.Index{Target: programindex.Target{ID: "t1"}, StructuralEdges: []groupindex.StructuralEdge{
		write("port", "server.port", 1701, programindex.RelationWrites),
		write("masterport", "server.masterport", 1710, programindex.RelationWrites),
		write("masterhost", "server.masterhost", 1709, programindex.RelationWrites),
		write("masterhost", "server.masterhost", 1711, programindex.RelationWrites),
		write("port", "server.port", 1711, programindex.RelationReads),
	}}
	// The branch the comparison guards, as the GroupsIndex saves it.
	comparison := at(1708, 21)
	index.Branches = []groupindex.InputBranch{{SubjectID: "load", Location: *comparison, Branch: programindex.LineRange{Line: 1708, EndLine: 1712}}}
	builder.indexes = []groupindex.Index{index}
	decls := builder.pathDecls("t1", func(id string) string {
		if id == "server" {
			return "t1-core"
		}
		return ""
	})
	raw := builder.settingSets(&index, groupindex.Operation{DeclaredBy: "load", Location: *comparison}, decls)
	var sets []pageDecl
	if err := json.Unmarshal([]byte(raw), &sets); err != nil {
		t.Fatalf("sets %q: %v", raw, err)
	}
	var said []string
	for _, set := range sets {
		said = append(said, set.Name+" → "+set.Part)
	}
	if want := []string{"server.masterhost → t1-core", "server.masterport → t1-core"}; !slices.Equal(said, want) {
		t.Fatalf("the setting writes %q, want %q", said, want)
	}
	if raw := builder.settingSets(&index, groupindex.Operation{DeclaredBy: "load", Location: *at(1700, 21)}, decls); raw != "" {
		t.Fatalf("a comparison with no branch names %s", raw)
	}
}
