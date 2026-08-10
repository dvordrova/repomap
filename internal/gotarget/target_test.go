package gotarget

import (
	"runtime"
	"slices"
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
	if explicit.String() != "freebsd/386" || explicit.Scenario() != "go:freebsd/386:tags=" {
		t.Fatalf("explicit target = %#v / %q", explicit, explicit.Scenario())
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
