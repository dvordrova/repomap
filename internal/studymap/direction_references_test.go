package studymap

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDirectionReferenceCatalogIsDeterministicAndRequestBound(t *testing.T) {
	t.Parallel()

	bundle, _ := directionReferenceFixture(t)
	first, err := BuildDirectionReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildDirectionReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest() == "" || first.Digest() != second.Digest() ||
		!bytes.Equal(first.IdentityJSON(), second.IdentityJSON()) ||
		!bytes.Equal(first.PromptBundleJSON(), second.PromptBundleJSON()) {
		t.Fatal("identical Study direction inputs produced different reference catalogs")
	}

	for _, id := range directionReferenceCanonicalIDs(bundle) {
		if bytes.Contains(first.PromptBundleJSON(), []byte(id)) {
			t.Fatalf("canonical id %q leaked into provider wire bundle", id)
		}
	}
	for _, marker := range []string{`"anchor_ref":`, `"document_ref":`, `"area_ref":`, `"mechanism_ref":`} {
		if !bytes.Contains(first.PromptBundleJSON(), []byte(marker)) {
			t.Fatalf("provider wire bundle omitted typed marker %s", marker)
		}
	}

	reordered := bundle
	reordered.Anchors = append([]Anchor(nil), bundle.Anchors...)
	reordered.Anchors[0], reordered.Anchors[1] = reordered.Anchors[1], reordered.Anchors[0]
	changed, err := BuildDirectionReferenceCatalog(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Digest() == first.Digest() ||
		bytes.Equal(changed.IdentityJSON(), first.IdentityJSON()) {
		t.Fatal("exact catalog order did not change the request-bound identity")
	}
	firstAnchor, err := first.ref(directionReferenceAnchor, bundle.Anchors[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	changedAnchor, err := changed.ref(directionReferenceAnchor, bundle.Anchors[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstAnchor == changedAnchor && first.CatalogRef() == changed.CatalogRef() {
		t.Fatal("catalog order changed neither the ordinal mapping nor request token")
	}
	if first.CatalogRef() == changed.CatalogRef() {
		t.Fatal("exact catalog order did not change the request token")
	}
}

func TestDirectionReferenceRoundTripPreservesCanonicalStudyBytes(t *testing.T) {
	t.Parallel()

	bundle, canonical := directionReferenceFixture(t)
	catalog, err := BuildDirectionReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	provider := directionReferenceProviderFixture(t, catalog, canonical)
	raw := mustEditingJSON(t, provider)
	resolved, diagnostics, err := DecodeAndResolveDirectionProposalWithDiagnostics(raw, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Received != len(canonical.Directions) ||
		diagnostics.Accepted != len(canonical.Directions) ||
		diagnostics.Rejected != 0 || len(diagnostics.Issues) != 0 {
		t.Fatalf("valid typed round-trip diagnostics = %#v", diagnostics)
	}
	if !reflect.DeepEqual(resolved, canonical) {
		t.Fatalf("resolved canonical directions changed:\n got %#v\nwant %#v", resolved, canonical)
	}
	canonicalJSON := mustEditingJSON(t, canonical)
	resolvedJSON := mustEditingJSON(t, resolved)
	if !bytes.Equal(resolvedJSON, canonicalJSON) {
		t.Fatalf("canonical direction JSON changed:\n got %s\nwant %s", resolvedJSON, canonicalJSON)
	}

	_, legacy := studyMapFixture(t)
	brief := briefShapeFromLegacy(legacy)
	reviews := make([]ReviewProposal, 0, len(canonical.Directions))
	for _, direction := range canonical.Directions {
		reviews = append(reviews, directReview(direction))
	}
	wantRecord, _, err := BuildReviewedRecord(bundle, brief, canonical, reviews)
	if err != nil {
		t.Fatal(err)
	}
	gotRecord, _, err := BuildReviewedRecord(bundle, brief, resolved, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := mustEditingJSON(t, gotRecord), mustEditingJSON(t, wantRecord); !bytes.Equal(got, want) {
		t.Fatalf("canonical Study JSON changed after typed round-trip:\n got %s\nwant %s", got, want)
	}
}

func TestDirectionReferenceResolutionRejectsInvalidHandles(t *testing.T) {
	t.Parallel()

	bundle, canonical := directionReferenceFixture(t)
	catalog, err := BuildDirectionReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	base := directionReferenceProviderFixture(t, catalog, canonical)
	documentRef, err := catalog.ref(directionReferenceDocument, bundle.Documents[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	anchorRef := base.Directions[0].AnchorRefs[0]

	tests := []struct {
		name   string
		mutate func(*directionReferenceProviderResponseFixture)
		want   string
	}{
		{
			name: "unknown",
			mutate: func(value *directionReferenceProviderResponseFixture) {
				value.Directions[0].AnchorRefs[0] = "a999"
			},
			want: "unknown_anchor_ref",
		},
		{
			name: "wrong kind",
			mutate: func(value *directionReferenceProviderResponseFixture) {
				value.Directions[0].AnchorRefs[0] = documentRef
			},
			want: "wrong_kind_anchor_ref",
		},
		{
			name: "duplicate",
			mutate: func(value *directionReferenceProviderResponseFixture) {
				value.Directions[0].AnchorRefs[1] = value.Directions[0].AnchorRefs[0]
			},
			want: "duplicate_anchor_refs",
		},
		{
			name: "mid handle corruption",
			mutate: func(value *directionReferenceProviderResponseFixture) {
				value.Directions[0].AnchorRefs[0] = "a"
			},
			want: "unknown_anchor_ref",
		},
		{
			name: "prefixed handle",
			mutate: func(value *directionReferenceProviderResponseFixture) {
				value.Directions[0].AnchorRefs[0] = "prefix-" + value.Directions[0].AnchorRefs[0]
			},
			want: "unknown_anchor_ref",
		},
		{
			name: "corrupted handle",
			mutate: func(value *directionReferenceProviderResponseFixture) {
				value.Directions[0].AnchorRefs[0] += "!"
			},
			want: "unknown_anchor_ref",
		},
		{
			name: "compacted prefix",
			mutate: func(value *directionReferenceProviderResponseFixture) {
				value.Directions[0].AnchorRefs[0] = value.Directions[0].AnchorRefs[0][:1]
			},
			want: "unknown_anchor_ref",
		},
		{
			name: "canonical id substitution",
			mutate: func(value *directionReferenceProviderResponseFixture) {
				value.Directions[0].AnchorRefs[0] = bundle.Anchors[0].ID
			},
			want: "unknown_anchor_ref",
		},
		{
			name: "document canonical id substitution",
			mutate: func(value *directionReferenceProviderResponseFixture) {
				value.Directions[0].DocumentRefs[0] = bundle.Documents[0].ID
			},
			want: "unknown_document_ref",
		},
		{
			name: "area canonical id substitution",
			mutate: func(value *directionReferenceProviderResponseFixture) {
				value.Directions[0].AreaRefs[0] = bundle.Areas[0].ID
			},
			want: "unknown_area_ref",
		},
		{
			name: "mechanism canonical id substitution",
			mutate: func(value *directionReferenceProviderResponseFixture) {
				value.Directions[0].MechanismRef = bundle.Mechanisms[0].ID
			},
			want: "unknown_mechanism_ref",
		},
		{
			name: "duplicate reading ref",
			mutate: func(value *directionReferenceProviderResponseFixture) {
				value.Directions[0].ReadingAnchors[1].AnchorRef =
					value.Directions[0].ReadingAnchors[0].AnchorRef
			},
			want: "duplicate_reading_anchor_refs",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := cloneDirectionReferenceProviderFixture(t, base)
			test.mutate(&value)
			resolved, diagnostics, err := DecodeAndResolveDirectionProposalWithDiagnostics(
				mustEditingJSON(t, value), catalog,
			)
			if err != nil {
				t.Fatalf("one invalid candidate discarded valid siblings: %v", err)
			}
			if diagnostics.Accepted != len(base.Directions)-1 ||
				diagnostics.Rejected != 1 || len(diagnostics.Issues) != 1 ||
				diagnostics.Issues[0].Position != 0 || diagnostics.Issues[0].Code != test.want {
				t.Fatalf("invalid typed response diagnostics = %#v, want code %q", diagnostics, test.want)
			}
			if !reflect.DeepEqual(resolved.Directions, canonical.Directions[1:]) {
				t.Fatal("invalid candidate changed the accepted siblings or their order")
			}
		})
	}

	if _, _, err := DecodeAndResolveDirectionProposalWithDiagnostics(
		mustEditingJSON(t, base), catalog,
	); err != nil {
		t.Fatalf("base response invalid after mutation table: %v", err)
	}
	if anchorRef == "" {
		t.Fatal("fixture anchor ref is empty")
	}
}

func TestDirectionReferenceResolutionRejectsCrossRequestHandles(t *testing.T) {
	t.Parallel()

	bundle, canonical := directionReferenceFixture(t)
	first, err := BuildDirectionReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	provider := directionReferenceProviderFixture(t, first, canonical)
	changedBundle := bundle
	changedBundle.DocumentedPurpose += " Exact request identity changed."
	second, err := BuildDirectionReferenceCatalog(changedBundle)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := DecodeAndResolveDirectionProposalWithDiagnostics(
		mustEditingJSON(t, provider), second,
	); err == nil || !strings.Contains(err.Error(), "catalog does not match") {
		t.Fatalf("cross-request typed response error = %v", err)
	}
	corrupt := cloneDirectionReferenceProviderFixture(t, provider)
	middle := len(corrupt.CatalogRef) / 2
	corrupt.CatalogRef = corrupt.CatalogRef[:middle] + corrupt.CatalogRef[middle+2:]
	if _, _, err := DecodeAndResolveDirectionProposalWithDiagnostics(
		mustEditingJSON(t, corrupt), first,
	); err == nil || !strings.Contains(err.Error(), "catalog does not match") {
		t.Fatalf("mid-token corruption error = %v", err)
	}
}

type directionReferenceProviderResponseFixture struct {
	Version    int                                          `json:"version"`
	CatalogRef string                                       `json:"catalog_ref"`
	Directions []directionReferenceProviderCandidateFixture `json:"directions"`
}

type directionReferenceProviderCandidateFixture struct {
	Question        string                            `json:"question"`
	WhyItMatters    string                            `json:"why_it_matters"`
	LearningOutcome string                            `json:"learning_outcome"`
	TargetJob       TargetJob                         `json:"target_user_job"`
	LearningStage   LearningStage                     `json:"learning_stage"`
	AnchorRefs      []string                          `json:"anchor_refs"`
	DocumentRefs    []string                          `json:"document_refs,omitempty"`
	AreaRefs        []string                          `json:"area_refs,omitempty"`
	MechanismRef    string                            `json:"mechanism_ref,omitempty"`
	ReadingAnchors  []directionReferenceReadingAnchor `json:"reading_anchors"`
	SearchQueries   []string                          `json:"search_queries,omitempty"`
}

func directionReferenceFixture(t *testing.T) (Bundle, DirectionProposal) {
	t.Helper()
	bundle, legacy := studyMapFixture(t)
	bundle.Mechanisms = []Mechanism{{
		ID: "mechanism-central", Question: "How does the central path work?",
		Title: "Central path", AnchorIDs: append([]string(nil), legacy.Candidates[0].AnchorIDs...),
		Paths: []string{bundle.Anchors[0].Path},
	}}
	legacy.Candidates[0].MechanismID = bundle.Mechanisms[0].ID
	return bundle, directionsFromLegacy(t, legacy)
}

func directionReferenceProviderFixture(
	t *testing.T,
	catalog DirectionReferenceCatalog,
	canonical DirectionProposal,
) directionReferenceProviderResponseFixture {
	t.Helper()
	result := directionReferenceProviderResponseFixture{
		Version: DirectionProposalVersion, CatalogRef: catalog.CatalogRef(),
	}
	for _, direction := range canonical.Directions {
		item := directionReferenceProviderCandidateFixture{
			Question: direction.Question, WhyItMatters: direction.WhyItMatters,
			LearningOutcome: direction.LearningOutcome, TargetJob: direction.TargetJob,
			LearningStage: direction.LearningStage,
			SearchQueries: append([]string(nil), direction.SearchQueries...),
		}
		for _, id := range direction.AnchorIDs {
			ref, err := catalog.ref(directionReferenceAnchor, id)
			if err != nil {
				t.Fatal(err)
			}
			item.AnchorRefs = append(item.AnchorRefs, ref)
		}
		for _, id := range direction.DocumentIDs {
			ref, err := catalog.ref(directionReferenceDocument, id)
			if err != nil {
				t.Fatal(err)
			}
			item.DocumentRefs = append(item.DocumentRefs, ref)
		}
		for _, id := range direction.AreaIDs {
			ref, err := catalog.ref(directionReferenceArea, id)
			if err != nil {
				t.Fatal(err)
			}
			item.AreaRefs = append(item.AreaRefs, ref)
		}
		if direction.MechanismID != "" {
			ref, err := catalog.ref(directionReferenceMechanism, direction.MechanismID)
			if err != nil {
				t.Fatal(err)
			}
			item.MechanismRef = ref
		}
		for _, reading := range direction.ReadingAnchors {
			ref, err := catalog.ref(directionReferenceAnchor, reading.AnchorID)
			if err != nil {
				t.Fatal(err)
			}
			item.ReadingAnchors = append(item.ReadingAnchors, directionReferenceReadingAnchor{
				AnchorRef: ref, Label: reading.Label, WhatToLookFor: reading.WhatToLookFor,
			})
		}
		result.Directions = append(result.Directions, item)
	}
	return result
}

func cloneDirectionReferenceProviderFixture(
	t *testing.T,
	value directionReferenceProviderResponseFixture,
) directionReferenceProviderResponseFixture {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result directionReferenceProviderResponseFixture
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func directionReferenceCanonicalIDs(bundle Bundle) []string {
	result := make([]string, 0,
		len(bundle.Anchors)+len(bundle.Documents)+len(bundle.Areas)+len(bundle.Mechanisms))
	for _, item := range bundle.Anchors {
		result = append(result, item.ID)
	}
	for _, item := range bundle.Documents {
		result = append(result, item.ID)
	}
	for _, item := range bundle.Areas {
		result = append(result, item.ID)
	}
	for _, item := range bundle.Mechanisms {
		result = append(result, item.ID)
	}
	return result
}
