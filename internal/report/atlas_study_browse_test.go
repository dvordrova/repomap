package report

import (
	"fmt"
	"sort"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/evidence"
)

// browseTestSpan builds one focused route span for the synthetic browse
// fixtures. The span ID doubles as the reading-target ID so the input stays
// self-consistent without a full atlas fixture.
func browseTestSpan(id string, stage atlasstudy.LearningStage) atlasstudy.RouteSpan {
	return atlasstudy.RouteSpan{
		ID:               id,
		Kind:             atlasstudy.RouteSpanFocused,
		LearningStage:    stage,
		QuestionEnglish:  "Question for " + id + "?",
		AllowedTargetIDs: []string{id},
	}
}

// browseTestInput assembles an input whose route spans are exactly ids, each
// with a focused reading target at open.go, except closeID (if non-empty)
// which points at closed.go and is deliberately not openable.
func browseTestInput(ids []string, closeID string) atlasstudy.Input {
	var input atlasstudy.Input
	input.Language = atlasstudy.LanguageEnglish
	for _, id := range ids {
		path := "open.go"
		if id == closeID {
			path = "closed.go"
		}
		input.ReadingTargets = append(input.ReadingTargets, atlasstudy.ReadingTarget{
			ID:       id,
			Kind:     "surface",
			Symbol:   "Symbol" + id,
			Location: evidence.Location{Path: path, Line: 10},
		})
		input.RouteSpans = append(input.RouteSpans, browseTestSpan(id, atlasstudy.StageOrientation))
	}
	return input
}

// browseTestResult builds a result with one accepted direction per accepted
// span ID (direction IDs dir-<span>) and the given model-selected refs.
func browseTestResult(acceptedIDs, modelSelectedIDs []string) atlasstudy.ResultRecord {
	var result atlasstudy.ResultRecord
	for _, id := range acceptedIDs {
		result.Directions = append(result.Directions, atlasstudy.Direction{
			ID:       "dir-" + id,
			Question: "Question for " + id + "?",
			Span:     atlasstudy.CanonicalRef{Kind: atlasstudy.RefRouteSpan, ID: id},
		})
	}
	for _, id := range modelSelectedIDs {
		result.ModelSelectedSpanRefs = append(result.ModelSelectedSpanRefs, atlasstudy.CanonicalRef{
			Kind: atlasstudy.RefRouteSpan, ID: id,
		})
	}
	return result
}

func repsAsRefs(ids []string) []atlasstudy.CanonicalRef {
	refs := make([]atlasstudy.CanonicalRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, atlasstudy.CanonicalRef{Kind: atlasstudy.RefRouteSpan, ID: id})
	}
	return refs
}

// browseTestStatus builds a status with the four exact counts and an
// advertised_budget omission aggregate carrying the representative refs.
func browseTestStatus(considered, advertised, modelSelected, accepted int, representatives []string) atlasstudy.Status {
	return atlasstudy.Status{
		Version:                atlasstudy.ResultVersion,
		State:                  atlasstudy.ProductStateAccepted,
		ConsideredSpanCount:    considered,
		AdvertisedSpanCount:    advertised,
		ModelSelectedSpanCount: modelSelected,
		AcceptedSpanCount:      accepted,
		CandidateCoverage: atlasstudy.CandidateCoverage{
			Omissions: []atlasstudy.CoverageOmission{
				{
					Reason:         atlasstudy.OmissionAdvertisedBudget,
					Count:          considered - advertised,
					Representatives: repsAsRefs(representatives),
				},
			},
		},
	}
}

func browseTestRequest(advertisedIDs []string) atlasstudy.RequestRecord {
	var request atlasstudy.RequestRecord
	request.Language = atlasstudy.LanguageEnglish
	for _, id := range advertisedIDs {
		request.Catalog = append(request.Catalog, atlasstudy.CatalogObject{
			Kind:        atlasstudy.RefRouteSpan,
			CanonicalID: id,
		})
	}
	return request
}

func browseTestPublished(acceptedIDs []string) []StudyDirection {
	directions := make([]StudyDirection, 0, len(acceptedIDs))
	for _, id := range acceptedIDs {
		directions = append(directions, StudyDirection{ID: "dir-" + id})
	}
	return directions
}

func deriveBrowseForTest(t *testing.T, ids, advertisedIDs, modelSelectedIDs, acceptedIDs, reps []string, closeID string) (*FrontierBrowse, error) {
	t.Helper()
	input := browseTestInput(ids, closeID)
	data := &ReportData{OpenablePaths: []string{"open.go"}}
	return deriveAtlasStudyFrontierBrowse(
		browseTestRequest(advertisedIDs),
		browseTestResult(acceptedIDs, modelSelectedIDs),
		input,
		browseTestStatus(len(ids), len(advertisedIDs), len(modelSelectedIDs), len(acceptedIDs), reps),
		data,
		browseTestPublished(acceptedIDs),
	)
}

// TestAtlasStudyBrowseStageDerivationAndTallies is the D212 stage contract:
// every considered span receives exactly one of the four stages and the
// per-stage tallies equal the four status counts exactly (the casdoor fixture
// shape 68/32/10/10 is reproduced at a smaller scale). The synthetic sets keep
// the chain accepted ⊆ model_selected ⊆ advertised ⊆ considered.
func TestAtlasStudyBrowseStageDerivationAndTallies(t *testing.T) {
	ids := []string{"s1", "s2", "s3", "s4", "s5", "s6", "s7", "s8"}
	advertised := []string{"s1", "s2", "s3", "s4", "s5", "s6"}
	modelSelected := []string{"s1", "s2"}
	accepted := []string{"s1"}
	browse, err := deriveBrowseForTest(t, ids, advertised, modelSelected, accepted, []string{"s8"}, "")
	if err != nil {
		t.Fatalf("deriveAtlasStudyFrontierBrowse: %v", err)
	}
	if browse == nil || browse.Total != 8 || browse.Shown != 8 || len(browse.Spans) != 8 {
		t.Fatalf("browse envelope = %#v", browse)
	}
	stageCounts := map[AtlasStudySpanStage]int{
		AtlasStudySpanStageAccepted:      0,
		AtlasStudySpanStageModelSelected: 0,
		AtlasStudySpanStageAdvertised:    0,
		AtlasStudySpanStageConsidered:    0,
	}
	stageRank := map[AtlasStudySpanStage]int{
		AtlasStudySpanStageAccepted:      0,
		AtlasStudySpanStageModelSelected: 1,
		AtlasStudySpanStageAdvertised:    2,
		AtlasStudySpanStageConsidered:    3,
	}
	lastRank := -1
	for _, span := range browse.Spans {
		stageCounts[span.Stage]++
		// Rows are emitted in stage-group order (accepted first, Local last).
		if rank := stageRank[span.Stage]; rank < lastRank {
			t.Fatalf("stage groups out of order at %q (%d after %d)", span.Title, rank, lastRank)
		}
		lastRank = stageRank[span.Stage]
	}
	if stageCounts[AtlasStudySpanStageAccepted] != 1 ||
		stageCounts[AtlasStudySpanStageModelSelected] != 1 ||
		stageCounts[AtlasStudySpanStageAdvertised] != 4 ||
		stageCounts[AtlasStudySpanStageConsidered] != 2 {
		t.Fatalf("stage tallies = accepted %d / model_selected %d / advertised %d / considered %d",
			stageCounts[AtlasStudySpanStageAccepted],
			stageCounts[AtlasStudySpanStageModelSelected],
			stageCounts[AtlasStudySpanStageAdvertised],
			stageCounts[AtlasStudySpanStageConsidered])
	}
	// Every row carries an ordinal 1..N in emitted order.
	for index, span := range browse.Spans {
		if span.Ordinal != index+1 {
			t.Fatalf("ordinal %d at position %d", span.Ordinal, index+1)
		}
	}
}

// TestAtlasStudyBrowseAcceptedDirectionIDResolution is the D212 DirectionID
// contract: accepted rows resolve 1:1 to the published study_map directions.
func TestAtlasStudyBrowseAcceptedDirectionIDResolution(t *testing.T) {
	ids := []string{"s1", "s2", "s3"}
	advertised := []string{"s1", "s2", "s3"}
	modelSelected := []string{"s1", "s2"}
	accepted := []string{"s1", "s2"}
	browse, err := deriveBrowseForTest(t, ids, advertised, modelSelected, accepted, nil, "")
	if err != nil {
		t.Fatalf("deriveAtlasStudyFrontierBrowse: %v", err)
	}
	got := map[string]struct{}{}
	acceptedRows := 0
	for _, span := range browse.Spans {
		if span.Stage != AtlasStudySpanStageAccepted {
			continue
		}
		acceptedRows++
		if span.DirectionID == "" {
			t.Fatalf("accepted row %q has no direction id", span.Title)
		}
		got[span.DirectionID] = struct{}{}
	}
	if acceptedRows != len(accepted) {
		t.Fatalf("accepted rows = %d, want %d", acceptedRows, len(accepted))
	}
	for _, directionID := range []string{"dir-s1", "dir-s2"} {
		if _, ok := got[directionID]; !ok {
			t.Fatalf("published direction %q missing from accepted rows", directionID)
		}
	}
	if len(got) != len(accepted) {
		t.Fatalf("accepted row direction ids are not 1:1 with published directions: %#v", got)
	}
}

// TestAtlasStudyBrowseRepresentativesFirstWithinConsidered is the D212 §4
// ordering contract: the Local group's representative rows are emitted first
// in canonical span-ID order, then the remaining considered rows, even when
// the representatives are not the lexicographically-first considered spans.
// The template must be able to tell representatives apart by Ordinal alone.
func TestAtlasStudyBrowseRepresentativesFirstWithinConsidered(t *testing.T) {
	ids := []string{"a1", "a2", "b1", "b2", "c1", "c2"}
	advertised := []string{"a1", "a2"}
	modelSelected := []string{"a1"}
	accepted := []string{"a1"}
	// Representatives are c1 and c2 — the LAST considered spans in canonical
	// span-ID order. They must still be emitted first within the Local group.
	reps := []string{"c1", "c2"}
	browse, err := deriveBrowseForTest(t, ids, advertised, modelSelected, accepted, reps, "")
	if err != nil {
		t.Fatalf("deriveAtlasStudyFrontierBrowse: %v", err)
	}
	var consideredTitles []string
	for _, span := range browse.Spans {
		if span.Stage == AtlasStudySpanStageConsidered {
			consideredTitles = append(consideredTitles, span.Title)
		}
	}
	// Considered rows: c1, c2 (representatives, span-ID order), then b1, b2
	// (remaining, span-ID order). Titles are Symbol + span ID.
	want := []string{"Symbolc1", "Symbolc2", "Symbolb1", "Symbolb2"}
	if len(consideredTitles) != len(want) {
		t.Fatalf("considered rows = %#v, want %#v", consideredTitles, want)
	}
	for i := range want {
		if consideredTitles[i] != want[i] {
			t.Fatalf("considered row %d = %q, want %q (full %#v)", i, consideredTitles[i], want[i], consideredTitles)
		}
	}
}

// TestAtlasStudyBrowseRepresentativeRefMustResolve is the D212 fail-closed
// contract: a representative ref that does not resolve to a considered span
// is a projection error, never a silently dropped row.
func TestAtlasStudyBrowseRepresentativeRefMustResolve(t *testing.T) {
	ids := []string{"s1", "s2"}
	advertised := []string{"s1"}
	_, err := deriveBrowseForTest(t, ids, advertised, nil, nil, []string{"missing"}, "")
	if err == nil {
		t.Fatal("unresolved representative ref was accepted")
	}
}

// TestAtlasStudyBrowseRepresentativeStageFailClosed is the D212 fail-closed
// contract: a representative ref that resolves to a higher-stage (advertised)
// span contradicts the omission aggregate and is a projection error.
func TestAtlasStudyBrowseRepresentativeStageFailClosed(t *testing.T) {
	ids := []string{"s1", "s2"}
	advertised := []string{"s1", "s2"}
	_, err := deriveBrowseForTest(t, ids, advertised, nil, nil, []string{"s2"}, "")
	if err == nil {
		t.Fatal("advertised representative ref was accepted")
	}
}

// TestAtlasStudyBrowseChainFailClosed is the D212 chain contract: accepted
// outside model_selected (or any other broken link of the chain
// accepted ⊆ model_selected ⊆ advertised ⊆ considered) rejects the browse.
func TestAtlasStudyBrowseChainFailClosed(t *testing.T) {
	ids := []string{"s1", "s2"}
	advertised := []string{"s1", "s2"}
	modelSelected := []string{"s1"}
	accepted := []string{"s1", "s2"} // s2 accepted but never model-selected
	_, err := deriveBrowseForTest(t, ids, advertised, modelSelected, accepted, nil, "")
	if err == nil {
		t.Fatal("chain violation was accepted")
	}
}

// TestAtlasStudyBrowseTallyMismatchFailClosed is the D212 tally contract:
// per-stage tallies over the full pre-truncation row set must equal the four
// status counts, enforced fail-closed.
func TestAtlasStudyBrowseTallyMismatchFailClosed(t *testing.T) {
	ids := []string{"s1", "s2"}
	advertised := []string{"s1"}
	input := browseTestInput(ids, "")
	data := &ReportData{OpenablePaths: []string{"open.go"}}
	status := browseTestStatus(len(ids), len(advertised), 0, 0, nil)
	status.ConsideredSpanCount = 3 // does not match the rebuilt input
	_, err := deriveAtlasStudyFrontierBrowse(
		browseTestRequest(advertised),
		browseTestResult(nil, nil),
		input,
		status,
		data,
		nil,
	)
	if err == nil {
		t.Fatal("status/row tally mismatch was accepted")
	}
}

// TestAtlasStudyBrowse256CeilingTruthful is the D212 §6 ceiling contract: a
// synthetic input with more than MaxAtlasStudyBrowseSpans considered spans
// embeds the deterministic first-256 in canonical span-ID order with truthful
// Total/Shown.
func TestAtlasStudyBrowse256CeilingTruthful(t *testing.T) {
	const total = 300
	ids := make([]string, 0, total)
	for i := 0; i < total; i++ {
		ids = append(ids, fmt.Sprintf("span-%03d", i))
	}
	advertised := []string{"span-000", "span-001"}
	modelSelected := []string{"span-000"}
	accepted := []string{"span-000"}
	browse, err := deriveBrowseForTest(t, ids, advertised, modelSelected, accepted, nil, "")
	if err != nil {
		t.Fatalf("deriveAtlasStudyFrontierBrowse: %v", err)
	}
	if browse.Total != total {
		t.Fatalf("Total = %d, want %d", browse.Total, total)
	}
	if browse.Shown != MaxAtlasStudyBrowseSpans {
		t.Fatalf("Shown = %d, want %d", browse.Shown, MaxAtlasStudyBrowseSpans)
	}
	if len(browse.Spans) != MaxAtlasStudyBrowseSpans {
		t.Fatalf("len(Spans) = %d, want %d", len(browse.Spans), MaxAtlasStudyBrowseSpans)
	}
	// The selected rows must be the first 256 in canonical span-ID order.
	wantTitles := make([]string, 0, MaxAtlasStudyBrowseSpans)
	for i := 0; i < MaxAtlasStudyBrowseSpans; i++ {
		wantTitles = append(wantTitles, "Symbol"+fmt.Sprintf("span-%03d", i))
	}
	gotTitles := make([]string, 0, len(browse.Spans))
	for _, span := range browse.Spans {
		gotTitles = append(gotTitles, span.Title)
	}
	sort.Strings(gotTitles)
	sort.Strings(wantTitles)
	for i := range wantTitles {
		if gotTitles[i] != wantTitles[i] {
			t.Fatalf("shown set differs at %d: %q != %q", i, gotTitles[i], wantTitles[i])
		}
	}
}

// TestAtlasStudyBrowseUnavailableSource is the D212 §5 contract: rows whose
// target path is not openable carry a zero source so the template renders the
// explicit neutral unavailable state instead of a dead button.
func TestAtlasStudyBrowseUnavailableSource(t *testing.T) {
	ids := []string{"s1", "s2"}
	advertised := []string{"s1"}
	browse, err := deriveBrowseForTest(t, ids, advertised, nil, nil, nil, "s2")
	if err != nil {
		t.Fatalf("deriveAtlasStudyFrontierBrowse: %v", err)
	}
	found := false
	for _, span := range browse.Spans {
		if span.Stage != AtlasStudySpanStageConsidered {
			continue
		}
		found = true
		if span.Source.Path != "" || span.Source.Line != 0 {
			t.Fatalf("unavailable row leaked a source: %#v", span.Source)
		}
	}
	if !found {
		t.Fatal("no considered row rendered")
	}
}

// TestAtlasStudyFailedBrowseNeutral is the D212 failed-state contract: the
// failed browse Total comes from the rebuilt input count, every row is the
// neutral local question stage, and it carries no direction ids.
func TestAtlasStudyFailedBrowseNeutral(t *testing.T) {
	ids := []string{"s1", "s2", "s3"}
	input := browseTestInput(ids, "")
	data := &ReportData{OpenablePaths: []string{"open.go"}}
	browse, err := deriveAtlasStudyFailedBrowse(input, data)
	if err != nil {
		t.Fatalf("deriveAtlasStudyFailedBrowse: %v", err)
	}
	if browse == nil || browse.Total != 3 || browse.Shown != 3 || len(browse.Spans) != 3 {
		t.Fatalf("failed browse envelope = %#v", browse)
	}
	for _, span := range browse.Spans {
		if span.Stage != AtlasStudySpanStageConsidered {
			t.Fatalf("failed row stage = %q, want considered", span.Stage)
		}
		if span.DirectionID != "" {
			t.Fatalf("failed row carries a direction id %q", span.DirectionID)
		}
	}
}

// TestAtlasStudyFailedBrowseEndToEnd drives the failed-state browse through
// readAtlasStudyReportProduct: the failed run publishes the neutral local
// browse (Total from input), no study map, and no DirectionID anywhere.
func TestAtlasStudyFailedBrowseEndToEnd(t *testing.T) {
	data := atlasStudyReportFixture(t)
	runDir := t.TempDir()
	product := compileAtlasStudyFixture(t, data)
	writeAtlasStudyRequest(t, runDir, product)
	failed, err := product.FailureStatus(atlasstudy.FailureProvider)
	if err != nil {
		t.Fatalf("FailureStatus: %v", err)
	}
	writeAtlasStudyStatus(t, runDir, failed)
	status, studyMap, err := readAtlasStudyReportProduct(runDir, data)
	if err != nil {
		t.Fatalf("read failed product: %v", err)
	}
	if studyMap != nil {
		t.Fatal("failed state produced a study map")
	}
	if status == nil || status.FrontierBrowse == nil {
		t.Fatalf("failed state produced no browse: %#v", status)
	}
	if status.FrontierBrowse.Total != status.FrontierBrowse.Shown ||
		status.FrontierBrowse.Total != len(status.FrontierBrowse.Spans) {
		t.Fatalf("failed browse envelope not truthful: %#v", status.FrontierBrowse)
	}
	for _, span := range status.FrontierBrowse.Spans {
		if span.Stage != AtlasStudySpanStageConsidered {
			t.Fatalf("failed row stage = %q, want considered", span.Stage)
		}
		if span.DirectionID != "" {
			t.Fatalf("failed row carries a direction id %q", span.DirectionID)
		}
	}
}

// TestAtlasStudyBrowseAbsentForNonAcceptedStates is the D212 gating contract:
// the browse stays nil for unavailable and uncalled product states, and the
// prepared state fails closed. The failed state is the one explicit exception
// (it owns the neutral browse, covered by TestAtlasStudyFailedBrowseEndToEnd).
func TestAtlasStudyBrowseAbsentForNonAcceptedStates(t *testing.T) {
	data := atlasStudyReportFixture(t)

	// Unavailable artifact: request + unavailable status; the report product
	// stays unavailable with no browse and no study map.
	runDir := t.TempDir()
	product := compileAtlasStudyFixture(t, data)
	writeAtlasStudyRequest(t, runDir, product)
	unavailable, err := product.UnavailableStatus(atlasstudy.UnavailableOffline)
	if err != nil {
		t.Fatalf("UnavailableStatus: %v", err)
	}
	writeAtlasStudyStatus(t, runDir, unavailable)
	status, studyMap, err := readAtlasStudyReportProduct(runDir, data)
	if err != nil {
		t.Fatalf("unavailable state: %v", err)
	}
	if status == nil || status.State != atlasstudy.ProductStateUnavailable {
		t.Fatalf("unavailable state status = %#v", status)
	}
	if status.FrontierBrowse != nil {
		t.Fatal("unavailable state produced a browse")
	}
	if studyMap != nil {
		t.Fatal("unavailable state produced a study map")
	}

	// Prepared artifact: the prepared state is not publishable, so the read
	// fails closed and no browse can leak.
	preparedDir := t.TempDir()
	writeAtlasStudyRequest(t, preparedDir, product)
	writeAtlasStudyStatus(t, preparedDir, product.PreparedStatus())
	if _, _, err := readAtlasStudyReportProduct(preparedDir, data); err == nil {
		t.Fatal("prepared state was publishable")
	}

	// Uncalled: no artifacts at all. With the standard fixture the local
	// catalog is sufficient, so the uncalled projection is absent entirely.
	status, studyMap, err = readAtlasStudyReportProduct(t.TempDir(), data)
	if err != nil {
		t.Fatalf("uncalled state: %v", err)
	}
	if status != nil || studyMap != nil {
		t.Fatalf("uncalled state produced status %#v / study map %#v", status, studyMap)
	}

	// Uncalled offline: no artifacts and an offline architecture synthesis
	// produce the explicit offline-unavailable projection with no browse.
	offline := *data
	offline.ArchitectureSynthesis = &ArchitectureSynthesisStatus{
		Version:         ArchitectureSynthesisStatusVersion,
		State:           ArchitectureSynthesisUnavailable,
		UnavailableCode: ArchitectureSynthesisUnavailableOfflineCode,
	}
	status, studyMap, err = readAtlasStudyReportProduct(t.TempDir(), &offline)
	if err != nil {
		t.Fatalf("uncalled offline state: %v", err)
	}
	if status == nil || status.State != atlasstudy.ProductStateUnavailable ||
		status.UnavailableCode != AtlasStudyUnavailableOffline {
		t.Fatalf("uncalled offline status = %#v", status)
	}
	if status.FrontierBrowse != nil {
		t.Fatal("uncalled offline state produced a browse")
	}
	if studyMap != nil {
		t.Fatal("uncalled offline state produced a study map")
	}
}
