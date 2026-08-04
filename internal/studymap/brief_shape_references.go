package studymap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	BriefShapeReferenceCatalogVersion   = "study-brief-shape-reference-catalog-v1"
	BriefShapeReferenceResponseVersion  = "study-brief-shape-reference-response-v1"
	BriefShapeReferenceValidatorVersion = "study-brief-shape-reference-validator-v1"
)

// BriefShapeReferenceCatalog binds one Brief/Shape request to the same short,
// typed repository-object namespace used by the direction editor. Its catalog
// token is stage-specific, so a response from another request cannot be
// replayed against this producer even when the underlying bundle is equal.
type BriefShapeReferenceCatalog struct {
	digest        string
	catalogRef    string
	canonicalJSON []byte
	wireJSON      []byte
	objects       DirectionReferenceCatalog
}

// BriefShapeReferenceDiagnostics records only closed item-level Shape losses.
// Raw provider refs and prose never enter the diagnostic.
type BriefShapeReferenceDiagnostics struct {
	ShapeReceived int                        `json:"shape_received"`
	ShapeAccepted int                        `json:"shape_accepted"`
	ShapeRejected int                        `json:"shape_rejected"`
	Issues        []BriefShapeReferenceIssue `json:"issues,omitempty"`
}

type BriefShapeReferenceIssue struct {
	Field    string `json:"field"`
	Position int    `json:"position"`
	Code     string `json:"code"`
}

type briefShapeReferenceProviderResponse struct {
	Version        int                              `json:"version"`
	CatalogRef     string                           `json:"catalog_ref"`
	RepositoryType RepositoryType                   `json:"repository_type"`
	Brief          briefShapeReferenceProviderBrief `json:"brief"`
	ShapeAreaRefs  []string                         `json:"shape_area_refs"`
}

type briefShapeReferenceProviderBrief struct {
	WhatItIs              briefShapeReferenceStatement    `json:"what_it_is"`
	Problem               briefShapeReferenceStatement    `json:"problem"`
	MainInput             briefShapeReferenceStatement    `json:"main_input"`
	CentralResponsibility briefShapeReferenceStatement    `json:"central_responsibility"`
	ObservableResult      briefShapeReferenceStatement    `json:"observable_result"`
	DomainTerms           []briefShapeReferenceDomainTerm `json:"domain_terms,omitempty"`
}

type briefShapeReferenceStatement struct {
	Text        string   `json:"text"`
	SupportRefs []string `json:"support_refs"`
}

type briefShapeReferenceDomainTerm struct {
	Term        string   `json:"term"`
	Meaning     string   `json:"meaning"`
	SupportRefs []string `json:"support_refs"`
}

// BuildBriefShapeReferenceCatalog builds one deterministic, request-local
// Brief/Shape catalog without creating a second object-mapping framework.
// The shared object catalog supplies refs and the wire projection; this stage
// owns only its exact contract identity and catalog token.
func BuildBriefShapeReferenceCatalog(bundle Bundle) (BriefShapeReferenceCatalog, error) {
	objects, err := BuildDirectionReferenceCatalog(bundle)
	if err != nil {
		return BriefShapeReferenceCatalog{}, err
	}
	bundleJSON, err := json.Marshal(bundle.PromptBundle())
	if err != nil {
		return BriefShapeReferenceCatalog{}, fmt.Errorf(
			"study brief refs: encode bundle identity: %w", err,
		)
	}
	identity := struct {
		CatalogContract   string          `json:"catalog_contract"`
		ResponseContract  string          `json:"response_contract"`
		ValidatorContract string          `json:"validator_contract"`
		PromptContract    string          `json:"prompt_contract"`
		Bundle            json.RawMessage `json:"bundle"`
	}{
		CatalogContract:   BriefShapeReferenceCatalogVersion,
		ResponseContract:  BriefShapeReferenceResponseVersion,
		ValidatorContract: BriefShapeReferenceValidatorVersion,
		PromptContract:    semanticdiscovery.StudyBriefPromptVersion,
		Bundle:            append(json.RawMessage(nil), bundleJSON...),
	}
	identityJSON, err := json.Marshal(identity)
	if err != nil {
		return BriefShapeReferenceCatalog{}, fmt.Errorf(
			"study brief refs: encode catalog identity: %w", err,
		)
	}
	digest := sha256.Sum256(identityJSON)
	catalogRef := "b_" + hex.EncodeToString(digest[:12])
	objects.catalogRef = catalogRef
	wireJSON, err := objects.buildWireBundle(bundle)
	if err != nil {
		return BriefShapeReferenceCatalog{}, err
	}
	return BriefShapeReferenceCatalog{
		digest:        hex.EncodeToString(digest[:]),
		catalogRef:    catalogRef,
		canonicalJSON: append([]byte(nil), identityJSON...),
		wireJSON:      wireJSON,
		objects:       objects,
	}, nil
}

func (catalog BriefShapeReferenceCatalog) Digest() string { return catalog.digest }

func (catalog BriefShapeReferenceCatalog) CatalogRef() string { return catalog.catalogRef }

func (catalog BriefShapeReferenceCatalog) IdentityJSON() []byte {
	return append([]byte(nil), catalog.canonicalJSON...)
}

func (catalog BriefShapeReferenceCatalog) PromptBundleJSON() []byte {
	return append([]byte(nil), catalog.wireJSON...)
}

// RecoverBriefShapeReferenceProviderJSON extracts only the typed Brief/Shape
// envelope. It never accepts the earlier canonical-ID provider shape.
func RecoverBriefShapeReferenceProviderJSON(raw []byte) ([]byte, error) {
	return recoverEditingProviderJSON(
		raw,
		"typed brief and shape proposal",
		func(candidate []byte) error {
			var response briefShapeReferenceProviderResponse
			return decodeEditingJSON(
				candidate,
				maxEditingArtifactBytes,
				"typed brief and shape proposal",
				&response,
			)
		},
	)
}

// DecodeAndResolveBriefShapeProposal resolves the exact typed provider DTO to
// canonical backend identities before the producer accepts the stage. Invalid
// Shape members are omitted item-locally; required Brief support remains
// fail-closed and an empty Shape is represented explicitly rather than filled.
func DecodeAndResolveBriefShapeProposal(
	raw []byte,
	catalog BriefShapeReferenceCatalog,
) (BriefShapeProposal, BriefShapeReferenceDiagnostics, error) {
	var response briefShapeReferenceProviderResponse
	if err := decodeEditingJSON(
		raw,
		maxEditingArtifactBytes,
		"typed brief and shape proposal",
		&response,
	); err != nil {
		return BriefShapeProposal{}, BriefShapeReferenceDiagnostics{}, err
	}
	if response.Version != BriefShapeProposalVersion {
		return BriefShapeProposal{}, BriefShapeReferenceDiagnostics{}, fmt.Errorf(
			"study map: unsupported brief and shape proposal version %d",
			response.Version,
		)
	}
	diagnostics := BriefShapeReferenceDiagnostics{}
	if response.CatalogRef == "" || response.CatalogRef != catalog.catalogRef {
		diagnostics.Issues = append(diagnostics.Issues, BriefShapeReferenceIssue{
			Field: "catalog_ref", Position: 0, Code: "catalog_ref_mismatch",
		})
		return BriefShapeProposal{}, diagnostics, fmt.Errorf(
			"study brief refs: response catalog does not match the exact request",
		)
	}

	resolveStatement := func(
		field string,
		statement briefShapeReferenceStatement,
	) (BriefStatement, error) {
		if !validText(statement.Text, 1024, false) {
			return BriefStatement{}, fmt.Errorf("study brief refs: invalid required statement")
		}
		support, issue := catalog.resolveSupportList(field, statement.SupportRefs)
		if issue != nil {
			diagnostics.Issues = append(diagnostics.Issues, *issue)
			return BriefStatement{}, fmt.Errorf("study brief refs: required support ref rejected")
		}
		if len(support) == 0 {
			diagnostics.Issues = append(diagnostics.Issues, BriefShapeReferenceIssue{
				Field: field, Position: 0, Code: "missing_support_ref",
			})
			return BriefStatement{}, fmt.Errorf("study brief refs: required statement has no support")
		}
		return BriefStatement{Text: strings.TrimSpace(statement.Text), SupportIDs: support}, nil
	}

	whatItIs, err := resolveStatement("brief.what_it_is.support_refs", response.Brief.WhatItIs)
	if err != nil {
		return BriefShapeProposal{}, diagnostics, err
	}
	problem, err := resolveStatement("brief.problem.support_refs", response.Brief.Problem)
	if err != nil {
		return BriefShapeProposal{}, diagnostics, err
	}
	mainInput, err := resolveStatement("brief.main_input.support_refs", response.Brief.MainInput)
	if err != nil {
		return BriefShapeProposal{}, diagnostics, err
	}
	central, err := resolveStatement(
		"brief.central_responsibility.support_refs",
		response.Brief.CentralResponsibility,
	)
	if err != nil {
		return BriefShapeProposal{}, diagnostics, err
	}
	observable, err := resolveStatement(
		"brief.observable_result.support_refs",
		response.Brief.ObservableResult,
	)
	if err != nil {
		return BriefShapeProposal{}, diagnostics, err
	}

	var terms []BriefDomainTerm
	for index, term := range response.Brief.DomainTerms {
		if len(terms) == 8 ||
			!validText(term.Term, 128, true) ||
			!validText(term.Meaning, 512, true) {
			continue
		}
		support, supportIssue := catalog.resolveSupportList(
			fmt.Sprintf("brief.domain_terms[%d].support_refs", index),
			term.SupportRefs,
		)
		if supportIssue != nil {
			diagnostics.Issues = append(diagnostics.Issues, *supportIssue)
			continue
		}
		if len(support) == 0 {
			continue
		}
		terms = append(terms, BriefDomainTerm{
			Term: strings.TrimSpace(term.Term), Meaning: strings.TrimSpace(term.Meaning),
			SupportIDs: support,
		})
	}

	diagnostics.ShapeReceived = len(response.ShapeAreaRefs)
	shapeIssueStart := len(diagnostics.Issues)
	var shape []string
	seenShape := make(map[string]struct{}, len(response.ShapeAreaRefs))
	for position, ref := range response.ShapeAreaRefs {
		if len(shape) == 7 {
			diagnostics.Issues = append(diagnostics.Issues, BriefShapeReferenceIssue{
				Field: "shape_area_refs", Position: position, Code: "shape_area_ref_limit",
			})
			continue
		}
		if _, duplicate := seenShape[ref]; duplicate {
			diagnostics.Issues = append(diagnostics.Issues, BriefShapeReferenceIssue{
				Field: "shape_area_refs", Position: position, Code: "duplicate_area_ref",
			})
			continue
		}
		seenShape[ref] = struct{}{}
		areaID, resolveErr := catalog.objects.resolve(
			fmt.Sprintf("shape_area_refs[%d]", position),
			directionReferenceArea,
			ref,
		)
		if resolveErr != nil {
			diagnostics.Issues = append(diagnostics.Issues, BriefShapeReferenceIssue{
				Field: "shape_area_refs", Position: position,
				Code: catalog.referenceIssueCode(
					ref,
					directionReferenceArea,
					directionReferenceResolutionCode(resolveErr),
				),
			})
			continue
		}
		shape = append(shape, areaID)
	}
	diagnostics.ShapeAccepted = len(shape)
	diagnostics.ShapeRejected = len(diagnostics.Issues) - shapeIssueStart

	proposal := BriefShapeProposal{
		Version: response.Version, RepositoryType: response.RepositoryType,
		Brief: Brief{
			WhatItIs: whatItIs, Problem: problem, MainInput: mainInput,
			CentralResponsibility: central, ObservableResult: observable,
			DomainTerms: terms,
		},
		ShapeAreaIDs: shape,
	}
	if err := validateBriefShapeStructure(proposal); err != nil {
		return BriefShapeProposal{}, diagnostics, err
	}
	return proposal, diagnostics, nil
}

func (catalog BriefShapeReferenceCatalog) resolveSupportList(
	field string,
	refs []string,
) ([]string, *BriefShapeReferenceIssue) {
	seen := make(map[string]struct{}, len(refs))
	result := make([]string, 0, len(refs))
	for position, ref := range refs {
		if _, duplicate := seen[ref]; duplicate {
			return nil, &BriefShapeReferenceIssue{
				Field: field, Position: position, Code: "duplicate_support_ref",
			}
		}
		seen[ref] = struct{}{}
		entry, ok := catalog.objects.byRef[ref]
		if !ok {
			return nil, &BriefShapeReferenceIssue{
				Field: field, Position: position,
				Code: catalog.referenceIssueCode(
					ref,
					"support",
					"unknown_support_ref",
				),
			}
		}
		switch entry.Kind {
		case directionReferenceAnchor, directionReferenceDocument, directionReferenceArea:
			result = append(result, entry.CanonicalID)
		default:
			return nil, &BriefShapeReferenceIssue{
				Field: field, Position: position, Code: "wrong_kind_support_ref",
			}
		}
	}
	return result, nil
}

func (catalog BriefShapeReferenceCatalog) referenceIssueCode(
	ref string,
	expected directionReferenceKind,
	fallback string,
) string {
	if !strings.HasPrefix(fallback, "unknown_") {
		return fallback
	}
	for knownRef, entry := range catalog.objects.byRef {
		if ref == entry.CanonicalID {
			return "raw_canonical_" + string(expected) + "_ref"
		}
		if ref != "" && (strings.HasPrefix(ref, knownRef) ||
			strings.HasPrefix(knownRef, ref) || strings.HasSuffix(ref, knownRef)) {
			return "non_exact_" + string(expected) + "_ref"
		}
	}
	return fallback
}
