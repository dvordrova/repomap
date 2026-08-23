package dependencydeclaration

import (
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
	encoded, err := Encode(result)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("Encode() = %d bytes, %v", len(encoded), err)
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
