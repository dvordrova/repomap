package report

import (
	"encoding/json"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A part's tiles stand by file, and a tile is the declaration its reading
// names: each carries its file and the same source link, served or static.
func TestPartTilesCarryTheirFileAndSource(t *testing.T) {
	part := groupindex.Group{ID: "lists", Title: "Linked list", MemberSubjectIDs: []string{"create", "node"}}
	for _, links := range []pageLinks{
		{sourceIDs: map[string]string{"adlist.c": "adlist-c", "adlist.h": "adlist-h"}},
		{repositoryURL: "https://github.com/redis/redis", blobPrefix: "/blob/", revision: "abc"},
	} {
		b := pageBuilder{subjects: map[string]subjectRef{}, links: links}
		b.subjects["create"] = subjectRef{subject: groupindex.Subject{ID: "create", Object: &groupindex.ObjectFacts{Name: "listCreate", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "adlist.c", Line: 40, Column: 1}}}}
		b.subjects["node"] = subjectRef{subject: groupindex.Subject{ID: "node", Object: &groupindex.ObjectFacts{Name: "listNode", Kind: programindex.ObjectType, Location: &programindex.Location{Path: "adlist.h", Line: 36, Column: 1}}}}
		raw, _ := b.groupSymbols("t1", part)
		var symbols []pageNodeSymbol
		if err := json.Unmarshal([]byte(raw), &symbols); err != nil {
			t.Fatal(err)
		}
		if len(symbols) != 2 || symbols[0].Path != "adlist.c" || symbols[1].Path != "adlist.h" {
			t.Fatalf("tiles lost their files: %+v", symbols)
		}
		for i, symbol := range symbols {
			anchor := links.anchor(symbol.Path, []int{40, 36}[i], 1)
			if symbol.Href != anchor.Href || symbol.Open != anchor.Open || symbol.Href+symbol.Open == "" {
				t.Fatalf("%s: tile source %q/%q differs from its reading's %q/%q", symbol.Name, symbol.Href, symbol.Open, anchor.Href, anchor.Open)
			}
		}
	}
}
