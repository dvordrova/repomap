package snapshot

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestTargetRunContainerProjectsTwoTargetsFromOneDeferredSnapshot(t *testing.T) {
	repo := newDeferredAnalysisTargetFixture(t)
	deferred, err := buildSnapshotForTest(Options{RepoPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	appRef := deferredTargetRef(t, deferred, "cmd/app")
	helperRef := deferredTargetRef(t, deferred, "cmd/helper")

	// The repository becomes unavailable after the sole complete snapshot. Both
	// target pages below must therefore be pure projections of retained facts.
	if err := os.Rename(repo, repo+".unavailable"); err != nil {
		t.Fatal(err)
	}
	container, err := BuildTargetRunContainer(deferred, TargetRunSelection{
		DefaultTargetRef: appRef,
		TargetRefs:       []string{helperRef, appRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := container.ValidateAgainst(deferred); err != nil {
		t.Fatalf("validate bound container: %v", err)
	}
	if len(container.Targets) != 2 ||
		container.Targets[0].DisplayPath != "cmd/app" ||
		container.Targets[1].DisplayPath != "cmd/helper" ||
		container.DefaultTargetRef != appRef {
		t.Fatalf("canonical target container = %#v", container)
	}

	app, err := container.ScopedSnapshot(appRef)
	if err != nil {
		t.Fatalf("project app page: %v", err)
	}
	helper, err := container.ScopedSnapshot(helperRef)
	if err != nil {
		t.Fatalf("project helper page: %v", err)
	}
	if app.AnalysisTarget == nil || app.AnalysisTarget.PackageDir != "cmd/app" ||
		helper.AnalysisTarget == nil || helper.AnalysisTarget.PackageDir != "cmd/helper" {
		t.Fatalf("projected targets = %#v / %#v", app.AnalysisTarget, helper.AnalysisTarget)
	}
	if app.TargetCatalog != nil || helper.TargetCatalog != nil ||
		app.GoFacts == nil || helper.GoFacts == nil ||
		len(app.GoFacts.Packages) != 2 || len(helper.GoFacts.Packages) != 1 {
		t.Fatalf("projected facts = %#v / %#v", app.GoFacts, helper.GoFacts)
	}
	if !slices.Contains(app.FilteredFiles, "core/core.go") ||
		slices.Contains(helper.FilteredFiles, "core/core.go") {
		t.Fatalf("projected files = %#v / %#v", app.FilteredFiles, helper.FilteredFiles)
	}
	if deferred.TargetCatalog == nil || deferred.AnalysisTarget != nil ||
		deferred.GoFacts == nil || len(deferred.GoFacts.Packages) != 3 {
		t.Fatalf("container mutated complete source = %#v", deferred)
	}

	permuted, err := BuildTargetRunContainer(deferred, TargetRunSelection{
		DefaultTargetRef: appRef,
		TargetRefs:       []string{appRef, helperRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := container.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	permutedWire, err := permuted.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wire, permutedWire) {
		t.Fatalf("response-order-dependent container:\n%s\n%s", wire, permutedWire)
	}

	decoded, err := DecodeTargetRunContainer(wire)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decoded.ScopedSnapshot(appRef); err == nil ||
		!strings.Contains(err.Error(), "deferred snapshot is unavailable") {
		t.Fatalf("unbound persisted container scope error = %v", err)
	}
	restored, err := BindTargetRunContainer(decoded, deferred)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restored.ScopedSnapshot(helperRef); err != nil {
		t.Fatalf("restored helper page: %v", err)
	}

	copyContainer := container.Snapshot()
	copyContainer.Targets[0].Target.Roots[0].Path = "mutated"
	if err := container.ValidateAgainst(deferred); err != nil {
		t.Fatalf("live handoff mutated producer: %v", err)
	}
	app.FilteredFiles[0] = "mutated"
	appAgain, err := container.ScopedSnapshot(appRef)
	if err != nil {
		t.Fatal(err)
	}
	if appAgain.FilteredFiles[0] == "mutated" {
		t.Fatal("scoped snapshot mutation leaked into later page projection")
	}
}

func TestTargetRunContainerRejectsInvalidSelectionAndArtifactDrift(t *testing.T) {
	deferred, err := buildSnapshotForTest(Options{
		RepoPath: newDeferredAnalysisTargetFixture(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	appRef := deferredTargetRef(t, deferred, "cmd/app")
	helperRef := deferredTargetRef(t, deferred, "cmd/helper")

	for name, selection := range map[string]TargetRunSelection{
		"empty":          {DefaultTargetRef: appRef},
		"unknown":        {DefaultTargetRef: appRef, TargetRefs: []string{appRef, "at-unknown"}},
		"duplicate":      {DefaultTargetRef: appRef, TargetRefs: []string{appRef, appRef}},
		"default absent": {DefaultTargetRef: appRef, TargetRefs: []string{helperRef}},
		"spaced":         {DefaultTargetRef: appRef, TargetRefs: []string{" " + appRef}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildTargetRunContainer(deferred, selection); err == nil {
				t.Fatalf("accepted invalid selection %#v", selection)
			}
		})
	}

	container, err := BuildTargetRunContainer(deferred, TargetRunSelection{
		DefaultTargetRef: appRef, TargetRefs: []string{appRef, helperRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	tampered := container.Snapshot()
	tampered.Targets[0].SnapshotSHA256 = strings.Repeat("0", 64)
	if err := tampered.Validate(); err == nil {
		t.Fatal("accepted projection digest tampering")
	}
	wire, err := container.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	withUnknown := bytes.Replace(wire, []byte(`{"version":2,`), []byte(`{"version":2,"unknown":true,`), 1)
	if _, err := DecodeTargetRunContainer(withUnknown); err == nil {
		t.Fatal("accepted unknown artifact field")
	}
	priorV1 := bytes.Replace(wire, []byte(`{"version":2,`), []byte(`{"version":1,`), 1)
	if _, err := DecodeTargetRunContainer(priorV1); err == nil {
		t.Fatal("accepted prior v1 target run container")
	}
	if _, err := DecodeTargetRunContainer(append(append([]byte(nil), wire...), []byte(` {}`)...)); err == nil {
		t.Fatal("accepted trailing artifact value")
	}
	if _, err := DecodeTargetRunContainer(append(wire, '\n')); err == nil {
		t.Fatal("accepted non-canonical artifact whitespace")
	}
}
