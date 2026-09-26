package localstore

// Ledger keeps the keys a run has stored, in order. Its Append method is
// declared in ledger_append.go; on the map of parts it goes with this type.
type Ledger struct{ keys []string }

// Keys returns the stored keys.
func (l *Ledger) Keys() []string { return l.keys }

// A package may declare init more than once. The map of parts reads the
// repeated name as one unit, and each init counts its own lines of code: this
// doc comment, the comment inside and the blank line are none of them.
func init() {
	// The ledger's own keys come first.
	registered = append(registered, "ledger")

	registered = append(registered, "keys")
}

func init() { registered = append(registered, "append") }

var registered []string
