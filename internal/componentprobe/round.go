package componentprobe

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/evidence"
)

// SHA256 returns the canonical JSON digest of a validated bundle. Struct field
// order is fixed and encoding/json orders map keys, making the binding stable
// for the same accepted artifact.
func SHA256(bundle Bundle) (string, error) {
	if err := bundle.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		return "", fmt.Errorf("component probe: marshal bundle digest: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// ValidateAgainst verifies a round-two bundle's cryptographic and semantic
// parent binding. Validate alone checks only the self-contained round shape.
func (b Bundle) ValidateAgainst(prior Bundle) error {
	if err := prior.Validate(); err != nil {
		return fmt.Errorf("component probe: invalid prior bundle: %w", err)
	}
	if err := b.Validate(); err != nil {
		return err
	}
	if prior.Round != RoundInitial || b.Round != RoundFrontier {
		return fmt.Errorf("component probe: frontier chaining requires round 1 -> round 2")
	}
	if len(b.SymbolProbes) > 1 {
		return fmt.Errorf("component probe: frontier round contains more than one symbol probe")
	}
	if !reflect.DeepEqual(b.Focus.Goal, prior.Focus.Goal) ||
		!reflect.DeepEqual(b.Focus.Component, prior.Focus.Component) {
		return fmt.Errorf("component probe: frontier round changed the research focus")
	}
	accepted, err := eligibleFrontier(prior, b.Parent.AcceptedFrontierID)
	if err != nil {
		return err
	}
	wantDigest, err := SHA256(prior)
	if err != nil {
		return err
	}
	if b.Parent.BundleSHA256 != wantDigest {
		return fmt.Errorf("component probe: frontier round parent digest does not match prior bundle")
	}
	if len(b.SymbolProbes) == 1 && !frontierMatchesSelected(accepted, b.SymbolProbes[0].SelectedSymbol) {
		return fmt.Errorf("component probe: frontier round probed a different symbol")
	}
	return nil
}

// CollectFrontier accepts one opaque frontier ID from a validated initial
// round. Paths and symbol names are recovered only from that local artifact.
func CollectFrontier(
	ctx context.Context,
	provider Provider,
	repoPath string,
	prior Bundle,
	opaqueFrontierID string,
	opts Options,
) (Bundle, error) {
	if err := prior.Validate(); err != nil {
		return Bundle{}, fmt.Errorf("component probe: invalid prior bundle: %w", err)
	}
	if prior.Round != RoundInitial {
		return Bundle{}, fmt.Errorf("component probe: only an initial round can be expanded")
	}
	accepted, err := eligibleFrontier(prior, opaqueFrontierID)
	if err != nil {
		return Bundle{}, err
	}
	digest, err := SHA256(prior)
	if err != nil {
		return Bundle{}, err
	}
	study, plan, err := frontierStudy(prior, accepted)
	if err != nil {
		return Bundle{}, err
	}
	next, collectErr := Collect(ctx, provider, repoPath, study, plan, opts)
	if next.Version != BundleVersion {
		return Bundle{}, collectErr
	}
	next.Round = RoundFrontier
	next.Parent = &Parent{
		BundleSHA256:       digest,
		AcceptedFrontierID: accepted.ID,
	}
	if err := next.ValidateAgainst(prior); err != nil {
		return Bundle{}, err
	}
	if collectErr != nil {
		return next, fmt.Errorf("component probe: collect accepted frontier: %w", collectErr)
	}
	return next, nil
}

func eligibleFrontier(bundle Bundle, id string) (Frontier, error) {
	if !globalIDPattern.MatchString(id) {
		return Frontier{}, fmt.Errorf("component probe: invalid opaque frontier id")
	}
	for _, candidate := range bundle.Frontier {
		if candidate.ID != id {
			continue
		}
		if candidate.Kind != FrontierCallEndpoint ||
			(candidate.EntityKind != evidence.EntityFunction && candidate.EntityKind != evidence.EntityMethod) ||
			candidate.Certainty != evidence.CertaintyStatic ||
			candidate.Basis != SupportStaticNavigation || candidate.RuntimeProof {
			return Frontier{}, fmt.Errorf("component probe: frontier is not an expandable static callable")
		}
		return candidate, nil
	}
	return Frontier{}, fmt.Errorf("component probe: opaque frontier id is absent from the prior bundle")
}

func frontierStudy(
	prior Bundle,
	accepted Frontier,
) (componentstudy.Bundle, componentstudy.Plan, error) {
	if len(prior.SymbolProbes) == 0 {
		return componentstudy.Bundle{}, componentstudy.Plan{}, fmt.Errorf("component probe: prior bundle has no repository identity")
	}
	provenance := componentstudy.Provenance{
		Source:    "componentprobe",
		Operation: "accepted_frontier",
		Detail:    string(accepted.Direction),
	}
	file := componentstudy.FileCandidate{
		ID:         stableID("file", "frontier", accepted.ID, accepted.Location.Path),
		Rank:       1,
		Path:       accepted.Location.Path,
		Reason:     "contains the accepted static call frontier",
		Provenance: provenance,
		Certainty:  componentstudy.CertaintyStatic,
	}
	selected := componentstudy.SymbolCandidate{
		ID:         stableID("symbol", "frontier", accepted.ID),
		Rank:       1,
		Name:       accepted.Name,
		Kind:       string(accepted.EntityKind),
		Path:       accepted.Location.Path,
		Line:       accepted.Location.Line,
		Column:     accepted.Location.Column,
		Reason:     "accepted static callable from the prior frontier",
		Provenance: provenance,
		Certainty:  componentstudy.CertaintyStatic,
	}
	question := prior.Focus.PrimaryQuestion
	question.EvidenceIDs = []string{selected.ID}
	study := componentstudy.Bundle{
		Version:   componentstudy.BundleVersion,
		RepoName:  prior.SymbolProbes[0].Structural.RepoName,
		Goal:      prior.Focus.Goal,
		Component: prior.Focus.Component,
		Anchors:   []componentstudy.AnchorCandidate{},
		Files:     []componentstudy.FileCandidate{file},
		Symbols:   []componentstudy.SymbolCandidate{selected},
		Evidence:  []componentstudy.EvidenceCandidate{},
	}
	plan := componentstudy.Plan{
		Version:           componentstudy.PlanVersion,
		Framing:           "Continue through one accepted static call frontier without importing prior source artifacts.",
		Questions:         []componentstudy.Question{question},
		PrimaryQuestionID: question.ID,
		SelectedFiles:     []componentstudy.FileCandidate{file},
		SelectedSymbols:   []componentstudy.SymbolCandidate{selected},
		Unknowns:          []string{},
		Warnings:          []string{},
	}
	if err := study.Validate(); err != nil {
		return componentstudy.Bundle{}, componentstudy.Plan{}, fmt.Errorf("component probe: synthesize frontier study: %w", err)
	}
	if err := plan.Validate(study); err != nil {
		return componentstudy.Bundle{}, componentstudy.Plan{}, fmt.Errorf("component probe: synthesize frontier plan: %w", err)
	}
	return study, plan, nil
}

func frontierMatchesSelected(frontier Frontier, selected componentstudy.SymbolCandidate) bool {
	return frontier.Name == selected.Name && string(frontier.EntityKind) == selected.Kind &&
		frontier.Location.Path == selected.Path && frontier.Location.Line == selected.Line &&
		frontier.Location.Column == selected.Column
}
