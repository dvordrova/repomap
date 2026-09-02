package programindex

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Category is one model-selected semantic role for an exact ProgramIndex
// subject. Categories are a cover rather than a partition: one subject may
// legitimately have several categories.
type Category string

const (
	CategoryInbound            Category = "inbound"
	CategoryBackgroundActivity Category = "background_activity"
	CategoryDependency         Category = "dependency"
	CategoryCore               Category = "core"
)

// Valid reports whether category belongs to the closed semantic vocabulary.
func (category Category) Valid() bool {
	switch category {
	case CategoryInbound, CategoryBackgroundActivity, CategoryDependency, CategoryCore:
		return true
	default:
		return false
	}
}

// CategoryAssignment binds semantic categories directly to an existing
// Object.ID or RelationPattern.ID. No second public subject identity is
// introduced. Callers must restore request-local model refs before Enrich.
type CategoryAssignment struct {
	SubjectID  string     `json:"subject_id"`
	Categories []Category `json:"categories"`
}

// Categorization is the optional sealed semantic section of an Index.
// Its two digests bind the assignments to both exact inputs that affected
// them: the un-enriched Index and the reduced repository documentation.
type Categorization struct {
	BaseIndexSHA256            string               `json:"base_index_sha256"`
	ReducedDocumentationSHA256 string               `json:"reduced_documentation_sha256"`
	Assignments                []CategoryAssignment `json:"assignments"`
}

// Enrich attaches accepted semantic assignments to one already sealed base
// Index, binds the exact reduced-documentation input, and reseals the same
// Index type. It never changes target, object, relation, or coverage authority.
// Unknown request-local refs are not accepted here: the owning semantic cube
// must filter and restore them first.
func Enrich(base Index, reducedDocumentationSHA256 string, accepted []CategoryAssignment) (Index, error) {
	if err := base.Validate(); err != nil {
		return Index{}, fmt.Errorf("program index: enrich base: %w", err)
	}
	if base.Categorization != nil {
		return Index{}, fmt.Errorf("program index: already enriched")
	}
	if !validSHA256(reducedDocumentationSHA256) {
		return Index{}, fmt.Errorf("program index: invalid reduced documentation sha256")
	}

	assignments, err := canonicalizeCategoryAssignments(base, accepted)
	if err != nil {
		return Index{}, err
	}
	result := base.Snapshot()
	result.Categorization = &Categorization{
		BaseIndexSHA256:            base.SHA256,
		ReducedDocumentationSHA256: reducedDocumentationSHA256,
		Assignments:                assignments,
	}
	result.SHA256 = ""
	digest, err := indexDigest(result)
	if err != nil {
		return Index{}, err
	}
	result.SHA256 = digest
	if err := result.Validate(); err != nil {
		return Index{}, err
	}
	return result, nil
}

// Base restores the exact deterministic adapter projection underlying index.
// An un-enriched index is returned as an owned snapshot. For an enriched index,
// Categorization is removed and its sealed BaseIndexSHA256 is restored. This is
// the only supported way for an adapter to validate that enrichment preserved
// its structural projection without rejecting the enriched ProgramIndex itself.
func Base(index Index) (Index, error) {
	if err := index.Validate(); err != nil {
		return Index{}, fmt.Errorf("program index: restore base: %w", err)
	}
	base := index.Snapshot()
	if base.Categorization == nil {
		return base, nil
	}
	baseSHA256 := base.Categorization.BaseIndexSHA256
	base.Categorization = nil
	base.SHA256 = baseSHA256
	if err := base.Validate(); err != nil {
		return Index{}, fmt.Errorf("program index: restore sealed base: %w", err)
	}
	return base, nil
}

func canonicalizeCategoryAssignments(index Index, values []CategoryAssignment) ([]CategoryAssignment, error) {
	result := make([]CategoryAssignment, len(values))
	for position, value := range values {
		if !validText(value.SubjectID) || !hasCategorizationSubject(index, value.SubjectID) {
			return nil, fmt.Errorf("program index: categorization has unknown subject %q", value.SubjectID)
		}
		if len(value.Categories) == 0 {
			return nil, fmt.Errorf("program index: categorization subject %q has no categories", value.SubjectID)
		}
		categories := append([]Category(nil), value.Categories...)
		for _, category := range categories {
			if !category.Valid() {
				return nil, fmt.Errorf("program index: categorization subject %q has invalid category %q", value.SubjectID, category)
			}
			if !CategorySupported(index, value.SubjectID, category) {
				return nil, fmt.Errorf("program index: categorization subject %q has unsupported category %q", value.SubjectID, category)
			}
		}
		sort.Slice(categories, func(i, j int) bool { return categories[i] < categories[j] })
		for categoryPosition := 1; categoryPosition < len(categories); categoryPosition++ {
			if categories[categoryPosition-1] == categories[categoryPosition] {
				return nil, fmt.Errorf("program index: categorization subject %q repeats category %q", value.SubjectID, categories[categoryPosition])
			}
		}
		result[position] = CategoryAssignment{SubjectID: value.SubjectID, Categories: categories}
	}

	sort.Slice(result, func(i, j int) bool { return result[i].SubjectID < result[j].SubjectID })
	for position := 1; position < len(result); position++ {
		if result[position-1].SubjectID != result[position].SubjectID {
			continue
		}
		if reflect.DeepEqual(result[position-1].Categories, result[position].Categories) {
			return nil, fmt.Errorf("program index: duplicate categorization assignment for %q", result[position].SubjectID)
		}
		return nil, fmt.Errorf("program index: conflicting categorization assignments for %q", result[position].SubjectID)
	}
	if result == nil {
		result = []CategoryAssignment{}
	}
	return result, nil
}

func validateCategorization(index Index) error {
	if index.Categorization == nil {
		return nil
	}
	if !validSHA256(index.Categorization.BaseIndexSHA256) ||
		!validSHA256(index.Categorization.ReducedDocumentationSHA256) ||
		index.Categorization.Assignments == nil {
		return fmt.Errorf("program index: invalid categorization")
	}
	for position, assignment := range index.Categorization.Assignments {
		if !validText(assignment.SubjectID) || !hasCategorizationSubject(index, assignment.SubjectID) || len(assignment.Categories) == 0 {
			return fmt.Errorf("program index: invalid categorization assignment")
		}
		if position > 0 && index.Categorization.Assignments[position-1].SubjectID >= assignment.SubjectID {
			return fmt.Errorf("program index: categorization assignments are not canonical")
		}
		for categoryPosition, category := range assignment.Categories {
			if !category.Valid() || categoryPosition > 0 && assignment.Categories[categoryPosition-1] >= category {
				return fmt.Errorf("program index: categorization categories are not canonical")
			}
			if !CategorySupported(index, assignment.SubjectID, category) {
				return fmt.Errorf("program index: categorization has unsupported category")
			}
		}
	}

	base := index.Snapshot()
	base.Categorization = nil
	base.SHA256 = ""
	wantBaseSHA256, err := indexDigest(base)
	if err != nil {
		return err
	}
	if index.Categorization.BaseIndexSHA256 != wantBaseSHA256 {
		return fmt.Errorf("program index: categorization base sha256 mismatch")
	}
	return nil
}

func hasCategorizationSubject(index Index, subjectID string) bool {
	_, _, known := categorizationSubject(index, subjectID)
	return known
}

// CategorySupported reports whether one closed category is permitted for an
// exact categorization subject. It applies only deterministic product
// exclusions; all other semantic choices remain model-owned.
//
// Explicit platform declarations are standard-runtime authority rather than
// repository dependencies. The same exclusion applies to an exact relation
// pattern when its complete target set contains only platform authorities.
func CategorySupported(index Index, subjectID string, category Category) bool {
	object, relation, known := categorizationSubject(index, subjectID)
	if !known || !category.Valid() {
		return false
	}
	if category != CategoryDependency {
		return true
	}
	if object != nil && object.Kind == ObjectExternalSymbol {
		return IsExternalPackageAuthority(object.External)
	}
	if relation == nil || relation.Kind != RelationInvokesExternal ||
		relation.Resolution != ResolutionExact || len(relation.ToIDs) == 0 {
		return true
	}

	sawPlatformTarget := false
	for _, targetID := range relation.ToIDs {
		position := sort.Search(len(index.Objects), func(position int) bool {
			return strings.Compare(index.Objects[position].ID, targetID) >= 0
		})
		if position >= len(index.Objects) || index.Objects[position].ID != targetID {
			// Index validation normally makes this impossible. Do not derive a
			// semantic exclusion from incomplete local authority.
			return true
		}
		target := index.Objects[position]
		if target.Kind != ObjectExternalSymbol || !IsExternalPlatformAuthority(target.External) {
			return true
		}
		sawPlatformTarget = true
	}
	return !sawPlatformTarget
}

func categorizationSubject(index Index, subjectID string) (*Object, *Relation, bool) {
	objectPosition := sort.Search(len(index.Objects), func(position int) bool {
		return strings.Compare(index.Objects[position].ID, subjectID) >= 0
	})
	if objectPosition < len(index.Objects) && index.Objects[objectPosition].ID == subjectID {
		return &index.Objects[objectPosition], nil, true
	}
	for relationPosition := range index.Relations {
		relation := &index.Relations[relationPosition]
		patternPosition := sort.Search(len(relation.Patterns), func(position int) bool {
			return strings.Compare(relation.Patterns[position].ID, subjectID) >= 0
		})
		if patternPosition < len(relation.Patterns) && relation.Patterns[patternPosition].ID == subjectID {
			return nil, relation, true
		}
	}
	return nil, nil, false
}

func cloneCategorization(value *Categorization) *Categorization {
	if value == nil {
		return nil
	}
	result := &Categorization{
		BaseIndexSHA256:            value.BaseIndexSHA256,
		ReducedDocumentationSHA256: value.ReducedDocumentationSHA256,
		Assignments:                make([]CategoryAssignment, len(value.Assignments)),
	}
	for position, assignment := range value.Assignments {
		result.Assignments[position] = CategoryAssignment{
			SubjectID:  assignment.SubjectID,
			Categories: append([]Category(nil), assignment.Categories...),
		}
	}
	return result
}
