package goadapter

import "testing"

func TestSignaturesUseShortPackageNames(t *testing.T) {
	for signature, want := range map[string]string{
		"func(entries []github.com/dvordrova/repomap/internal/corpus.Entry) (map[string]github.com/x/y/z.T, error)": "func(entries []corpus.Entry) (map[string]z.T, error)",
		"func(*golang.org/x/tools/go/ssa.Function) bool":                                                            "func(*ssa.Function) bool",
		"func(a int) string": "func(a int) string",
	} {
		if got := shortSignature(signature); got != want {
			t.Errorf("%q -> %q, want %q", signature, got, want)
		}
	}
	for signature, want := range map[string]string{
		"type example.com/app/model.User struct{ID int64; Name string}":    "struct",
		"type example.com/app/handler.Service interface{Get(int64) error}": "interface",
		"type example.com/app/model.Status string":                         "string",
		"type example.com/app/model.Users []example.com/app/model.User":    "[]model.User",
	} {
		if got := typeSignature(signature); got != want {
			t.Errorf("%q -> %q, want %q", signature, got, want)
		}
	}
}

func TestTagAliasesNameEachFormat(t *testing.T) {
	got := tagAliases(`json:"count_label,omitempty" db:"count" yaml:"-" validate:",required"`)
	if len(got) != 2 || got[0].Format != "json" || got[0].Name != "count_label" || got[1].Format != "db" || got[1].Name != "count" {
		t.Fatalf("aliases = %+v", got)
	}
	if tagAliases("") != nil {
		t.Fatal("an absent tag has aliases")
	}
}
