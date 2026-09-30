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

// An entry a Lisp form hands over under a keyword is written as that
// keyword and its value: othello's q/sketch hands six functions over under
// ten keywords, and each entry had been written as the whole form. A value
// no keyword precedes keeps the call around it.
func TestALispKeywordEntryIsItsOwnPair(t *testing.T) {
	src := []byte("(q/sketch\n  :title \"Othello\"\n  :key-pressed host/on-key\n  :middleware [m/fun-mode])\n(ws/websocket url {:on-message (fn [m] (read m))})\n(future (greet names))\n")
	file := NewCallFile(src, "sketch.clj")
	for _, c := range []struct {
		line, column int
		want         string
	}{
		{3, 16, ":key-pressed host/on-key"},
		{5, 32, ":on-message (fn [m] (read m))"},
		{6, 9, "(greet names)"},
	} {
		if got := file.EntryText(c.line, c.column); got != c.want {
			t.Fatalf("entry at %d:%d = %q, want %q", c.line, c.column, got, c.want)
		}
	}
}
