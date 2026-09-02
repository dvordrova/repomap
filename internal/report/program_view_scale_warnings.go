package report

// ProgramViewScaleWarningKind identifies one former local presentation bound.
// These values are diagnostic only and never authorize dropping or rejecting
// a ProgramView row.
type ProgramViewScaleWarningKind string

const (
	ProgramViewScaleWarningSeeds     ProgramViewScaleWarningKind = "seeds"
	ProgramViewScaleWarningObjects   ProgramViewScaleWarningKind = "objects"
	ProgramViewScaleWarningRelations ProgramViewScaleWarningKind = "relations"
	ProgramViewScaleWarningWitnesses ProgramViewScaleWarningKind = "witnesses_per_relation"
	ProgramViewScaleWarningText      ProgramViewScaleWarningKind = "semantic_text_bytes"
)

// ProgramViewScaleWarning aggregates complete retained measurements above one
// former usual-size threshold.
type ProgramViewScaleWarning struct {
	Kind                ProgramViewScaleWarningKind
	AdvisorySize        int
	AffectedCollections int
	MaximumRetained     int
}

// ProgramViewScaleWarnings is a pure, best-effort diagnostic over the complete
// projection. It does not validate the view and cannot affect publication.
func ProgramViewScaleWarnings(view ProgramView) []ProgramViewScaleWarning {
	type aggregate struct {
		kind     ProgramViewScaleWarningKind
		advisory int
		affected int
		maximum  int
	}
	aggregates := []aggregate{
		{kind: ProgramViewScaleWarningSeeds, advisory: MaxProgramViewSeeds},
		{kind: ProgramViewScaleWarningObjects, advisory: MaxProgramViewObjects},
		{kind: ProgramViewScaleWarningRelations, advisory: MaxProgramViewRelations},
		{kind: ProgramViewScaleWarningWitnesses, advisory: MaxProgramViewWitnessesPerRelation},
		{kind: ProgramViewScaleWarningText, advisory: maxProgramViewTextBytes},
	}
	positions := make(map[ProgramViewScaleWarningKind]int, len(aggregates))
	for position := range aggregates {
		positions[aggregates[position].kind] = position
	}
	record := func(kind ProgramViewScaleWarningKind, retained int) {
		position := positions[kind]
		if retained <= aggregates[position].advisory {
			return
		}
		aggregates[position].affected++
		if retained > aggregates[position].maximum {
			aggregates[position].maximum = retained
		}
	}
	record(ProgramViewScaleWarningSeeds, len(view.Seeds))
	record(ProgramViewScaleWarningObjects, len(view.Objects))
	record(ProgramViewScaleWarningRelations, len(view.Relations))
	for _, relation := range view.Relations {
		record(ProgramViewScaleWarningWitnesses, len(relation.Witnesses))
	}
	record(ProgramViewScaleWarningText, programViewTextBytes(view))

	warnings := make([]ProgramViewScaleWarning, 0, len(aggregates))
	for _, aggregate := range aggregates {
		if aggregate.affected == 0 {
			continue
		}
		warnings = append(warnings, ProgramViewScaleWarning{
			Kind:                aggregate.kind,
			AdvisorySize:        aggregate.advisory,
			AffectedCollections: aggregate.affected,
			MaximumRetained:     aggregate.maximum,
		})
	}
	return warnings
}
