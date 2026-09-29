package report

import "slices"

// libraryFacet is the program's library facet (pageSection Libraries): the
// import names of a library of the same manifest the portfolio folded into
// it, as its ProgramTarget records them.
func (builder *pageBuilder) libraryFacet(programTargetID string) []string {
	if builder.data == nil || builder.data.ProgramPortfolio == nil {
		return nil
	}
	for _, entry := range builder.data.ProgramPortfolio.Entries {
		if entry.Target.ID == programTargetID {
			return slices.Clone(entry.Target.Libraries)
		}
	}
	return nil
}
