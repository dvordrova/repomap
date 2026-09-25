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
		// Type parameters are part of what a generic type is. Their
		// constraints hold spaces and brackets; the type's fields and struct
		// tags after them are never signature text.
		`type example.com/app/llm.Call[T any] struct{SplitRejectedResponse bool "json:\"split\""; Value T}`: "[T any] struct",
		"type example.com/app/model.Index[K comparable, V map[string]int] struct{Entries map[K]V}":          "[K comparable, V map[string]int] struct",
		"type example.com/app/model.Label[T interface{~int | ~string}] interface{Value() T}":                "[T interface{~int | ~string}] interface",
		"type example.com/app/model.Grid[T interface{~[]map[string][2]int}, U []T] func(T) U":               "[T interface{~[]map[string][2]int}, U []T] func(T) U",
		"type example.com/app/model.Batch[T example.com/app/model.Label[int]] []T":                          "[T model.Label[int]] []T",
		"type example.com/app/model.Pages[T any] = example.com/app/model.Page[T]":                           "[T any] = model.Page[T]",
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
