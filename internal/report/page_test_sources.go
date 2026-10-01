package report

import (
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// overviewBuilder removes known testing material only from the orientation
// view. The saved graph, full ProgramIndex, question evidence and source checks
// retain every original declaration and connection.
func (builder *pageBuilder) overviewBuilder() *pageBuilder {
	view := *builder
	view.sourceIndexes = builder.indexes
	var programs []programindex.Index
	if builder.data != nil && builder.data.ProgramPortfolio != nil {
		programs = builder.data.ProgramPortfolio.Entries
	}
	view.testPaths = groupindex.TestPaths(builder.indexes, programs)
	// The test-free views and what they derive were computed once at
	// analysis (groupindex.WithTestFreeViews): the page applies them and
	// never derives.
	view.indexes = groupindex.TestFreeViews(builder.indexes, view.testPaths)
	for i := range view.indexes {
		if saved := builder.indexes[i].TestFree; saved != nil {
			if err := view.indexes[i].ApplyDerived(*saved); err == nil {
				continue
			}
		}
		// A view without its saved derivations has none: each input
		// reaches nothing the page could show, and the page computes
		// nothing.
		reach := make([]groupindex.Reach, len(view.indexes[i].Operations))
		for position, operation := range view.indexes[i].Operations {
			reach[position].OperationID = operation.ID
		}
		view.indexes[i].Reach, view.indexes[i].Dispatch, view.indexes[i].Entries, view.indexes[i].Catalogues, view.indexes[i].Launch = reach, nil, nil, nil, groupindex.Launch{}
	}
	return &view
}
