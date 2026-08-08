package themestudy

import (
	"fmt"
	"strings"
	"testing"
)

func TestD239ScoutCapsKnownAnchorExcessWithoutDroppingTheme(t *testing.T) {
	t.Parallel()

	known := make(map[string]struct{})
	refs := make([]string, MaxThemeAnchors+2)
	for index := range refs {
		refs[index] = fmt.Sprintf("a%d", index+1)
		known[refs[index]] = struct{}{}
	}
	raw := []byte(`{"themes":[{"title":"Storage","question":"How is storage organized?","theme_kind":"shared_domain_responsibility","anchor_refs":["` +
		strings.Join(refs, `","`) +
		`"],"expansion_file_refs":[],"why_it_matters":"Storage is central.","expected_learning":"Understand the storage boundary."}]}`)

	candidates, status, err := ValidateScout(raw, known, map[string]struct{}{}, "catalog")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "accepted" || status.Accepted != 1 || status.Rejected != 0 || len(status.Issues) != 0 {
		t.Fatalf("status = %#v", status)
	}
	if status.Normalized["anchor_refs_capped"] != 2 {
		t.Fatalf("normalization = %#v, want two capped anchors", status.Normalized)
	}
	if len(candidates) != 1 || len(candidates[0].AnchorRefs) != MaxThemeAnchors {
		t.Fatalf("candidates = %#v", candidates)
	}
	for index, ref := range candidates[0].AnchorRefs {
		if ref != refs[index] {
			t.Fatalf("retained anchor %d = %q, want %q", index, ref, refs[index])
		}
	}
}

func TestD239ScoutDoesNotNormalizeUnknownAnchorInExcessSuffix(t *testing.T) {
	t.Parallel()

	known := make(map[string]struct{})
	refs := make([]string, MaxThemeAnchors)
	for index := range refs {
		refs[index] = fmt.Sprintf("a%d", index+1)
		known[refs[index]] = struct{}{}
	}
	refs = append(refs, "a999")
	raw := []byte(`{"themes":[{"title":"Storage","question":"How is storage organized?","theme_kind":"shared_domain_responsibility","anchor_refs":["` +
		strings.Join(refs, `","`) +
		`"],"expansion_file_refs":[],"why_it_matters":"Storage is central.","expected_learning":"Understand the storage boundary."}]}`)

	candidates, status, err := ValidateScout(raw, known, map[string]struct{}{}, "catalog")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 || status.State != "failed" || status.Rejected != 1 ||
		len(status.Issues) != 1 || status.Issues[0].Code != ScoutIssueUnknownRef {
		t.Fatalf("unknown suffix was hidden by normalization: candidates=%#v status=%#v", candidates, status)
	}
	if status.Normalized != nil {
		t.Fatalf("unknown suffix recorded a normalization: %#v", status.Normalized)
	}
}

func TestD239ScoutNormalizesUnknownOrEmptyThemeKindWithValidEvidence(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		kind string
	}{
		{name: "provider invented module registry", kind: "module_registry"},
		{name: "empty", kind: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			raw := []byte(fmt.Sprintf(
				`{"themes":[{"title":"Module registry","question":"How are modules registered?","theme_kind":%q,"anchor_refs":["a1"],"expansion_file_refs":["f1"],"why_it_matters":"Registration connects modules to runtime behavior.","expected_learning":"Understand the module registration boundary."}]}`,
				test.kind,
			))

			candidates, status, err := ValidateScout(
				raw,
				map[string]struct{}{"a1": {}},
				map[string]struct{}{"f1": {}},
				"catalog",
			)
			if err != nil {
				t.Fatal(err)
			}
			if status.State != "accepted" || status.Accepted != 1 || status.Rejected != 0 ||
				len(status.Issues) != 0 || status.Normalized["theme_kind"] != 1 {
				t.Fatalf("status = %#v", status)
			}
			if len(candidates) != 1 || candidates[0].ThemeKind != KindSharedDomainResponsibility ||
				len(candidates[0].AnchorRefs) != 1 || candidates[0].AnchorRefs[0] != "a1" ||
				len(candidates[0].ExpansionFileRefs) != 1 || candidates[0].ExpansionFileRefs[0] != "f1" {
				t.Fatalf("candidate = %#v", candidates)
			}
		})
	}
}

func TestD239ScoutThemeKindFallbackDoesNotRelaxSchemaOrRefs(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		raw  string
		code ScoutIssueCode
	}{
		{
			name: "non string kind remains schema invalid",
			raw:  `{"themes":[{"title":"Module registry","question":"How are modules registered?","theme_kind":7,"anchor_refs":["a1"],"why_it_matters":"Registration is central.","expected_learning":"Understand registration."}]}`,
			code: ScoutIssueDecodeCandidate,
		},
		{
			name: "invented kind does not hide unknown ref",
			raw:  `{"themes":[{"title":"Module registry","question":"How are modules registered?","theme_kind":"module_registry","anchor_refs":["a999"],"why_it_matters":"Registration is central.","expected_learning":"Understand registration."}]}`,
			code: ScoutIssueUnknownRef,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidates, status, err := ValidateScout(
				[]byte(test.raw),
				map[string]struct{}{"a1": {}},
				map[string]struct{}{},
				"catalog",
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(candidates) != 0 || status.State != "failed" || status.Rejected != 1 ||
				len(status.Issues) != 1 || status.Issues[0].Code != test.code {
				t.Fatalf("candidates = %#v, status = %#v", candidates, status)
			}
			if status.Normalized != nil {
				t.Fatalf("rejected candidate recorded normalization: %#v", status.Normalized)
			}
		})
	}
}
