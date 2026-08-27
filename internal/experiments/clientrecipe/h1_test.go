package clientrecipe

import (
	"bytes"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestH1StructuralExtraction(t *testing.T) {
	repoRoot := filepath.Join(experimentRoot(t), "repo")
	loader := &countingProductionLoader{delegate: defaultProductionPackageLoader{}}
	authority, err := prepareAuthority(t.Context(), repoRoot, loader)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ExtractH1(repoRoot, authority)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ExtractH1(repoRoot, authority)
	if err != nil {
		t.Fatal(err)
	}
	if loader.count != 1 {
		t.Fatalf("package loads = %d after Authority plus two H1 extractions, want 1", loader.count)
	}
	firstRaw, err := EncodeH1(first)
	if err != nil {
		t.Fatal(err)
	}
	secondRaw, err := EncodeH1(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstRaw, secondRaw) {
		t.Fatal("H1 bytes changed across identical source/Authority input")
	}
	if err := first.ValidateAgainst(authority); err != nil {
		t.Fatal(err)
	}
	forgedCallbacks := first
	forgedCallbacks.Callbacks.Observed++
	forgedCallbacks.Callbacks.Frontier++
	forgedCallbacks, err = sealH1(forgedCallbacks)
	if err != nil {
		t.Fatal(err)
	}
	if err := forgedCallbacks.ValidateAgainst(authority); err == nil {
		t.Fatal("H1 accepted callback observations not bound to Authority")
	}
	decoded, err := DecodeH1(firstRaw)
	if err != nil {
		t.Fatal(err)
	}
	if err := decoded.ValidateAgainst(authority); err != nil {
		t.Fatal(err)
	}
	if err := decoded.ValidateAgainstRepository(repoRoot, authority); err != nil {
		t.Fatal(err)
	}
	assertStrictDecoder(t, firstRaw, func(candidate []byte) error {
		_, err := DecodeH1(candidate)
		return err
	})
	assertExperimentGolden(t, "03-h1-structural.json", firstRaw)

	if first.Ledger != (H1Ledger{Observed: 10, Admitted: 4, Excluded: 6}) {
		t.Fatalf("H1 ledger = %#v", first.Ledger)
	}
	if first.ObservedUniverse != (H1ObservedUniverse{
		H0Candidates: 6, GeneratedH0Groups: 1, QualifyingTestFakes: 2,
		ProseCandidates: 1, StdlibHelpers: 1,
	}) {
		t.Fatalf("H1 observed universe = %#v", first.ObservedUniverse)
	}
	if first.Callbacks.Observed != 4 || first.Callbacks.Closed != 2 || first.Callbacks.Frontier != 2 {
		t.Fatalf("H1 callbacks = %#v", first.Callbacks)
	}
	reachedNames := map[string]bool{"main": false, "Service.Run": false, "Handler.HandleStartup": false, "Service.Resolve": false}
	reached := stringSet(first.Reachability.ReachedObjectIDs)
	for _, object := range authority.Program.Objects {
		if _, found := reachedNames[object.Name]; found {
			_, isReached := reached[object.ID]
			reachedNames[object.Name] = isReached
		}
	}
	for name, found := range reachedNames {
		if !found {
			t.Errorf("main-to-callback reachability did not retain %s", name)
		}
	}

	type instanceWant struct {
		complete     bool
		verification string
		missing      []H1Role
	}
	wantInstances := map[string]instanceWant{
		"clickhouse": {complete: true, verification: "integration_test", missing: []H1Role{}},
		"kubernetes": {complete: true, verification: "unit_test", missing: []H1Role{}},
		"notifier": {
			complete: false, verification: "none",
			missing: []H1Role{H1RoleVerification, H1RoleObservability, H1RoleFailurePolicy},
		},
		"vault": {complete: true, verification: "unit_test", missing: []H1Role{}},
	}
	for _, instance := range first.Instances {
		name := path.Base(instance.ImporterRepositoryPath)
		want, found := wantInstances[name]
		if !found {
			t.Errorf("unexpected admitted instance %s", name)
			continue
		}
		if instance.Complete != want.complete || instance.VerificationKind != want.verification ||
			!reflect.DeepEqual(instance.Missing, want.missing) {
			t.Errorf("instance %s = complete %t, verification %s, missing %v", name,
				instance.Complete, instance.VerificationKind, instance.Missing)
		}
		delete(wantInstances, name)
	}
	if len(wantInstances) != 0 {
		t.Fatalf("missing instances: %#v", wantInstances)
	}
	for _, role := range first.Roles {
		wantCount, wantNecessity := 3, H1Required
		if role.Role == H1RoleFailurePolicy {
			wantCount, wantNecessity = 2, H1Common
		}
		if role.CompleteInstances != wantCount || role.Necessity != wantNecessity {
			t.Errorf("role %s = %d/%s, want %d/%s", role.Role, role.CompleteInstances,
				role.Necessity, wantCount, wantNecessity)
		}
	}
	reasons := map[H1ExclusionReason]int{}
	for _, row := range first.Excluded {
		reasons[row.Reason]++
	}
	wantReasons := map[H1ExclusionReason]int{
		H1ExcludedGenerated: 1, H1ExcludedTestOnly: 2,
		H1ExcludedNotProductionReachable: 2, H1ExcludedNotExternalBoundary: 1,
	}
	if !reflect.DeepEqual(reasons, wantReasons) {
		t.Fatalf("exclusion reasons = %#v, want %#v", reasons, wantReasons)
	}
}

func TestH1ImplementationHasNoEvaluatorOrFixtureClientLiterals(t *testing.T) {
	for _, filename := range []string{"h1.go", "h1_extract.go"} {
		raw := readExperimentFile(t, filepath.Join(filepath.Dir(sourceFile(t)), filename))
		lower := strings.ToLower(string(raw))
		for _, forbidden := range []string{"oracle", "vault", "kubernetes", "clickhouse", "notifier"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("H1 implementation %s contains evaluator/fixture literal %q", filename, forbidden)
			}
		}
	}
}
