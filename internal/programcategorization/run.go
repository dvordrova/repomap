package programcategorization

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

//go:embed prompt.md
var promptText string

type normalizedAssignment struct {
	ref        string
	categories []Category
}

type normalizedResponse struct {
	assignments []normalizedAssignment
	diagnostics map[DiagnosticKind]int
	samples     map[DiagnosticKind][]string
	outOfBatch  int
}

// maxDiagnosticSamples bounds how many discarded rows one diagnostic names.
// It is a logging bound: the count stays exact.
const maxDiagnosticSamples = 5

// Run executes the first semantic ProgramIndex enrichment. Every accepted row
// is restored locally; an empty sparse response is a legitimate empty result.
func Run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	index programindex.Index,
	documentation documentationreduce.Result,
) (Result, error) {
	compilation, err := Compile(index, documentation)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		ProgramTargetID:            compilation.index.Target.ID,
		BaseProgramIndexSHA256:     compilation.index.SHA256,
		ReducedDocumentationSHA256: compilation.documentation.ReductionSHA256,
		Assignments:                []Assignment{},
		Diagnostics:                []Diagnostic{},
	}
	if len(compilation.subjects) == 0 {
		if err := result.Validate(compilation.index, compilation.documentation); err != nil {
			return Result{}, err
		}
		return result, nil
	}

	plan, err := compilation.batchesForProvider(provider)
	if err != nil {
		return Result{}, err
	}
	finalPlan, outcomes, err := llm.ExecuteAdaptiveJSONBatch(
		ctx, executor, provider, plan,
		func(items []batch) ([]llm.Call[normalizedResponse], error) {
			calls := make([]llm.Call[normalizedResponse], len(items))
			for position := range items {
				item := items[position]
				request, requestErr := compilation.request(item.subjectRefs, item.documentationRefs)
				if requestErr != nil {
					return nil, requestErr
				}
				wire, encodeErr := json.Marshal(request)
				if encodeErr != nil {
					return nil, fmt.Errorf("program categorization: encode request: %w", encodeErr)
				}
				owned := make(map[string]subjectAuthority, len(item.subjectRefs))
				for _, ref := range item.subjectRefs {
					owned[ref] = compilation.subjectByRef[ref]
				}
				state, stateErr := cubeState(compilation, wire)
				if stateErr != nil {
					return nil, stateErr
				}
				calls[position] = llm.Call[normalizedResponse]{
					State: state,
					Prompt: llm.Prompt{
						System: strings.TrimSpace(promptText), User: string(wire), ResponseFormatJSON: true,
					},
					Limits: limits(),
					DecodeValidate: func(raw []byte) (normalizedResponse, error) {
						return normalizeResponse(raw, owned, compilation.subjectByRef, compilation.index)
					},
				}
			}
			return calls, nil
		},
		splitBatch,
	)
	if err != nil {
		return Result{}, fmt.Errorf("program categorization: model batch: %w", err)
	}
	if err := compilation.validatePlan(finalPlan); err != nil {
		return Result{}, err
	}

	categoriesBySubject := make(map[string]map[Category]struct{})
	diagnostics := make(map[DiagnosticKind]int)
	samples := make(map[DiagnosticKind][]string)
	for _, outcome := range outcomes {
		result.OutOfBatchAssignments += outcome.Value.outOfBatch
		for kind, count := range outcome.Value.diagnostics {
			diagnostics[kind] += count
		}
		for kind, rows := range outcome.Value.samples {
			for _, sample := range rows {
				if len(samples[kind]) < maxDiagnosticSamples {
					samples[kind] = append(samples[kind], sample)
				}
			}
		}
		for _, assignment := range outcome.Value.assignments {
			subject, known := compilation.subjectByRef[assignment.ref]
			if !known {
				return Result{}, fmt.Errorf("program categorization: normalized response retained unknown ref")
			}
			if categoriesBySubject[subject.id] == nil {
				categoriesBySubject[subject.id] = make(map[Category]struct{})
			}
			for _, category := range assignment.categories {
				categoriesBySubject[subject.id][category] = struct{}{}
			}
		}
	}
	for subjectID, categories := range categoriesBySubject {
		row := Assignment{SubjectID: subjectID, Categories: make([]Category, 0, len(categories))}
		for category := range categories {
			row.Categories = append(row.Categories, category)
		}
		sort.Slice(row.Categories, func(i, j int) bool { return row.Categories[i] < row.Categories[j] })
		result.Assignments = append(result.Assignments, row)
	}
	sort.Slice(result.Assignments, func(i, j int) bool {
		return result.Assignments[i].SubjectID < result.Assignments[j].SubjectID
	})
	for _, kind := range []DiagnosticKind{
		DiagnosticEmptyCategories, DiagnosticInvalidCategory,
		DiagnosticMalformedRow, DiagnosticUnknownRef,
		DiagnosticUnsupportedCategory,
	} {
		if diagnostics[kind] > 0 {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Kind: kind, Count: diagnostics[kind], Samples: samples[kind],
			})
		}
	}
	if err := result.Validate(compilation.index, compilation.documentation); err != nil {
		return Result{}, err
	}
	return result, nil
}

func normalizeResponse(
	raw []byte,
	owned map[string]subjectAuthority,
	all map[string]subjectAuthority,
	index programindex.Index,
) (normalizedResponse, error) {
	normalized, err := llm.NormalizeJSON(raw)
	if err != nil {
		return normalizedResponse{}, err
	}
	var envelope struct {
		Assignments []json.RawMessage `json:"assignments"`
	}
	decoder := json.NewDecoder(bytes.NewReader(normalized))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return normalizedResponse{}, fmt.Errorf("program categorization: decode response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return normalizedResponse{}, fmt.Errorf("program categorization: response has trailing data")
	}
	if envelope.Assignments == nil {
		return normalizedResponse{}, fmt.Errorf("program categorization: response assignments must be an array")
	}

	result := normalizedResponse{
		diagnostics: make(map[DiagnosticKind]int),
		samples:     make(map[DiagnosticKind][]string),
	}
	note := func(kind DiagnosticKind, sample string) {
		result.diagnostics[kind]++
		if sample != "" && len(result.samples[kind]) < maxDiagnosticSamples {
			result.samples[kind] = append(result.samples[kind], sample)
		}
	}
	byRef := make(map[string]map[Category]struct{})
	for _, rawRow := range envelope.Assignments {
		var row responseAssignment
		rowDecoder := json.NewDecoder(bytes.NewReader(rawRow))
		rowDecoder.DisallowUnknownFields()
		if err := rowDecoder.Decode(&row); err != nil {
			note(DiagnosticMalformedRow, "")
			continue
		}
		if err := rowDecoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			note(DiagnosticMalformedRow, "")
			continue
		}
		// A model shown context subjects often categorizes them too. Those rows
		// name real subjects of this same target and the exhaustive cover would
		// ask about them in a later request anyway, so they are accepted here
		// rather than paid for twice. Every other check still applies.
		subject, known := owned[row.Ref]
		if !known {
			subject, known = all[row.Ref]
			if !known {
				note(DiagnosticUnknownRef, row.Ref)
				continue
			}
			result.outOfBatch++
		}
		if len(row.Categories) == 0 {
			note(DiagnosticEmptyCategories, row.Ref)
			continue
		}
		categories := make(map[Category]struct{}, len(row.Categories))
		valid := true
		for _, rawCategory := range row.Categories {
			category := programindex.Category(rawCategory)
			if !category.Valid() {
				valid = false
				break
			}
			categories[category] = struct{}{}
		}
		if !valid {
			note(DiagnosticInvalidCategory, row.Ref)
			continue
		}
		for category := range categories {
			if !categorySupported(index, subject, category) {
				note(DiagnosticUnsupportedCategory, string(category)+" on "+row.Ref)
				continue
			}
			if byRef[row.Ref] == nil {
				byRef[row.Ref] = make(map[Category]struct{})
			}
			byRef[row.Ref][category] = struct{}{}
		}
	}
	refs := make([]string, 0, len(byRef))
	for ref := range byRef {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		assignment := normalizedAssignment{ref: ref}
		for category := range byRef[ref] {
			assignment.categories = append(assignment.categories, category)
		}
		sort.Slice(assignment.categories, func(i, j int) bool {
			return assignment.categories[i] < assignment.categories[j]
		})
		result.assignments = append(result.assignments, assignment)
	}
	return result, nil
}

func cubeState(compilation Compilation, request []byte) ([]byte, error) {
	promptDigest := sha256.Sum256([]byte(strings.TrimSpace(promptText)))
	requestDigest := sha256.Sum256(request)
	state := struct {
		Contract                   string `json:"contract"`
		PreparationVersion         int    `json:"preparation_version"`
		ResponseSchemaVersion      int    `json:"response_schema_version"`
		PromptSHA256               string `json:"prompt_sha256"`
		ProgramIndexSHA256         string `json:"program_index_sha256"`
		ReducedDocumentationSHA256 string `json:"reduced_documentation_sha256,omitempty"`
		RequestSHA256              string `json:"request_sha256"`
	}{
		Contract: executionContract, PreparationVersion: preparationVersion,
		ResponseSchemaVersion:      responseSchemaVersion,
		PromptSHA256:               hex.EncodeToString(promptDigest[:]),
		ProgramIndexSHA256:         compilation.index.SHA256,
		ReducedDocumentationSHA256: compilation.documentation.ReductionSHA256,
		RequestSHA256:              hex.EncodeToString(requestDigest[:]),
	}
	wire, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("program categorization: encode cube state: %w", err)
	}
	return wire, nil
}
