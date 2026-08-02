package surfacediscovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const GroupingContractVersion = 1

type GroupingBundle struct {
	Version  int               `json:"version"`
	Triggers []GroupingTrigger `json:"triggers"`
	Coverage GroupingCoverage  `json:"coverage"`
}

type GroupingTrigger struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Method       string   `json:"method,omitempty"`
	Path         string   `json:"path"`
	Framework    string   `json:"framework"`
	Handler      string   `json:"handler"`
	Registration Location `json:"registration"`
	Status       string   `json:"status"`
	Frontiers    []string `json:"frontiers"`
}

type GroupingCoverage struct {
	ScopeStatement       string `json:"scope_statement"`
	DirectTriggers       int    `json:"direct_triggers"`
	WrapperDerived       int    `json:"wrapper_derived_triggers"`
	PossibleRegistration int    `json:"possible_registrations"`
}

type GroupingResponse struct {
	Version         int                     `json:"version"`
	Groups          []TriggerGroup          `json:"groups"`
	Recommendations []TriggerRecommendation `json:"recommendations"`
	NextQuestion    string                  `json:"next_question,omitempty"`
}

type TriggerGroup struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	TriggerIDs []string `json:"trigger_ids"`
}

type TriggerRecommendation struct {
	TriggerID string `json:"trigger_id"`
	Name      string `json:"name"`
	Reason    string `json:"reason"`
}

type GroupingValidation struct {
	Valid              bool     `json:"valid"`
	SuppliedTriggerIDs []string `json:"supplied_trigger_ids"`
	ReferencedIDs      []string `json:"referenced_ids"`
	Warnings           []string `json:"warnings"`
}

func BuildGroupingBundle(result Result) GroupingBundle {
	bundle := GroupingBundle{
		Version:  GroupingContractVersion,
		Triggers: []GroupingTrigger{},
		Coverage: GroupingCoverage{
			ScopeStatement:       result.Coverage.ScopeStatement,
			DirectTriggers:       result.Coverage.DirectTriggers,
			WrapperDerived:       result.Coverage.WrapperDerivedTriggers,
			PossibleRegistration: result.Coverage.PossibleRegistrations,
		},
	}
	for _, trigger := range result.Catalog.Triggers {
		frontiers := make([]string, 0, len(trigger.DynamicFrontier))
		for _, frontier := range trigger.DynamicFrontier {
			frontiers = append(frontiers, frontier.Kind+": "+frontier.Detail)
		}
		sort.Strings(frontiers)
		path := trigger.Identity.Path.Text
		if path == "" {
			path = trigger.Identity.Name
		}
		bundle.Triggers = append(bundle.Triggers, GroupingTrigger{
			ID: trigger.ID, Kind: trigger.Kind, Method: trigger.Identity.Method,
			Path: path, Framework: trigger.Framework,
			Handler: trigger.Handler.Text, Registration: trigger.RegistrationSite,
			Status: trigger.Status, Frontiers: frontiers,
		})
	}
	sort.Slice(bundle.Triggers, func(i, j int) bool { return bundle.Triggers[i].ID < bundle.Triggers[j].ID })
	return bundle
}

func ParseGroupingResponse(data []byte, bundle GroupingBundle) (GroupingResponse, GroupingValidation, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var response GroupingResponse
	if err := decoder.Decode(&response); err != nil {
		return GroupingResponse{}, GroupingValidation{}, fmt.Errorf("surface grouping: decode: %w", err)
	}
	validation := GroupingValidation{
		Valid: true, SuppliedTriggerIDs: []string{}, ReferencedIDs: []string{}, Warnings: []string{},
	}
	allowed := map[string]bool{}
	for _, trigger := range bundle.Triggers {
		allowed[trigger.ID] = true
		validation.SuppliedTriggerIDs = append(validation.SuppliedTriggerIDs, trigger.ID)
	}
	if response.Version != GroupingContractVersion {
		validation.Valid = false
		return GroupingResponse{}, validation, fmt.Errorf("surface grouping: unsupported version %d", response.Version)
	}
	seenReferences := map[string]bool{}
	seenGroups := map[string]bool{}
	for _, group := range response.Groups {
		if strings.TrimSpace(group.ID) == "" || strings.TrimSpace(group.Name) == "" || seenGroups[group.ID] {
			validation.Valid = false
			return GroupingResponse{}, validation, fmt.Errorf("surface grouping: group ids and names must be non-empty and unique")
		}
		seenGroups[group.ID] = true
		for _, id := range group.TriggerIDs {
			if !allowed[id] {
				validation.Valid = false
				return GroupingResponse{}, validation, fmt.Errorf("surface grouping: group %q references unknown trigger id %q", group.ID, id)
			}
			seenReferences[id] = true
		}
	}
	for _, recommendation := range response.Recommendations {
		if !allowed[recommendation.TriggerID] {
			validation.Valid = false
			return GroupingResponse{}, validation, fmt.Errorf("surface grouping: recommendation references unknown trigger id %q", recommendation.TriggerID)
		}
		if strings.TrimSpace(recommendation.Name) == "" || strings.TrimSpace(recommendation.Reason) == "" {
			validation.Valid = false
			return GroupingResponse{}, validation, fmt.Errorf("surface grouping: recommendation name and reason are required")
		}
		seenReferences[recommendation.TriggerID] = true
	}
	for id := range seenReferences {
		validation.ReferencedIDs = append(validation.ReferencedIDs, id)
	}
	sort.Strings(validation.SuppliedTriggerIDs)
	sort.Strings(validation.ReferencedIDs)
	sort.Slice(response.Groups, func(i, j int) bool { return response.Groups[i].ID < response.Groups[j].ID })
	for index := range response.Groups {
		sort.Strings(response.Groups[index].TriggerIDs)
	}
	sort.Slice(response.Recommendations, func(i, j int) bool {
		return response.Recommendations[i].TriggerID < response.Recommendations[j].TriggerID
	})
	return response, validation, nil
}

func DeterministicGrouping(bundle GroupingBundle) GroupingResponse {
	response := GroupingResponse{
		Version:         GroupingContractVersion,
		Groups:          []TriggerGroup{{ID: "http-surfaces", Name: "HTTP surfaces", TriggerIDs: []string{}}},
		Recommendations: []TriggerRecommendation{},
	}
	for _, trigger := range bundle.Triggers {
		response.Groups[0].TriggerIDs = append(response.Groups[0].TriggerIDs, trigger.ID)
		if len(response.Recommendations) == 0 && trigger.Status != "dynamic_unknown" {
			name := strings.TrimSpace(trigger.Method + " " + trigger.Path)
			response.Recommendations = append(response.Recommendations, TriggerRecommendation{
				TriggerID: trigger.ID, Name: name,
				Reason: "grounded route registration with a statically identified handler",
			})
		}
	}
	return response
}

func WriteGroupingReplayArtifacts(
	directory string,
	bundle GroupingBundle,
	rawResponse []byte,
) error {
	normalized, validation, err := ParseGroupingResponse(rawResponse, bundle)
	if err != nil {
		return err
	}
	fallback := DeterministicGrouping(bundle)
	bundleJSON, err := MarshalDeterministic(bundle)
	if err != nil {
		return err
	}
	requestJSON, err := MarshalDeterministic(struct {
		Contract string         `json:"contract"`
		Input    GroupingBundle `json:"input"`
	}{Contract: "surface-grouping-v1", Input: bundle})
	if err != nil {
		return err
	}
	normalizedJSON, err := MarshalDeterministic(normalized)
	if err != nil {
		return err
	}
	validationJSON, err := MarshalDeterministic(validation)
	if err != nil {
		return err
	}
	comparison := groupingComparison(len(bundle.Triggers), bundleJSON, rawResponse, fallback, normalized)
	files := []struct {
		name string
		data []byte
	}{
		{name: "bundle.json", data: bundleJSON},
		{name: "request.redacted.json", data: requestJSON},
		{name: "response.raw.txt", data: append(append([]byte{}, rawResponse...), '\n')},
		{name: "normalized.json", data: normalizedJSON},
		{name: "validation.json", data: validationJSON},
		{name: "comparison.md", data: []byte(comparison)},
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("surface grouping: create artifact directory: %w", err)
	}
	for _, file := range files {
		if err := writeAtomic(filepath.Join(directory, file.name), file.data); err != nil {
			return err
		}
	}
	return nil
}

func groupingComparison(
	triggerCount int,
	bundleJSON []byte,
	raw []byte,
	fallback GroupingResponse,
	model GroupingResponse,
) string {
	return fmt.Sprintf(
		"# Surface grouping replay\n\n- Trigger count: %d\n- Bundle bytes: %d\n- Saved response bytes: %d\n- Live calls: 0\n- Deterministic groups/recommendations: %d/%d\n- Model-assisted groups/recommendations: %d/%d\n- Unsupported trigger references: 0 (locally validated)\n\nThe model can name, group, and recommend supplied opaque IDs; it does not determine trigger existence, certainty, handlers, or edges.\n",
		triggerCount,
		len(bundleJSON),
		len(raw),
		len(fallback.Groups),
		len(fallback.Recommendations),
		len(model.Groups),
		len(model.Recommendations),
	)
}
