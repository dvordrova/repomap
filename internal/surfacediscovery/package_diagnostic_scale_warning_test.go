package surfacediscovery

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestPackageLoadOutcomesRetainEveryDiagnosticAndCompleteMessage(t *testing.T) {
	analyzer := analyzer{root: "/repository", result: Result{}}
	packagesByPath := make(map[string]*packages.Package, MaxPackageDiagnostics+1)
	longMessage := strings.Repeat("diagnostic ", advisoryDiagnosticBytes/11+2)
	for index := 0; index < MaxPackageDiagnostics+1; index++ {
		message := fmt.Sprintf("package %03d failed", index)
		if index == MaxPackageDiagnostics {
			message = longMessage
		}
		packagePath := fmt.Sprintf("example.invalid/package/%03d", index)
		packageErrors := []packages.Error{{Kind: packages.TypeError, Msg: message}}
		if index == MaxPackageDiagnostics {
			packageErrors = append(packageErrors, packages.Error{
				Kind: packages.TypeError, Msg: longMessage + "distinct suffix",
			})
		}
		packagesByPath[packagePath] = &packages.Package{
			PkgPath: packagePath,
			Errors:  packageErrors,
		}
	}
	analyzer.recordPackageLoadOutcomes(packagesByPath)
	coverage := analyzer.result.Coverage
	if len(coverage.PackageDiagnostics) != MaxPackageDiagnostics+2 {
		t.Fatalf("retained package diagnostics = %d, want %d", len(coverage.PackageDiagnostics), MaxPackageDiagnostics+2)
	}
	foundLong, foundDistinctSuffix := false, false
	for _, diagnostic := range coverage.PackageDiagnostics {
		foundLong = foundLong || diagnostic.Message == strings.TrimSpace(longMessage)
		foundDistinctSuffix = foundDistinctSuffix || diagnostic.Message == strings.TrimSpace(longMessage+"distinct suffix")
	}
	if !foundLong || !foundDistinctSuffix {
		t.Fatal("complete diagnostic messages were truncated or deduplicated by their former prefix")
	}
	warnings := PackageDiagnosticScaleWarnings(coverage)
	if len(warnings) != 2 {
		t.Fatalf("package diagnostic warnings = %#v", warnings)
	}
}

func TestTargetProgramCoverageReplacesUnionDiagnosticsWithExactClosure(t *testing.T) {
	const root = "/repository"
	target := &packages.Package{
		PkgPath: "example.invalid/target",
		Errors: []packages.Error{{
			Kind: packages.TypeError,
			Pos:  root + "/target.go:7:3",
			Msg:  "target diagnostic",
		}},
	}
	sibling := &packages.Package{
		PkgPath: "example.invalid/sibling",
		Errors: []packages.Error{{
			Kind: packages.TypeError,
			Pos:  root + "/sibling.go:9:5",
			Msg:  "sibling diagnostic",
		}},
	}
	unionRecorder := analyzer{root: root, result: Result{}}
	unionRecorder.recordPackageLoadOutcomes(map[string]*packages.Package{
		target.PkgPath: target, sibling.PkgPath: sibling,
	})
	preparation := unionRecorder.result.Coverage
	preparation.Phases = []PhaseMetric{{Phase: "package_load", Completed: 2, Total: 2}}

	got := targetProgramCoverage(
		root,
		preparation,
		map[string]*packages.Package{target.PkgPath: target},
	)
	if len(got.PackageDiagnostics) != 1 || got.PackageDiagnostics[0].Package != target.PkgPath {
		t.Fatalf("target diagnostics = %#v, want one exact target diagnostic", got.PackageDiagnostics)
	}
	if !reflect.DeepEqual(got.Phases, preparation.Phases) {
		t.Fatalf("target phases = %#v, want preparation phases %#v", got.Phases, preparation.Phases)
	}
	if len(preparation.PackageDiagnostics) != 2 {
		t.Fatalf("target projection mutated union diagnostics: %#v", preparation.PackageDiagnostics)
	}
}
