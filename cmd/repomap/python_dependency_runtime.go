package main

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/dependencies"
)

func pythonDependencyCoverageError(catalog dependencies.Catalog) error {
	if catalog.Coverage.State == dependencies.CoverageComplete {
		return nil
	}
	detail := "unclassified direct import"
	if len(catalog.Coverage.Omissions) > 0 {
		first := catalog.Coverage.Omissions[0]
		detail = fmt.Sprintf("%s for %s", first.Reason, first.PackagePath)
		if len(catalog.Coverage.Omissions) > 1 {
			detail += fmt.Sprintf(" and %d more", len(catalog.Coverage.Omissions)-1)
		}
	}
	return fmt.Errorf(
		"Python dependency authority is incomplete: %s; replace or remove the unresolved import before analysis",
		detail,
	)
}
