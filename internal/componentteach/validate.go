package componentteach

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (b Bundle) Validate() error {
	if b.Version != BundleVersion {
		return fmt.Errorf("component teach: unsupported bundle version %d", b.Version)
	}
	if strings.TrimSpace(b.GoalObjective) == "" || strings.TrimSpace(b.Component.Name) == "" ||
		strings.TrimSpace(b.Component.PurposeHypothesis) == "" {
		return fmt.Errorf("component teach: bundle focus is incomplete")
	}
	if b.Component.SupportBasis != SupportOrientationHypothesis {
		return fmt.Errorf("component teach: component purpose must remain a hypothesis")
	}
	if strings.TrimSpace(b.PrimaryQuestion.ID) == "" || strings.TrimSpace(b.PrimaryQuestion.Question) == "" ||
		strings.TrimSpace(b.PrimaryQuestion.Why) == "" {
		return fmt.Errorf("component teach: primary question is incomplete")
	}
	if len(b.Evidence) == 0 {
		return fmt.Errorf("component teach: bundle has no included evidence")
	}
	known := make(map[string]struct{}, len(b.Evidence))
	for index, item := range b.Evidence {
		if err := validateEvidenceItem(item); err != nil {
			return fmt.Errorf("component teach: evidence[%d]: %w", index, err)
		}
		if _, exists := known[item.ID]; exists {
			return fmt.Errorf("component teach: duplicate evidence id %q", item.ID)
		}
		known[item.ID] = struct{}{}
	}
	frontier := make(map[string]struct{}, len(b.UnresolvedFrontierIDs))
	for _, id := range b.UnresolvedFrontierIDs {
		if !frontierIDPattern.MatchString(id) {
			return fmt.Errorf("component teach: invalid frontier id")
		}
		if _, exists := frontier[id]; exists {
			return fmt.Errorf("component teach: duplicate frontier id")
		}
		frontier[id] = struct{}{}
	}
	if len(b.UnresolvedFrontiers) != len(frontier) {
		return fmt.Errorf("component teach: frontier hints do not match unresolved frontier ids")
	}
	seenHints := make(map[string]struct{}, len(b.UnresolvedFrontiers))
	for _, hint := range b.UnresolvedFrontiers {
		if _, exists := frontier[hint.ID]; !exists || strings.TrimSpace(hint.Name) == "" ||
			strings.TrimSpace(hint.Kind) == "" || strings.TrimSpace(hint.Direction) == "" || strings.TrimSpace(hint.EntityKind) == "" {
			return fmt.Errorf("component teach: invalid unresolved frontier hint")
		}
		if _, exists := seenHints[hint.ID]; exists {
			return fmt.Errorf("component teach: duplicate unresolved frontier hint")
		}
		seenHints[hint.ID] = struct{}{}
		if hint.NavigationOnly {
			if hint.SupportBasis != SupportTestNavigation {
				return fmt.Errorf("component teach: navigation frontier overstates support")
			}
		} else if hint.SupportBasis != SupportStaticActiveBuild {
			return fmt.Errorf("component teach: callable frontier lost active-build support")
		}
	}
	if size, err := modelBytes(b); err != nil {
		return err
	} else if size > MaxModelBytes {
		return fmt.Errorf("component teach: bundle exceeds hard model byte bound")
	}
	return nil
}

func validateEvidenceItem(item EvidenceItem) error {
	if !teacherEvidenceIDPattern.MatchString(item.ID) || strings.TrimSpace(item.Summary) == "" {
		return fmt.Errorf("evidence identity is incomplete")
	}
	if len(item.Summary) > maxItemText {
		return fmt.Errorf("evidence summary exceeds text bound")
	}
	switch item.Kind {
	case EvidenceOrientationNote:
		if item.SupportBasis != SupportOrientationHypothesis || len(item.Content) != 0 {
			return fmt.Errorf("orientation note overstates its support")
		}
	case EvidenceStaticRelation:
		if item.SupportBasis != SupportStaticActiveBuild || item.Caller == "" || item.Callee == "" ||
			item.Direction == "" || item.ActiveBuildCaveat != activeBuildCaveat || len(item.Content) != 0 {
			return fmt.Errorf("static relation is incomplete")
		}
	case EvidenceSourceSlice, EvidenceCallsiteSlice:
		if item.SupportBasis != SupportSource || len(item.Content) == 0 || len(item.Content) > maxSliceLines {
			return fmt.Errorf("source slice is incomplete")
		}
		bytes := 0
		for index, line := range item.Content {
			if index > 0 {
				bytes++
			}
			bytes += len(line)
		}
		if bytes > maxSliceBytes {
			return fmt.Errorf("source slice exceeds byte bound")
		}
	case EvidenceTestReference:
		if item.SupportBasis != SupportTestNavigation || !item.NavigationOnly || len(item.Content) != 0 {
			return fmt.Errorf("test reference is not navigation-only")
		}
	default:
		return fmt.Errorf("invalid evidence kind %q", item.Kind)
	}
	return nil
}

func (i Index) Validate(bundle Bundle) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	if i.Version != IndexVersion {
		return fmt.Errorf("component teach: unsupported index version %d", i.Version)
	}
	want := make(map[string]LocatorKind, len(bundle.Evidence)+len(bundle.UnresolvedFrontierIDs))
	for _, item := range bundle.Evidence {
		want[item.ID] = LocatorEvidence
	}
	for _, id := range bundle.UnresolvedFrontierIDs {
		want[id] = LocatorFrontier
	}
	seen := make(map[string]struct{}, len(i.Entries))
	for index, entry := range i.Entries {
		kind, exists := want[entry.ID]
		if !exists || kind != entry.Kind {
			return fmt.Errorf("component teach: index entry[%d] is not model-addressable", index)
		}
		if _, exists := seen[entry.ID]; exists {
			return fmt.Errorf("component teach: duplicate locator id %q", entry.ID)
		}
		if !validLocatorPath(entry.Path) || entry.StartLine <= 0 || entry.EndLine < entry.StartLine || entry.Column < 0 || len(entry.Origins) == 0 {
			return fmt.Errorf("component teach: index entry[%d] has invalid local locator", index)
		}
		for _, origin := range entry.Origins {
			if (origin.Round != 1 && origin.Round != 2) || origin.ProbeID == "" || origin.Artifact == "" || origin.LocalID == "" {
				return fmt.Errorf("component teach: index entry[%d] has invalid origin", index)
			}
		}
		seen[entry.ID] = struct{}{}
	}
	if len(seen) != len(want) {
		return fmt.Errorf("component teach: index does not locate every evidence and frontier id")
	}
	return nil
}

func (t SelectionTrace) Validate() error {
	if t.Version != SelectionTraceVersion {
		return fmt.Errorf("component teach: unsupported selection trace version %d", t.Version)
	}
	if err := t.Budget.Validate(); err != nil {
		return err
	}
	if t.EstimatedModelBytes <= 0 || t.EstimatedModelBytes > t.Budget.MaxModelBytes {
		return fmt.Errorf("component teach: invalid estimated model size")
	}
	for index, decision := range t.Decisions {
		if decision.ID == "" || decision.Kind == "" || decision.Reason == "" || decision.EstimatedBytes < 0 {
			return fmt.Errorf("component teach: selection decision[%d] is incomplete", index)
		}
		if decision.Included != (decision.Reason == SelectionWithinBudget) {
			return fmt.Errorf("component teach: selection decision[%d] has inconsistent outcome", index)
		}
	}
	return nil
}

// MarshalModelBundle exists primarily for experiment/debug callers that want
// to assert that the local Index is never mixed into provider input.
func MarshalModelBundle(bundle Bundle) ([]byte, error) {
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("component teach: marshal model bundle: %w", err)
	}
	return data, nil
}
