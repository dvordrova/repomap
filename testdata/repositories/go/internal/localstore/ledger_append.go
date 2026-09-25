package localstore

// Append is a method of Ledger declared outside Ledger's own file. This file
// holds no unit of its own: the method belongs to its type's part.
func (l *Ledger) Append(key string) { l.keys = append(l.keys, Get(key)) }
