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
// branch selects, and whether the branch runs only for those words.
type ComparedCase struct {
	Words     []string
	Line      int
	EndLine   int
	Exclusive bool
}

// OutsideItsBranch says a value written at anchor, inside a function of the
// file path, is not what that function gives a call that supplies its
// parameters the words supplied reports: the anchor lies in the branch of an
// exclusive case comparing a parameter with words, the call supplies that
// parameter a word none of them, and the anchor lies in no other case's
// branch (Go's `} else if` line belongs to two). casdoor's
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
			if item.Line < 1 || anchor.Line < item.Line || anchor.Line > item.EndLine {
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
