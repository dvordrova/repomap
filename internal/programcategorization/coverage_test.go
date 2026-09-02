package programcategorization

import (
	"fmt"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

// TestCoverageSeparatesObjectsFromConnections proves the instrument reports a
// denominator for both subject kinds. A run that assigns only relation
// patterns must not read as covered objects, and the reverse.
func TestCoverageSeparatesObjectsFromConnections(t *testing.T) {
	index := categorizationTestIndex(t, "go")
	documentation := reducedDocumentationFixture(t)
	provider := &presetProvider{}
	provider.respond = func(request Request, _ int) []byte {
		var patternRef string
		for _, subject := range request.Subjects {
			if subject.Selector == "HandleFunc" {
				patternRef = subject.Ref
			}
		}
		return []byte(fmt.Sprintf(
			`{"assignments":[{"ref":%q,"categories":["background_activity"]}]}`, patternRef,
		))
	}

	result, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, index, documentation)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	coverage := result.Coverage(index)
	objects, patterns := len(index.Objects), 0
	for _, relation := range index.Relations {
		patterns += len(relation.Patterns)
	}
	if coverage.Objects != objects || coverage.Patterns != patterns {
		t.Fatalf("denominators = %d objects / %d connections, want %d / %d",
			coverage.Objects, coverage.Patterns, objects, patterns)
	}
	if coverage.CoveredObjects != 0 || coverage.CoveredPatterns != 1 {
		t.Fatalf("covered = %d objects / %d connections, want 0 / 1",
			coverage.CoveredObjects, coverage.CoveredPatterns)
	}
	if coverage.OutsideIndex != 0 {
		t.Fatalf("accepted rows outside the index = %d, want 0", coverage.OutsideIndex)
	}
	if coverage.ByCategory[CategoryBackgroundActivity] != 1 || coverage.ByCategory[CategoryCore] != 0 {
		t.Fatalf("per-category split = %#v", coverage.ByCategory)
	}
	if coverage.Requests != 1 || coverage.Empty != 0 {
		t.Fatalf("requests = %d, of which empty %d, want 1 / 0", coverage.Requests, coverage.Empty)
	}
}

// TestCoverageCountsRequestsThatAssignedNothing keeps the empty-request count
// honest: a plan whose requests all come back empty is the shape of a lost
// signal, and it must be visible without rerunning the stage.
func TestCoverageCountsRequestsThatAssignedNothing(t *testing.T) {
	index := categorizationTestIndex(t, "go")
	documentation := reducedDocumentationFixture(t)
	provider := &presetProvider{}
	provider.respond = func(Request, int) []byte { return []byte(`{"assignments":[]}`) }

	result, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, index, documentation)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	coverage := result.Coverage(index)
	if coverage.Requests == 0 || coverage.Requests != coverage.Empty {
		t.Fatalf("requests = %d, of which empty %d, want every request empty",
			coverage.Requests, coverage.Empty)
	}
	if coverage.CoveredSubjects() != 0 || coverage.Subjects() != coverage.Objects+coverage.Patterns {
		t.Fatalf("coverage = %d/%d", coverage.CoveredSubjects(), coverage.Subjects())
	}
}

// TestValidateRefusesASubjectThatIsNeitherObjectNorConnection pins the answer
// to the question the coverage split raises: assignments naming something
// that is not an object are relation patterns, and nothing else can be
// accepted. A result that names an id belonging to neither is refused whole.
func TestValidateRefusesASubjectThatIsNeitherObjectNorConnection(t *testing.T) {
	index := categorizationTestIndex(t, "go")
	documentation := reducedDocumentationFixture(t)
	patternID := patternIDBySourceRef(t, index, "registration-pattern")

	accepted := Result{
		ProgramTargetID:            index.Target.ID,
		BaseProgramIndexSHA256:     index.SHA256,
		ReducedDocumentationSHA256: documentation.ReductionSHA256,
		Assignments:                []Assignment{{SubjectID: patternID, Categories: []Category{CategoryCore}}},
		Diagnostics:                []Diagnostic{},
	}
	if err := accepted.Validate(index, documentation); err != nil {
		t.Fatalf("a relation pattern is a real subject: %v", err)
	}
	coverage := accepted.Coverage(index)
	if coverage.CoveredPatterns != 1 || coverage.CoveredObjects != 0 || coverage.OutsideIndex != 0 {
		t.Fatalf("coverage = %#v", coverage)
	}

	invented := accepted
	invented.Assignments = []Assignment{
		{SubjectID: "program-pattern-not-in-this-index", Categories: []Category{CategoryCore}},
	}
	if err := invented.Validate(index, documentation); err == nil {
		t.Fatal("a subject in neither set was accepted")
	}
}
