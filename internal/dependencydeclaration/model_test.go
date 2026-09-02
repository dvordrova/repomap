package dependencydeclaration

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
)

func TestBuildSealsCanonicalDeclarationLedger(t *testing.T) {
	t.Parallel()
	digest := func(character string) string { return strings.Repeat(character, 64) }
	result, err := Build(Input{
		CorpusSHA256: digest("a"), ProgramIndexSHA256: digest("b"), TargetID: "target-1",
		Scope: Scope{Language: "python", Ecosystem: "pypi", AuthoritySHA256: digest("c")},
		Sources: []SourceInput{{
			Key: "source-one", FileRef: corpus.FileID("f1"), Path: "pyproject.toml",
			Format: "pyproject_toml", State: SourceParsed, ContentSHA256: digest("d"), ByteCount: 128,
		}},
		Statements: []StatementInput{
			{
				SourceKey: "source-one", Kind: StatementRequirement, Role: RoleRuntime,
				Name: "Foo.Bar", NormalizedName: "foo-bar", Extras: []string{}, Specifier: ">=1",
				Locator: Locator{Kind: LocatorRegistry}, Section: "project.dependencies", Ordinal: 1,
				ExpressionSHA256: digest("e"),
			},
			{
				SourceKey: "source-one", Kind: StatementConstraint, Role: RoleUnspecified,
				Name: "foo-bar", NormalizedName: "foo-bar", Extras: []string{},
				Locator: Locator{Kind: LocatorRegistry}, Section: "constraints", Ordinal: 2,
				ExpressionSHA256: digest("f"),
			},
		},
		Includes: []IncludeInput{{
			SourceKey: "source-one", Kind: IncludeConstraint, Resolution: IncludeMissing,
			Location: &Location{Line: 3, Column: 1}, ExpressionSHA256: digest("1"),
		}},
		Frontiers: []FrontierInput{{
			SourceKey: "source-one", Kind: FrontierDirective, Reason: FrontierUnsupportedOption,
			Section: "requirements", Ordinal: 4, Location: &Location{Line: 4, Column: 1},
			ExpressionSHA256: digest("2"),
		}},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(result.Packages) != 1 || len(result.Packages[0].Statements) != 2 {
		t.Fatalf("package ledger = %#v", result.Packages)
	}
	if want := []string{"Foo.Bar", "foo-bar"}; !reflect.DeepEqual(result.Packages[0].Names, want) {
		t.Fatalf("names = %v, want %v", result.Packages[0].Names, want)
	}
	if result.Coverage.State != CoverageFrontier || result.Coverage.StatementsObserved != 2 ||
		result.Coverage.IncludesFrontier != 1 || result.Coverage.Boundaries != 2 {
		t.Fatalf("coverage = %#v", result.Coverage)
	}
	snapshot := result.Snapshot()
	snapshot.Packages[0].Names[0] = "mutated"
	if result.Packages[0].Names[0] != "Foo.Bar" {
		t.Fatal("Snapshot() aliases package names")
	}

	tampered := result.Snapshot()
	tampered.Sources[0].Path = "other.toml"
	if err := tampered.Validate(); err == nil {
		t.Fatal("tampered source path validated")
	}
}

func TestFormerResultSizeIsWarningOnly(t *testing.T) {
	warnings := resultScaleWarningsForBytes(AdvisoryResultBytes + 1)
	if len(warnings) != 1 || warnings[0].Kind != ScaleWarningResultBytes ||
		warnings[0].Retained != AdvisoryResultBytes+1 ||
		warnings[0].AdvisorySize != AdvisoryResultBytes {
		t.Fatalf("warnings = %#v", warnings)
	}
	if warnings := resultScaleWarningsForBytes(AdvisoryResultBytes); len(warnings) != 0 {
		t.Fatalf("threshold warning = %#v", warnings)
	}
}

func TestFrontierSourceRequiresExactBoundary(t *testing.T) {
	t.Parallel()
	digest := strings.Repeat("a", 64)
	_, err := Build(Input{
		CorpusSHA256: digest, ProgramIndexSHA256: digest, TargetID: "target-1",
		Scope: Scope{Language: "python", Ecosystem: "pypi", AuthoritySHA256: digest},
		Sources: []SourceInput{{
			Key: "source-one", FileRef: corpus.FileID("f1"), Path: "setup.py",
			Format: "setup_py", State: SourceFrontier, ContentSHA256: digest,
		}},
		Statements: []StatementInput{}, Includes: []IncludeInput{}, Frontiers: []FrontierInput{},
	})
	if err == nil {
		t.Fatal("frontier source without an exact boundary validated")
	}
}

func TestBuildRetainsSourcesAboveFormerCountThreshold(t *testing.T) {
	digest := strings.Repeat("a", 64)
	sources := make([]SourceInput, 0, AdvisorySources+1)
	for position := 0; position <= AdvisorySources; position++ {
		sources = append(sources, SourceInput{
			Key: fmt.Sprintf("source-%05d", position), FileRef: corpus.FileID(fmt.Sprintf("f%d", position+1)),
			Path: fmt.Sprintf("requirements/%05d.txt", position), Format: "requirements_txt",
			State: SourceParsed, ContentSHA256: digest, ByteCount: 1,
		})
	}
	result, err := Build(Input{
		CorpusSHA256: digest, ProgramIndexSHA256: digest, TargetID: "target-1",
		Scope:   Scope{Language: "python", Ecosystem: "pypi", AuthoritySHA256: digest},
		Sources: sources, Statements: []StatementInput{}, Includes: []IncludeInput{}, Frontiers: []FrontierInput{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != AdvisorySources+1 {
		t.Fatalf("sources = %d, want %d", len(result.Sources), AdvisorySources+1)
	}
	warnings := ScaleWarnings(result)
	if len(warnings) != 1 || warnings[0].Kind != ScaleWarningSources ||
		warnings[0].Retained != AdvisorySources+1 {
		t.Fatalf("scale warnings = %#v", warnings)
	}
}

func TestBuildRetainsFormerPerValueThresholds(t *testing.T) {
	digest := strings.Repeat("b", 64)
	extras := make([]string, 0, AdvisoryStatementExtras+1)
	for position := 0; position <= AdvisoryStatementExtras; position++ {
		extras = append(extras, fmt.Sprintf("extra-%03d", position))
	}
	longName := strings.Repeat("x", AdvisoryStringBytes+1)
	result, err := Build(Input{
		CorpusSHA256: digest, ProgramIndexSHA256: digest, TargetID: "target-1",
		Scope: Scope{Language: "python", Ecosystem: "pypi", AuthoritySHA256: digest},
		Sources: []SourceInput{
			{Key: "one", FileRef: "f1", Path: "requirements/one.txt", Format: "requirements_txt", State: SourceParsed, ContentSHA256: digest, ByteCount: 32<<20 + 1},
			{Key: "two", FileRef: "f2", Path: "requirements/two.txt", Format: "requirements_txt", State: SourceParsed, ContentSHA256: digest, ByteCount: 32<<20 + 1},
		},
		Statements: []StatementInput{{
			SourceKey: "one", Kind: StatementRequirement, Role: RoleRuntime,
			Name: longName, NormalizedName: longName, Extras: extras,
			Locator: Locator{Kind: LocatorRegistry}, Section: "requirements", Ordinal: 1,
			ExpressionSHA256: digest,
		}},
		Includes: []IncludeInput{}, Frontiers: []FrontierInput{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Packages) != 1 || len(result.Packages[0].Statements[0].Extras) != AdvisoryStatementExtras+1 ||
		result.Packages[0].Name != longName {
		t.Fatalf("retained declarations = %#v", result.Packages)
	}
	kinds := make(map[ScaleWarningKind]struct{})
	for _, warning := range ScaleWarnings(result) {
		kinds[warning.Kind] = struct{}{}
	}
	for _, kind := range []ScaleWarningKind{
		ScaleWarningSourceBytes, ScaleWarningTotalBytes, ScaleWarningStringBytes, ScaleWarningStatementExtras,
	} {
		if _, ok := kinds[kind]; !ok {
			t.Fatalf("scale warnings missing %q: %#v", kind, ScaleWarnings(result))
		}
	}
}
