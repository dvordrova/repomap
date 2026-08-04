package semanticdiscovery

import (
	"fmt"
	"sort"
	"strings"
)

// MaterializeArtifacts resolves every repository-bearing field from the local
// bundle. Model prose cannot supply focus, evidence, artifact IDs, statement
// IDs, step IDs, confidence, or unknowns.
func MaterializeArtifacts(
	bundle Bundle,
	results []LeafResult,
	artifact FanInArtifact,
) ([]Artifact, error) {
	if err := ValidateFanInArtifact(bundle, results, artifact); err != nil {
		return nil, err
	}
	return materializeArtifacts(bundle, results, artifact)
}

// MaterializePartialArtifacts resolves a validated subset of fan-in
// proposals. Navigation relations to candidates absent from the subset are
// omitted rather than materialized as dangling artifact IDs.
func MaterializePartialArtifacts(
	bundle Bundle,
	results []LeafResult,
	artifact FanInArtifact,
) ([]Artifact, error) {
	if err := ValidatePartialFanInArtifact(bundle, results, artifact); err != nil {
		return nil, err
	}
	return materializeArtifacts(bundle, results, artifact)
}

func materializeArtifacts(
	bundle Bundle,
	results []LeafResult,
	artifact FanInArtifact,
) ([]Artifact, error) {
	_, context, err := validateLeafResults(bundle, results)
	if err != nil {
		return nil, err
	}
	normalized, err := canonicalizeFanInArtifact(context, artifact)
	if err != nil {
		return nil, fmt.Errorf("semantic discovery: derive canonical verdicts: %w", err)
	}
	artifactIDs := make(map[string]string, len(normalized.Artifacts))
	for _, proposal := range normalized.Artifacts {
		artifactIDs[proposal.CandidateID] = stableID("semantic-artifact", proposal.CandidateID)
	}

	materialized := make([]Artifact, 0, len(normalized.Artifacts))
	for _, proposal := range normalized.Artifacts {
		candidate := context.candidates[proposal.CandidateID]
		artifactID := artifactIDs[proposal.CandidateID]
		item := Artifact{
			Version:         ArtifactVersion,
			ID:              artifactID,
			CandidateID:     candidate.ID,
			Kind:            candidate.Kind,
			Title:           proposal.Title,
			Summary:         proposal.Summary,
			Question:        materializedQuestion(candidate),
			Verdict:         proposal.Verdict,
			Aliases:         append([]string(nil), proposal.Aliases...),
			LikelyQuestions: append([]string(nil), proposal.LikelyQuestions...),
			Confidence:      materializedConfidence(candidate.Confidence, proposal.Verdict),
		}
		if candidate.Kind == ArtifactDependencyUsage {
			item.Aliases = append(
				item.Aliases,
				localDependencyAliases(candidate.SupportIDs, context.bundleFacts)...,
			)
			item.Aliases = sortedUnique(item.Aliases)
			if len(item.Aliases) > maxAliasesPerArtifact {
				item.Aliases = item.Aliases[:maxAliasesPerArtifact]
			}
		}
		unknowns := []string{}
		artifactFactIDs := make(map[string]struct{})
		for _, claim := range proposal.Claims {
			aspectIDs, err := claimAnswerAspectIDs(context, candidate, claim)
			if err != nil {
				return nil, err
			}
			statementID := stableID(
				"semantic-statement",
				artifactID,
				string(claim.Basis),
				claim.Text,
				strings.Join(claim.SupportIDs, "\x00"),
			)
			facts := factsForKnownIDs(claim.SupportIDs, context.bundleFacts)
			sourceGroups := make([]string, 0, len(facts))
			for _, fact := range facts {
				sourceGroups = append(sourceGroups, fact.SourceGroup)
			}
			focus, evidence := navigationForFacts(facts)
			item.Statements = append(item.Statements, Statement{
				ID:           statementID,
				Text:         claim.Text,
				Basis:        claim.Basis,
				SupportIDs:   append([]string(nil), claim.SupportIDs...),
				SourceGroups: sortedUnique(sourceGroups),
				AspectIDs:    aspectIDs,
			})
			item.Steps = append(item.Steps, Step{
				ID:           stableID("semantic-step", artifactID, statementID),
				Title:        claim.Title,
				Explanation:  claim.Text,
				StatementIDs: []string{statementID},
				Focus:        focus,
				Evidence:     evidence,
			})
			addIDs(artifactFactIDs, claim.SupportIDs)
			if claim.Basis == ClaimUnresolved {
				unknowns = append(unknowns, claim.Text)
			}
			for _, ref := range claim.MissingRefs {
				result := context.results[ref.TaskID]
				unknowns = append(unknowns, result.Artifact.MissingEvidence[ref.MissingIndex].Explanation)
			}
		}
		item.UsedFactIDs = sortedSet(artifactFactIDs)
		if result := resultForCandidate(context, proposal.CandidateID); result != nil {
			for _, id := range result.Artifact.CandidateConnection.SupportIDs {
				if _, used := artifactFactIDs[id]; !used {
					item.UnusedAvailableFactIDs = append(
						item.UnusedAvailableFactIDs,
						id,
					)
				}
			}
			item.UnusedAvailableFactIDs = sortedUnique(item.UnusedAvailableFactIDs)
			for _, contradiction := range result.Artifact.Contradictions {
				unknowns = append(unknowns, contradiction.Explanation)
			}
			if proposal.Verdict == VerdictInsufficientEvidence && len(unknowns) == 0 {
				unknowns = append(unknowns, "The selected facts are insufficient for a supported explanation")
			}
		}
		item.Unknowns = joinUnknowns(unknowns)
		item.Focus, item.Evidence = navigationForFacts(
			factsForKnownIDs(sortedSet(artifactFactIDs), context.bundleFacts),
		)
		for _, relatedCandidateID := range proposal.RelatedCandidateIDs {
			relatedArtifactID, exists := artifactIDs[relatedCandidateID]
			if exists {
				item.RelatedArtifactIDs = append(item.RelatedArtifactIDs, relatedArtifactID)
			}
		}
		item.RelatedArtifactIDs = sortedUnique(item.RelatedArtifactIDs)
		applyIntentMaterialization(&item, candidate, proposal.Claims, context.bundleFacts)
		if err := validateMaterializedArtifact(item); err != nil {
			return nil, err
		}
		materialized = append(materialized, item)
	}
	sort.Slice(materialized, func(i, j int) bool { return materialized[i].ID < materialized[j].ID })
	return materialized, nil
}

func materializedQuestion(
	candidate OpportunityCandidate,
) string {
	return candidate.QuestionAnswered
}

func localDependencyAliases(ids []string, known map[string]Fact) []string {
	aliases := []string{}
	for _, fact := range factsForKnownIDs(ids, known) {
		if fact.Kind != FactDependency && fact.Kind != FactPackageImport {
			continue
		}
		aliases = append(aliases, fact.Keywords...)
	}
	return sortedUnique(aliases)
}

func materializedConfidence(candidate Confidence, verdict Verdict) Confidence {
	switch verdict {
	case VerdictUnsupported, VerdictInsufficientEvidence:
		return ConfidenceLow
	case VerdictMixed:
		if candidate == ConfidenceLow {
			return ConfidenceLow
		}
		return ConfidenceMedium
	case VerdictSupported:
		return candidate
	default:
		return ConfidenceLow
	}
}

func navigationForFacts(facts []Fact) (Focus, []EvidenceRef) {
	focus := Focus{}
	evidenceByID := make(map[string]EvidenceRef)
	for _, fact := range facts {
		focus.ComponentIDs = append(focus.ComponentIDs, fact.Focus.ComponentIDs...)
		focus.FlowIDs = append(focus.FlowIDs, fact.Focus.FlowIDs...)
		focus.SurfaceIDs = append(focus.SurfaceIDs, fact.Focus.SurfaceIDs...)
		for _, reference := range fact.Evidence {
			if _, exists := evidenceByID[reference.ID]; !exists {
				evidenceByID[reference.ID] = reference
			}
		}
	}
	focus = canonicalFocus(focus)
	evidence := make([]EvidenceRef, 0, len(evidenceByID))
	for _, reference := range evidenceByID {
		evidence = append(evidence, reference)
	}
	sort.Slice(evidence, func(i, j int) bool {
		if evidence[i].ID != evidence[j].ID {
			return evidence[i].ID < evidence[j].ID
		}
		if evidence[i].Path != evidence[j].Path {
			return evidence[i].Path < evidence[j].Path
		}
		if evidence[i].Line != evidence[j].Line {
			return evidence[i].Line < evidence[j].Line
		}
		return evidence[i].Column < evidence[j].Column
	})
	return focus, evidence
}

func validateMaterializedArtifact(artifact Artifact) error {
	if artifact.Version != ArtifactVersion {
		return fmt.Errorf("semantic discovery: unsupported artifact version %d", artifact.Version)
	}
	if err := validateOpaque("artifact id", artifact.ID); err != nil {
		return err
	}
	if err := validateOpaque("artifact candidate id", artifact.CandidateID); err != nil {
		return err
	}
	if !validArtifactKind(artifact.Kind) || !validConfidence(artifact.Confidence) {
		return fmt.Errorf("semantic discovery: invalid materialized artifact enum")
	}
	if len(artifact.Statements) == 0 || len(artifact.Steps) != len(artifact.Statements) {
		return fmt.Errorf("semantic discovery: materialized artifact has inconsistent statements and steps")
	}
	if err := validateAspectPartition(artifact); err != nil {
		return err
	}
	if err := validateMaterializedFactUsage(artifact); err != nil {
		return err
	}
	return nil
}

func validateAspectPartition(artifact Artifact) error {
	if err := validateIDList("required aspect ids", artifact.RequiredAspectIDs, false); err != nil {
		return err
	}
	if err := validateIDList("covered aspect ids", artifact.CoveredAspectIDs, false); err != nil {
		return err
	}
	if err := validateIDList("uncovered aspect ids", artifact.UncoveredAspectIDs, false); err != nil {
		return err
	}
	required := make(map[string]struct{}, len(artifact.RequiredAspectIDs))
	covered := make(map[string]struct{}, len(artifact.CoveredAspectIDs))
	addIDs(required, artifact.RequiredAspectIDs)
	addIDs(covered, artifact.CoveredAspectIDs)
	for _, id := range artifact.CoveredAspectIDs {
		if _, exists := required[id]; !exists {
			return fmt.Errorf("semantic discovery: covered aspect %q is not required", id)
		}
	}
	for _, id := range artifact.UncoveredAspectIDs {
		if _, exists := required[id]; !exists {
			return fmt.Errorf("semantic discovery: uncovered aspect %q is not required", id)
		}
		if _, exists := covered[id]; exists {
			return fmt.Errorf("semantic discovery: aspect %q is covered and uncovered", id)
		}
	}
	if len(artifact.CoveredAspectIDs)+len(artifact.UncoveredAspectIDs) != len(artifact.RequiredAspectIDs) {
		return fmt.Errorf("semantic discovery: materialized aspect partition is incomplete")
	}
	for statementIndex, statement := range artifact.Statements {
		if err := validateIDList("statement answer aspect ids", statement.AspectIDs, false); err != nil {
			return err
		}
		for _, id := range statement.AspectIDs {
			if _, exists := required[id]; !exists {
				return fmt.Errorf(
					"semantic discovery: statement %d answer aspect %q is not required",
					statementIndex,
					id,
				)
			}
		}
	}
	return nil
}

func validateMaterializedFactUsage(artifact Artifact) error {
	if err := validateIDList("used fact ids", artifact.UsedFactIDs, true); err != nil {
		return err
	}
	if err := validateIDList(
		"unused available fact ids",
		artifact.UnusedAvailableFactIDs,
		false,
	); err != nil {
		return err
	}
	used := make(map[string]struct{}, len(artifact.UsedFactIDs))
	addIDs(used, artifact.UsedFactIDs)
	for _, id := range artifact.UnusedAvailableFactIDs {
		if _, exists := used[id]; exists {
			return fmt.Errorf(
				"semantic discovery: fact %q is both used and unused",
				id,
			)
		}
	}
	expected := make(map[string]struct{})
	for _, statement := range artifact.Statements {
		addIDs(expected, statement.SupportIDs)
	}
	if !equalStringSets(artifact.UsedFactIDs, sortedSet(expected)) {
		return fmt.Errorf(
			"semantic discovery: used fact ids do not match statement support",
		)
	}
	return nil
}
