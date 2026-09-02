package programindex

import "encoding/json"

const (
	advisoryTargetSources     = MaxTargetSources
	advisoryTargetSeeds       = MaxTargetSeeds
	advisoryObjects           = 65_536
	advisoryRelations         = 10_000
	advisoryTotalWitnesses    = MaxWitnesses
	advisoryTotalPatterns     = MaxPatterns
	advisoryTotalArguments    = MaxPatternArguments
	advisoryRelationTargets   = 64
	advisoryRelationWitnesses = 64
	advisoryRelationPatterns  = 64
	advisoryPatternArguments  = 128
	advisoryTemplateParts     = 64
	advisoryPatternObjectRefs = 64
	advisorySymbolLinks       = MaxSymbolLinkIdentitiesPerObject
	advisorySymbolLinkParts   = MaxSymbolLinkIdentityParts
	advisorySemanticTextBytes = MaxTextBytes
	advisoryObservedCount     = MaxObservedCount
)

// ScaleWarningKind identifies one formerly bounded local collection. These
// values are diagnostics only: they never authorize dropping or rejecting a
// retained ProgramIndex row.
type ScaleWarningKind string

const (
	ScaleWarningTargetSources     ScaleWarningKind = "target_sources"
	ScaleWarningTargetSeeds       ScaleWarningKind = "target_seeds"
	ScaleWarningObjects           ScaleWarningKind = "objects"
	ScaleWarningRelations         ScaleWarningKind = "relations"
	ScaleWarningTotalWitnesses    ScaleWarningKind = "total_witnesses"
	ScaleWarningTotalPatterns     ScaleWarningKind = "total_patterns"
	ScaleWarningTotalArguments    ScaleWarningKind = "total_pattern_arguments"
	ScaleWarningRelationTargets   ScaleWarningKind = "relation_targets"
	ScaleWarningRelationWitnesses ScaleWarningKind = "relation_witnesses"
	ScaleWarningRelationPatterns  ScaleWarningKind = "relation_patterns"
	ScaleWarningPatternArguments  ScaleWarningKind = "pattern_arguments"
	ScaleWarningTemplateParts     ScaleWarningKind = "template_parts"
	ScaleWarningPatternObjectRefs ScaleWarningKind = "pattern_object_refs"
	ScaleWarningSymbolLinks       ScaleWarningKind = "symbol_link_identities"
	ScaleWarningSymbolLinkParts   ScaleWarningKind = "symbol_link_identity_parts"
	ScaleWarningSemanticText      ScaleWarningKind = "semantic_text_bytes"
	ScaleWarningObservedCount     ScaleWarningKind = "observed_count"
	ScaleWarningAggregateText     ScaleWarningKind = "aggregate_semantic_text_bytes"
	ScaleWarningArtifactBytes     ScaleWarningKind = "program_index_artifact_bytes"
)

// ScaleWarning aggregates every unusually large collection of one kind. At
// most one warning per kind is returned, regardless of repository size.
// MaximumRetained is the largest complete collection present in the index.
type ScaleWarning struct {
	Kind                ScaleWarningKind
	AdvisorySize        int
	AffectedCollections int
	MaximumRetained     int
}

// ScaleWarnings reports unusually large retained collections. It is a pure,
// best-effort diagnostic over whatever value it receives: it neither validates
// semantic authority nor returns an error that could affect publication.
func ScaleWarnings(index Index) []ScaleWarning {
	type aggregate struct {
		kind     ScaleWarningKind
		advisory int
		affected int
		maximum  int
	}
	aggregates := []aggregate{
		{kind: ScaleWarningTargetSources, advisory: advisoryTargetSources},
		{kind: ScaleWarningTargetSeeds, advisory: advisoryTargetSeeds},
		{kind: ScaleWarningObjects, advisory: advisoryObjects},
		{kind: ScaleWarningRelations, advisory: advisoryRelations},
		{kind: ScaleWarningTotalWitnesses, advisory: advisoryTotalWitnesses},
		{kind: ScaleWarningTotalPatterns, advisory: advisoryTotalPatterns},
		{kind: ScaleWarningTotalArguments, advisory: advisoryTotalArguments},
		{kind: ScaleWarningRelationTargets, advisory: advisoryRelationTargets},
		{kind: ScaleWarningRelationWitnesses, advisory: advisoryRelationWitnesses},
		{kind: ScaleWarningRelationPatterns, advisory: advisoryRelationPatterns},
		{kind: ScaleWarningPatternArguments, advisory: advisoryPatternArguments},
		{kind: ScaleWarningTemplateParts, advisory: advisoryTemplateParts},
		{kind: ScaleWarningPatternObjectRefs, advisory: advisoryPatternObjectRefs},
		{kind: ScaleWarningSymbolLinks, advisory: advisorySymbolLinks},
		{kind: ScaleWarningSymbolLinkParts, advisory: advisorySymbolLinkParts},
		{kind: ScaleWarningSemanticText, advisory: advisorySemanticTextBytes},
		{kind: ScaleWarningObservedCount, advisory: advisoryObservedCount},
		{kind: ScaleWarningAggregateText, advisory: AdvisoryAggregateTextBytes},
		{kind: ScaleWarningArtifactBytes, advisory: AdvisoryIndexBytes},
	}
	positions := make(map[ScaleWarningKind]int, len(aggregates))
	for position := range aggregates {
		positions[aggregates[position].kind] = position
	}
	record := func(kind ScaleWarningKind, retained int) {
		position := positions[kind]
		if retained <= aggregates[position].advisory {
			return
		}
		aggregates[position].affected++
		if retained > aggregates[position].maximum {
			aggregates[position].maximum = retained
		}
	}
	aggregateTextBytes := 0
	maxInt := int(^uint(0) >> 1)
	recordText := func(values ...string) {
		for _, value := range values {
			record(ScaleWarningSemanticText, len(value))
			if aggregateTextBytes > maxInt-len(value) {
				aggregateTextBytes = maxInt
			} else {
				aggregateTextBytes += len(value)
			}
		}
	}
	record(ScaleWarningTargetSources, len(index.Target.Sources))
	record(ScaleWarningTargetSeeds, len(index.Target.Seeds))
	record(ScaleWarningObjects, len(index.Objects))
	record(ScaleWarningRelations, len(index.Relations))
	for _, count := range []int{
		index.Coverage.ObjectsObserved, index.Coverage.RelationsObserved,
		index.Coverage.TargetsObserved, index.Coverage.WitnessesObserved,
		index.Coverage.PatternsObserved, index.Coverage.ArgumentsObserved,
		index.Coverage.ReceiverOriginsObserved, index.Coverage.ArgumentObjectsObserved,
	} {
		record(ScaleWarningObservedCount, count)
	}

	recordText(index.ScenarioSHA256, index.SourceSHA256, index.Target.ID,
		index.Target.Language, index.Target.Kind, index.Target.Name,
		index.Target.Selector, index.Target.AnchorFileRef)
	for _, source := range index.Target.Sources {
		recordText(source.FileRef, source.Path)
	}
	for _, seed := range index.Target.Seeds {
		recordText(seed.ObjectID, string(seed.Kind))
		if seed.Location != nil {
			recordText(seed.Location.Path)
		}
	}
	for _, object := range index.Objects {
		recordText(object.ID, object.SourceRef, string(object.Kind), object.Name,
			string(object.Visibility), object.Signature, object.OwnerID, object.ContainerID)
		if object.External != nil {
			recordText(string(object.External.AuthorityKind), object.External.PackagePath,
				object.External.Receiver, object.External.Name)
		}
		record(ScaleWarningSymbolLinks, len(object.SymbolLinkIdentities))
		for _, identity := range object.SymbolLinkIdentities {
			recordText(identity.Domain, identity.Key, identity.Display)
			record(ScaleWarningSymbolLinkParts, identity.PartCount)
		}
		if object.Location != nil {
			recordText(object.Location.Path)
		}
	}

	totalWitnesses := 0
	totalPatterns := 0
	totalArguments := 0
	for _, relation := range index.Relations {
		for _, count := range []int{
			relation.TargetsObserved, relation.WitnessesObserved, relation.PatternsObserved,
		} {
			record(ScaleWarningObservedCount, count)
		}
		recordText(relation.ID, relation.SourceRef, string(relation.Kind), relation.FromID,
			string(relation.Resolution), relation.Invocation, relation.SourceArgumentID)
		recordText(relation.ToIDs...)
		if relation.Location != nil {
			recordText(relation.Location.Path)
		}
		record(ScaleWarningRelationTargets, len(relation.ToIDs))
		record(ScaleWarningRelationWitnesses, len(relation.Witnesses))
		record(ScaleWarningRelationPatterns, len(relation.Patterns))
		totalWitnesses += len(relation.Witnesses)
		totalPatterns += len(relation.Patterns)
		for _, witness := range relation.Witnesses {
			recordText(witness.Kind, witness.Detail, witness.SourceExpression)
			if witness.Location != nil {
				recordText(witness.Location.Path)
			}
		}
		for _, pattern := range relation.Patterns {
			record(ScaleWarningObservedCount, pattern.ReceiverOriginsObserved)
			record(ScaleWarningObservedCount, pattern.ArgumentsObserved)
			recordText(pattern.ID, pattern.SourceRef, string(pattern.Form), pattern.Selector,
				pattern.ResultID, pattern.ReceiverID, string(pattern.ReceiverOriginResolution))
			recordText(pattern.ReceiverOriginIDs...)
			if pattern.Location != nil {
				recordText(pattern.Location.Path)
			}
			record(ScaleWarningPatternArguments, len(pattern.Arguments))
			totalArguments += len(pattern.Arguments)
			record(ScaleWarningPatternObjectRefs, len(pattern.ReceiverOriginIDs))
			for _, argument := range pattern.Arguments {
				record(ScaleWarningObservedCount, argument.ObjectsObserved)
				record(ScaleWarningObservedCount, argument.ValueCandidatesObserved)
				recordText(argument.ID, argument.Keyword, string(argument.Kind), argument.Value,
					string(argument.Resolution))
				recordText(argument.ObjectIDs...)
				record(ScaleWarningTemplateParts, len(argument.Parts))
				record(ScaleWarningPatternObjectRefs, len(argument.ObjectIDs))
				for _, part := range argument.Parts {
					recordText(string(part.Kind), part.Text)
				}
				for _, candidate := range argument.ValueCandidates {
					record(ScaleWarningObservedCount, candidate.SourceObjectsObserved)
					record(ScaleWarningObservedCount, candidate.SourceArgumentsObserved)
					recordText(candidate.ID, string(candidate.Kind), candidate.Value,
						string(candidate.Resolution), string(candidate.SourceKind))
					recordText(candidate.SourceObjectIDs...)
					recordText(candidate.SourceArgumentIDs...)
					record(ScaleWarningTemplateParts, len(candidate.Parts))
					record(ScaleWarningPatternObjectRefs, len(candidate.SourceObjectIDs))
					for _, part := range candidate.Parts {
						recordText(string(part.Kind), part.Text)
					}
				}
			}
		}
	}
	record(ScaleWarningTotalWitnesses, totalWitnesses)
	record(ScaleWarningTotalPatterns, totalPatterns)
	record(ScaleWarningTotalArguments, totalArguments)
	record(ScaleWarningAggregateText, aggregateTextBytes)
	if encoded, err := json.Marshal(index); err == nil {
		record(ScaleWarningArtifactBytes, len(encoded))
	}

	warnings := make([]ScaleWarning, 0, len(aggregates))
	for _, aggregate := range aggregates {
		if aggregate.affected == 0 {
			continue
		}
		warnings = append(warnings, ScaleWarning{
			Kind:                aggregate.kind,
			AdvisorySize:        aggregate.advisory,
			AffectedCollections: aggregate.affected,
			MaximumRetained:     aggregate.maximum,
		})
	}
	return warnings
}
