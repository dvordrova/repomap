package storefixture

// Package holds a name in a source declaration, not a package description.
type Package struct {
	Name string
}

// DocumentedEntry retains an author's description beside its declaration.
//
// Example:
//
//	entry := DocumentedEntry{Name: "level"}
//	_ = entry.Name
//
// The example is documentation rather than another native declaration.
// The name still binds the whole comment to this type.
//
// A newcomer should see that author's statement when reading the type,
// even when its example and prose make the comment longer than a nearby
// comment heuristic would permit.
//
// This describes source structure, not a runtime storage effect.
type DocumentedEntry struct {
	Name string
}

type UndocumentedEntry struct {
	Count int
}

// Label returns the authored entry name.
func (
	entry *DocumentedEntry,
) Label() string {
	return entry.Name
}

func (entry *DocumentedEntry) UndocumentedLabel() string {
	return entry.Name
}

// SameLineDocumented keeps its own author statement.
type SameLineDocumented struct{}; type SameLineUndocumented struct{}

// SameLineValue documents the whole Go variable declaration.
var SameLineValue, SameLineOther = 1, 2

// SameLineConstant documents the whole Go constant declaration.
const SameLineConstant, SameLineOtherConstant = 1, 2
