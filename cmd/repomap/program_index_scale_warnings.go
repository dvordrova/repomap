package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/gotarget"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

func reportCorpusScaleWarnings(output *runOutput, repository *corpus.Corpus) {
	if output == nil || repository == nil {
		return
	}
	warnings := repository.ScaleWarnings()
	if len(warnings) == 0 {
		return
	}
	details := []string{"all tracked paths, snapshot identity, and complete source reads were retained"}
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: affected collections %d; largest retained %d; usual size %d",
			warning.Kind, warning.AffectedCollections, warning.MaximumRetained, warning.AdvisorySize,
		))
	}
	output.Warn("Large repository corpus retained", details...)
}

func reportRepositorySnapshotScaleWarnings(output *runOutput, value snapshot.Snapshot) {
	warnings := snapshot.RepositoryScaleWarnings(value)
	if output == nil || len(warnings) == 0 {
		return
	}
	details := []string{"all repository identity and Go target advisory evidence was retained"}
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: retained %d; usual size %d", warning.Kind, warning.Retained, warning.AdvisorySize,
		))
	}
	output.Warn("Large repository preflight evidence retained", details...)
}

func reportJSTSProjectScaleWarnings(
	output *runOutput,
	target programindex.Target,
	result jstsproject.Result,
) {
	warnings := jstsproject.ScaleWarnings(result)
	if output == nil || len(warnings) == 0 {
		return
	}
	details := targetScaleWarningDetails(
		target,
		"the complete compiler-backed project authority was retained",
	)
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: retained %d bytes; usual size %d bytes",
			warning.Kind, warning.Retained, warning.AdvisorySize,
		))
	}
	output.Warn("Large JavaScript/TypeScript project retained", details...)
}

func reportPythonTargetCatalogScaleWarnings(output *runOutput, catalog pythontarget.Catalog) {
	warnings := pythontarget.ScaleWarnings(catalog)
	if output == nil || len(warnings) == 0 {
		return
	}
	details := []string{"the complete discovered Python target catalog was retained"}
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: retained %d bytes; usual size %d bytes",
			warning.Kind, warning.Retained, warning.AdvisorySize,
		))
	}
	output.Warn("Large Python target catalog retained", details...)
}

func reportDependencyCatalogScaleWarnings(
	output *runOutput,
	target programindex.Target,
	catalog dependencies.Catalog,
) {
	reportDependencyCatalogScaleWarningsForTarget(
		output, semanticWarningTargetDetail(target), catalog,
	)
}

func reportDependencyCatalogScaleWarningsForTarget(
	output *runOutput,
	targetDetail string,
	catalog dependencies.Catalog,
) {
	warnings := dependencies.ScaleWarnings(catalog)
	if output == nil || len(warnings) == 0 {
		return
	}
	details := []string{"the complete validated dependency catalog was retained"}
	if targetDetail = strings.TrimSpace(targetDetail); targetDetail != "" {
		if !strings.HasPrefix(targetDetail, "target: ") {
			targetDetail = "target: " + targetDetail
		}
		details = append(details, targetDetail)
	}
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: retained %d bytes; usual size %d bytes",
			warning.Kind, warning.Retained, warning.AdvisorySize,
		))
	}
	output.Warn("Large dependency catalog retained", details...)
}

func reportGoFactScaleWarnings(output *runOutput, facts *gofacts.Facts) {
	warnings := gofacts.ScaleWarnings(facts)
	if output == nil || len(warnings) == 0 {
		return
	}
	details := []string{"all locally observed Go facts were retained; no usual-size threshold truncated the data"}
	details = append(details, warnings...)
	output.Warn("Large Go source facts retained", details...)
}

func reportDirectCallIndexScaleWarnings(
	output *runOutput,
	target string,
	index surfacediscovery.DirectCallIndex,
) {
	warnings := surfacediscovery.DirectCallScaleWarnings(index)
	if output == nil || len(warnings) == 0 {
		return
	}
	emitDirectCallIndexScaleWarnings(output, target, warnings)
}

func reportPackageDiagnosticScaleWarnings(
	output *runOutput,
	target string,
	coverage surfacediscovery.ProgramCoverage,
) {
	warnings := surfacediscovery.PackageDiagnosticScaleWarnings(coverage)
	if output == nil || len(warnings) == 0 {
		return
	}
	details := []string{
		"target: " + target,
		"all package diagnostics and complete normalized messages were retained",
	}
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: retained %d; usual size %d",
			warning.Kind, warning.Retained, warning.AdvisorySize,
		))
	}
	output.Warn("Large Go package diagnostics retained", details...)
}

func emitDirectCallIndexScaleWarnings(
	output *runOutput,
	target string,
	warnings []surfacediscovery.DirectCallScaleWarning,
) {
	if output == nil || len(warnings) == 0 {
		return
	}
	details := []string{
		"target: " + target,
		"all rows admitted by the selected controls retained; no implicit graph-size truncation was applied",
	}
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: retained %d; usual size %d",
			warning.Kind,
			warning.Retained,
			warning.AdvisorySize,
		))
	}
	output.Warn("Large Go call graph retained", details...)
}

func reportProgramIndexScaleWarnings(output *runOutput, indexes []programindex.Index) {
	for _, index := range indexes {
		warnings := programindex.ScaleWarnings(index)
		if len(warnings) == 0 {
			continue
		}
		emitProgramIndexScaleWarnings(output, index, warnings)
	}
}

func reportProgramIndexSetScaleWarnings(
	output *runOutput,
	set programindex.ArtifactSet,
	indexes []programindex.Index,
) {
	warnings := programindex.ArtifactSetScaleWarnings(set)
	if output == nil || len(warnings) == 0 {
		return
	}
	details := []string{"all selected target/index bindings were retained in the sealed inventory"}
	for _, index := range indexes {
		if detail := semanticWarningTargetDetail(index.Target); detail != "" {
			details = append(details, detail)
		}
	}
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: retained %d; usual size %d", warning.Kind, warning.Retained, warning.AdvisorySize,
		))
	}
	output.Warn("Large ProgramIndex inventory retained", details...)
}

func emitProgramIndexScaleWarnings(
	output *runOutput,
	index programindex.Index,
	warnings []programindex.ScaleWarning,
) {
	if output == nil || len(warnings) == 0 {
		return
	}
	details := targetScaleWarningDetails(
		index.Target,
		"all rows retained; no local truncation was applied",
	)
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: affected collections %d; largest retained %d; usual size %d",
			warning.Kind,
			warning.AffectedCollections,
			warning.MaximumRetained,
			warning.AdvisorySize,
		))
	}
	output.Warn("Large ProgramIndex collections retained", details...)
}

func reportProgramViewScaleWarnings(output *runOutput, indexes []programindex.Index) {
	if output == nil {
		return
	}
	for _, index := range indexes {
		view, err := report.NewProgramView(index)
		if err != nil {
			// Diagnostics cannot introduce another validation or publication path.
			continue
		}
		warnings := report.ProgramViewScaleWarnings(*view)
		if len(warnings) == 0 {
			continue
		}
		emitProgramViewScaleWarnings(output, index, warnings)
	}
}

func reportProgramPagePortfolioScaleWarnings(output *runOutput, portfolio programpage.Portfolio) {
	warnings := programpage.ScaleWarnings(portfolio)
	if output == nil || len(warnings) == 0 {
		return
	}
	details := []string{"all completed target pages were retained in the neutral page portfolio"}
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: retained %d; usual size %d", warning.Kind, warning.Retained, warning.AdvisorySize,
		))
	}
	output.Warn("Large program-page portfolio retained", details...)
}

func reportTargetOutcomeScaleWarnings(output *runOutput, portfolio targetoutcome.Portfolio) {
	warnings := targetoutcome.ScaleWarnings(portfolio)
	filtered := warnings[:0]
	for _, warning := range warnings {
		if warning.Kind != targetoutcome.ScaleWarningAllowedLanguages {
			filtered = append(filtered, warning)
		}
	}
	warnings = filtered
	if output == nil || len(warnings) == 0 {
		return
	}
	details := []string{"every selected target outcome and its complete language authority were retained"}
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: affected collections %d; largest retained %d; usual size %d",
			warning.Kind, warning.AffectedCollections, warning.MaximumRetained, warning.AdvisorySize,
		))
	}
	output.Warn("Large target-outcome portfolio retained", details...)
}

func reportSelectedTargetOutcomeScaleWarnings(
	output *runOutput,
	targets []targetoutcome.SelectedTarget,
) {
	warnings := targetoutcome.SelectedTargetScaleWarnings(targets)
	if output == nil || len(warnings) == 0 {
		return
	}
	details := []string{"every selected target's complete allowed-language authority was retained before analysis"}
	affectedTargets := make([]string, 0)
	for _, target := range targets {
		if len(target.AllowedProgramLanguages) <= targetoutcome.MaxAllowedProgramLanguages {
			continue
		}
		affectedTargets = append(affectedTargets, fmt.Sprintf(
			"target: selected_id=%q; language_group=%q; name=%q; selector=%q",
			target.ID, target.LanguageGroup, target.DisplayName, target.Selector,
		))
	}
	sort.Strings(affectedTargets)
	details = append(details, affectedTargets...)
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: affected collections %d; largest retained %d; usual size %d",
			warning.Kind, warning.AffectedCollections, warning.MaximumRetained, warning.AdvisorySize,
		))
	}
	output.Warn("Large selected-target language authority retained", details...)
}

func reportGoBuildTagScaleWarnings(output *runOutput, tags []string) {
	warnings := gotarget.ScaleWarnings(tags)
	if output == nil || len(warnings) == 0 {
		return
	}
	details := []string{"all explicitly selected Go build tags were retained"}
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: retained %d; usual size %d", warning.Kind, warning.Retained, warning.AdvisorySize,
		))
	}
	output.Warn("Large Go build-tag set retained", details...)
}

// targetReportScaleWarningOutputKey is console-only de-duplication identity.
// ProgramTargetID prevents one target's early report-view warning from
// suppressing the same kind for a sibling target that shares the runOutput.
type targetReportScaleWarningOutputKey struct {
	programTargetID string
	kind            string
	advisory        int
}

func claimTargetReportScaleWarningOutput(
	output *runOutput,
	target programindex.Target,
	warning report.ReportInputScaleWarning,
) bool {
	if output == nil || target.ID == "" {
		return true
	}
	key := targetReportScaleWarningOutputKey{
		programTargetID: target.ID,
		kind:            warning.Kind,
		advisory:        warning.AdvisorySize,
	}
	output.mu.Lock()
	defer output.mu.Unlock()
	if output.reportedTargetReportWarnings == nil {
		output.reportedTargetReportWarnings = make(map[targetReportScaleWarningOutputKey]struct{})
	}
	if _, duplicate := output.reportedTargetReportWarnings[key]; duplicate {
		return false
	}
	output.reportedTargetReportWarnings[key] = struct{}{}
	return true
}

// reportSemanticViewScaleWarning measures a validated, complete report view
// while it is still resident at the semantic Accepted boundary. JSON encoding
// is best-effort diagnostic work: an encoding failure cannot add a new
// semantic or publication error path.
func reportSemanticViewScaleWarning(
	output *runOutput,
	target programindex.Target,
	kind string,
	view any,
	advisory int,
) {
	encoded, err := json.Marshal(view)
	if err != nil || len(encoded) <= advisory {
		return
	}
	reportInputScaleWarnings(output, []report.ReportInputScaleWarning{{
		Kind: kind, Retained: len(encoded), AdvisorySize: advisory,
	}}, target)
}

func reportInputScaleWarnings(
	output *runOutput,
	warnings []report.ReportInputScaleWarning,
	targets ...programindex.Target,
) {
	if output == nil || len(warnings) == 0 {
		return
	}
	type warningKey struct {
		kind     string
		advisory int
	}
	type aggregate struct {
		warning report.ReportInputScaleWarning
	}
	positions := make(map[warningKey]int, len(warnings))
	aggregates := make([]aggregate, 0, len(warnings))
	for _, warning := range warnings {
		key := warningKey{kind: warning.Kind, advisory: warning.AdvisorySize}
		if position, found := positions[key]; found {
			if warning.Retained > aggregates[position].warning.Retained {
				aggregates[position].warning.Retained = warning.Retained
			}
			continue
		}
		positions[key] = len(aggregates)
		aggregates = append(aggregates, aggregate{warning: warning})
	}
	if len(targets) == 1 && targets[0].ID != "" {
		fresh := aggregates[:0]
		for _, aggregate := range aggregates {
			if !claimTargetReportScaleWarningOutput(output, targets[0], aggregate.warning) {
				continue
			}
			fresh = append(fresh, aggregate)
		}
		aggregates = fresh
		if len(aggregates) == 0 {
			return
		}
	}
	details := []string{"complete report handoff authority was retained"}
	for _, target := range targets {
		if detail := semanticWarningTargetDetail(target); detail != "" {
			details = append(details, detail)
		}
	}
	for _, aggregate := range aggregates {
		warning := aggregate.warning
		details = append(details, fmt.Sprintf(
			"%s: largest retained %d; usual size %d",
			warning.Kind, warning.Retained, warning.AdvisorySize,
		))
	}
	output.Warn("Large report handoff retained", details...)
}

func reportTargetBoundInputScaleWarnings(
	output *runOutput,
	warnings []report.TargetReportScaleWarning,
	targets ...programindex.Target,
) []report.ReportInputScaleWarning {
	if len(warnings) == 0 {
		return nil
	}
	targetByID := make(map[string]programindex.Target, len(targets))
	for _, target := range targets {
		if target.ID != "" {
			targetByID[target.ID] = target
		}
	}
	reported := make([]report.ReportInputScaleWarning, 0, len(warnings))
	for _, bound := range warnings {
		reported = append(reported, bound.Warning)
		if target, ok := targetByID[bound.ProgramTargetID]; ok {
			reportInputScaleWarnings(output, []report.ReportInputScaleWarning{bound.Warning}, target)
			continue
		}
		if output == nil {
			continue
		}
		details := []string{
			"complete target transport authority was retained",
			fmt.Sprintf(
				"target: program_id=%q; selected_id=%q",
				bound.ProgramTargetID, bound.SelectedTargetID,
			),
			fmt.Sprintf(
				"%s: retained %d; usual size %d",
				bound.Warning.Kind, bound.Warning.Retained, bound.Warning.AdvisorySize,
			),
		}
		output.Warn("Large report handoff retained", details...)
	}
	return reported
}

func excludeTargetReportScaleWarnings(
	warnings []report.TargetReportScaleWarning,
	alreadyReported map[reportScaleWarningKey]struct{},
) []report.TargetReportScaleWarning {
	filtered := make([]report.TargetReportScaleWarning, 0, len(warnings))
	for _, warning := range warnings {
		key := reportScaleWarningKey{
			kind: warning.Warning.Kind, advisory: warning.Warning.AdvisorySize,
		}
		if _, duplicate := alreadyReported[key]; duplicate {
			continue
		}
		filtered = append(filtered, warning)
	}
	return filtered
}

// reportTargetPageRunScaleWarnings performs a best-effort diagnostic pass
// after a successful multi-target publication. Target-local projection and
// then-current manifest warnings were emitted as soon as they were measurable;
// the retained keys suppress repeats here. Only newly crossed final manifest
// and physical-file sizes are emitted after portfolio finalization. Diagnostic
// read failures are ignored: scale reporting must never become another
// validation or publication boundary.
func reportTargetPageRunScaleWarnings(
	output *runOutput,
	runs []targetPublishedRun,
	additional []report.ReportInputScaleWarning,
	targetAdditional []report.TargetReportScaleWarning,
) {
	if output == nil {
		return
	}
	type warningKey struct {
		kind     string
		advisory int
	}
	type warningAggregate struct {
		warning report.ReportInputScaleWarning
		targets []string
		seen    map[string]struct{}
	}
	positions := make(map[warningKey]int)
	aggregates := make([]warningAggregate, 0)
	for _, run := range runs {
		target := targetPublishedRunScaleWarningLabel(run)
		alreadyReported := reportScaleWarningKeySet(run.ReportScaleWarnings)
		warnings := make([]report.ReportInputScaleWarning, 0)
		if manifest, err := report.ReadRunManifest(run.RunDir); err == nil {
			warnings = append(warnings, report.RunManifestScaleWarnings(manifest)...)
		}
		warnings = append(warnings, report.PublishedReportScaleWarnings(run.RunDir)...)
		warnings = excludeReportScaleWarnings(warnings, alreadyReported)
		for _, warning := range warnings {
			key := warningKey{kind: warning.Kind, advisory: warning.AdvisorySize}
			position, found := positions[key]
			if !found {
				position = len(aggregates)
				positions[key] = position
				aggregates = append(aggregates, warningAggregate{
					warning: warning,
					seen:    make(map[string]struct{}),
				})
			}
			aggregate := &aggregates[position]
			if warning.Retained > aggregate.warning.Retained {
				aggregate.warning.Retained = warning.Retained
			}
			if _, duplicate := aggregate.seen[target]; !duplicate {
				aggregate.seen[target] = struct{}{}
				aggregate.targets = append(aggregate.targets, target)
			}
		}
	}
	for _, warning := range additional {
		key := warningKey{kind: warning.Kind, advisory: warning.AdvisorySize}
		position, found := positions[key]
		if !found {
			position = len(aggregates)
			positions[key] = position
			aggregates = append(aggregates, warningAggregate{
				warning: warning,
				seen:    make(map[string]struct{}),
			})
		}
		aggregate := &aggregates[position]
		if warning.Retained > aggregate.warning.Retained {
			aggregate.warning.Retained = warning.Retained
		}
		const portfolioWide = "portfolio-wide"
		if _, duplicate := aggregate.seen[portfolioWide]; !duplicate {
			aggregate.seen[portfolioWide] = struct{}{}
			aggregate.targets = append(aggregate.targets, portfolioWide)
		}
	}
	for _, bound := range targetAdditional {
		warning := bound.Warning
		key := warningKey{kind: warning.Kind, advisory: warning.AdvisorySize}
		position, found := positions[key]
		if !found {
			position = len(aggregates)
			positions[key] = position
			aggregates = append(aggregates, warningAggregate{
				warning: warning,
				seen:    make(map[string]struct{}),
			})
		}
		aggregate := &aggregates[position]
		if warning.Retained > aggregate.warning.Retained {
			aggregate.warning.Retained = warning.Retained
		}
		target := ""
		for _, run := range runs {
			if bound.ProgramTargetID != "" &&
				run.ProgramPage.ProgramTarget.ID == bound.ProgramTargetID {
				target = targetPublishedRunScaleWarningLabel(run)
				break
			}
		}
		if target == "" {
			target = bound.ProgramTargetID
			if target == "" {
				target = bound.SelectedTargetID
			}
			if target == "" {
				target = "unknown-target"
			}
		}
		if _, duplicate := aggregate.seen[target]; !duplicate {
			aggregate.seen[target] = struct{}{}
			aggregate.targets = append(aggregate.targets, target)
		}
	}
	if len(aggregates) == 0 {
		return
	}
	details := []string{"complete report handoff authority was retained for every listed target"}
	for _, aggregate := range aggregates {
		details = append(details, fmt.Sprintf(
			"%s: largest retained %d; usual size %d; targets %s",
			aggregate.warning.Kind,
			aggregate.warning.Retained,
			aggregate.warning.AdvisorySize,
			strings.Join(aggregate.targets, ", "),
		))
	}
	output.Warn("Large report handoff retained", details...)
}

func reportCurrentTargetPageScaleWarnings(output *runOutput, run *targetPublishedRun) {
	if run == nil {
		return
	}
	warnings := make([]report.ReportInputScaleWarning, 0)
	if manifest, err := report.ReadRunManifest(run.RunDir); err == nil {
		warnings = append(warnings, report.RunManifestScaleWarnings(manifest)...)
	}
	warnings = append(warnings, report.PublishedReportScaleWarnings(run.RunDir)...)
	warnings = excludeReportScaleWarnings(
		warnings,
		reportScaleWarningKeySet(run.ReportScaleWarnings),
	)
	if len(warnings) == 0 {
		return
	}
	reportInputScaleWarnings(output, warnings, run.ProgramPage.ProgramTarget)
	run.ReportScaleWarnings = append(run.ReportScaleWarnings, warnings...)
}

func reportCurrentCapturedInputScaleWarnings(output *runOutput, run *targetPublishedRun) {
	if run == nil {
		return
	}
	warnings := excludeReportScaleWarnings(
		report.CapturedReportInputFileScaleWarnings(run.RunDir),
		reportScaleWarningKeySet(run.ReportScaleWarnings),
	)
	if len(warnings) == 0 {
		return
	}
	reportInputScaleWarnings(output, warnings, run.ProgramPage.ProgramTarget)
	run.ReportScaleWarnings = append(run.ReportScaleWarnings, warnings...)
}

func targetPublishedRunScaleWarningLabel(run targetPublishedRun) string {
	if run.SelectedTargetDisplay != "" && run.SelectedTargetKey != "" {
		return run.SelectedTargetDisplay + " (" + run.SelectedTargetKey + ")"
	}
	if run.SelectedTargetDisplay != "" {
		return run.SelectedTargetDisplay
	}
	if run.SelectedTargetKey != "" {
		return run.SelectedTargetKey
	}
	target := run.ProgramPage.ProgramTarget
	if target.Selector != "" {
		return target.Language + ":" + target.Selector
	}
	if target.Name != "" {
		return target.Language + ":" + target.Name
	}
	if run.RunID != "" {
		return run.RunID
	}
	return "unknown target"
}

func emitProgramViewScaleWarnings(
	output *runOutput,
	index programindex.Index,
	warnings []report.ProgramViewScaleWarning,
) {
	if output == nil || len(warnings) == 0 {
		return
	}
	details := targetScaleWarningDetails(
		index.Target,
		"all ProgramView rows and relation witnesses retained; no presentation sampling was applied",
	)
	for _, warning := range warnings {
		details = append(details, fmt.Sprintf(
			"%s: affected collections %d; largest retained %d; usual size %d",
			warning.Kind,
			warning.AffectedCollections,
			warning.MaximumRetained,
			warning.AdvisorySize,
		))
	}
	output.Warn("Large ProgramView retained", details...)
}

type reportScaleWarningKey struct {
	kind     string
	advisory int
}

func reportScaleWarningKeySet(warnings []report.ReportInputScaleWarning) map[reportScaleWarningKey]struct{} {
	result := make(map[reportScaleWarningKey]struct{}, len(warnings))
	for _, warning := range warnings {
		result[reportScaleWarningKey{kind: warning.Kind, advisory: warning.AdvisorySize}] = struct{}{}
	}
	return result
}

func excludeReportScaleWarnings(
	warnings []report.ReportInputScaleWarning,
	excluded map[reportScaleWarningKey]struct{},
) []report.ReportInputScaleWarning {
	if len(warnings) == 0 || len(excluded) == 0 {
		return warnings
	}
	result := make([]report.ReportInputScaleWarning, 0, len(warnings))
	for _, warning := range warnings {
		key := reportScaleWarningKey{kind: warning.Kind, advisory: warning.AdvisorySize}
		if _, found := excluded[key]; found {
			continue
		}
		result = append(result, warning)
	}
	return result
}

func targetScaleWarningDetails(target programindex.Target, retained string) []string {
	details := make([]string, 0, 2)
	if detail := semanticWarningTargetDetail(target); detail != "" {
		details = append(details, detail)
	}
	return append(details, retained)
}

func reportDefaultProgramTarget(data *report.ReportData) programindex.Target {
	if data == nil || data.ProgramPortfolio == nil {
		return programindex.Target{}
	}
	for _, entry := range data.ProgramPortfolio.Entries {
		if entry.Target.ID == data.ProgramPortfolio.DefaultTargetID {
			return entry.Target.Snapshot()
		}
	}
	return programindex.Target{}
}
