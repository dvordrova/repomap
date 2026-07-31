package tasklens

import (
	"context"
	"slices"
	"testing"
)

func TestReduceProposalEmitsTypedWarningDiagnosticsBesideRawWarnings(t *testing.T) {
	repo := newTaskLensTestRepo(t, "typed-warning-diagnostics")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Areas = nil
	proposal.Joins = append(proposal.Joins, ProposedJoin{
		LeftID: proposal.Anchors[0].AnchorID, RightID: proposal.Anchors[1].AnchorID,
		Kind: "test_usage", SupportType: SupportDocument,
		SupportIDs:  bundle.Anchors[0].EvidenceIDs,
		Explanation: "The exact source and test are nearby.",
		Scope:       "This does not prove runtime execution.",
	})
	baseGuidance := proposal.ReproduceOrObserve[0]
	proposal.ReproduceOrObserve = nil
	for index := 0; index < MaxGuidanceSteps+1; index++ {
		proposal.ReproduceOrObserve = append(
			proposal.ReproduceOrObserve,
			baseGuidance,
		)
	}

	pack, warnings, diagnostics, err := ReduceProposalWithDiagnostics(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Anchors) == 0 || len(warnings) != len(diagnostics) {
		t.Fatalf(
			"reducer result warnings=%d diagnostics=%d pack=%#v",
			len(warnings),
			len(diagnostics),
			pack,
		)
	}
	wantCodes := []WarningCode{
		WarningAreaFallbackAdded,
		WarningJoinRejected,
		WarningReproductionDuplicate,
		WarningReproductionBounded,
	}
	gotCodes := make([]WarningCode, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		gotCodes = append(gotCodes, diagnostic.Code)
	}
	for _, want := range wantCodes {
		if !slices.Contains(gotCodes, want) {
			t.Errorf("typed reducer diagnostics %q omit %q", gotCodes, want)
		}
	}
	for index, diagnostic := range diagnostics {
		switch diagnostic.Code {
		case WarningJoinRejected:
			if diagnostic.Index <= 0 {
				t.Errorf("detail-bearing join diagnostic = %#v", diagnostic)
			}
		case WarningReproductionDuplicate:
			if diagnostic.Index <= 0 {
				t.Errorf("indexed duplicate diagnostic = %#v", diagnostic)
			}
		case WarningAreaFallbackAdded, WarningReproductionBounded:
			if diagnostic.Index != 0 {
				t.Errorf("non-indexed diagnostic %d = %#v", index, diagnostic)
			}
		}
	}

	ordinaryPack, ordinaryWarnings, err := ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(warnings, ordinaryWarnings) || ordinaryPack.ID != pack.ID {
		t.Fatalf(
			"typed reducer changed canonical result: warnings=%#v/%#v pack=%q/%q",
			warnings,
			ordinaryWarnings,
			pack.ID,
			ordinaryPack.ID,
		)
	}
}

func TestFixedTaskWarningsUseStructuralTypedEmissions(t *testing.T) {
	for _, test := range []struct {
		name string
		got  WarningEmission
		want WarningCode
	}{
		{
			name: "response size",
			got: func() WarningEmission {
				value, _ := RawResponseOmissionEmission(RawResponseOmittedSize)
				return value
			}(),
			want: WarningAttemptResponseSize,
		},
		{
			name: "provider failure",
			got: func() WarningEmission {
				value, _ := AttemptStateWarningEmission("provider_failed")
				return value
			}(),
			want: WarningAttemptProviderFailed,
		},
		{
			name: "model partial",
			got:  PartialPackWarningEmission("accepted_with_rejections"),
			want: WarningPackModelPartial,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.got.Raw == "" || test.got.Diagnostic.Code != test.want ||
				test.got.Diagnostic.Index != 0 {
				t.Fatalf("emission = %#v, want %q", test.got, test.want)
			}
		})
	}
}
