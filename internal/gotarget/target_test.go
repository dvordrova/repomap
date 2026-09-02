package gotarget

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestResolveUsesExplicitAtomicTargetAndIndependentEnvironmentFallback(t *testing.T) {
	environment := map[string]string{"GOOS": "linux"}
	getenv := func(key string) string { return environment[key] }

	fromEnvironment, err := Resolve("", getenv)
	if err != nil {
		t.Fatal(err)
	}
	if fromEnvironment.GOOS != "linux" || fromEnvironment.GOARCH != runtime.GOARCH {
		t.Fatalf("environment target = %#v", fromEnvironment)
	}

	explicit, err := Resolve("freebsd/386", getenv)
	if err != nil {
		t.Fatal(err)
	}
	if explicit.String() != "freebsd/386" {
		t.Fatalf("explicit target = %#v", explicit)
	}
}

func TestParseRejectsNonCanonicalTargetAndApplyEnvReplacesPair(t *testing.T) {
	for _, value := range []string{"linux", "linux/amd64/extra", "Linux/amd64", "linux/amd-64", "/amd64", "linux/"} {
		if _, err := Parse(value); err == nil {
			t.Fatalf("Parse(%q) succeeded", value)
		}
	}
	target, err := Parse("linux/arm64")
	if err != nil {
		t.Fatal(err)
	}
	got := target.ApplyEnv([]string{"A=1", "GOOS=darwin", "GOARCH=amd64", "B=2"})
	want := []string{"A=1", "B=2", "GOOS=linux", "GOARCH=arm64"}
	if !slices.Equal(got, want) {
		t.Fatalf("target environment = %v, want %v", got, want)
	}
}

func TestParseBuildTagsCanonicalizesCommaAndWhitespaceInput(t *testing.T) {
	got, err := ParseBuildTags(" integration,netgo\tdebug integration ")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"debug", "integration", "netgo"}
	if !slices.Equal(got, want) {
		t.Fatalf("build tags = %v, want %v", got, want)
	}

	for _, value := range []string{"broken-tag", "prod!", "a/b"} {
		if _, err := ParseBuildTags(value); err == nil {
			t.Fatalf("ParseBuildTags(%q) succeeded", value)
		}
	}
	long, err := ParseBuildTags(strings.Repeat("x", AdvisoryBuildTagBytes+1))
	if err != nil || len(long) != 1 || len(long[0]) != AdvisoryBuildTagBytes+1 {
		t.Fatalf("long build tag was not retained: %v, %v", long, err)
	}
	many := make([]string, AdvisoryMaximumBuildTags+1)
	for index := range many {
		many[index] = fmt.Sprintf("tag%d", index)
	}
	retained, err := CanonicalBuildTags(many)
	if err != nil || len(retained) != len(many) {
		t.Fatalf("large build tag set was not retained: %d, %v", len(retained), err)
	}
	warnings := append(ScaleWarnings(long), ScaleWarnings(retained)...)
	if len(warnings) != 2 {
		t.Fatalf("scale warnings = %#v", warnings)
	}
}
