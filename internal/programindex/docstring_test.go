package programindex

import (
	"bytes"
	"slices"
	"testing"
)

func TestNativeDocstringAttachmentsAreSealedClonedAndRoundTrip(t *testing.T) {
	input := representativeInput()
	input.Objects[0].DocstringRanges = []LineRange{{Line: 3, EndLine: 3, Column: 1, EndColumn: 9}, {Line: 2, EndLine: 2, Column: 1, EndColumn: 9}, {Line: 3, EndLine: 3, Column: 1, EndColumn: 9}}
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatal(err)
	}
	object := objectWithSourceRef(t, index, "object-method")
	if !slices.Equal(object.DocstringRanges, []LineRange{{Line: 2, EndLine: 2, Column: 1, EndColumn: 9}, {Line: 3, EndLine: 3, Column: 1, EndColumn: 9}}) {
		t.Fatalf("complete canonical comment lines: %v", object.DocstringRanges)
	}
	input.Objects[0].DocstringRanges[0].Line = 99
	snapshot := index.Snapshot()
	for position := range snapshot.Objects {
		if snapshot.Objects[position].ID == object.ID {
			snapshot.Objects[position].DocstringRanges[0].Line = 99
		}
	}
	if err := index.Validate(); err != nil || !slices.Equal(object.DocstringRanges, []LineRange{{Line: 2, EndLine: 2, Column: 1, EndColumn: 9}, {Line: 3, EndLine: 3, Column: 1, EndColumn: 9}}) {
		t.Fatalf("external mutable storage changed the native seal: %v", err)
	}
	encoded, err := Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Decode(encoded)
	if err != nil || restored.SHA256 != index.SHA256 {
		t.Fatalf("native attachment roundtrip: %v", err)
	}
	for position := range restored.Objects {
		if restored.Objects[position].ID == object.ID {
			if !slices.Equal(restored.Objects[position].DocstringRanges, []LineRange{{Line: 2, EndLine: 2, Column: 1, EndColumn: 9}, {Line: 3, EndLine: 3, Column: 1, EndColumn: 9}}) {
				t.Fatal("native attachment lost during decoding")
			}
			restored.Objects[position].DocstringRanges[0].Column = 2
		}
	}
	if restored.Validate() == nil {
		t.Fatal("changed attachment retained a previous native seal")
	}
}

func TestAbsentNativeDocstringMetadataPreservesV28CanonicalBytesAndSeal(t *testing.T) {
	input := representativeInput()
	absent, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatal(err)
	}
	before, err := Encode(absent)
	if err != nil {
		t.Fatal(err)
	}
	if absent.Version != 28 || bytes.Contains(before, []byte("docstring_ranges")) {
		t.Fatal("absence is no longer the unchanged v28 optional-field representation")
	}
	input.Objects[0].DocstringRanges = []LineRange{}
	empty, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatal(err)
	}
	after, err := Encode(empty)
	if err != nil || !bytes.Equal(before, after) || empty.SHA256 != absent.SHA256 {
		t.Fatalf("empty optional attachment changed original canonical bytes/seal: %v", err)
	}
	restored, err := Decode(before)
	if err != nil || restored.SHA256 != absent.SHA256 {
		t.Fatalf("absent-field artifact cannot retain its existing seal: %v", err)
	}
}

func TestNativeDocstringAttachmentsRequireClosedLocatedPositiveCanonicalRanges(t *testing.T) {
	good := LineRange{Line: 1, EndLine: 1, Column: 1, EndColumn: 9}
	for _, ranges := range [][]LineRange{
		{{Line: 0, EndLine: 1, Column: 1, EndColumn: 9}},
		{{Line: 1, EndLine: 1, Column: 0, EndColumn: 9}},
		{{Line: 2, EndLine: 1, Column: 1, EndColumn: 9}},
		{{Line: 1, EndLine: 1, Column: 9, EndColumn: 1}},
		{{Line: 2, EndLine: 2, Column: 1, EndColumn: 9}, good},
		{good, good},
	} {
		if validDocstringRanges(&Location{Path: "src/a.ts", Line: 3, Column: 1}, ranges) {
			t.Fatalf("invalid native attachment accepted: %v", ranges)
		}
	}
	if validDocstringRanges(nil, []LineRange{good}) || validDocstringRanges(&Location{Path: "/host/a.ts", Line: 3, Column: 1}, []LineRange{good}) {
		t.Fatal("attachment lost its closed source location")
	}
}
