package lines

import "testing"

// A Lisp map's row is its key and value as written, from its first word up
// to the next row's: othello's key->command at :n reads `:n :new-game`,
// where the text around the word is the whole map.
func TestALispMapRowIsItsOwnPair(t *testing.T) {
	src := []byte("(def key->command\n  {:n :new-game\n   :u :undo})\n")
	file := NewCallFile(src, "events.cljc")
	if got := file.RowText(2, 4, [][2]int{{3, 4}}); got != ":n :new-game" {
		t.Fatalf("row at :n = %q", got)
	}
	if got := file.RowText(3, 4, [][2]int{{2, 12}}); got != ":u :undo" {
		t.Fatalf("row at :u = %q", got)
	}
}
