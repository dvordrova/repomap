package report

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/localization"
)

// buildArchitectureLocalization extracts only model-authored prose from an
// Architecture Canvas previously returned by ProjectArchitectureCanvas.
// Stable ownership comes from locally reconstructed member-set IDs;
// presentation order and prose never participate in identity.
func buildArchitectureLocalization(
	canvas ArchitectureCanvas,
	targetLocale string,
) (localization.CanonicalArtifact, localization.Input, error) {
	specs := make([]localization.FieldSpec, 0, 2*(len(canvas.Subsystems)+len(canvas.Components)))
	components := make(map[componentmap.ComponentID]ArchitectureComponent, len(canvas.Components))
	for _, component := range canvas.Components {
		components[component.ID] = component
	}
	for _, subsystem := range canvas.Subsystems {
		protected := subsystemProtectedValues(subsystem, components)
		specs = append(specs, localization.FieldSpec{
			OwnerKind:      localization.OwnerSubsystem,
			OwnerID:        string(subsystem.ID),
			Name:           localization.FieldNameText,
			Text:           subsystem.Name,
			ProtectedTerms: presentProtectedValues(subsystem.Name, protected),
		})
		if subsystem.Description != "" {
			specs = append(specs, localization.FieldSpec{
				OwnerKind:      localization.OwnerSubsystem,
				OwnerID:        string(subsystem.ID),
				Name:           localization.FieldDescription,
				Text:           subsystem.Description,
				ProtectedTerms: presentProtectedValues(subsystem.Description, protected),
			})
		}
	}
	for _, component := range canvas.Components {
		protected := componentProtectedValues(component)
		specs = append(specs, localization.FieldSpec{
			OwnerKind:      localization.OwnerComponent,
			OwnerID:        string(component.ID),
			Name:           localization.FieldNameText,
			Text:           component.Name,
			ProtectedTerms: presentProtectedValues(component.Name, protected),
		})
		if component.Description != "" {
			specs = append(specs, localization.FieldSpec{
				OwnerKind:      localization.OwnerComponent,
				OwnerID:        string(component.ID),
				Name:           localization.FieldDescription,
				Text:           component.Description,
				ProtectedTerms: presentProtectedValues(component.Description, protected),
			})
		}
	}

	canonical, err := localization.NewCanonical(specs)
	if err != nil {
		return localization.CanonicalArtifact{}, localization.Input{},
			fmt.Errorf("architecture localization: build canonical artifact: %w", err)
	}
	input, err := localization.BuildInput(canonical, targetLocale)
	if err != nil {
		return localization.CanonicalArtifact{}, localization.Input{},
			fmt.Errorf("architecture localization: build input: %w", err)
	}
	return canonical, input, nil
}

// applyArchitectureLocalization validates and applies one supplied projection
// to a copy of canvas. Invalid projection fields fall back through the
// localization contract; no caller-visible mutation occurs before the complete
// projected field set has been matched to the current canvas.
func applyArchitectureLocalization(
	canvas ArchitectureCanvas,
	canonical localization.CanonicalArtifact,
	input localization.Input,
	projection localization.Projection,
) (ArchitectureCanvas, localization.Result, error) {
	expectedCanonical, expectedInput, err := buildArchitectureLocalization(canvas, input.TargetLocale)
	if err != nil {
		return canvas, localization.Result{}, fmt.Errorf(
			"architecture localization: bind current canvas: %w",
			err,
		)
	}
	canonicalJSON, err := localization.MarshalCanonical(canonical)
	if err != nil {
		return canvas, localization.Result{}, fmt.Errorf(
			"architecture localization: validate canonical artifact: %w",
			err,
		)
	}
	expectedCanonicalJSON, err := localization.MarshalCanonical(expectedCanonical)
	if err != nil {
		return canvas, localization.Result{}, fmt.Errorf(
			"architecture localization: validate current canvas canonical artifact: %w",
			err,
		)
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return canvas, localization.Result{}, fmt.Errorf(
			"architecture localization: encode localization input: %w",
			err,
		)
	}
	expectedInputJSON, err := json.Marshal(expectedInput)
	if err != nil {
		return canvas, localization.Result{}, fmt.Errorf(
			"architecture localization: encode current canvas localization input: %w",
			err,
		)
	}
	if !bytes.Equal(canonicalJSON, expectedCanonicalJSON) ||
		!bytes.Equal(inputJSON, expectedInputJSON) {
		return canvas, localization.Result{}, fmt.Errorf(
			"architecture localization: canonical input does not match current canvas",
		)
	}

	result, err := localization.Apply(canonical, input, projection)
	if err != nil {
		return canvas, localization.Result{}, fmt.Errorf(
			"architecture localization: apply projection: %w",
			err,
		)
	}

	expectedIDs, err := architectureLocalizationFieldIDs(canvas)
	if err != nil {
		return canvas, result, err
	}
	if len(result.Fields) != len(expectedIDs) {
		return canvas, result, fmt.Errorf("architecture localization: projected field set mismatch")
	}
	projectedByID := make(map[string]string, len(expectedIDs))
	maxIDBytes := 0
	for id := range expectedIDs {
		if len(id) > maxIDBytes {
			maxIDBytes = len(id)
		}
	}
	for _, field := range result.Fields {
		if len(field.ID) > maxIDBytes {
			return canvas, result, fmt.Errorf("architecture localization: projected field set mismatch")
		}
		if _, ok := expectedIDs[field.ID]; !ok {
			return canvas, result, fmt.Errorf("architecture localization: projected field set mismatch")
		}
		if _, duplicate := projectedByID[field.ID]; duplicate {
			return canvas, result, fmt.Errorf("architecture localization: projected field set mismatch")
		}
		projectedByID[field.ID] = field.Text
	}

	projected := canvas
	if canvas.Subsystems != nil {
		projected.Subsystems = make([]ArchitectureSubsystem, len(canvas.Subsystems))
		copy(projected.Subsystems, canvas.Subsystems)
	}
	if canvas.Components != nil {
		projected.Components = make([]ArchitectureComponent, len(canvas.Components))
		copy(projected.Components, canvas.Components)
	}
	for index := range projected.Subsystems {
		subsystem := &projected.Subsystems[index]
		subsystem.Name = projectedByID[mustArchitectureFieldID(
			localization.OwnerSubsystem,
			string(subsystem.ID),
			localization.FieldNameText,
		)]
		if subsystem.Description != "" {
			subsystem.Description = projectedByID[mustArchitectureFieldID(
				localization.OwnerSubsystem,
				string(subsystem.ID),
				localization.FieldDescription,
			)]
		}
	}
	for index := range projected.Components {
		component := &projected.Components[index]
		component.Name = projectedByID[mustArchitectureFieldID(
			localization.OwnerComponent,
			string(component.ID),
			localization.FieldNameText,
		)]
		if component.Description != "" {
			component.Description = projectedByID[mustArchitectureFieldID(
				localization.OwnerComponent,
				string(component.ID),
				localization.FieldDescription,
			)]
		}
	}
	return projected, result, nil
}

func architectureLocalizationFieldIDs(canvas ArchitectureCanvas) (map[string]struct{}, error) {
	ids := make(map[string]struct{}, 2*(len(canvas.Subsystems)+len(canvas.Components)))
	add := func(kind localization.OwnerKind, ownerID string, name localization.FieldName) error {
		id, err := localization.FieldID(kind, ownerID, name)
		if err != nil {
			return err
		}
		if _, duplicate := ids[id]; duplicate {
			return fmt.Errorf("architecture localization: duplicate semantic owner")
		}
		ids[id] = struct{}{}
		return nil
	}
	for _, subsystem := range canvas.Subsystems {
		if err := add(localization.OwnerSubsystem, string(subsystem.ID), localization.FieldNameText); err != nil {
			return nil, err
		}
		if subsystem.Description != "" {
			if err := add(localization.OwnerSubsystem, string(subsystem.ID), localization.FieldDescription); err != nil {
				return nil, err
			}
		}
	}
	for _, component := range canvas.Components {
		if err := add(localization.OwnerComponent, string(component.ID), localization.FieldNameText); err != nil {
			return nil, err
		}
		if component.Description != "" {
			if err := add(localization.OwnerComponent, string(component.ID), localization.FieldDescription); err != nil {
				return nil, err
			}
		}
	}
	return ids, nil
}

func mustArchitectureFieldID(
	kind localization.OwnerKind,
	ownerID string,
	name localization.FieldName,
) string {
	id, err := localization.FieldID(kind, ownerID, name)
	if err != nil {
		panic(err)
	}
	return id
}

func subsystemProtectedValues(
	subsystem ArchitectureSubsystem,
	components map[componentmap.ComponentID]ArchitectureComponent,
) []localization.ProtectedValue {
	values := []localization.ProtectedValue{
		{Kind: localization.ProtectedIdentifier, Value: string(subsystem.ID)},
	}
	for _, sourceID := range subsystem.SourceIDs {
		values = append(values, localization.ProtectedValue{
			Kind:  localization.ProtectedIdentifier,
			Value: string(sourceID),
		})
	}
	for _, componentID := range subsystem.ComponentIDs {
		values = append(values, localization.ProtectedValue{
			Kind:  localization.ProtectedIdentifier,
			Value: string(componentID),
		})
		if component, ok := components[componentID]; ok {
			values = append(values, componentProtectedValues(component)...)
		}
	}
	return values
}

func componentProtectedValues(component ArchitectureComponent) []localization.ProtectedValue {
	values := []localization.ProtectedValue{
		{Kind: localization.ProtectedIdentifier, Value: string(component.ID)},
		{Kind: localization.ProtectedIdentifier, Value: string(component.SubsystemID)},
	}
	for _, flowID := range component.ParticipatingFlowIDs {
		values = append(values, localization.ProtectedValue{
			Kind:  localization.ProtectedIdentifier,
			Value: string(flowID),
		})
	}
	for _, surfaceID := range component.OwnedSurfaceIDs {
		values = append(values, localization.ProtectedValue{
			Kind:  localization.ProtectedIdentifier,
			Value: surfaceID,
		})
	}
	for _, surfaceID := range component.ParticipatingSurfaceIDs {
		values = append(values, localization.ProtectedValue{
			Kind:  localization.ProtectedIdentifier,
			Value: surfaceID,
		})
	}
	for _, investigationID := range component.SuggestedInvestigationIDs {
		values = append(values, localization.ProtectedValue{
			Kind:  localization.ProtectedIdentifier,
			Value: investigationID,
		})
	}
	for _, anchorID := range component.AnchorIDs {
		values = append(values, localization.ProtectedValue{
			Kind:  localization.ProtectedIdentifier,
			Value: anchorID,
		})
	}
	for _, sourceID := range component.SourceIDs {
		values = append(values, localization.ProtectedValue{
			Kind:  localization.ProtectedIdentifier,
			Value: string(sourceID),
		})
	}
	for _, member := range component.Members {
		values = append(values,
			localization.ProtectedValue{
				Kind:  protectedMemberKind(member.ID.Kind),
				Value: member.ID.Value,
			},
			localization.ProtectedValue{
				Kind:  protectedMemberKind(member.ID.Kind),
				Value: member.Name,
			},
		)
		if member.ParentID != nil {
			values = append(values, localization.ProtectedValue{
				Kind:  protectedMemberKind(member.ParentID.Kind),
				Value: member.ParentID.Value,
			})
		}
		for _, participation := range member.Participations {
			values = append(values, localization.ProtectedValue{
				Kind:  localization.ProtectedIdentifier,
				Value: string(participation.FlowID),
			})
			values = append(values, factProtectedValues(participation.Evidence)...)
		}
		for _, fact := range member.Facts {
			values = append(values, factProtectedValues(fact)...)
		}
	}
	return values
}

func factProtectedValues(fact componentmap.LocalFact) []localization.ProtectedValue {
	kind := localization.ProtectedIdentifier
	switch fact.Kind {
	case componentmap.FactRepositoryPath:
		kind = localization.ProtectedPath
	case componentmap.FactDeclaration:
		kind = localization.ProtectedSymbol
	case componentmap.FactContainment:
		kind = localization.ProtectedPackage
	}
	values := []localization.ProtectedValue{{Kind: kind, Value: fact.Value}}
	if fact.Location != nil {
		values = append(values, localization.ProtectedValue{
			Kind:  localization.ProtectedPath,
			Value: fact.Location.Path,
		})
	}
	return values
}

func protectedMemberKind(kind componentmap.MemberKind) localization.ProtectedKind {
	switch kind {
	case componentmap.MemberPackage:
		return localization.ProtectedPackage
	case componentmap.MemberFile:
		return localization.ProtectedPath
	case componentmap.MemberSymbol, componentmap.MemberEntrypoint:
		return localization.ProtectedSymbol
	default:
		return localization.ProtectedIdentifier
	}
}

func presentProtectedValues(
	text string,
	values []localization.ProtectedValue,
) []localization.ProtectedValue {
	seen := make(map[string]struct{})
	result := make([]localization.ProtectedValue, 0)
	for _, value := range values {
		if value.Value == "" || !localization.ContainsProtectedValue(text, value.Value) {
			continue
		}
		if _, duplicate := seen[value.Value]; duplicate {
			continue
		}
		seen[value.Value] = struct{}{}
		result = append(result, value)
	}
	return result
}
