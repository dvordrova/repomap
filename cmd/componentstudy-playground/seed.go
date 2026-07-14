package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/report"
)

func buildSeed(
	cfg config,
	run authorizedRun,
	packageScope packageScopeArtifact,
	packageFiles []packageFile,
	symbols []componentstudy.SymbolCandidate,
) (componentstudy.Seed, componentstudy.Budget) {
	repoName := cleanText(run.data.RepoName, 128)
	if repoName == "" {
		repoName = cleanText(filepath.Base(run.analysisRoot), 128)
	}
	componentName := cleanText(run.component.Name, 256)
	if componentName == "" {
		componentName = "Selected component"
	}
	purpose := cleanText(run.component.ModelPurpose, 1024)
	if purpose == "" {
		purpose = "Initial orientation did not provide a component purpose"
	}
	componentID := stableID("component", run.component.ID)
	goalID := stableID("goal", componentID, cfg.goal)
	seed := componentstudy.Seed{
		Version:  componentstudy.SeedVersion,
		RepoName: repoName,
		Goal: componentstudy.Goal{
			ID:        goalID,
			Kind:      componentstudy.GoalOnboarding,
			Objective: cfg.goal,
		},
		Component: componentstudy.Component{
			ID:      componentID,
			Name:    componentName,
			Purpose: purpose,
		},
	}

	anchorIDs := make(map[string]string, len(run.componentAuthority.Anchors))
	authorityAnchors := append([]report.AnchorAuthority(nil), run.componentAuthority.Anchors...)
	sort.SliceStable(authorityAnchors, func(i, j int) bool {
		if authorityAnchors[i].Path == run.anchor.Path {
			return true
		}
		if authorityAnchors[j].Path == run.anchor.Path {
			return false
		}
		return authorityAnchors[i].Path < authorityAnchors[j].Path
	})
	for index, anchor := range authorityAnchors {
		id := stableID("anchor", componentID, anchor.ID, anchor.Path)
		anchorIDs[anchor.Path] = id
		reason := "Component anchor from the verified orientation run"
		if anchor.Path == run.anchor.Path {
			reason = "User-selected component anchor from the verified orientation run"
		}
		line := 0
		if len(anchor.AllowedLines) > 0 {
			line = anchor.AllowedLines[0]
		}
		seed.Anchors = append(seed.Anchors, componentstudy.AnchorCandidate{
			ID:     id,
			Rank:   index + 1,
			Path:   anchor.Path,
			Line:   line,
			Reason: reason,
			Provenance: componentstudy.Provenance{
				Source:    "run_manifest",
				Operation: "authorize_anchor",
				Detail:    cleanText(anchor.ID, 256),
			},
			Certainty: componentstudy.CertaintyNavigation,
		})
	}

	fileIDs := make(map[string]string)
	for _, file := range packageFiles {
		id := stableID("file", file.Path)
		fileIDs[file.Path] = id
		reason := "Build-selected " + file.Kind + " file from the anchor package"
		if file.Path == run.anchor.Path {
			reason = "User-selected anchor in the build-selected package"
		}
		seed.Files = append(seed.Files, componentstudy.FileCandidate{
			ID:     id,
			Rank:   len(seed.Files) + 1,
			Path:   file.Path,
			Reason: reason,
			Provenance: componentstudy.Provenance{
				Source:    "go_list",
				Operation: "package_files",
				Detail:    cleanText(packageScope.ImportPath, 256),
			},
			Certainty: componentstudy.CertaintyStatic,
		})
	}
	for _, anchor := range authorityAnchors {
		if _, exists := fileIDs[anchor.Path]; exists {
			continue
		}
		id := stableID("file", anchor.Path)
		fileIDs[anchor.Path] = id
		seed.Files = append(seed.Files, componentstudy.FileCandidate{
			ID:     id,
			Rank:   len(seed.Files) + 1,
			Path:   anchor.Path,
			Reason: "Openable component anchor outside the selected Go package",
			Provenance: componentstudy.Provenance{
				Source:    "run_manifest",
				Operation: "openable_anchor",
				Detail:    cleanText(anchor.ID, 256),
			},
			Certainty: componentstudy.CertaintyNavigation,
		})
	}
	seed.Symbols = append(seed.Symbols, symbols...)
	seed.Evidence = buildSeedEvidence(
		run,
		componentID,
		anchorIDs,
		fileIDs,
		rankTerms(run.component.Name, cfg.goal),
	)

	budget := componentstudy.Budget{
		Version:       componentstudy.BudgetVersion,
		MaxAnchors:    8,
		MaxFiles:      maxPackageFiles,
		MaxSymbols:    maxSeedSymbols,
		MaxEvidence:   maxSeedEvidence,
		MaxModelBytes: maxPlannerBundleBytes,
	}
	return seed, budget
}

func buildSeedEvidence(
	run authorizedRun,
	componentID string,
	anchorIDs map[string]string,
	fileIDs map[string]string,
	terms []string,
) []componentstudy.EvidenceCandidate {
	var result []componentstudy.EvidenceCandidate
	add := func(
		kind componentstudy.EvidenceKind,
		statement string,
		related []string,
		provenance componentstudy.Provenance,
		certainty componentstudy.Certainty,
	) {
		statement = cleanText(statement, 1024)
		if statement == "" {
			return
		}
		related = uniqueIDs(related, 8)
		if len(related) == 0 {
			return
		}
		result = append(result, componentstudy.EvidenceCandidate{
			ID:         stableID("evidence", string(kind), statement, strings.Join(related, ",")),
			Rank:       len(result) + 1,
			Kind:       kind,
			Statement:  statement,
			RelatedIDs: related,
			Reason:     "Candidate context for choosing the next onboarding questions",
			Provenance: provenance,
			Certainty:  certainty,
		})
	}

	componentNames := make(map[string]string, len(run.data.Components))
	for _, component := range run.data.Components {
		componentNames[component.ID] = component.Name
	}
	for _, relation := range run.data.ComponentRelations {
		if relation.From != run.component.ID && relation.To != run.component.ID {
			continue
		}
		otherID := relation.To
		direction := "to"
		if relation.To == run.component.ID {
			otherID = relation.From
			direction = "from"
		}
		var edges []string
		for _, edge := range relation.Evidence {
			edges = append(edges, edge.From+" -> "+edge.To)
		}
		statement := fmt.Sprintf(
			"Local %s relation %s %s: %s",
			relation.Kind,
			direction,
			componentNames[otherID],
			strings.Join(edges, "; "),
		)
		add(
			componentstudy.EvidenceRelation,
			statement,
			[]string{componentID},
			componentstudy.Provenance{Source: "report", Operation: "component_relation"},
			componentCertainty(relation.Certainty),
		)
	}

	for _, anchor := range run.component.AnchorGroups {
		anchorID, exists := anchorIDs[anchor.Path]
		if !exists {
			continue
		}
		for _, signal := range anchor.LocalContext {
			location := signal.Path
			if signal.Line > 0 {
				location += fmt.Sprintf(":%d", signal.Line)
			}
			statement := location + " — " + signal.Snippet
			if signal.Reason != "" {
				statement += " (" + signal.Reason + ")"
			}
			related := []string{anchorID}
			if fileID := fileIDs[signal.Path]; fileID != "" {
				related = append(related, fileID)
			}
			add(
				componentstudy.EvidenceRelation,
				statement,
				related,
				componentstudy.Provenance{Source: "local_snapshot", Operation: "source_signal"},
				componentstudy.CertaintyPossible,
			)
		}
	}

	for _, flowID := range run.component.RelatedFlowIDs {
		direction, ok := findDirection(run.data.CandidateDirections, flowID)
		if !ok {
			continue
		}
		statement := fmt.Sprintf(
			"Model-proposed direction %s; trigger: %s; why: %s",
			direction.Name,
			direction.Trigger,
			direction.WhyInteresting,
		)
		related := []string{componentID}
		paths := append([]string{direction.LikelyEntrypoint}, direction.LikelyFiles...)
		for _, path := range paths {
			if id := fileIDs[path]; id != "" {
				related = append(related, id)
			}
		}
		add(
			componentstudy.EvidenceDirection,
			statement,
			related,
			componentstudy.Provenance{Source: "model_orientation", Operation: "candidate_direction", Detail: cleanText(flowID, 256)},
			componentstudy.CertaintyNavigation,
		)
	}

	for _, anchor := range run.component.AnchorGroups {
		anchorID, exists := anchorIDs[anchor.Path]
		if !exists {
			continue
		}
		for _, note := range anchor.ModelNotes {
			related := []string{anchorID}
			if fileID := fileIDs[anchor.Path]; fileID != "" {
				related = append(related, fileID)
			}
			add(
				componentstudy.EvidenceDirection,
				"Initial model note: "+note,
				related,
				componentstudy.Provenance{Source: "model_orientation", Operation: "anchor_note"},
				componentstudy.CertaintyHypothesis,
			)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return evidenceRelevance(result[i], terms) > evidenceRelevance(result[j], terms)
	})
	if len(result) > maxSeedEvidence {
		result = result[:maxSeedEvidence]
	}
	for index := range result {
		result[index].Rank = index + 1
	}
	return result
}

func evidenceRelevance(candidate componentstudy.EvidenceCandidate, terms []string) int {
	statement := strings.ToLower(candidate.Statement)
	score := 0
	for _, term := range terms {
		if strings.Contains(statement, term) {
			score += 2
		}
	}
	switch candidate.Certainty {
	case componentstudy.CertaintyVerified, componentstudy.CertaintyObserved:
		score += 3
	case componentstudy.CertaintyStatic:
		score += 2
	case componentstudy.CertaintyPossible, componentstudy.CertaintyNavigation:
		score++
	}
	return score
}

func findDirection(directions []report.CandidateDirection, id string) (report.CandidateDirection, bool) {
	for _, direction := range directions {
		if direction.ID == id {
			return direction, true
		}
	}
	return report.CandidateDirection{}, false
}

func uniqueIDs(ids []string, limit int) []string {
	seen := make(map[string]struct{}, len(ids))
	result := make([]string, 0, min(len(ids), limit))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
		if len(result) == limit {
			break
		}
	}
	return result
}

func stableID(kind string, values ...string) string {
	hash := sha256.New()
	for _, value := range values {
		hash.Write([]byte(value))
		hash.Write([]byte{0})
	}
	return kind + "-" + hex.EncodeToString(hash.Sum(nil))[:16]
}

func cleanText(value string, maxBytes int) string {
	value = strings.Join(strings.FieldsFunc(value, unicode.IsSpace), " ")
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if len(value) <= maxBytes {
		return value
	}
	for len(value) > maxBytes {
		_, size := utf8.DecodeLastRuneInString(value)
		if size <= 0 {
			return ""
		}
		value = value[:len(value)-size]
	}
	return strings.TrimSpace(value)
}

func cleanRepoPath(value string) (string, error) {
	value = filepath.ToSlash(strings.TrimSpace(value))
	local := filepath.FromSlash(value)
	if value == "" || filepath.IsAbs(local) || !filepath.IsLocal(local) {
		return "", fmt.Errorf("path must be repository-relative")
	}
	clean := filepath.ToSlash(filepath.Clean(local))
	if clean != value || clean == "." {
		return "", fmt.Errorf("path must be canonical")
	}
	return clean, nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && (relative == "." || filepath.IsLocal(relative))
}

func repoRelativePath(root, absolute string) (string, error) {
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == "." || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("file is outside analysis root")
	}
	return cleanRepoPath(filepath.ToSlash(relative))
}
