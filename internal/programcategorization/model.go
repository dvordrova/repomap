// Package programcategorization owns the first semantic enrichment of one
// sealed ProgramIndex. It classifies exact program objects and relation
// patterns into a small overlapping category cover, then restores the model's
// request-local refs to the original ProgramIndex identities.
package programcategorization

import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	requestVersion          = 2
	executionContract       = "repomap.program-categorization.v4"
	preparationVersion      = 4
	responseSchemaVersion   = 1
	maxOutputTokens         = 128_000
	ownedSubjectsPerRequest = 32
)

// Category is the same closed vocabulary accepted by ProgramIndex.Enrich.
// The alias avoids a second vocabulary while Result keeps its own restored
// assignment shape.
type Category = programindex.Category

const (
	CategoryInbound            = programindex.CategoryInbound
	CategoryBackgroundActivity = programindex.CategoryBackgroundActivity
	CategoryDependency         = programindex.CategoryDependency
	CategoryCore               = programindex.CategoryCore
)

// Assignment binds accepted categories directly to one canonical Object.ID or
// RelationPattern.ID. No request-local ref survives this boundary.
type Assignment struct {
	SubjectID  string     `json:"subject_id"`
	Categories []Category `json:"categories"`
}

// DiagnosticKind describes only locally discarded response rows. Diagnostics
// never promote, repair, or invent a semantic assignment.
type DiagnosticKind string

const (
	DiagnosticUnknownRef          DiagnosticKind = "unknown_ref"
	DiagnosticMalformedRow        DiagnosticKind = "malformed_row"
	DiagnosticInvalidCategory     DiagnosticKind = "invalid_category"
	DiagnosticEmptyCategories     DiagnosticKind = "empty_categories"
	DiagnosticUnsupportedCategory DiagnosticKind = "unsupported_category"
)

type Diagnostic struct {
	Kind  DiagnosticKind `json:"kind"`
	Count int            `json:"count"`
	// Samples names a few of the discarded rows so a reader can tell a weak
	// prompt from a badly shaped request without rerunning the stage.
	Samples []string `json:"samples,omitempty"`
}

// Result is the private restored handoff to ProgramIndex.Enrich.
type Result struct {
	ProgramTargetID            string       `json:"program_target_id"`
	BaseProgramIndexSHA256     string       `json:"base_program_index_sha256"`
	ReducedDocumentationSHA256 string       `json:"reduced_documentation_sha256,omitempty"`
	Assignments                []Assignment `json:"assignments"`
	Diagnostics                []Diagnostic `json:"diagnostics"`
	// OutOfBatchAssignments counts accepted rows the model volunteered for
	// subjects of this target that its request did not ask about. They are
	// real answers, not hallucinations, so they are kept rather than discarded.
	OutOfBatchAssignments int `json:"out_of_batch_assignments,omitempty"`
}

// EnrichmentAssignments returns an independently owned ProgramIndex handoff.
func (result Result) EnrichmentAssignments() []programindex.CategoryAssignment {
	assignments := make([]programindex.CategoryAssignment, len(result.Assignments))
	for position, assignment := range result.Assignments {
		assignments[position] = programindex.CategoryAssignment{
			SubjectID:  assignment.SubjectID,
			Categories: append([]programindex.Category(nil), assignment.Categories...),
		}
	}
	return assignments
}

// Enrich validates the complete restored result against both exact inputs and
// applies it to the same ProgramIndex type. Runtime callers use this boundary
// so they cannot accidentally persist assignments without their reduced-
// documentation binding.
func (result Result) Enrich(
	base programindex.Index,
	documentation documentationreduce.Result,
) (programindex.Index, error) {
	if err := result.Validate(base, documentation); err != nil {
		return programindex.Index{}, err
	}
	return programindex.Enrich(
		base,
		result.ReducedDocumentationSHA256,
		result.EnrichmentAssignments(),
	)
}

// Validate proves that Result is a canonical sparse enrichment restored only
// to subjects from the exact un-enriched ProgramIndex and bound to the exact
// reduced-documentation handoff.
func (result Result) Validate(base programindex.Index, documentation documentationreduce.Result) error {
	if err := base.Validate(); err != nil {
		return fmt.Errorf("program categorization: validate base ProgramIndex: %w", err)
	}
	if base.Categorization != nil {
		return fmt.Errorf("program categorization: validate base ProgramIndex is already enriched")
	}
	if err := documentation.Validate(); err != nil {
		return fmt.Errorf("program categorization: validate reduced documentation: %w", err)
	}
	if result.ProgramTargetID != base.Target.ID ||
		result.BaseProgramIndexSHA256 != base.SHA256 ||
		result.ReducedDocumentationSHA256 != documentation.ReductionSHA256 ||
		result.Assignments == nil || result.Diagnostics == nil {
		return fmt.Errorf("program categorization: result authority is invalid")
	}
	for position, assignment := range result.Assignments {
		subject, known := subjectByID(base, assignment.SubjectID)
		if !known || len(assignment.Categories) == 0 {
			return fmt.Errorf("program categorization: assignment %d is invalid", position)
		}
		if position > 0 && result.Assignments[position-1].SubjectID >= assignment.SubjectID {
			return fmt.Errorf("program categorization: assignments are not canonical")
		}
		for categoryPosition, category := range assignment.Categories {
			if !category.Valid() ||
				categoryPosition > 0 && assignment.Categories[categoryPosition-1] >= category {
				return fmt.Errorf("program categorization: assignment %d categories are not canonical", position)
			}
			if !categorySupported(base, subject, category) {
				return fmt.Errorf(
					"program categorization: assignment %d category %q is unsupported for subject",
					position, category,
				)
			}
		}
	}
	for position, diagnostic := range result.Diagnostics {
		if !diagnostic.Kind.valid() || diagnostic.Count <= 0 {
			return fmt.Errorf("program categorization: diagnostic %d is invalid", position)
		}
		if position > 0 && result.Diagnostics[position-1].Kind >= diagnostic.Kind {
			return fmt.Errorf("program categorization: diagnostics are not canonical")
		}
	}
	if _, err := programindex.Enrich(base, result.ReducedDocumentationSHA256, result.EnrichmentAssignments()); err != nil {
		return fmt.Errorf("program categorization: result cannot enrich ProgramIndex: %w", err)
	}
	return nil
}

func (kind DiagnosticKind) valid() bool {
	switch kind {
	case DiagnosticUnknownRef, DiagnosticMalformedRow,
		DiagnosticInvalidCategory, DiagnosticEmptyCategories,
		DiagnosticUnsupportedCategory:
		return true
	default:
		return false
	}
}

func subjectByID(index programindex.Index, subjectID string) (subjectAuthority, bool) {
	objectPosition := sort.Search(len(index.Objects), func(position int) bool {
		return index.Objects[position].ID >= subjectID
	})
	if objectPosition < len(index.Objects) && index.Objects[objectPosition].ID == subjectID {
		object := index.Objects[objectPosition]
		return subjectAuthority{id: object.ID, kind: subjectObject, object: &object}, true
	}
	for _, relation := range index.Relations {
		patternPosition := sort.Search(len(relation.Patterns), func(position int) bool {
			return relation.Patterns[position].ID >= subjectID
		})
		if patternPosition < len(relation.Patterns) && relation.Patterns[patternPosition].ID == subjectID {
			relationCopy := relation
			pattern := relation.Patterns[patternPosition]
			return subjectAuthority{
				id: pattern.ID, kind: subjectPattern,
				relation: &relationCopy, pattern: &pattern,
			}, true
		}
	}
	return subjectAuthority{}, false
}

// categorySupported delegates the deterministic product exclusions to the
// ProgramIndex boundary that ultimately seals the same assignment.
func categorySupported(index programindex.Index, subject subjectAuthority, category Category) bool {
	return programindex.CategorySupported(index, subject.id, category)
}
