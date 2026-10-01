package report

import (
	"encoding/json"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// settingSets are the fields a setting's branch writes (owner, 2026-09-29:
// a reader looks for slaveof and what it sets, and the setting had read as
// the variables its declaring code uses): the lines the comparison naming
// it guards (the C adapter's pattern Branch, `strcasecmp(argv[0],"slaveof")`
// guarding its block, or a comparison's case: ProgramIndex Comparison), and
// the exact field writes the declaring function
// makes on those lines, in the order they are written, each field once,
// each reading the type declaring it. Exact lines only: without a branch
// the setting names none.
func (builder *pageBuilder) settingSets(index *groupindex.Index, operation groupindex.Operation, decls *pathDecls) string {
	if operation.DeclaredBy == "" || operation.Location.Path == "" {
		return ""
	}
	branch := builder.branchAt(index.Target.ID, operation.DeclaredBy, operation.Location)
	if branch == nil {
		return ""
	}
	var sets []pageDecl
	seen := map[string]bool{}
	for _, access := range builder.fieldFacts(index).byFrom[operation.DeclaredBy] {
		if !access.writes || access.path != operation.Location.Path || access.line < branch.Line || access.line > branch.EndLine || seen[access.label] {
			continue
		}
		seen[access.label] = true
		set := pageDecl{Name: access.label}
		if ref, known := builder.subject(index.Target.ID, access.field); known && ref.subject.Object != nil && ref.subject.Object.OwnerID != "" {
			typed := decls.list[decls.of(ref.subject.Object.OwnerID)]
			set.Href, set.Open, set.Code, set.NoSource, set.Part, set.Source = typed.Href, typed.Open, typed.Code, typed.NoSource, typed.Part, typed.Source
		}
		sets = append(sets, set)
	}
	if len(sets) == 0 {
		return ""
	}
	raw, err := json.Marshal(sets)
	if err != nil {
		return ""
	}
	return string(raw)
}

// branchAt is the branch a declaring function's call at a place guards: the
// one its program's GroupsIndex saved for that function at that place
// (Index.Branches, a comparison's case before a guarding call's pattern).
func (builder *pageBuilder) branchAt(programTargetID, fromID string, at programindex.Location) *programindex.LineRange {
	index := builder.graphIndex(programTargetID)
	if index == nil {
		return nil
	}
	// Saved branches keep a column of at least one, as their places do.
	at.Column = max(1, at.Column)
	for position := range index.Branches {
		if branch := &index.Branches[position]; branch.SubjectID == fromID && branch.Location == at {
			return &branch.Branch
		}
	}
	return nil
}
