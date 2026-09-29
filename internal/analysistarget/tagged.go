package analysistarget

import (
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/gofacts"
)

// buildFacts are the facts a target was found in: the run's own load, or the
// tagged build whose tags make its package a program (gofacts.TaggedBuild).
// Scoping, the catalogue and file discovery read a target only from these.
func buildFacts(facts gofacts.Facts, target Target) (gofacts.Facts, error) {
	if len(target.BuildTags) == 0 {
		return facts, nil
	}
	tags := strings.Join(target.BuildTags, ",")
	for _, build := range facts.TaggedBuilds {
		if build.Facts == nil || strings.Join(build.Tags, ",") != tags || build.Platform != target.BuildPlatform {
			continue
		}
		for _, pkg := range build.Packages {
			if pkg.PackageDir == target.PackageDir {
				return *build.Facts, nil
			}
		}
	}
	return gofacts.Facts{}, fmt.Errorf("analysis target: no tagged build makes %q a program with -tags %s for %s", target.PackageDir, tags, target.BuildPlatform)
}

// taggedCandidates are the programs tagged builds make: each executable of a
// tagged load whose package a build description builds with those tags,
// carrying the tags and the description lines.
func taggedCandidates(facts gofacts.Facts) ([]Candidate, error) {
	var result []Candidate
	for _, build := range facts.TaggedBuilds {
		if build.Facts == nil || len(build.Facts.TaggedBuilds) != 0 {
			return nil, fmt.Errorf("analysis target: tagged build %s -tags %s has no facts of its own", build.ModuleDir, strings.Join(build.Tags, ","))
		}
		described := make(map[string]gofacts.TaggedPackage, len(build.Packages))
		for _, pkg := range build.Packages {
			described[pkg.PackageDir] = pkg
		}
		candidates, err := Candidates(*build.Facts)
		if err != nil {
			return nil, err
		}
		for _, candidate := range candidates {
			pkg, ok := described[candidate.Target.PackageDir]
			if candidate.Target.Kind != KindExecutablePackage || !ok {
				continue
			}
			target := candidate.Target
			target.BuildTags = append([]string(nil), build.Tags...)
			target.BuildPlatform = build.Platform
			target.BuildSources = append([]gofacts.BuildTagSource(nil), pkg.Sources...)
			target.Ref, err = targetRef(target)
			if err != nil {
				return nil, err
			}
			if err := target.Validate(); err != nil {
				return nil, err
			}
			candidate.Target = target
			result = append(result, candidate)
		}
	}
	return result, nil
}

// BuildTagsHypothesis is what a tagged main file tells the target
// portfolio: the tags and the first description line building it.
func BuildTagsHypothesis(target Target) string {
	if len(target.BuildTags) == 0 || len(target.BuildSources) == 0 {
		return ""
	}
	source := target.BuildSources[0]
	return fmt.Sprintf("builds only with Go build tags %s, as %s:%d builds it for %s", strings.Join(target.BuildTags, ","), source.Path, source.Line, target.BuildPlatform)
}
