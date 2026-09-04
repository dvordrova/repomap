package run

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/programcategorization"
)

func TestFormatCategorizationCoveragePrintsDenominators(t *testing.T) {
	details := formatCategorizationCoverage(programcategorization.Coverage{
		Objects: 219, CoveredObjects: 214, Patterns: 77, CoveredPatterns: 25,
		ByCategory: map[programcategorization.Category]int{
			programcategorization.CategoryCore:       225,
			programcategorization.CategoryDependency: 11,
			programcategorization.CategoryInbound:    10,
		},
		Requests: 12, Empty: 3,
	})
	want := []string{
		"subjects covered: 239/296 (80%) (objects 214/219 (97%), connections 25/77 (32%))",
		"categories: inbound 10 (3%), background activity 0 (0%), dependency 11 (3%), core 225 (76%)",
		"requests: 12, of which 3 assigned nothing",
		"accepted rows naming a subject outside the index: 0",
	}
	if !reflect.DeepEqual(details, want) {
		t.Fatalf("details =\n%#v\nwant\n%#v", details, want)
	}
}

// TestFormatCategorizationCoverageWithoutSubjects keeps the instrument safe on
// an index with nothing to categorize.
func TestFormatCategorizationCoverageWithoutSubjects(t *testing.T) {
	details := formatCategorizationCoverage(programcategorization.Coverage{
		ByCategory: map[programcategorization.Category]int{},
	})
	want := []string{
		"subjects covered: 0/0 (objects 0/0, connections 0/0)",
		"categories: inbound 0, background activity 0, dependency 0, core 0",
		"accepted rows naming a subject outside the index: 0",
	}
	if !reflect.DeepEqual(details, want) {
		t.Fatalf("details =\n%#v\nwant\n%#v", details, want)
	}
}
