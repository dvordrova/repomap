package report

import (
	"encoding/json"
	"slices"
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
		lines := map[string]int{"listCreate": 40, "listNode": 36}
		files := map[string]string{"listCreate": "adlist.c", "listNode": "adlist.h"}
		if len(symbols) != 2 || symbols[0].Path != files[symbols[0].Name] || symbols[1].Path != files[symbols[1].Name] {
			t.Fatalf("tiles lost their files: %+v", symbols)
		}
		for _, symbol := range symbols {
			anchor := links.anchor(symbol.Path, lines[symbol.Name], 1)
			if symbol.Href != anchor.Href || symbol.Open != anchor.Open || symbol.Href+symbol.Open == "" {
				t.Fatalf("%s: tile source %q/%q differs from its reading's %q/%q", symbol.Name, symbol.Href, symbol.Open, anchor.Href, anchor.Open)
			}
		}
	}
}

// A part's tiles stand in the page's order: the model's keys, then the
// types, then the rest, so a part's data stands before the code that works
// on it (owner, 2026-09-28).
func TestPartTilesStandKeysThenTypesThenTheRest(t *testing.T) {
	part := groupindex.Group{ID: "zset", Title: "Sorted sets", MemberSubjectIDs: []string{"insert", "node", "add", "list"}}
	b := pageBuilder{subjects: map[string]subjectRef{}}
	object := func(id, name string, kind programindex.ObjectKind, line int, key bool) {
		subject := groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: name, Kind: kind, Location: &programindex.Location{Path: "redis.c", Line: line, Column: 1}}}
		if key {
			subject.Interpretation = &groupindex.Interpretation{Key: true}
		}
		b.subjects[id] = subjectRef{subject: subject}
	}
	object("insert", "zslInsert", programindex.ObjectFunction, 10, false)
	object("node", "zskiplistNode", programindex.ObjectType, 20, false)
	object("add", "zaddCommand", programindex.ObjectFunction, 30, true)
	object("list", "zskiplist", programindex.ObjectType, 40, false)
	raw, _ := b.groupSymbols("t1", part)
	var symbols []pageNodeSymbol
	if err := json.Unmarshal([]byte(raw), &symbols); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, symbol := range symbols {
		names = append(names, symbol.Name)
	}
	if want := []string{"zaddCommand", "zskiplistNode", "zskiplist", "zslInsert"}; !slices.Equal(names, want) {
		t.Fatalf("tiles %v, want %v", names, want)
	}
}
