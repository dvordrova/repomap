package gitfiles

import (
	"reflect"
	"testing"
)

func TestSplitNull(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want []string
	}{
		{"empty", nil, nil},
		{"single with trailing null", []byte("README.md\x00"), []string{"README.md"}},
		{"two with trailing null", []byte("cmd/main.go\x00pkg/foo.go\x00"), []string{"cmd/main.go", "pkg/foo.go"}},
		{"no trailing null", []byte("a.go\x00b.go"), []string{"a.go", "b.go"}},
		{"single no trailing null", []byte("README.md"), []string{"README.md"}},
		{"only null", []byte{0}, nil},
		{"three files", []byte("a\x00b\x00c\x00"), []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitNull(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("splitNull(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
