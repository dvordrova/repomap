package studymap

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestBriefShapeReferenceCatalogIsDeterministicAndHidesCanonicalIDs(t *testing.T) {
	t.Parallel()

	bundle, _ := studyMapFixture(t)
	first, err := BuildBriefShapeReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildBriefShapeReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest() != second.Digest() ||
		first.CatalogRef() != second.CatalogRef() ||
		!bytes.Equal(first.PromptBundleJSON(), second.PromptBundleJSON()) {
		t.Fatal("exact Brief/Shape catalog is not deterministic")
	}
	for _, id := range directionReferenceCanonicalIDs(bundle) {
		if bytes.Contains(first.PromptBundleJSON(), []byte(id)) {
			t.Fatalf("typed Brief/Shape wire leaked canonical id %q", id)
		}
	}
	for _, marker := range []string{
		`"catalog_ref":"b_`, `"anchor_ref":"a1"`,
		`"document_ref":"d1"`, `"area_ref":"r1"`,
	} {
		if !bytes.Contains(first.PromptBundleJSON(), []byte(marker)) {
			t.Fatalf("typed Brief/Shape wire omitted %q", marker)
		}
	}

	changed := bundle
	changed.DocumentedPurpose += " Changed request identity."
	drifted, err := BuildBriefShapeReferenceCatalog(changed)
	if err != nil {
		t.Fatal(err)
	}
	if drifted.CatalogRef() == first.CatalogRef() || drifted.Digest() == first.Digest() {
		t.Fatal("Brief/Shape request drift retained the catalog identity")
	}
}

func TestBriefShapeReferenceResolutionOmitsWrongKindShapeItem(t *testing.T) {
	t.Parallel()

	bundle, proposal := studyMapFixture(t)
	catalog, err := BuildBriefShapeReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	response := briefShapeReferenceResponseFixture(t, catalog, proposal)
	documentRef, err := catalog.objects.ref(directionReferenceDocument, bundle.Documents[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	response.ShapeAreaRefs = append(response.ShapeAreaRefs, documentRef)

	resolved, diagnostics, err := DecodeAndResolveBriefShapeProposal(
		mustEditingJSON(t, response), catalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := briefShapeFromLegacy(proposal)
	if !reflect.DeepEqual(resolved, want) {
		t.Fatalf("resolved Brief/Shape = %#v, want %#v", resolved, want)
	}
	if diagnostics.ShapeReceived != len(proposal.ShapeAreaIDs)+1 ||
		diagnostics.ShapeAccepted != len(proposal.ShapeAreaIDs) ||
		diagnostics.ShapeRejected != 1 || len(diagnostics.Issues) != 1 ||
		diagnostics.Issues[0].Position != len(proposal.ShapeAreaIDs) ||
		diagnostics.Issues[0].Field != "shape_area_refs" ||
		diagnostics.Issues[0].Code != "wrong_kind_area_ref" {
		t.Fatalf("wrong-kind Shape diagnostics = %#v", diagnostics)
	}
}

func TestBriefShapeReferenceResolutionAllowsExplicitEmptyShape(t *testing.T) {
	t.Parallel()

	bundle, proposal := studyMapFixture(t)
	catalog, err := BuildBriefShapeReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	response := briefShapeReferenceResponseFixture(t, catalog, proposal)
	documentRef, err := catalog.objects.ref(directionReferenceDocument, bundle.Documents[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	response.ShapeAreaRefs = []string{documentRef, "r999"}

	resolved, diagnostics, err := DecodeAndResolveBriefShapeProposal(
		mustEditingJSON(t, response), catalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.ShapeAreaIDs) != 0 || diagnostics.ShapeAccepted != 0 ||
		diagnostics.ShapeRejected != 2 ||
		diagnostics.Issues[0].Code != "wrong_kind_area_ref" ||
		diagnostics.Issues[1].Code != "unknown_area_ref" {
		t.Fatalf("explicit empty Shape/diagnostics = %#v/%#v", resolved.ShapeAreaIDs, diagnostics)
	}
	brief := briefShapeFromLegacy(proposal)
	brief.ShapeAreaIDs = nil
	if !reflect.DeepEqual(resolved, brief) {
		t.Fatal("empty Shape changed the supported Brief")
	}
}

func TestBriefShapeReferenceResolutionRejectsCatalogAndRequiredSupportFailures(t *testing.T) {
	t.Parallel()

	bundle, proposal := studyMapFixture(t)
	catalog, err := BuildBriefShapeReferenceCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	base := briefShapeReferenceResponseFixture(t, catalog, proposal)

	crossRequest := base
	crossRequest.CatalogRef += "x"
	_, diagnostics, err := DecodeAndResolveBriefShapeProposal(
		mustEditingJSON(t, crossRequest), catalog,
	)
	if err == nil || !strings.Contains(err.Error(), "catalog does not match") ||
		len(diagnostics.Issues) != 1 || diagnostics.Issues[0].Field != "catalog_ref" ||
		diagnostics.Issues[0].Code != "catalog_ref_mismatch" {
		t.Fatalf("cross-request error/diagnostics = %v/%#v", err, diagnostics)
	}

	canonicalSubstitution := base
	canonicalSubstitution.Brief.WhatItIs.SupportRefs = []string{bundle.Anchors[0].ID}
	_, diagnostics, err = DecodeAndResolveBriefShapeProposal(
		mustEditingJSON(t, canonicalSubstitution), catalog,
	)
	if err == nil || len(diagnostics.Issues) != 1 ||
		diagnostics.Issues[0].Field != "brief.what_it_is.support_refs" ||
		diagnostics.Issues[0].Position != 0 ||
		diagnostics.Issues[0].Code != "raw_canonical_support_ref" {
		t.Fatalf("canonical-ID substitution error/diagnostics = %v/%#v", err, diagnostics)
	}

	nonExact := base
	nonExact.Brief.WhatItIs.SupportRefs = []string{
		base.Brief.WhatItIs.SupportRefs[0] + "-suffix",
	}
	_, diagnostics, err = DecodeAndResolveBriefShapeProposal(
		mustEditingJSON(t, nonExact), catalog,
	)
	if err == nil || len(diagnostics.Issues) != 1 ||
		diagnostics.Issues[0].Code != "non_exact_support_ref" {
		t.Fatalf("non-exact support error/diagnostics = %v/%#v", err, diagnostics)
	}

	duplicate := base
	duplicate.Brief.WhatItIs.SupportRefs = []string{
		base.Brief.WhatItIs.SupportRefs[0], base.Brief.WhatItIs.SupportRefs[0],
	}
	_, diagnostics, err = DecodeAndResolveBriefShapeProposal(
		mustEditingJSON(t, duplicate), catalog,
	)
	if err == nil || len(diagnostics.Issues) != 1 ||
		diagnostics.Issues[0].Position != 1 ||
		diagnostics.Issues[0].Code != "duplicate_support_ref" {
		t.Fatalf("duplicate support error/diagnostics = %v/%#v", err, diagnostics)
	}

	mechanismBundle := bundle
	mechanismBundle.Mechanisms = []Mechanism{{
		ID: "mechanism-one", Question: "How does the central path work?",
		Title: "Central path", AnchorIDs: []string{bundle.Anchors[0].ID},
	}}
	mechanismCatalog, err := BuildBriefShapeReferenceCatalog(mechanismBundle)
	if err != nil {
		t.Fatal(err)
	}
	wrongKind := briefShapeReferenceResponseFixture(t, mechanismCatalog, proposal)
	mechanismRef, err := mechanismCatalog.objects.ref(
		directionReferenceMechanism, mechanismBundle.Mechanisms[0].ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	wrongKind.Brief.WhatItIs.SupportRefs = []string{mechanismRef}
	_, diagnostics, err = DecodeAndResolveBriefShapeProposal(
		mustEditingJSON(t, wrongKind), mechanismCatalog,
	)
	if err == nil || len(diagnostics.Issues) != 1 ||
		diagnostics.Issues[0].Code != "wrong_kind_support_ref" {
		t.Fatalf("wrong-kind required support error/diagnostics = %v/%#v", err, diagnostics)
	}
}

func TestBriefShapeReferenceRecoveryRejectsCanonicalProviderShape(t *testing.T) {
	t.Parallel()

	_, proposal := studyMapFixture(t)
	canonical, err := json.Marshal(briefShapeFromLegacy(proposal))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverBriefShapeReferenceProviderJSON(canonical); err == nil {
		t.Fatal("typed Brief/Shape recovery accepted canonical-ID provider DTO")
	}
}

func briefShapeReferenceResponseFixture(
	t *testing.T,
	catalog BriefShapeReferenceCatalog,
	proposal Proposal,
) briefShapeReferenceProviderResponse {
	t.Helper()
	resolveSupports := func(statement BriefStatement) []string {
		refs := make([]string, 0, len(statement.SupportIDs))
		for _, id := range statement.SupportIDs {
			var ref string
			var err error
			for _, kind := range []directionReferenceKind{
				directionReferenceAnchor,
				directionReferenceDocument,
				directionReferenceArea,
			} {
				ref, err = catalog.objects.ref(kind, id)
				if err == nil {
					break
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			refs = append(refs, ref)
		}
		return refs
	}
	statement := func(value BriefStatement) briefShapeReferenceStatement {
		return briefShapeReferenceStatement{
			Text: value.Text, SupportRefs: resolveSupports(value),
		}
	}
	response := briefShapeReferenceProviderResponse{
		Version: BriefShapeProposalVersion, CatalogRef: catalog.CatalogRef(),
		RepositoryType: proposal.RepositoryType,
		Brief: briefShapeReferenceProviderBrief{
			WhatItIs:              statement(proposal.Brief.WhatItIs),
			Problem:               statement(proposal.Brief.Problem),
			MainInput:             statement(proposal.Brief.MainInput),
			CentralResponsibility: statement(proposal.Brief.CentralResponsibility),
			ObservableResult:      statement(proposal.Brief.ObservableResult),
		},
	}
	for _, areaID := range proposal.ShapeAreaIDs {
		ref, err := catalog.objects.ref(directionReferenceArea, areaID)
		if err != nil {
			t.Fatal(err)
		}
		response.ShapeAreaRefs = append(response.ShapeAreaRefs, ref)
	}
	return response
}
