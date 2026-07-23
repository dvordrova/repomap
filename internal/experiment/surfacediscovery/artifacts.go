package surfacediscovery

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	TriggerCatalogFilename        = "trigger_catalog.json"
	SurfaceCoverageFilename       = "surface_coverage.json"
	SemanticSummaryFilename       = "semantic_summaries.json"
	SurfaceSummaryFilename        = "surface_summary.md"
	ArchitectureGroundingFilename = "architecture_grounding.json"
)

func WriteArtifacts(directory string, result Result) error {
	if strings.TrimSpace(directory) == "" {
		return fmt.Errorf("surface discovery: output directory is required")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("surface discovery: create output directory: %w", err)
	}
	artifacts := []struct {
		name string
		data []byte
	}{
		{name: SurfaceSummaryFilename, data: []byte(Markdown(result))},
	}
	values := []struct {
		name  string
		value any
	}{
		{name: TriggerCatalogFilename, value: result.Catalog},
		{name: SurfaceCoverageFilename, value: result.Coverage},
		{name: SemanticSummaryFilename, value: result.Summaries},
		{name: ArchitectureGroundingFilename, value: result.Grounding},
	}
	for _, value := range values {
		data, err := MarshalDeterministic(value.value)
		if err != nil {
			return fmt.Errorf("surface discovery: encode %s: %w", value.name, err)
		}
		artifacts = append(artifacts, struct {
			name string
			data []byte
		}{name: value.name, data: data})
	}
	for _, artifact := range artifacts {
		if err := writeAtomic(filepath.Join(directory, artifact.name), artifact.data); err != nil {
			return err
		}
	}
	return nil
}

func Markdown(result Result) string {
	var builder strings.Builder
	builder.WriteString("# Runtime surfaces\n\n")
	fmt.Fprintf(&builder, "Repository: `%s`\n\n", result.Catalog.Repository.Root)
	fmt.Fprintf(&builder, "Scenario: `%s`\n\n", result.Catalog.Scenario.ID)
	fmt.Fprintf(
		&builder,
		"Discovered %d static surface record(s): %d direct, %d wrapper-derived.\n\n",
		len(result.Catalog.Triggers),
		result.Coverage.DirectTriggers,
		result.Coverage.WrapperDerivedTriggers,
	)
	builder.WriteString("This is not a runtime trace. " + result.Coverage.ScopeStatement + ".\n\n")
	if len(result.Coverage.Phases) > 0 {
		builder.WriteString("## Discovery phases\n\n")
		for _, phase := range result.Coverage.Phases {
			fmt.Fprintf(&builder, "- `%s`: %d ms", phase.Phase, phase.LatencyMillis)
			if phase.Total > 0 {
				fmt.Fprintf(&builder, " (%d/%d)", phase.Completed, phase.Total)
			}
			if phase.Detail != "" {
				fmt.Fprintf(&builder, " — %s", phase.Detail)
			}
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	for _, trigger := range result.Catalog.Triggers {
		title := strings.TrimSpace(trigger.Identity.Method + " " + trigger.Identity.Path.Text)
		if trigger.Identity.Name != "" {
			title = trigger.Identity.Name
		}
		if title == "" {
			title = "<dynamic>"
		}
		fmt.Fprintf(&builder, "## %s\n\n", title)
		fmt.Fprintf(&builder, "- ID: `%s`\n", trigger.ID)
		fmt.Fprintf(&builder, "- Status: `%s`; certainty `%s`; resolution `%s`\n", trigger.Status, trigger.Certainty, trigger.Resolution)
		fmt.Fprintf(&builder, "- Surface role: `%s`; trace readiness `%s`\n", trigger.SurfaceRole, trigger.TraceReadiness)
		fmt.Fprintf(&builder, "- Trace readiness reason: %s\n", trigger.TraceReadinessReason)
		fmt.Fprintf(&builder, "- Executable: `%s`; role `%s`; availability `%s`\n", trigger.OwningExecutable, trigger.ExecutableRole, trigger.Availability)
		if trigger.UnavailableReason != "" {
			fmt.Fprintf(&builder, "- Unavailable reason: %s\n", trigger.UnavailableReason)
		}
		fmt.Fprintf(&builder, "- Handler: `%s`\n", displayValue(trigger.Handler))
		fmt.Fprintf(&builder, "- Dispatcher: `%s`\n", displayValue(trigger.Dispatcher))
		fmt.Fprintf(&builder, "- Registration: `%s:%d` via `%s`\n", trigger.RegistrationSite.Path, trigger.RegistrationSite.Line, trigger.FinalSeed)
		if trigger.ServerStartSite != nil {
			fmt.Fprintf(&builder, "- Server start: `%s:%d`\n", trigger.ServerStartSite.Path, trigger.ServerStartSite.Line)
		}
		if len(trigger.WrapperChain) > 0 {
			wrappers := make([]string, 0, len(trigger.WrapperChain))
			for _, wrapper := range trigger.WrapperChain {
				wrappers = append(wrappers, wrapper.Symbol.ID)
			}
			fmt.Fprintf(&builder, "- Wrapper chain: `%s`\n", strings.Join(wrappers, "` → `"))
		}
		if len(trigger.DynamicFrontier) > 0 {
			frontiers := make([]string, 0, len(trigger.DynamicFrontier))
			for _, frontier := range trigger.DynamicFrontier {
				frontiers = append(frontiers, frontier.Kind+": "+frontier.Detail)
			}
			sort.Strings(frontiers)
			fmt.Fprintf(&builder, "- Frontiers: %s\n", strings.Join(frontiers, "; "))
		}
		builder.WriteString("\n")
	}
	if len(result.Coverage.UnavailablePackages) > 0 {
		builder.WriteString("## Unavailable packages\n\n")
		for _, pkg := range result.Coverage.UnavailablePackages {
			fmt.Fprintf(&builder, "- `%s`", pkg.Package)
			if pkg.OwningExecutable != "" {
				fmt.Fprintf(&builder, " (`%s`)", pkg.OwningExecutable)
			}
			fmt.Fprintf(&builder, ": %s\n", pkg.Reason)
		}
		builder.WriteString("\n")
	}
	if len(result.Coverage.PackageDiagnostics) > 0 {
		builder.WriteString("## Package diagnostics\n\n")
		for _, diagnostic := range result.Coverage.PackageDiagnostics {
			fmt.Fprintf(&builder, "- `%s`: %s", diagnostic.Package, diagnostic.Message)
			if diagnostic.Location != nil {
				fmt.Fprintf(&builder, " (`%s:%d`)", diagnostic.Location.Path, diagnostic.Location.Line)
			}
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	if len(result.Coverage.LoopSignals) > 0 {
		builder.WriteString("## Loop signals\n\n")
		for _, signal := range result.Coverage.LoopSignals {
			fmt.Fprintf(
				&builder,
				"- `%s` in `%s` at `%s:%d`: %s\n",
				signal.Kind,
				signal.FunctionID,
				signal.Location.Path,
				signal.Location.Line,
				signal.Detail,
			)
		}
		builder.WriteString("\n")
	}
	if len(result.Coverage.BudgetsReached) > 0 {
		fmt.Fprintf(&builder, "Budgets reached: `%s`.\n", strings.Join(result.Coverage.BudgetsReached, "`, `"))
	}
	return builder.String()
}

func displayValue(value Value) string {
	if value.Text != "" {
		return value.Text
	}
	return "unknown"
}

func writeAtomic(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".surface-*.tmp")
	if err != nil {
		return fmt.Errorf("surface discovery: create temporary artifact: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("surface discovery: set artifact permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("surface discovery: write artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("surface discovery: close artifact: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("surface discovery: install artifact: %w", err)
	}
	return nil
}
