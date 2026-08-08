package themestudy

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestExpandFilesUnreadableSiblingClosesAndPublishedThemePipelineContinues(t *testing.T) {
	t.Parallel()

	files := []FileRef{
		{Ref: "f1", Path: "missing.go"},
		{Ref: "f2", Path: "accepted.go"},
	}
	expansion, err := ExpandFiles(
		files,
		func(path string, _, _ int) ([]string, error) {
			if path != "accepted.go" {
				return nil, errors.New("source must not be read after line-count failure")
			}
			return []string{"package accepted"}, nil
		},
		func(path string) (int, error) {
			if path == "missing.go" {
				return 0, errors.New("permission denied: private path must not persist")
			}
			return 1, nil
		},
	)
	if err != nil {
		t.Fatalf("unreadable sibling terminated source expansion: %v", err)
	}
	if len(expansion.Files) != 2 {
		t.Fatalf("files = %#v, want one closed file plus one accepted sibling", expansion.Files)
	}
	closed, accepted := expansion.Files[0], expansion.Files[1]
	if !closed.Closed || closed.ClosedReason != ExpansionClosedReasonUnreadable ||
		closed.Ref != "f1" || len(closed.Objects) != 0 || closed.ExpandedLines != 0 {
		t.Fatalf("unreadable file closure = %#v", closed)
	}
	if accepted.Closed || accepted.Ref != "f2" || len(accepted.Objects) != 1 ||
		!reflect.DeepEqual(accepted.Objects[0].Lines, []string{"package accepted"}) {
		t.Fatalf("accepted sibling = %#v", accepted)
	}
	if !reflect.DeepEqual(expansion.OmittedRefs, []string{"f1"}) ||
		expansion.ExpandedLines != 1 || expansion.ExpandedBytes != len("package accepted") {
		t.Fatalf("expansion accounting = omitted %#v lines=%d bytes=%d",
			expansion.OmittedRefs, expansion.ExpandedLines, expansion.ExpandedBytes)
	}

	encodedExpansion, err := EncodeExpansion(expansion)
	if err != nil {
		t.Fatalf("EncodeExpansion after item-local closure: %v", err)
	}
	decodedExpansion, err := DecodeExpansion(encodedExpansion)
	if err != nil {
		t.Fatalf("DecodeExpansion after item-local closure: %v", err)
	}
	if !reflect.DeepEqual(decodedExpansion, expansion) {
		t.Fatalf("expansion round trip changed closure:\nwant=%#v\ngot=%#v", expansion, decodedExpansion)
	}

	candidates := []ScoutCandidate{{
		Ref: "t1", Title: "Accepted source", Question: "What does the accepted source establish?",
		ThemeKind: KindUserJourney, AnchorRefs: []string{"a1"},
		ExpansionFileRefs: []string{"f1", "f2"},
		WhyItMatters:      "It preserves a usable sibling.", ExpectedLearning: "Read the accepted source.",
		RelationClaim: RelationClaimEditorialOnly,
	}}
	anchors := map[string]AnchorInfo{
		"a1": {Path: "accepted.go", Symbol: "accepted.Main", Line: 1},
	}
	seedPacks := []SeedPack{{
		Seed: SeedSpec{Ref: "a1", Path: "accepted.go", Symbol: "accepted.Main", Line: 1},
		Objects: []SourceObject{{
			Role: SourceRoleDeclaration, Path: "accepted.go", Line: 1,
			Lines: []string{"package accepted"}, FullBody: true,
		}},
	}}
	request, err := CompileAdjudication(LanguageEnglish, candidates, expansion, anchors, seedPacks)
	if err != nil {
		t.Fatalf("adjudication compile stopped after sibling closure: %v", err)
	}
	var wire wireAdjudication
	if err := json.Unmarshal([]byte(request.WireJSON), &wire); err != nil {
		t.Fatalf("decode adjudication wire: %v", err)
	}
	if _, leaked := wire.Sources["f1"]; leaked {
		t.Fatalf("closed source leaked into provider wire: %#v", wire.Sources["f1"])
	}
	if source, ok := wire.Sources["f2"]; !ok || len(source.Objects) != 1 {
		t.Fatalf("accepted sibling missing from provider wire: %#v", wire.Sources)
	}

	mock, err := MockAdjudicationResponse(request)
	if err != nil {
		t.Fatalf("build provider-free adjudication response: %v", err)
	}
	result, _, err := ReplayAdjudicationResponse(request, mock)
	if err != nil {
		t.Fatalf("adjudication replay after sibling closure: %v", err)
	}
	reduction, err := ReduceAccepted(candidates, result.Themes, anchors)
	if err != nil || len(reduction.Cards) != 1 {
		t.Fatalf("publishable reduction after sibling closure = %#v, err=%v", reduction, err)
	}
	if _, err := EncodeStudyThemes(StudyThemes{
		Version: StudyThemesVersion, Cards: reduction.Cards,
		Omitted: reduction.Omitted, CoProjected: reduction.CoProjected,
		Partial: reduction.Partial, Diagnostics: reduction.Diagnostics,
	}); err != nil {
		t.Fatalf("report-facing Study artifact did not continue: %v", err)
	}
}

func TestExpansionClosureIntegrityRejectsUnboundedOrContentBearingReason(t *testing.T) {
	t.Parallel()

	base := SourceExpansion{
		Version: expansionVersion, CandidateSHA256: "digest", OmittedRefs: []string{"f1"},
		Files: []ExpansionFile{{
			Ref: "f1", Path: "closed.go", Closed: true,
			ClosedReason: ExpansionClosedReasonUnreadable,
		}},
	}
	if _, err := EncodeExpansion(base); err != nil {
		t.Fatalf("bounded empty closure rejected: %v", err)
	}

	unbounded := base
	unbounded.Files = append([]ExpansionFile(nil), base.Files...)
	unbounded.Files[0].ClosedReason = "permission denied: /private/repository/closed.go"
	if _, err := EncodeExpansion(unbounded); err == nil {
		t.Fatal("artifact accepted an unbounded source-read error")
	}

	contentBearing := base
	contentBearing.Files = append([]ExpansionFile(nil), base.Files...)
	contentBearing.Files[0].Objects = []SourceObject{{Lines: []string{"private source prose"}}}
	if _, err := EncodeExpansion(contentBearing); err == nil {
		t.Fatal("artifact accepted source content inside a closed file")
	}
}

func TestExpandFilesInvalidReadShapeClosesAndContinuesSibling(t *testing.T) {
	t.Parallel()

	expansion, err := ExpandFiles(
		[]FileRef{
			{Ref: "f1", Path: "changed-during-read.go"},
			{Ref: "f2", Path: "stable.go"},
		},
		func(path string, _, _ int) ([]string, error) {
			if path == "changed-during-read.go" {
				return []string{"only one of two lines"}, nil
			}
			return []string{"package stable"}, nil
		},
		func(path string) (int, error) {
			if path == "changed-during-read.go" {
				return 2, nil
			}
			return 1, nil
		},
	)
	if err != nil {
		t.Fatalf("invalid sibling terminated expansion: %v", err)
	}
	if len(expansion.Files) != 2 ||
		!expansion.Files[0].Closed ||
		expansion.Files[0].ClosedReason != ExpansionClosedReasonInvalid ||
		expansion.Files[1].Closed || len(expansion.Files[1].Objects) != 1 {
		t.Fatalf("invalid/accepted sibling projection = %#v", expansion.Files)
	}
	if !reflect.DeepEqual(expansion.OmittedRefs, []string{"f1"}) {
		t.Fatalf("omitted refs = %#v, want [f1]", expansion.OmittedRefs)
	}
}
