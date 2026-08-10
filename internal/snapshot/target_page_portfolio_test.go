package snapshot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestTargetPagePortfolioCanonicalRoundTripAndContainerBinding(t *testing.T) {
	container, appRef, helperRef, coreRef := targetPagePortfolioFixture(t)
	outcomes := []TargetPageOutcome{
		{TargetRef: coreRef, State: TargetPageReady, RunID: "20260810-020000-core-a1b2c3"},
		{TargetRef: appRef, State: TargetPageReady, RunID: "20260810-020000-app-d4e5f6"},
		{TargetRef: helperRef, State: TargetPageUnavailable, UnavailableCode: TargetPageUnavailableTargetRunFailed},
	}
	portfolio, err := BuildTargetPagePortfolio(container, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	if err := portfolio.ValidateAgainstContainer(container); err != nil {
		t.Fatalf("ValidateAgainstContainer: %v", err)
	}
	if portfolio.Version != TargetPagePortfolioVersion ||
		portfolio.TargetRunContainerSHA256 != container.SHA256 ||
		portfolio.TargetCatalogRef != container.CatalogRef || len(portfolio.Targets) != 3 {
		t.Fatalf("portfolio identity = %#v", portfolio)
	}
	wantRefs := []string{coreRef, appRef, helperRef}
	gotRefs := make([]string, 0, len(portfolio.Targets))
	for _, page := range portfolio.Targets {
		gotRefs = append(gotRefs, page.TargetRef)
	}
	if !slices.Equal(gotRefs, wantRefs) {
		t.Fatalf("canonical target refs = %#v, want %#v", gotRefs, wantRefs)
	}
	if portfolio.Targets[0].Default || !portfolio.Targets[1].Default || portfolio.Targets[2].Default ||
		portfolio.Targets[2].State != TargetPageUnavailable || portfolio.Targets[2].RunID != "" ||
		portfolio.Targets[2].UnavailableCode != TargetPageUnavailableTargetRunFailed {
		t.Fatalf("derived default/closed states = %#v", portfolio.Targets)
	}

	wire, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) > MaxTargetPagePortfolioBytes || bytes.Contains(wire, []byte("cmd/app")) ||
		bytes.Contains(wire, []byte("internal/core")) || bytes.Contains(wire, []byte("target analysis failed because")) {
		t.Fatalf("artifact leaked display path or prose: %s", wire)
	}
	artifactSHA, err := portfolio.ArtifactSHA256()
	if err != nil {
		t.Fatal(err)
	}
	wantArtifactSHA := sha256.Sum256(wire)
	if artifactSHA != hex.EncodeToString(wantArtifactSHA[:]) || artifactSHA == portfolio.SHA256 {
		t.Fatalf("artifact/self digests = %s / %s", artifactSHA, portfolio.SHA256)
	}

	decoded, err := DecodeTargetPagePortfolio(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, portfolio) {
		t.Fatalf("decoded portfolio = %#v, want %#v", decoded, portfolio)
	}
	decodedWire, err := decoded.CanonicalJSON()
	if err != nil || !bytes.Equal(decodedWire, wire) {
		t.Fatalf("canonical round trip: err=%v\n%s\n%s", err, wire, decodedWire)
	}

	reorderedOutcomes := slices.Clone(outcomes)
	slices.Reverse(reorderedOutcomes)
	reordered, err := BuildTargetPagePortfolio(container, reorderedOutcomes)
	if err != nil {
		t.Fatal(err)
	}
	reorderedWire, err := reordered.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wire, reorderedWire) {
		t.Fatalf("outcome order changed artifact:\n%s\n%s", wire, reorderedWire)
	}

	var encoded struct {
		Targets []map[string]json.RawMessage `json:"targets"`
	}
	if err := json.Unmarshal(wire, &encoded); err != nil {
		t.Fatal(err)
	}
	if _, found := encoded.Targets[0]["unavailable_code"]; found {
		t.Fatal("ready target encoded unavailable_code")
	}
	if _, found := encoded.Targets[2]["run_id"]; found {
		t.Fatal("unavailable target encoded run_id")
	}
}

func TestTargetPagePortfolioRejectsUnsafeRunIDsAndInvalidTerminalStates(t *testing.T) {
	container, appRef, helperRef, coreRef := targetPagePortfolioFixture(t)

	for _, runID := range []string{
		"a", "A0", "20260810-020000-repomap-a1b2c3", "run.v1_2-x",
	} {
		if err := ValidateTargetPageRunID(runID); err != nil {
			t.Errorf("valid run id %q: %v", runID, err)
		}
	}
	for _, runID := range []string{
		"", ".", "..", ".hidden", "run-", "run_", "run.", "../run", "run/child",
		`run\child`, "/absolute", "file:run", "run%2fchild", " run", "run ", "rún",
		"run\nchild", "CON", "nul.txt", "LPT1", "com9.run",
		strings.Repeat("a", maxTargetPageRunIDBytes+1),
	} {
		if err := ValidateTargetPageRunID(runID); err == nil {
			t.Errorf("accepted unsafe run id %q", runID)
		}
	}

	base := []TargetPageOutcome{
		{TargetRef: appRef, State: TargetPageReady, RunID: "run-app-1"},
		{TargetRef: helperRef, State: TargetPageUnavailable, UnavailableCode: TargetPageUnavailableTargetRunFailed},
		{TargetRef: coreRef, State: TargetPageReady, RunID: "run-core-1"},
	}
	mustReject := func(name string, outcomes []TargetPageOutcome) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			if _, err := BuildTargetPagePortfolio(container, outcomes); err == nil {
				t.Fatalf("accepted invalid outcomes %#v", outcomes)
			}
		})
	}
	mutate := func(index int, change func(*TargetPageOutcome)) []TargetPageOutcome {
		result := slices.Clone(base)
		change(&result[index])
		return result
	}

	mustReject("ready has code", mutate(0, func(outcome *TargetPageOutcome) {
		outcome.UnavailableCode = TargetPageUnavailableTargetRunFailed
	}))
	mustReject("ready unsafe run", mutate(0, func(outcome *TargetPageOutcome) { outcome.RunID = "../escape" }))
	mustReject("unavailable has run", mutate(1, func(outcome *TargetPageOutcome) { outcome.RunID = "partial-run" }))
	mustReject("unavailable lacks code", mutate(1, func(outcome *TargetPageOutcome) { outcome.UnavailableCode = "" }))
	mustReject("unavailable unknown code", mutate(1, func(outcome *TargetPageOutcome) {
		outcome.UnavailableCode = "report_failed"
	}))
	mustReject("unknown state", mutate(0, func(outcome *TargetPageOutcome) { outcome.State = "partial" }))
	mustReject("duplicate run", mutate(2, func(outcome *TargetPageOutcome) { outcome.RunID = "run-app-1" }))
	mustReject("duplicate target", []TargetPageOutcome{base[0], base[0], base[2]})
	mustReject("unknown target", mutate(1, func(outcome *TargetPageOutcome) {
		outcome.TargetRef = "at-000000000000000000000000"
	}))
	mustReject("invalid target ref", mutate(1, func(outcome *TargetPageOutcome) { outcome.TargetRef = "../target" }))
	mustReject("incomplete outcomes", base[:2])
	mustReject("no ready sibling", []TargetPageOutcome{
		{TargetRef: appRef, State: TargetPageUnavailable, UnavailableCode: TargetPageUnavailableTargetRunFailed},
		{TargetRef: helperRef, State: TargetPageUnavailable, UnavailableCode: TargetPageUnavailableTargetRunFailed},
		{TargetRef: coreRef, State: TargetPageUnavailable, UnavailableCode: TargetPageUnavailableTargetRunFailed},
	})

	// The backend-owned default remains marked even if that target failed. A
	// different ready sibling can still carry the byte-identical portfolio.
	defaultUnavailable := slices.Clone(base)
	defaultUnavailable[0] = TargetPageOutcome{
		TargetRef: appRef, State: TargetPageUnavailable,
		UnavailableCode: TargetPageUnavailableTargetRunFailed,
	}
	portfolio, err := BuildTargetPagePortfolio(container, defaultUnavailable)
	if err != nil {
		t.Fatalf("default unavailable with ready sibling: %v", err)
	}
	if !portfolio.Targets[1].Default || portfolio.Targets[1].State != TargetPageUnavailable {
		t.Fatalf("default marker drifted after failure: %#v", portfolio.Targets[1])
	}
}

func TestTargetPagePortfolioDecodeRejectsTamperAndNonCanonicalBytes(t *testing.T) {
	container, appRef, helperRef, coreRef := targetPagePortfolioFixture(t)
	portfolio := mustTargetPagePortfolio(t, container, []TargetPageOutcome{
		{TargetRef: appRef, State: TargetPageReady, RunID: "run-app-1"},
		{TargetRef: helperRef, State: TargetPageUnavailable, UnavailableCode: TargetPageUnavailableTargetRunFailed},
		{TargetRef: coreRef, State: TargetPageReady, RunID: "run-core-1"},
	})
	wire, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}

	withUnknown := bytes.Replace(wire, []byte(`{"version":1,`), []byte(`{"version":1,"unknown":true,`), 1)
	for name, raw := range map[string][]byte{
		"empty":          nil,
		"unknown field":  withUnknown,
		"trailing value": append(append([]byte(nil), wire...), []byte(` {}`)...),
		"newline":        append(append([]byte(nil), wire...), '\n'),
		"leading space":  append([]byte{' '}, wire...),
		"oversized":      bytes.Repeat([]byte{'x'}, MaxTargetPagePortfolioBytes+1),
		"seal tamper":    bytes.Replace(wire, []byte("run-app-1"), []byte("run-app-2"), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeTargetPagePortfolio(raw); err == nil {
				t.Fatalf("accepted invalid artifact: %s", raw)
			}
		})
	}

	// A self-consistent rewrite still cannot move a target, default, or
	// container identity when checked against the bound container authority.
	for name, mutate := range map[string]func(*TargetPagePortfolio){
		"target order": func(value *TargetPagePortfolio) {
			value.Targets[0], value.Targets[1] = value.Targets[1], value.Targets[0]
		},
		"default": func(value *TargetPagePortfolio) {
			value.Targets[1].Default = false
			value.Targets[0].Default = true
		},
		"container": func(value *TargetPagePortfolio) {
			value.TargetRunContainerSHA256 = strings.Repeat("0", sha256.Size*2)
		},
		"catalog": func(value *TargetPagePortfolio) {
			value.TargetCatalogRef = "atc-000000000000000000000000"
		},
	} {
		t.Run("resealed "+name, func(t *testing.T) {
			drifted := portfolio
			drifted.Targets = slices.Clone(portfolio.Targets)
			mutate(&drifted)
			drifted.SHA256 = ""
			digest, err := targetPagePortfolioDigest(drifted)
			if err != nil {
				t.Fatal(err)
			}
			drifted.SHA256 = digest
			if err := drifted.Validate(); err != nil {
				t.Fatalf("control rewrite should be standalone-valid: %v", err)
			}
			if err := drifted.ValidateAgainstContainer(container); err == nil {
				t.Fatal("accepted portfolio drift against container")
			}
		})
	}
}

func TestTargetPagePortfolioValidatesCompleteSiblingManifestAuthorities(t *testing.T) {
	container, appRef, helperRef, coreRef := targetPagePortfolioFixture(t)
	portfolio := mustTargetPagePortfolio(t, container, []TargetPageOutcome{
		{TargetRef: appRef, State: TargetPageReady, RunID: "run-app-1"},
		{TargetRef: helperRef, State: TargetPageUnavailable, UnavailableCode: TargetPageUnavailableTargetRunFailed},
		{TargetRef: coreRef, State: TargetPageReady, RunID: "run-core-1"},
	})
	containerArtifactSHA, err := targetRunContainerArtifactSHA256(container)
	if err != nil {
		t.Fatal(err)
	}
	portfolioArtifactSHA, err := portfolio.ArtifactSHA256()
	if err != nil {
		t.Fatal(err)
	}
	authorities := []TargetPageSiblingAuthority{
		{
			RunID: "run-core-1", AnalysisTargetRef: coreRef,
			TargetRunContainerArtifactSHA256:  containerArtifactSHA,
			TargetPagePortfolioArtifactSHA256: portfolioArtifactSHA,
		},
		{
			RunID: "run-app-1", AnalysisTargetRef: appRef,
			TargetRunContainerArtifactSHA256:  containerArtifactSHA,
			TargetPagePortfolioArtifactSHA256: portfolioArtifactSHA,
		},
	}
	if err := portfolio.ValidateSiblingAuthorities(container, authorities); err != nil {
		t.Fatalf("ValidateSiblingAuthorities: %v", err)
	}

	mustReject := func(name string, values []TargetPageSiblingAuthority) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			if err := portfolio.ValidateSiblingAuthorities(container, values); err == nil {
				t.Fatalf("accepted invalid sibling authorities %#v", values)
			}
		})
	}
	mutate := func(index int, change func(*TargetPageSiblingAuthority)) []TargetPageSiblingAuthority {
		result := slices.Clone(authorities)
		change(&result[index])
		return result
	}

	mustReject("missing sibling", authorities[:1])
	mustReject("duplicate sibling", []TargetPageSiblingAuthority{authorities[0], authorities[0]})
	mustReject("unavailable target authority", mutate(0, func(authority *TargetPageSiblingAuthority) {
		authority.AnalysisTargetRef = helperRef
	}))
	mustReject("unknown target authority", mutate(0, func(authority *TargetPageSiblingAuthority) {
		authority.AnalysisTargetRef = "at-000000000000000000000000"
	}))
	mustReject("wrong run", mutate(0, func(authority *TargetPageSiblingAuthority) {
		authority.RunID = "run-core-2"
	}))
	mustReject("unsafe run", mutate(0, func(authority *TargetPageSiblingAuthority) {
		authority.RunID = "../run-core-1"
	}))
	mustReject("container artifact tamper", mutate(0, func(authority *TargetPageSiblingAuthority) {
		authority.TargetRunContainerArtifactSHA256 = strings.Repeat("0", sha256.Size*2)
	}))
	mustReject("portfolio artifact tamper", mutate(0, func(authority *TargetPageSiblingAuthority) {
		authority.TargetPagePortfolioArtifactSHA256 = strings.Repeat("0", sha256.Size*2)
	}))
	mustReject("noncanonical digest", mutate(0, func(authority *TargetPageSiblingAuthority) {
		authority.TargetPagePortfolioArtifactSHA256 = strings.ToUpper(portfolioArtifactSHA)
	}))
}

func targetPagePortfolioFixture(t *testing.T) (TargetRunContainer, string, string, string) {
	t.Helper()
	deferred, err := Build(Options{
		RepoPath: newDeferredAnalysisTargetFixture(t), DeferAnalysisTargetResolution: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	appRef := deferredTargetRef(t, deferred, "cmd/app")
	helperRef := deferredTargetRef(t, deferred, "cmd/helper")
	coreRef := deferredModuleTargetRef(t, deferred)
	container, err := BuildTargetRunContainer(deferred, TargetRunSelection{
		DefaultTargetRef: appRef,
		TargetRefs:       []string{coreRef, helperRef, appRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	return container, appRef, helperRef, coreRef
}

func mustTargetPagePortfolio(
	t *testing.T,
	container TargetRunContainer,
	outcomes []TargetPageOutcome,
) TargetPagePortfolio {
	t.Helper()
	portfolio, err := BuildTargetPagePortfolio(container, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	return portfolio
}
