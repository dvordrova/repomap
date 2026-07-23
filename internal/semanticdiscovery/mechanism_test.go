package semanticdiscovery

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestExtractMechanismProjectsExistingArtifact(t *testing.T) {
	fixture, record, identity, probe := mechanismTestSource(t)
	mechanism, projected, err := ExtractMechanism(
		fixture.bundle,
		record,
		fixture.candidate.ID,
		identity,
		probe,
	)
	if err != nil {
		t.Fatal(err)
	}
	original, err := MaterializeArtifacts(
		fixture.bundle,
		[]LeafResult{fixture.leaf},
		fixture.artifact,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(original) != 1 || !reflect.DeepEqual(projected, original[0]) {
		t.Fatalf("scoped projection differs from existing artifact\nprojected: %#v\noriginal: %#v", projected, original)
	}
	if mechanism.ID == projected.ID || mechanism.ID == fixture.candidate.ID {
		t.Fatalf("logical mechanism id reused an unstable projection id: %q", mechanism.ID)
	}
	if len(mechanism.Input.Facts) != len(candidateFactIDs(fixture.candidate)) {
		t.Fatalf("manifest facts = %d, want %d", len(mechanism.Input.Facts), len(candidateFactIDs(fixture.candidate)))
	}
	roles := map[MechanismFactRole]int{}
	for _, input := range mechanism.Input.Facts {
		roles[input.Role]++
	}
	if roles[MechanismFactClaimSupport] != 5 ||
		roles[MechanismFactAvailableUnused] != 3 ||
		roles[MechanismFactCandidateSeed] != 0 {
		t.Fatalf("manifest fact roles = %#v", roles)
	}

	raw, err := EncodeMechanism(mechanism)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMechanism(raw)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ReplayMechanism(fixture.bundle, probe, decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replayed, projected) {
		t.Fatalf("decoded replay differs from extracted projection")
	}
}

func TestMechanismIdentityIgnoresEditorialProjection(t *testing.T) {
	fixture, record, identity, probe := mechanismTestSource(t)
	mechanism, original, err := ExtractMechanism(
		fixture.bundle,
		record,
		fixture.candidate.ID,
		identity,
		probe,
	)
	if err != nil {
		t.Fatal(err)
	}
	originalID := mechanism.ID
	originalSHA := mechanism.ContentSHA256

	mechanism.Payload.Proposal.Title = "Directory listing walkthrough"
	slices.Reverse(mechanism.Payload.Proposal.Claims)
	mechanism.ContentSHA256, err = MechanismContentHash(mechanism)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ReplayMechanism(fixture.bundle, probe, mechanism)
	if err != nil {
		t.Fatal(err)
	}
	if mechanism.ID != originalID {
		t.Fatalf("editorial edit changed logical id: %q != %q", mechanism.ID, originalID)
	}
	if mechanism.ContentSHA256 == originalSHA {
		t.Fatal("editorial edit did not change content hash")
	}
	if replayed.Title == original.Title || reflect.DeepEqual(replayed.Steps, original.Steps) {
		t.Fatalf("editorial projection was not updated: %#v", replayed)
	}
}

func TestReplayMechanismIgnoresPlannerUnboundFactsAndFocus(t *testing.T) {
	fixture, record, identity, probe := mechanismTestSource(t)
	mechanism, _, err := ExtractMechanism(
		fixture.bundle,
		record,
		fixture.candidate.ID,
		identity,
		probe,
	)
	if err != nil {
		t.Fatal(err)
	}

	changed := cloneJSON(t, fixture.bundle)
	changed.RepoName = "renamed-report"
	changed.PlannerContext = []PlannerContext{{
		Kind: PlannerContextGuidedTour,
		Text: "Completely revised presentation-only onboarding prose",
	}}
	changed.Facts = append(changed.Facts, Fact{
		ID:           "fact-unbound-new",
		Kind:         FactWarning,
		Statement:    "An unrelated saved warning changed",
		SourceGroup:  "group-unbound-new",
		Capabilities: []Capability{CapabilityLimitation},
		Scope:        FactScopeRepository,
	})
	for index := range changed.Facts {
		if changed.Facts[index].ID == goldenFactTrigger {
			changed.Facts[index].Focus = Focus{ComponentIDs: []string{"component-new-navigation"}}
		}
	}
	replayed, err := ReplayMechanism(changed, probe, mechanism)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(replayed.Focus.ComponentIDs, "component-new-navigation") {
		t.Fatalf("current presentation focus was not rematerialized: %#v", replayed.Focus)
	}
}

func TestReplayMechanismRejectsChangedBoundInputs(t *testing.T) {
	fixture, record, identity, probe := mechanismTestSource(t)
	mechanism, _, err := ExtractMechanism(
		fixture.bundle,
		record,
		fixture.candidate.ID,
		identity,
		probe,
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("changed fact", func(t *testing.T) {
		changed := cloneJSON(t, fixture.bundle)
		for index := range changed.Facts {
			if changed.Facts[index].ID == goldenFactOutput {
				changed.Facts[index].Statement = "The response output fact changed"
			}
		}
		_, err := ReplayMechanism(changed, probe, mechanism)
		if err == nil || !strings.Contains(err.Error(), "bound fact") {
			t.Fatalf("ReplayMechanism() error = %v, want bound fact rejection", err)
		}
	})

	t.Run("missing fact", func(t *testing.T) {
		changed := cloneJSON(t, fixture.bundle)
		changed.Facts = slices.DeleteFunc(changed.Facts, func(fact Fact) bool {
			return fact.ID == goldenFactOutput
		})
		_, err := ReplayMechanism(changed, probe, mechanism)
		if err == nil || !strings.Contains(err.Error(), "missing") {
			t.Fatalf("ReplayMechanism() error = %v, want missing fact rejection", err)
		}
	})

	t.Run("probe", func(t *testing.T) {
		changed := probe
		changed.SHA256 = strings.Repeat("b", 64)
		_, err := ReplayMechanism(fixture.bundle, changed, mechanism)
		if err == nil || !strings.Contains(err.Error(), "probe") {
			t.Fatalf("ReplayMechanism() error = %v, want probe rejection", err)
		}
	})

	t.Run("contract", func(t *testing.T) {
		changed := cloneJSON(t, mechanism)
		changed.Payload.Candidate.IntentContract.MinCovered--
		changed.ContentSHA256, err = MechanismContentHash(changed)
		if err != nil {
			t.Fatal(err)
		}
		_, err := ReplayMechanism(fixture.bundle, probe, changed)
		if err == nil || !strings.Contains(err.Error(), "contract") {
			t.Fatalf("ReplayMechanism() error = %v, want contract rejection", err)
		}
	})

	t.Run("claim", func(t *testing.T) {
		changed := cloneJSON(t, mechanism)
		changed.Payload.Proposal.Claims[0].Text = "An unsupported replacement claim"
		changed.ContentSHA256, err = MechanismContentHash(changed)
		if err != nil {
			t.Fatal(err)
		}
		_, err := ReplayMechanism(fixture.bundle, probe, changed)
		if err == nil {
			t.Fatal("ReplayMechanism() accepted a changed unsupported claim")
		}
	})
}

func TestDecodeMechanismRejectsUnknownFieldsAndInvalidContentHash(t *testing.T) {
	fixture, record, identity, probe := mechanismTestSource(t)
	mechanism, _, err := ExtractMechanism(
		fixture.bundle,
		record,
		fixture.candidate.ID,
		identity,
		probe,
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeMechanism(mechanism)
	if err != nil {
		t.Fatal(err)
	}
	withUnknown := append([]byte(nil), raw[:len(raw)-1]...)
	withUnknown = append(withUnknown, []byte(`,"unknown":true}`)...)
	if _, err := DecodeMechanism(withUnknown); err == nil {
		t.Fatal("DecodeMechanism() accepted an unknown field")
	}

	changed := cloneJSON(t, mechanism)
	changed.Payload.Proposal.Title = "Changed without rehashing"
	changedRaw, err := hashJSONBytes(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeMechanism(changedRaw); err == nil ||
		!strings.Contains(err.Error(), "content hash") {
		t.Fatalf("DecodeMechanism() error = %v, want content hash rejection", err)
	}
}

func mechanismTestSource(
	t *testing.T,
) (goldenMechanismFixture, Record, MechanismIdentity, MechanismProbeInput) {
	t.Helper()
	fixture := newGoldenMechanismFixture(t)
	raw, err := EncodeRecord(
		fixture.bundle,
		fixture.opportunity,
		[]OpportunityCandidate{fixture.candidate},
		[]LeafResult{fixture.leaf},
		fixture.artifact,
	)
	if err != nil {
		t.Fatal(err)
	}
	record, err := DecodeRecord(raw)
	if err != nil {
		t.Fatal(err)
	}
	identity := MechanismIdentity{
		RepositoryNamespace: "example.com/caddy/v2",
		IntentKey:           "caddy-directory-listing",
		Scope: MechanismScope{
			Kind:  MechanismScopeGoPackage,
			Value: "example.com/caddy/v2/fileserver",
		},
	}
	probe := MechanismProbeInput{
		ContractVersion: 1,
		ID:              identity.IntentKey,
		SHA256:          strings.Repeat("a", 64),
	}
	return fixture, record, identity, probe
}

func hashJSONBytes(value any) ([]byte, error) {
	_, raw, err := hashJSON("test json", value)
	return raw, err
}
