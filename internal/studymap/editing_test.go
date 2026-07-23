package studymap

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

func TestEditingContractsDecodeStrictIndependentArtifacts(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	brief := briefShapeFromLegacy(legacy)
	rawDirections := rawDirectionsFromLegacy(legacy)
	directions, err := NormalizeDirectionProposal(rawDirections)
	if err != nil {
		t.Fatal(err)
	}
	reviewBundle, err := BuildReviewBundle(bundle, directions.Directions[0])
	if err != nil {
		t.Fatal(err)
	}
	review := directReview(directions.Directions[0])

	tests := []struct {
		name   string
		value  any
		decode func([]byte) error
	}{
		{
			name: "brief and shape", value: brief,
			decode: func(raw []byte) error {
				_, decodeErr := DecodeBriefShapeProposal(raw)
				return decodeErr
			},
		},
		{
			name: "directions", value: rawDirections,
			decode: func(raw []byte) error {
				_, decodeErr := DecodeDirectionProposal(raw)
				return decodeErr
			},
		},
		{
			name: "review bundle", value: reviewBundle,
			decode: func(raw []byte) error {
				_, decodeErr := DecodeReviewBundle(raw)
				return decodeErr
			},
		},
		{
			name: "review proposal", value: review,
			decode: func(raw []byte) error {
				_, decodeErr := DecodeReviewProposal(raw)
				return decodeErr
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			raw := mustEditingJSON(t, test.value)
			if err := test.decode(raw); err != nil {
				t.Fatalf("decode valid artifact: %v", err)
			}
			withUnknown := append([]byte(`{"unexpected":true,`), raw[1:]...)
			if err := test.decode(withUnknown); err == nil {
				t.Fatal("decoder accepted an unknown field")
			}
			if err := test.decode(append(raw, []byte(` {}`)...)); err == nil {
				t.Fatal("decoder accepted trailing JSON")
			}
		})
	}
	decodedDirections, err := DecodeDirectionProposal(mustEditingJSON(t, rawDirections))
	if err != nil {
		t.Fatal(err)
	}
	if decodedDirections.Directions[0].DirectionID == "" ||
		decodedDirections.Directions[0].DirectionID != directions.Directions[0].DirectionID {
		t.Fatalf("locally derived direction id = %q", decodedDirections.Directions[0].DirectionID)
	}
	normalizedRaw := mustEditingJSON(t, decodedDirections)
	replayedDirections, err := DecodeNormalizedDirectionProposal(normalizedRaw)
	if err != nil {
		t.Fatalf("replay normalized directions: %v", err)
	}
	if !slices.EqualFunc(
		replayedDirections.Directions,
		decodedDirections.Directions,
		func(left, right DirectionCandidate) bool {
			return left.DirectionID == right.DirectionID
		},
	) {
		t.Fatal("normalized direction IDs changed during replay")
	}
	if _, err := DecodeDirectionProposal(normalizedRaw); err == nil ||
		!strings.Contains(err.Error(), "model-supplied direction id") {
		t.Fatalf("raw direction decoder accepted normalized artifact: %v", err)
	}
	missingLocalID := decodedDirections
	missingLocalID.Directions = append([]DirectionCandidate(nil), decodedDirections.Directions...)
	missingLocalID.Directions[0].DirectionID = ""
	if _, err := DecodeNormalizedDirectionProposal(mustEditingJSON(t, missingLocalID)); err == nil {
		t.Fatal("normalized direction decoder accepted an empty local ID")
	}
	suppliedID := rawDirections
	suppliedID.Directions = append([]DirectionCandidate(nil), rawDirections.Directions...)
	suppliedID.Directions[0].DirectionID = "model-direction"
	if _, err := DecodeDirectionProposal(mustEditingJSON(t, suppliedID)); err == nil ||
		!strings.Contains(err.Error(), "model-supplied direction id") {
		t.Fatalf("DecodeDirectionProposal(model id) error = %v", err)
	}
	reordered := rawDirections.Directions[0]
	reordered.Question = "  " + strings.ToUpper(reordered.Question) + "  "
	reordered.AnchorIDs = append([]string(nil), reordered.AnchorIDs...)
	slices.Reverse(reordered.AnchorIDs)
	reordered.ReadingAnchors = append([]ReadingAnchor(nil), reordered.ReadingAnchors...)
	slices.Reverse(reordered.ReadingAnchors)
	normalizedReordered, err := NormalizeDirectionProposal(DirectionProposal{
		Version: DirectionProposalVersion, Directions: []DirectionCandidate{reordered},
	})
	if err != nil {
		t.Fatal(err)
	}
	if normalizedReordered.Directions[0].DirectionID != directions.Directions[0].DirectionID {
		t.Fatalf(
			"stable IDs differ after question/anchor normalization: %q / %q",
			normalizedReordered.Directions[0].DirectionID,
			directions.Directions[0].DirectionID,
		)
	}

	invalidFit := review
	invalidFit.Reviews = append([]AnchorReview(nil), review.Reviews...)
	invalidFit.Reviews[0].Fit = AnchorFit("mostly")
	if _, err := DecodeReviewProposal(mustEditingJSON(t, invalidFit)); err == nil {
		t.Fatal("review decoder accepted an open-ended fit")
	}
	invalidRole := review
	invalidRole.Reviews = append([]AnchorReview(nil), review.Reviews...)
	invalidRole.Reviews[0].Role = ReadingRole("helper")
	if _, err := DecodeReviewProposal(mustEditingJSON(t, invalidRole)); err == nil {
		t.Fatal("review decoder accepted an open-ended reading role")
	}
	invalidReason := review
	invalidReason.Reviews = append([]AnchorReview(nil), review.Reviews...)
	invalidReason.Reviews[0].OverclaimReasons = []OverclaimReason{OverclaimNone, OverclaimVagueOrGeneric}
	if _, err := DecodeReviewProposal(mustEditingJSON(t, invalidReason)); err == nil {
		t.Fatal("review decoder accepted none alongside an overclaim")
	}
}

func TestBuildReviewBundleBoundsExactLineNumberedSource(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	direction := directionsFromLegacy(t, legacy).Directions[0]
	for index, anchorID := range direction.AnchorIDs {
		anchorIndex := anchorIndexByID(t, bundle, anchorID)
		anchor := &bundle.Anchors[anchorIndex]
		function := largeReviewFunction(t, anchor.Path, anchor.Symbol, 100+index*200, 96, 240)
		anchor.Function = function
		anchor.Line = function.StartLine + 48
	}

	reviewBundle, err := BuildReviewBundle(bundle, direction)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviewBundle.Anchors) != len(direction.AnchorIDs) {
		t.Fatalf("review anchors = %d, want %d", len(reviewBundle.Anchors), len(direction.AnchorIDs))
	}
	totalBytes := 0
	shrunk := false
	for index, anchor := range reviewBundle.Anchors {
		if len(anchor.SourceFragment) == 0 || len(anchor.SourceFragment) > maxReviewSourceLines {
			t.Fatalf("anchor %q source lines = %d", anchor.AnchorID, len(anchor.SourceFragment))
		}
		shrunk = shrunk || len(anchor.SourceFragment) < maxReviewSourceLines
		if !slices.ContainsFunc(anchor.SourceFragment, func(line ReviewSourceLine) bool {
			return line.Line == anchor.Line
		}) {
			t.Fatalf("anchor %q exact line %d is absent", anchor.AnchorID, anchor.Line)
		}
		for _, line := range anchor.SourceFragment {
			totalBytes += reviewSourceLineBytes(line)
		}
		original := bundle.Anchors[anchorIndexByID(t, bundle, anchor.AnchorID)]
		if anchor.RepositoryRole != original.Role || anchor.Path != original.Path ||
			anchor.Symbol != original.Symbol || anchor.CurrentSentence != direction.ReadingAnchors[index].WhatToLookFor {
			t.Fatalf("review anchor lost local identity: %#v", anchor)
		}
		if len(anchor.Areas) == 0 || anchor.Areas[0].ID != original.AreaIDs[0] {
			t.Fatalf("review anchor areas = %#v", anchor.Areas)
		}
	}
	if totalBytes > maxReviewSourceBytes {
		t.Fatalf("review source bytes = %d, want <= %d", totalBytes, maxReviewSourceBytes)
	}
	if !shrunk {
		t.Fatal("aggregate source budget did not shrink any 60-line fragment")
	}
	raw := mustEditingJSON(t, reviewBundle)
	decoded, err := DecodeReviewBundle(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.DirectionID != direction.DirectionID {
		t.Fatalf("decoded direction = %q", decoded.DirectionID)
	}
}

func TestApplyReviewsNarrowsCopyAndKeepsWeakAnchor(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	direction := directionsFromLegacy(t, legacy).Directions[0]
	direction.AnchorIDs = []string{"fact-1", "fact-2", "fact-3", "fact-4", "fact-5"}
	direction.ReadingAnchors = readingAnchors(direction.AnchorIDs)
	direction = normalizeDirectionForTest(t, direction)
	proposal := DirectionProposal{Version: DirectionProposalVersion, Directions: []DirectionCandidate{direction}}
	review := ReviewProposal{
		Version: ReviewProposalVersion, DirectionID: direction.DirectionID,
		Reviews: []AnchorReview{
			reviewAnchor("fact-1", AnchorFitDirect, ReadingRolePublicOrCLIEntry, OverclaimNone),
			reviewAnchor("fact-2", AnchorFitSupporting, ReadingRoleCoreOrchestration, OverclaimWrongResponsibility),
			reviewAnchor("fact-3", AnchorFitSupporting, ReadingRoleStateOrDataModel, OverclaimNone),
			reviewAnchor("fact-4", AnchorFitWeak, ReadingRoleRepresentativeImplementation, OverclaimBehaviorOutsideWindow),
			reviewAnchor("fact-5", AnchorFitIrrelevant, ReadingRoleExampleOrUsage, OverclaimNone),
		},
	}
	review.Reviews[0].NarrowerDisplaySentence = "Inspect the exact public entry shown here."
	review.Reviews[1].NarrowerDisplaySentence = "Inspect the local value returned by this function."
	review.Reviews[3].SupportedObservation = "The visible function assigns a local value."

	reduction, err := ApplyReviews(bundle, proposal, []ReviewProposal{review})
	if err != nil {
		t.Fatal(err)
	}
	if reduction.Reviewed != 1 || len(reduction.Directions) != 1 || len(reduction.Issues) != 0 {
		t.Fatalf("reduction = %#v", reduction)
	}
	reviewed := reduction.Directions[0]
	if slices.Contains(reviewed.Candidate.AnchorIDs, "fact-5") || len(reviewed.Candidate.AnchorIDs) != 4 {
		t.Fatalf("retained anchors = %#v", reviewed.Candidate.AnchorIDs)
	}
	reading := readingByID(reviewed.Candidate.ReadingAnchors)
	if got := reading["fact-1"].WhatToLookFor; got != review.Reviews[0].NarrowerDisplaySentence {
		t.Fatalf("bounded direct copy = %q", got)
	}
	if got := reading["fact-2"].WhatToLookFor; got != review.Reviews[1].NarrowerDisplaySentence {
		t.Fatalf("narrowed direct copy = %q", got)
	}
	if got := reading["fact-4"].WhatToLookFor; got != review.Reviews[3].SupportedObservation {
		t.Fatalf("weak fallback copy = %q", got)
	}
	if reviewed.RoleDiversity != 4 || reviewed.QualityScore != ReviewQualityScore(reviewed.Reviews) {
		t.Fatalf("quality/diversity = %d/%d", reviewed.QualityScore, reviewed.RoleDiversity)
	}
}

func TestApplyReviewsIsolatesMalformedAndMissingResponses(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	directions := directionsFromLegacy(t, legacy)
	directions.Directions = directions.Directions[:3]
	malformed := directReview(directions.Directions[0])
	malformed.Reviews[0].Fit = AnchorFit("unknown")
	accepted := directReview(directions.Directions[1])

	reduction, err := ApplyReviews(bundle, directions, []ReviewProposal{malformed, accepted})
	if err != nil {
		t.Fatal(err)
	}
	if reduction.Proposed != 3 || reduction.Reviewed != 1 ||
		len(reduction.Directions) != 1 || reduction.Directions[0].DirectionID != directions.Directions[1].DirectionID {
		t.Fatalf("reduction = %#v", reduction)
	}
	if !hasReviewIssue(reduction, directions.Directions[0].DirectionID, "review_malformed") ||
		!hasReviewIssue(reduction, directions.Directions[2].DirectionID, "review_missing") {
		t.Fatalf("issues = %#v", reduction.Issues)
	}
}

func TestApplyReviewsRejectsDirectionLevelQualityFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prepare  func(*Bundle, *DirectionCandidate, *ReviewProposal)
		wantCode string
	}{
		{
			name: "fewer than three strong fits",
			prepare: func(_ *Bundle, _ *DirectionCandidate, review *ReviewProposal) {
				review.Reviews[1].Fit = AnchorFitWeak
			},
			wantCode: "fewer_than_three_direct_or_supporting_anchors",
		},
		{
			name: "question broader in majority",
			prepare: func(_ *Bundle, _ *DirectionCandidate, review *ReviewProposal) {
				review.Reviews[0].OverclaimReasons = []OverclaimReason{OverclaimQuestionScopeBroader}
				review.Reviews[1].OverclaimReasons = []OverclaimReason{OverclaimQuestionScopeBroader}
			},
			wantCode: "question_scope_broader",
		},
		{
			name: "learning outcome broader in majority",
			prepare: func(_ *Bundle, _ *DirectionCandidate, review *ReviewProposal) {
				review.Reviews[0].OverclaimReasons = []OverclaimReason{OverclaimLearningOutcomeScopeBroader}
				review.Reviews[1].OverclaimReasons = []OverclaimReason{OverclaimLearningOutcomeScopeBroader}
			},
			wantCode: "learning_outcome_scope_broader",
		},
		{
			name: "unsupported order remains in the question",
			prepare: func(_ *Bundle, direction *DirectionCandidate, review *ReviewProposal) {
				direction.Question = "How does this code preserve the execution order?"
				review.Reviews[0].OverclaimReasons = []OverclaimReason{OverclaimUnsupportedRuntimeOrder}
			},
			wantCode: "unsupported_runtime_order",
		},
		{
			name: "unknown anchor",
			prepare: func(_ *Bundle, _ *DirectionCandidate, review *ReviewProposal) {
				review.Reviews[2].AnchorID = "fact-8"
			},
			wantCode: "review_anchor_unknown",
		},
		{
			name: "production anchor removed",
			prepare: func(bundle *Bundle, direction *DirectionCandidate, review *ReviewProposal) {
				direction.AnchorIDs = []string{"fact-1", "fact-2", "fact-3", "fact-4"}
				direction.ReadingAnchors = readingAnchors(direction.AnchorIDs)
				for _, anchorID := range []string{"fact-2", "fact-3", "fact-4"} {
					bundle.Anchors[anchorIndexByID(t, *bundle, anchorID)].Role = artifactrole.RoleTest
				}
				*review = directReview(*direction)
				review.Reviews[0].Fit = AnchorFitIrrelevant
			},
			wantCode: "production_or_operational_anchor_missing",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			bundle, legacy := studyMapFixture(t)
			direction := directionsFromLegacy(t, legacy).Directions[0]
			review := directReview(direction)
			test.prepare(&bundle, &direction, &review)
			direction = normalizeDirectionForTest(t, direction)
			review.DirectionID = direction.DirectionID
			proposal := DirectionProposal{
				Version: DirectionProposalVersion, Directions: []DirectionCandidate{direction},
			}
			reduction, err := ApplyReviews(bundle, proposal, []ReviewProposal{review})
			if err != nil {
				t.Fatal(err)
			}
			if reduction.Reviewed != 0 || !hasReviewIssue(reduction, direction.DirectionID, test.wantCode) {
				t.Fatalf("reduction = %#v, want %q", reduction, test.wantCode)
			}
		})
	}
}

func TestApplyReviewsPreservesEditorialReadingOrder(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	direction := directionsFromLegacy(t, legacy).Directions[0]
	direction.AnchorIDs = []string{"fact-3", "fact-1", "fact-2"}
	direction.ReadingAnchors = []ReadingAnchor{
		{AnchorID: "fact-2", Label: "Start here", WhatToLookFor: "Inspect the bounded declaration."},
		{AnchorID: "fact-3", Label: "Then inspect", WhatToLookFor: "Inspect the bounded declaration."},
		{AnchorID: "fact-1", Label: "Related implementation", WhatToLookFor: "Inspect the bounded declaration."},
	}
	direction = normalizeDirectionForTest(t, direction)
	review := directReview(direction)
	reduction, err := ApplyReviews(
		bundle,
		DirectionProposal{Version: DirectionProposalVersion, Directions: []DirectionCandidate{direction}},
		[]ReviewProposal{review},
	)
	if err != nil {
		t.Fatal(err)
	}
	if reduction.Reviewed != 1 {
		t.Fatalf("reduction = %#v", reduction)
	}
	got := reduction.Directions[0].Candidate.ReadingAnchors
	if len(got) != 3 || got[0].Label != "Start here" || got[0].AnchorID != "fact-2" {
		t.Fatalf("reading order = %#v", got)
	}
}

func TestApplyReviewsScoresQuestionFitFromLocalSources(t *testing.T) {
	t.Parallel()

	bundle, directions := questionFitFixture(t)
	directions, err := NormalizeDirectionProposal(directions)
	if err != nil {
		t.Fatal(err)
	}
	reviews := make([]ReviewProposal, 0, len(directions.Directions))
	for _, direction := range directions.Directions {
		reviews = append(reviews, directReview(direction))
	}
	reduction, err := ApplyReviews(bundle, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if len(reduction.Directions) != 2 {
		t.Fatalf("reviewed directions = %d", len(reduction.Directions))
	}
	relevant := reduction.Directions[0]
	irrelevant := reduction.Directions[1]
	if relevant.QuestionFitScore <= 0 || relevant.QuestionFitScore <= irrelevant.QuestionFitScore {
		t.Fatalf(
			"question fit did not prefer local storage/backend/interface evidence: relevant=%d irrelevant=%d",
			relevant.QuestionFitScore,
			irrelevant.QuestionFitScore,
		)
	}
}

func TestCompressReviewedDirectionsPrefersQuestionFitOnTies(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	directions := directionsFromLegacy(t, legacy)
	reviews := make([]ReviewProposal, 0, len(directions.Directions))
	for _, direction := range directions.Directions {
		reviews = append(reviews, directReview(direction))
	}
	reduction, err := ApplyReviews(bundle, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if len(reduction.Directions) <= MaxReviewedDirections {
		t.Fatalf("fixture has %d reviewed directions, need more than %d", len(reduction.Directions), MaxReviewedDirections)
	}
	for index := range reduction.Directions {
		reduction.Directions[index].Candidate.TargetJob = JobContribute
		reduction.Directions[index].Candidate.LearningStage = StageOperations
		reduction.Directions[index].QualityScore = 10
		reduction.Directions[index].RoleDiversity = 1
		reduction.Directions[index].QuestionFitScore = 0
	}
	lateFitID := reduction.Directions[len(reduction.Directions)-1].DirectionID
	reduction.Directions[len(reduction.Directions)-1].QuestionFitScore = 50

	selected := CompressReviewedDirections(bundle, reduction.Directions)
	if !slices.ContainsFunc(selected, func(direction ReviewedDirection) bool {
		return direction.DirectionID == lateFitID
	}) {
		t.Fatalf("late high-fit direction %q was displaced: %#v", lateFitID, selected)
	}
}

func TestComposeReviewedRecordCompressesDuplicatesAndKeepsV1Contract(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	brief := briefShapeFromLegacy(legacy)
	directions := directionsFromLegacy(t, legacy)
	duplicateID := directions.Directions[len(directions.Directions)-1].DirectionID
	directions.Directions[len(directions.Directions)-1].Question = directions.Directions[0].Question
	directions.Directions[len(directions.Directions)-1].LearningOutcome = directions.Directions[0].LearningOutcome
	directions.Directions[len(directions.Directions)-1].DirectionID = ""
	var err error
	directions, err = NormalizeDirectionProposal(directions)
	if err != nil {
		t.Fatal(err)
	}
	reviews := make([]ReviewProposal, 0, len(directions.Directions))
	for _, direction := range directions.Directions {
		reviews = append(reviews, directReview(direction))
	}
	for index := range reviews[len(reviews)-1].Reviews {
		reviews[len(reviews)-1].Reviews[index].OverclaimReasons = []OverclaimReason{OverclaimVagueOrGeneric}
	}

	proposal, reduction, err := ComposeReviewedProposal(bundle, brief, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Candidates) < MinReviewedDirections || len(proposal.Candidates) > MaxReviewedDirections {
		t.Fatalf("composed candidates = %d", len(proposal.Candidates))
	}
	if reduction.Proposed != len(directions.Directions) ||
		reduction.Reviewed != len(directions.Directions) || reduction.Selected != len(proposal.Candidates) {
		t.Fatalf("reduction = %#v", reduction)
	}
	for _, candidate := range proposal.Candidates {
		if candidate.Confidence != "" {
			t.Fatalf("composed proposal retained model confidence %q", candidate.Confidence)
		}
	}
	questionCount := 0
	for _, candidate := range proposal.Candidates {
		if candidate.Question == directions.Directions[0].Question {
			questionCount++
		}
	}
	if questionCount != 1 {
		t.Fatalf("semantic duplicate count = %d; lower-quality %q survived", questionCount, duplicateID)
	}

	record, recordReduction, err := BuildReviewedRecord(bundle, brief, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if record.Version != RecordVersion || RecordVersion != 1 {
		t.Fatalf("record version = %d", record.Version)
	}
	if len(record.Directions) < MinReviewedDirections || len(record.Directions) > MaxReviewedDirections ||
		recordReduction.Selected != len(record.Directions) {
		t.Fatalf("record/reduction = %d/%#v", len(record.Directions), recordReduction)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCompressReviewedDirectionsReservesFirstContact(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	directions := directionsFromLegacy(t, legacy)
	reviews := make([]ReviewProposal, 0, len(directions.Directions))
	for _, direction := range directions.Directions {
		reviews = append(reviews, directReview(direction))
	}
	reduction, err := ApplyReviews(bundle, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if len(reduction.Directions) <= MaxReviewedDirections {
		t.Fatalf("fixture has %d reviewed directions, need more than %d", len(reduction.Directions), MaxReviewedDirections)
	}
	for index := range reduction.Directions {
		reduction.Directions[index].Candidate.TargetJob = JobContribute
		reduction.Directions[index].QualityScore = 100 - index
	}
	firstContactID := reduction.Directions[len(reduction.Directions)-1].DirectionID
	reduction.Directions[len(reduction.Directions)-1].Candidate.TargetJob = JobFirstContact
	reduction.Directions[len(reduction.Directions)-1].QualityScore = 1

	selected := CompressReviewedDirections(bundle, reduction.Directions)
	if !slices.ContainsFunc(selected, func(direction ReviewedDirection) bool {
		return direction.DirectionID == firstContactID
	}) {
		t.Fatalf("first-contact direction %q was displaced: %#v", firstContactID, selected)
	}
}

func TestBuildReviewedRecordTreatsBriefSupportIDsAsUnordered(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	brief := briefShapeFromLegacy(legacy)
	brief.Brief.WhatItIs.SupportIDs = []string{bundle.Anchors[0].ID, bundle.Areas[0].ID}
	if brief.Brief.WhatItIs.SupportIDs[0] < brief.Brief.WhatItIs.SupportIDs[1] {
		brief.Brief.WhatItIs.SupportIDs[0], brief.Brief.WhatItIs.SupportIDs[1] =
			brief.Brief.WhatItIs.SupportIDs[1], brief.Brief.WhatItIs.SupportIDs[0]
	}
	directions := directionsFromLegacy(t, legacy)
	reviews := make([]ReviewProposal, 0, len(directions.Directions))
	for _, direction := range directions.Directions {
		reviews = append(reviews, directReview(direction))
	}

	record, _, err := BuildReviewedRecord(bundle, brief, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
}

func briefShapeFromLegacy(proposal Proposal) BriefShapeProposal {
	return BriefShapeProposal{
		Version: BriefShapeProposalVersion, RepositoryType: proposal.RepositoryType,
		Brief: proposal.Brief, ShapeAreaIDs: append([]string(nil), proposal.ShapeAreaIDs...),
	}
}

func rawDirectionsFromLegacy(proposal Proposal) DirectionProposal {
	result := DirectionProposal{Version: DirectionProposalVersion}
	for _, candidate := range proposal.Candidates {
		result.Directions = append(result.Directions, DirectionCandidate{
			Question: candidate.Question, WhyItMatters: candidate.WhyItMatters,
			LearningOutcome: candidate.LearningOutcome, TargetJob: candidate.TargetJob,
			LearningStage: candidate.LearningStage,
			AnchorIDs:     append([]string(nil), candidate.AnchorIDs...),
			DocumentIDs:   append([]string(nil), candidate.DocumentIDs...),
			AreaIDs:       append([]string(nil), candidate.AreaIDs...), MechanismID: candidate.MechanismID,
			ReadingAnchors: append([]ReadingAnchor(nil), candidate.ReadingAnchors...),
			SearchQueries:  append([]string(nil), candidate.SearchQueries...),
		})
	}
	return result
}

func directionsFromLegacy(t *testing.T, proposal Proposal) DirectionProposal {
	t.Helper()
	result, err := NormalizeDirectionProposal(rawDirectionsFromLegacy(proposal))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func normalizeDirectionForTest(t *testing.T, direction DirectionCandidate) DirectionCandidate {
	t.Helper()
	direction.DirectionID = ""
	proposal, err := NormalizeDirectionProposal(DirectionProposal{
		Version: DirectionProposalVersion, Directions: []DirectionCandidate{direction},
	})
	if err != nil {
		t.Fatal(err)
	}
	return proposal.Directions[0]
}

func directReview(direction DirectionCandidate) ReviewProposal {
	roles := []ReadingRole{
		ReadingRolePublicOrCLIEntry,
		ReadingRoleCoreOrchestration,
		ReadingRoleStateOrDataModel,
		ReadingRoleEffectOrIntegrationBoundary,
		ReadingRoleRepresentativeImplementation,
	}
	proposal := ReviewProposal{Version: ReviewProposalVersion, DirectionID: direction.DirectionID}
	for index, anchorID := range direction.AnchorIDs {
		proposal.Reviews = append(proposal.Reviews, reviewAnchor(
			anchorID, AnchorFitDirect, roles[index%len(roles)], OverclaimNone,
		))
	}
	return proposal
}

func reviewAnchor(
	anchorID string,
	fit AnchorFit,
	role ReadingRole,
	reason OverclaimReason,
) AnchorReview {
	return AnchorReview{
		AnchorID: anchorID, Fit: fit,
		SupportedObservation: "The visible function contains the selected local implementation.",
		Role:                 role, OverclaimReasons: []OverclaimReason{reason},
	}
}

func readingAnchors(anchorIDs []string) []ReadingAnchor {
	labels := []string{"Start here", "Then inspect", "Related implementation", "Public boundary", "Core data type"}
	result := make([]ReadingAnchor, 0, len(anchorIDs))
	for index, anchorID := range anchorIDs {
		result = append(result, ReadingAnchor{
			AnchorID: anchorID, Label: labels[index],
			WhatToLookFor: "Inspect the bounded declaration and its local implementation.",
		})
	}
	return result
}

func readingByID(reading []ReadingAnchor) map[string]ReadingAnchor {
	result := make(map[string]ReadingAnchor, len(reading))
	for _, anchor := range reading {
		result[anchor.AnchorID] = anchor
	}
	return result
}

func questionFitFixture(t *testing.T) (Bundle, DirectionProposal) {
	t.Helper()

	areas := []Area{
		{
			ID:             "area-storage",
			Name:           "Storage backend interface",
			Responsibility: "Defines the common storage backend interface.",
		},
		{
			ID:             "area-routing",
			Name:           "HTTP routing",
			Responsibility: "Routes HTTP requests to handlers.",
		},
	}
	anchorSpecs := []struct {
		id        string
		path      string
		symbol    string
		statement string
		areaID    string
	}{
		{"fact-storage", "internal/backend/storage.go", "storageBackend", "Storage backend accepts blob writes.", "area-storage"},
		{"fact-backend", "internal/backend/backend.go", "backendFactory", "Backend factory creates storage backends.", "area-storage"},
		{"fact-interface", "internal/backend/interface.go", "commonInterface", "Common interface methods hide backend details.", "area-storage"},
		{"fact-router", "internal/http/router.go", "routeRequest", "Router dispatches requests.", "area-routing"},
		{"fact-cache", "internal/http/cache.go", "responseCache", "Cache stores HTTP responses.", "area-routing"},
		{"fact-log", "internal/http/log.go", "requestLogger", "Logger records request metadata.", "area-routing"},
	}
	allowed := []string{"docs/storage.md", "docs/routing.md"}
	anchors := make([]Anchor, 0, len(anchorSpecs))
	for index, spec := range anchorSpecs {
		function := testFunction(t, spec.path, spec.symbol, 10+index*20)
		anchors = append(anchors, Anchor{
			ID: spec.id, Path: spec.path, Symbol: spec.symbol,
			Line: function.StartLine + 1, Role: artifactrole.RoleProductionCore,
			Statement: spec.statement, AreaIDs: []string{spec.areaID}, Function: function,
		})
		allowed = append(allowed, spec.path)
	}
	bundle := Bundle{
		Version: BundleVersion, RepoName: "fixture",
		DocumentedPurpose:  "Fixture demonstrates local question fit.",
		RepositoryTypeHint: RepositoryLibrary,
		Areas:              areas,
		Anchors:            anchors,
		Documents: []Document{
			{
				ID:      "doc-storage",
				Path:    "docs/storage.md",
				Label:   "Storage backends",
				Excerpt: "Storage backends expose a common interface.",
			},
			{
				ID:      "doc-routing",
				Path:    "docs/routing.md",
				Label:   "Routing",
				Excerpt: "HTTP routes are dispatched to request handlers.",
			},
		},
		AllowedPaths: allowed,
	}
	question := "How do storage backends expose a common interface?"
	proposal := DirectionProposal{
		Version: DirectionProposalVersion,
		Directions: []DirectionCandidate{
			{
				Question: question, WhyItMatters: "This explains a useful repository responsibility.",
				LearningOutcome: "The reader can name the relevant code and responsibility.",
				TargetJob:       JobMaintain, LearningStage: StageCoreModel,
				AnchorIDs:      []string{"fact-storage", "fact-backend", "fact-interface"},
				DocumentIDs:    []string{"doc-storage"},
				AreaIDs:        []string{"area-storage"},
				ReadingAnchors: readingAnchors([]string{"fact-storage", "fact-backend", "fact-interface"}),
			},
			{
				Question: question, WhyItMatters: "This explains a useful repository responsibility.",
				LearningOutcome: "The reader can name the relevant code and responsibility.",
				TargetJob:       JobMaintain, LearningStage: StageCoreModel,
				AnchorIDs:      []string{"fact-router", "fact-cache", "fact-log"},
				DocumentIDs:    []string{"doc-routing"},
				AreaIDs:        []string{"area-routing"},
				ReadingAnchors: readingAnchors([]string{"fact-router", "fact-cache", "fact-log"}),
			},
		},
	}
	return bundle, proposal
}

func largeReviewFunction(
	t *testing.T,
	filePath string,
	symbol string,
	startLine int,
	bodyLines int,
	width int,
) sourcewindowfacts.Function {
	t.Helper()
	lines := []string{"func " + symbol + "() int {", "\tvalue := 1"}
	for index := 0; index < bodyLines; index++ {
		lines = append(lines, fmt.Sprintf("\t_ = \"%03d-%s\"", index, strings.Repeat("x", width)))
	}
	lines = append(lines, "\treturn value", "}")
	window, err := sourcewindowfacts.NewWindow("review-"+symbol, filePath, startLine, lines)
	if err != nil {
		t.Fatal(err)
	}
	function, err := sourcewindowfacts.ExtractGoFunction(window, symbol)
	if err != nil {
		t.Fatal(err)
	}
	return function
}

func anchorIndexByID(t *testing.T, bundle Bundle, anchorID string) int {
	t.Helper()
	for index, anchor := range bundle.Anchors {
		if anchor.ID == anchorID {
			return index
		}
	}
	t.Fatalf("missing fixture anchor %q", anchorID)
	return -1
}

func hasReviewIssue(reduction ReviewReduction, directionID, code string) bool {
	for _, issue := range reduction.Issues {
		if issue.DirectionID == directionID && issue.Code == code {
			return true
		}
	}
	return false
}

func mustEditingJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
