package dependencydeclaration

import "encoding/json"

// ScaleWarningKind identifies a former local ceiling that is now only an
// ordinary diagnostic over a complete accepted declaration result.
type ScaleWarningKind string

const (
	ScaleWarningSources         ScaleWarningKind = "sources"
	ScaleWarningPackages        ScaleWarningKind = "packages"
	ScaleWarningStatements      ScaleWarningKind = "statements"
	ScaleWarningIncludes        ScaleWarningKind = "includes"
	ScaleWarningFrontiers       ScaleWarningKind = "frontiers"
	ScaleWarningSourceBytes     ScaleWarningKind = "source_bytes"
	ScaleWarningTotalBytes      ScaleWarningKind = "total_source_bytes"
	ScaleWarningStringBytes     ScaleWarningKind = "string_bytes"
	ScaleWarningStatementExtras ScaleWarningKind = "statement_extras"
	ScaleWarningResultBytes     ScaleWarningKind = "result_bytes"
)

type ScaleWarning struct {
	Kind         ScaleWarningKind
	Retained     int64
	AdvisorySize int64
}

// ScaleWarnings is diagnostic-only. It never validates, mutates, truncates,
// or rejects declaration authority.
func ScaleWarnings(result Result) []ScaleWarning {
	statements := 0
	frontiers := len(result.Frontiers)
	var totalBytes int64
	maxSourceBytes := 0
	maxStringBytes := 0
	maxExtras := 0
	observeString := func(values ...string) {
		for _, value := range values {
			if len(value) > maxStringBytes {
				maxStringBytes = len(value)
			}
		}
	}
	observeString(
		result.TargetID, result.Scope.Language, result.Scope.Ecosystem,
		result.Scope.RepositoryPath,
	)
	for _, source := range result.Sources {
		totalBytes += int64(source.ByteCount)
		if source.ByteCount > maxSourceBytes {
			maxSourceBytes = source.ByteCount
		}
		observeString(source.Path, source.Format)
	}
	for _, pkg := range result.Packages {
		observeString(pkg.Ecosystem, pkg.Name, pkg.NormalizedName)
		observeString(pkg.Names...)
		statements += len(pkg.Statements)
		for _, statement := range pkg.Statements {
			observeString(
				statement.Group, statement.Name, statement.NormalizedName,
				statement.Specifier, statement.Locator.Host,
				statement.Locator.RepositoryPath, statement.Section,
			)
			observeString(statement.Extras...)
			if len(statement.Extras) > maxExtras {
				maxExtras = len(statement.Extras)
			}
		}
	}
	for _, frontier := range result.Frontiers {
		observeString(frontier.Section)
	}

	var warnings []ScaleWarning
	appendIf := func(kind ScaleWarningKind, retained, advisory int64) {
		if retained > advisory {
			warnings = append(warnings, ScaleWarning{Kind: kind, Retained: retained, AdvisorySize: advisory})
		}
	}
	appendIf(ScaleWarningSources, int64(len(result.Sources)), AdvisorySources)
	appendIf(ScaleWarningPackages, int64(len(result.Packages)), AdvisoryPackages)
	appendIf(ScaleWarningStatements, int64(statements), AdvisoryStatements)
	appendIf(ScaleWarningIncludes, int64(len(result.Includes)), AdvisoryIncludes)
	appendIf(ScaleWarningFrontiers, int64(frontiers), AdvisoryFrontiers)
	appendIf(ScaleWarningSourceBytes, int64(maxSourceBytes), AdvisorySourceBytes)
	appendIf(ScaleWarningTotalBytes, totalBytes, AdvisoryTotalBytes)
	appendIf(ScaleWarningStringBytes, int64(maxStringBytes), AdvisoryStringBytes)
	appendIf(ScaleWarningStatementExtras, int64(maxExtras), AdvisoryStatementExtras)
	if encoded, err := json.Marshal(result); err == nil {
		warnings = append(warnings, resultScaleWarningsForBytes(len(encoded))...)
	}
	return warnings
}

func resultScaleWarningsForBytes(retained int) []ScaleWarning {
	if retained <= AdvisoryResultBytes {
		return nil
	}
	return []ScaleWarning{{
		Kind: ScaleWarningResultBytes, Retained: int64(retained),
		AdvisorySize: AdvisoryResultBytes,
	}}
}
