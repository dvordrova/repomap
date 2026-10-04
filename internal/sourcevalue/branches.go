package sourcevalue

import "slices"

// Compared is one value a function compares with words, as the value walks
// read it: the parameter it is (Position, from one, and Name; zero for any
// other value) and each case of the comparison.
type Compared struct {
	Position int
	Name     string
	Cases    []ComparedCase
}

// ComparedCase is one case: its words, the lines of the function's file its
// branch selects (with the columns it begins and ends at on them, when
// known), and whether the branch runs only for those words.
type ComparedCase struct {
	Words     []string
	Line      int
	EndLine   int
	Column    int
	EndColumn int
	Exclusive bool
}

// holds says whether a branch holds an anchor: in, out, or not proven. A
// line strictly between its first and last is in; on its first or last
// line it is in or out by the columns, and not proven when either side
// writes none (another statement may share the line).
func (item ComparedCase) holds(anchor *Anchor) (in, proven bool) {
	if item.Line < 1 || anchor.Line < item.Line || anchor.Line > item.EndLine {
		return false, true
	}
	if anchor.Line > item.Line && anchor.Line < item.EndLine {
		return true, true
	}
	if anchor.Column < 1 || item.Column < 1 || item.EndColumn < 1 {
		return false, false
	}
	if anchor.Line == item.Line && anchor.Column < item.Column {
		return false, true
	}
	if anchor.Line == item.EndLine && anchor.Column > item.EndColumn {
		return false, true
	}
	return true, true
}

// OutsideItsBranch says a value written at anchor, inside a function of the
// file path, is not what that function gives a call that supplies its
// parameters the words supplied reports: the anchor lies in the branch of an
// exclusive case comparing a parameter with words, the call supplies that
// parameter a word none of them, and the anchor lies in no other case's
// branch (by lines, and by columns on a branch's first and last line: a
// value whose place on such a line is not known is kept). casdoor's
// GetConfigString("dataSourceName") never returns the URL its
// `key == "staticBaseUrl"` branch stores. A parameter the call supplies no
// word for, a case not known exclusive or a value with no anchor keeps the
// value: unknown stays unknown.
func OutsideItsBranch(anchor *Anchor, path string, comparisons []Compared, supplied func(position int, name string) (string, bool)) bool {
	if anchor == nil || anchor.Path != path || supplied == nil {
		return false
	}
	excluded := false
	for _, comparison := range comparisons {
		for _, item := range comparison.Cases {
			in, proven := item.holds(anchor)
			if !proven {
				// Whether the value is written in this branch is not
				// known: it is kept.
				return false
			}
			if !in {
				continue
			}
			if !item.Exclusive || comparison.Position < 1 {
				return false
			}
			word, known := supplied(comparison.Position, comparison.Name)
			if !known || slices.Contains(item.Words, word) {
				return false
			}
			if excluded {
				// In two cases' branches: which one holds it is not known.
				return false
			}
			excluded = true
		}
	}
	return excluded
}
