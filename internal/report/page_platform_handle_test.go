package report

import (
	"encoding/json"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A file's own handle on the platform, a module logger, is no tile: the
// part's tiles are its functions and its other module variables
// (freqtrade's Trading bot core had stood three "logger" tiles).
func TestAModuleLoggerIsNoTile(t *testing.T) {
	part := groupindex.Group{ID: "bot", Title: "Trading bot core", MemberSubjectIDs: []string{"module", "logger", "limit", "process"}}
	b := pageBuilder{subjects: map[string]subjectRef{}}
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "bot.py", Line: line, Column: 1}
	}
	b.subjects["module"] = subjectRef{subject: groupindex.Subject{ID: "module", Object: &groupindex.ObjectFacts{Name: "bot", Kind: programindex.ObjectModule, Location: at(1)}}}
	b.subjects["logger"] = subjectRef{subject: groupindex.Subject{ID: "logger", Object: &groupindex.ObjectFacts{Name: "logger", Kind: programindex.ObjectVariable, ContainerID: "module", Location: at(3), PlatformHandle: true}}}
	b.subjects["limit"] = subjectRef{subject: groupindex.Subject{ID: "limit", Object: &groupindex.ObjectFacts{Name: "LIMIT", Kind: programindex.ObjectVariable, ContainerID: "module", Location: at(4)}}}
	b.subjects["process"] = subjectRef{subject: groupindex.Subject{ID: "process", Object: &groupindex.ObjectFacts{Name: "process", Kind: programindex.ObjectFunction, ContainerID: "module", Location: at(6)}}}
	raw, _ := b.groupSymbols("t1", part)
	var symbols []pageNodeSymbol
	if err := json.Unmarshal([]byte(raw), &symbols); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, symbol := range symbols {
		names = append(names, symbol.Name)
	}
	if len(names) != 2 || names[0] != "LIMIT" && names[1] != "LIMIT" || names[0] != "process" && names[1] != "process" {
		t.Fatalf("tiles %v, want LIMIT and process without the logger", names)
	}
}
