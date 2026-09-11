package groupindex

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestDataOwnerUsesExactUniqueNativeTypeSource(t *testing.T) {
	location := &programindex.Location{Path: "models.py", Line: 9, Column: 1}
	program := programindex.Index{Objects: []programindex.Object{
		{ID: "model", Kind: programindex.ObjectType, Name: "Trade", Location: location},
		{ID: "same-name", Kind: programindex.ObjectType, Name: "Trade", Location: &programindex.Location{Path: "archive.py", Line: 9, Column: 1}},
		{ID: "function", Kind: programindex.ObjectFunction, Name: "Trade", Location: location},
	}}
	rows := []atlas.DataRecord{{ID: "table", Path: "models.py", Line: 10, Data: &facts.DataObject{Kind: "table", Origin: "orm", Scope: "orm:models.Base", Name: "trades", Owner: &facts.Anchor{Path: "models.py", Line: 9}}}}
	result := projectData(program, rows)
	if len(result) != 1 || result[0].OwnerSubjectID != "model" {
		t.Fatal("schema did not attach to its unique class anchor")
	}
	result[0].Data.Owner.Line = 11
	if rows[0].Data.Owner.Line != 9 {
		t.Fatal("projection shares mutable source ownership")
	}
	program.Objects = append(program.Objects, programindex.Object{ID: "ambiguous", Kind: programindex.ObjectType, Name: "Trade", Location: location})
	result = projectData(program, rows)
	if len(result) != 1 || result[0].OwnerSubjectID != "" {
		t.Fatal("ambiguous source invented one owner or removed schema")
	}
}

func TestDataQueryOwnerKeepsExactNativeCallableColumn(t *testing.T) {
	program := programindex.Index{Objects: []programindex.Object{
		{ID: "first", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "query.ts", Line: 3, Column: 1}},
		{ID: "second", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "query.ts", Line: 3, Column: 30}},
	}}
	rows := []atlas.DataRecord{{ID: "query", Path: "query.ts", Line: 3, Data: &facts.DataObject{Kind: "query", Origin: "query", Scope: "source:query.ts", Name: "read", SQL: "SELECT 1", Owner: &facts.Anchor{Path: "query.ts", Line: 3, Column: 30}}}}
	if got := projectData(program, rows); len(got) != 1 || got[0].OwnerSubjectID != "second" {
		t.Fatalf("precise query owner lost: %+v", got)
	}
	rows[0].Data.Owner.Column = 0
	if got := projectData(program, rows); len(got) != 1 || got[0].OwnerSubjectID != "" {
		t.Fatal("ambiguous same-line query picked one owner")
	}
}
