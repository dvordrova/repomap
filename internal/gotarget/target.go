// Package gotarget owns the single resolved Go build target used by one run.
package gotarget

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"unicode"
)

const (
	AdvisoryMaximumBuildTags = 64
	AdvisoryBuildTagBytes    = 128
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

func (target Target) String() string {
	return target.GOOS + "/" + target.GOARCH
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

// ParseBuildTags restores the conventional comma-or-whitespace separated Go
// build-tag input into one canonical, uniquely sorted set. The accepted
// alphabet is the one used by Go build constraints: Unicode letters and
// digits, underscore, and dot.
func ParseBuildTags(value string) ([]string, error) {
	return CanonicalBuildTags(strings.FieldsFunc(value, func(character rune) bool {
		return character == ',' || unicode.IsSpace(character)
	}))
}

// CanonicalBuildTags validates and owns an already tokenized build-tag set.
// Keeping this validation beside Target lets deterministic fact loading and
// the typed-program load share exactly one spelling contract.
func CanonicalBuildTags(values []string) ([]string, error) {
	canonical := append([]string(nil), values...)
	for _, value := range canonical {
		if !validBuildTag(value) {
			return nil, fmt.Errorf(
				"Go build tag %q must contain only letters, digits, underscores, or dots",
				value,
			)
		}
	}
	sort.Strings(canonical)
	result := canonical[:0]
	for _, value := range canonical {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	if result == nil {
		return []string{}, nil
	}
	return result, nil
}

type ScaleWarning struct {
	Kind         string
	Retained     int
	AdvisorySize int
}

func ScaleWarnings(tags []string) []ScaleWarning {
	result := []ScaleWarning{}
	if len(tags) > AdvisoryMaximumBuildTags {
		result = append(result, ScaleWarning{Kind: "go_build_tags", Retained: len(tags), AdvisorySize: AdvisoryMaximumBuildTags})
	}
	maximum := 0
	for _, tag := range tags {
		if len(tag) > maximum {
			maximum = len(tag)
		}
	}
	if maximum > AdvisoryBuildTagBytes {
		result = append(result, ScaleWarning{Kind: "go_build_tag_bytes", Retained: maximum, AdvisorySize: AdvisoryBuildTagBytes})
	}
	return result
}

func validBuildTag(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) &&
			character != '_' && character != '.' {
			return false
		}
	}
	return true
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
