package report

import "testing"

func TestD241ArchitectureStatusV15GatesItemLocalSalvageCodes(t *testing.T) {
	t.Parallel()

	if ArchitectureSynthesisStatusVersion != 17 {
		t.Fatalf("Architecture status version = %d, want 17", ArchitectureSynthesisStatusVersion)
	}
	for _, code := range []string{
		"proposal.empty_component",
		"proposal.supporting_only_unit_coverage_salvaged",
	} {
		t.Run(code, func(t *testing.T) {
			current := architectureSynthesisV4AcceptedFixture()
			current.ValidationCodes = []string{code}
			if err := current.Validate(); err != nil {
				t.Fatalf("current status rejected D241 code %q: %v", code, err)
			}

			d241 := current
			d241.Version = 15
			if err := d241.Validate(); err != nil {
				t.Fatalf("v15 rejected its own D241 code %q: %v", code, err)
			}

			historical := current
			historical.Version = 14
			if err := historical.Validate(); err == nil {
				t.Fatalf("v14 accepted v15-only code %q", code)
			}
		})
	}

	unknown := architectureSynthesisV4AcceptedFixture()
	unknown.ValidationCodes = []string{"proposal.future_item_local_salvage"}
	if err := unknown.Validate(); err == nil {
		t.Fatal("v15 accepted an open validation code")
	}
}
