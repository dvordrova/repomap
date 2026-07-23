package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const freshOnboardingEditorMaxMechanisms = 8

const freshOnboardingEditorSystem = `You edit a presentation over already accepted repository mechanisms. The supplied canonical steps and opaque statement IDs are authoritative. Return valid JSON only. Group steps into human phases; do not create claims, facts, relations, runtime order, paths, symbols, evidence, or IDs. Output titles are presentation copy, not semantic authority.`

const freshOnboardingEditorUser = `Return exactly this JSON shape:
{
  "version": 1,
  "compressions": [
    {
      "artifact_id": "exact supplied artifact id",
      "ordering_basis": "editorial",
      "phases": [
        {
          "title": "short action title using only concepts present in member steps",
          "explanation": "optional short presentation copy",
          "member_statement_ids": ["exact supplied statement ids"]
        }
      ]
    }
  ]
}

Rules:
- Return at most one compression for each supplied artifact.
- Each compression has three to six non-empty phases.
- Keep every supplied statement ID exactly once.
- Keep all statement IDs from one canonical step in the same phase.
- Preserve canonical step order as editorial order; do not claim runtime sequence.
- Merge error checks and error returns into the nearest substantive phase. Do not create an error-only phase.
- Do not mention repository paths, file names, symbols, support IDs, evidence, verdicts, gaps, or internal diagnostics.
- If safe compression is impossible for one artifact, omit that artifact.

Variable accepted mechanism bundle JSON:
`

type freshOnboardingEditorBundle struct {
	Version    int                              `json:"version"`
	Mechanisms []freshOnboardingEditorMechanism `json:"mechanisms"`
}

type freshOnboardingEditorMechanism struct {
	ArtifactID string                      `json:"artifact_id"`
	Question   string                      `json:"question"`
	Answer     string                      `json:"answer"`
	Steps      []freshOnboardingEditorStep `json:"canonical_steps"`
}

type freshOnboardingEditorStep struct {
	Title        string   `json:"title"`
	Explanation  string   `json:"explanation"`
	StatementIDs []string `json:"statement_ids"`
}

func editFreshRepositoryOnboarding(
	ctx context.Context,
	runDir string,
	provider semanticDiscoveryEditor,
	preferredArtifactID string,
) (semanticDiscoveryStageMetrics, bool, error) {
	path := filepath.Join(runDir, report.RepositoryOnboardingFile)
	baseline := report.RepositoryOnboardingEditorial{
		Version:             report.RepositoryOnboardingVersion,
		PreferredArtifactID: strings.TrimSpace(preferredArtifactID),
		Compressions:        []report.NarrativeCompression{},
	}
	if err := writeGoldenJSON(path, baseline); err != nil {
		return semanticDiscoveryStageMetrics{}, false, fmt.Errorf(
			"fresh repository onboarding: save local baseline: %w",
			err,
		)
	}
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return semanticDiscoveryStageMetrics{}, false, err
	}
	bundle := buildFreshOnboardingEditorBundle(data)
	if len(bundle.Mechanisms) == 0 {
		return semanticDiscoveryStageMetrics{}, false, nil
	}
	rawBundle, err := json.Marshal(bundle)
	if err != nil {
		return semanticDiscoveryStageMetrics{}, false, err
	}
	prompt := semanticdiscovery.Prompt{
		Version:         semanticdiscovery.OnboardingEditorPromptVersion,
		System:          freshOnboardingEditorSystem,
		User:            freshOnboardingEditorUser + string(rawBundle),
		ThinkingProfile: semanticdiscovery.ThinkingMax,
		ProgressLabel:   "repository onboarding editing",
	}
	plan, err := newSemanticDiscoveryStagePlan(provider, prompt, "repository_onboarding_editor")
	if err != nil {
		return semanticDiscoveryStageMetrics{}, false, err
	}
	var accepted report.RepositoryOnboardingEditorial
	metrics, err := executeSemanticDiscoveryStage(
		ctx,
		provider,
		plan,
		&semanticDiscoveryBudget{},
		func(raw []byte) error {
			proposal, decodeErr := report.DecodeRepositoryOnboardingEditorial(raw)
			if decodeErr != nil {
				return decodeErr
			}
			proposal.Compressions = validateFreshOnboardingCompressions(data, proposal.Compressions)
			if len(proposal.Compressions) == 0 {
				return fmt.Errorf("no locally valid narrative compression")
			}
			accepted = proposal
			return nil
		},
	)
	if err != nil {
		return metrics, true, err
	}
	accepted.PreferredArtifactID = baseline.PreferredArtifactID
	if err := writeGoldenJSON(path, accepted); err != nil {
		return metrics, true, err
	}
	return metrics, true, nil
}

func buildFreshOnboardingEditorBundle(data *report.ReportData) freshOnboardingEditorBundle {
	bundle := freshOnboardingEditorBundle{Version: report.RepositoryOnboardingVersion}
	if data == nil {
		return bundle
	}
	userByArtifact := make(map[string]report.UserMechanism, len(data.UserMechanisms))
	for _, mechanism := range data.UserMechanisms {
		userByArtifact[mechanism.ArtifactID] = mechanism
	}
	artifacts := append([]semanticdiscovery.Artifact(nil), data.SemanticArtifacts...)
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].ID < artifacts[j].ID })
	for _, artifact := range artifacts {
		mechanism, ok := userByArtifact[artifact.ID]
		if !ok || len(artifact.Steps) < 3 {
			continue
		}
		supported := make(map[string]struct{}, len(artifact.Statements))
		for _, statement := range artifact.Statements {
			if statement.Basis == semanticdiscovery.ClaimDirect ||
				statement.Basis == semanticdiscovery.ClaimCompositional {
				supported[statement.ID] = struct{}{}
			}
		}
		item := freshOnboardingEditorMechanism{
			ArtifactID: artifact.ID,
			Question:   mechanism.Question,
			Answer:     mechanism.Answer,
		}
		valid := true
		for _, step := range artifact.Steps {
			projected := freshOnboardingEditorStep{
				Title: step.Title, Explanation: step.Explanation,
			}
			for _, statementID := range step.StatementIDs {
				if _, ok := supported[statementID]; ok {
					projected.StatementIDs = append(projected.StatementIDs, statementID)
				}
			}
			if len(projected.StatementIDs) == 0 {
				valid = false
				break
			}
			item.Steps = append(item.Steps, projected)
		}
		if !valid || len(item.Steps) < 3 {
			continue
		}
		bundle.Mechanisms = append(bundle.Mechanisms, item)
		if len(bundle.Mechanisms) == freshOnboardingEditorMaxMechanisms {
			break
		}
	}
	return bundle
}

func validateFreshOnboardingCompressions(
	data *report.ReportData,
	compressions []report.NarrativeCompression,
) []report.NarrativeCompression {
	if data == nil {
		return nil
	}
	mechanisms := make(map[string]report.UserMechanism, len(data.UserMechanisms))
	for _, mechanism := range data.UserMechanisms {
		mechanisms[mechanism.ArtifactID] = mechanism
	}
	seen := make(map[string]struct{}, len(compressions))
	accepted := make([]report.NarrativeCompression, 0, len(compressions))
	for _, compression := range compressions {
		artifactID := strings.TrimSpace(compression.ArtifactID)
		if _, duplicate := seen[artifactID]; duplicate {
			continue
		}
		mechanism, ok := mechanisms[artifactID]
		if !ok {
			continue
		}
		if _, ok := report.ProjectNarrativeCompression(mechanism, compression); !ok {
			continue
		}
		seen[artifactID] = struct{}{}
		accepted = append(accepted, compression)
	}
	sort.Slice(accepted, func(i, j int) bool {
		return accepted[i].ArtifactID < accepted[j].ArtifactID
	})
	return accepted
}
