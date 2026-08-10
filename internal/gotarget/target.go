// Package gotarget owns the single resolved Go build target used by one run.
package gotarget

import (
	"fmt"
	"runtime"
	"strings"
)

// Target is one atomic GOOS/GOARCH pair.
type Target struct {
	GOOS   string
	GOARCH string
}

// Parse accepts only the canonical GOOS/GOARCH spelling used by repomap.
func Parse(value string) (Target, error) {
	if strings.Count(value, "/") != 1 {
		return Target{}, fmt.Errorf("Go target %q must have the form GOOS/GOARCH", value)
	}
	goos, goarch, _ := strings.Cut(value, "/")
	return FromParts(goos, goarch)
}

// FromParts validates a target without maintaining a second platform registry.
// The Go command remains authoritative for whether a syntactically safe pair is
// supported by the selected toolchain.
func FromParts(goos, goarch string) (Target, error) {
	if !safeToken(goos) {
		return Target{}, fmt.Errorf("GOOS %q must contain only lowercase ASCII letters and digits", goos)
	}
	if !safeToken(goarch) {
		return Target{}, fmt.Errorf("GOARCH %q must contain only lowercase ASCII letters and digits", goarch)
	}
	return Target{GOOS: goos, GOARCH: goarch}, nil
}

// Resolve applies an explicit atomic target, or resolves each standard Go
// environment dimension independently before falling back to the host.
func Resolve(explicit string, getenv func(string) string) (Target, error) {
	if explicit != "" {
		return Parse(explicit)
	}
	goos, goarch := runtime.GOOS, runtime.GOARCH
	if getenv != nil {
		if value := getenv("GOOS"); value != "" {
			goos = value
		}
		if value := getenv("GOARCH"); value != "" {
			goarch = value
		}
	}
	return FromParts(goos, goarch)
}

// Host returns the target of the running repomap binary.
func Host() Target {
	return Target{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
}

func (target Target) String() string {
	return target.GOOS + "/" + target.GOARCH
}

// Scenario is the target-specific semantic and cache identity for a run with
// no user-selectable build-tag dimension.
func (target Target) Scenario() string {
	return "go:" + target.String() + ":tags="
}

// ApplyEnv replaces, rather than duplicates, the standard target variables.
func (target Target) ApplyEnv(base []string) []string {
	result := make([]string, 0, len(base)+2)
	for _, entry := range base {
		if strings.HasPrefix(entry, "GOOS=") || strings.HasPrefix(entry, "GOARCH=") {
			continue
		}
		result = append(result, entry)
	}
	return append(result, "GOOS="+target.GOOS, "GOARCH="+target.GOARCH)
}

func safeToken(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		if (value[index] < 'a' || value[index] > 'z') &&
			(value[index] < '0' || value[index] > '9') {
			return false
		}
	}
	return true
}
