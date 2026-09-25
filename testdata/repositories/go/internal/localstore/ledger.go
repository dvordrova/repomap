package localstore

// Ledger keeps the keys a run has stored, in order. Its Append method is
// declared in ledger_append.go; on the map of parts it goes with this type.
type Ledger struct{ keys []string }

// Keys returns the stored keys.
func (l *Ledger) Keys() []string { return l.keys }
