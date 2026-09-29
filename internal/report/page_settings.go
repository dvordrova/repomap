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

// branchAt is the branch a declaring function's call at a place guards, from
// its program's index: the pattern written exactly there, or the case of one
// of the function's comparisons whose first word is written there (a
// switch's case body, the block an if/elif condition guards).
func (builder *pageBuilder) branchAt(programTargetID, fromID string, at programindex.Location) *programindex.LineRange {
	if builder.data == nil || builder.data.ProgramPortfolio == nil {
		return nil
	}
	for _, entry := range builder.data.ProgramPortfolio.Entries {
		if entry.Target.ID != programTargetID {
			continue
		}
		for _, object := range entry.Objects {
			if object.ID != fromID {
				continue
			}
			for _, comparison := range object.Comparisons {
				for _, item := range comparison.Cases {
					if item.Branch != nil && item.Location != nil && *item.Location == at {
						return item.Branch
					}
				}
			}
		}
		for _, relation := range entry.Relations {
			if relation.FromID != fromID {
				continue
			}
			for _, pattern := range relation.Patterns {
				if pattern.Branch != nil && pattern.Location != nil && *pattern.Location == at {
					return pattern.Branch
				}
			}
		}
	}
	return nil
}
