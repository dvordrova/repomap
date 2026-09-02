package main

import (
	"slices"
	"testing"
)

func TestAutomaticGoTargetAuthorityHonorsExplicitCLIAndEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		env      map[string]string
		want     bool
	}{
		{name: "host default", want: true},
		{name: "explicit flag", explicit: "darwin/amd64"},
		{name: "GOOS environment", env: map[string]string{"GOOS": "darwin"}},
		{name: "GOARCH environment", env: map[string]string{"GOARCH": "amd64"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := automaticGoTargetAllowed(test.explicit, func(name string) string {
				return test.env[name]
			})
			if got != test.want {
				t.Fatalf("automaticGoTargetAllowed() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestResolveGoBuildTagsOnlyBindsGoRepositories(t *testing.T) {
	tags, err := resolveGoBuildTagsForRepository(true, " netgo,integration netgo ")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"integration", "netgo"}; !slices.Equal(tags, want) {
		t.Fatalf("Go repository tags = %v, want %v", tags, want)
	}
	if _, err := resolveGoBuildTagsForRepository(true, "bad-tag"); err == nil {
		t.Fatal("invalid Go repository GOTAGS was accepted")
	}
	tags, err = resolveGoBuildTagsForRepository(false, "bad-tag")
	if err != nil || len(tags) != 0 {
		t.Fatalf("irrelevant non-Go GOTAGS = %v, %v", tags, err)
	}
}

func TestInvalidGoBuildTagsWaitForExplicitAdapterResolution(t *testing.T) {
	tags, deferred, immediate := prepareGoBuildTags(
		true,
		"bad-tag",
		"src/service.py",
	)
	if immediate != nil || deferred == nil || len(tags) != 0 {
		t.Fatalf("explicit mixed-repository resolution = %v, %v, %v", tags, deferred, immediate)
	}
	nonGo := repositoryTargetPlan{Targets: []repositoryTypedTarget{{
		Key: repositoryTargetKey{Adapter: repositoryTargetAdapterPython, Ref: "python-target"},
	}}}
	if repositoryTargetPlanContainsGo(nonGo) {
		t.Fatal("exact non-Go target was treated as a Go target")
	}
	withGo := nonGo
	withGo.Targets = append(withGo.Targets, repositoryTypedTarget{
		Key: repositoryTargetKey{Adapter: repositoryTargetAdapterGo, Ref: "go-target"},
	})
	if !repositoryTargetPlanContainsGo(withGo) {
		t.Fatal("selected Go target did not retain deferred GOTAGS failure")
	}

	_, deferred, immediate = prepareGoBuildTags(true, "bad-tag", "")
	if deferred != nil || immediate == nil {
		t.Fatalf("default selection did not fail immediately: %v / %v", deferred, immediate)
	}
}
