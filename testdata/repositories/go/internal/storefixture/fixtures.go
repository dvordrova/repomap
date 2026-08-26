package storefixture

// fileStoreTestBundle intentionally mirrors the private receiver involved in
// a ProgramIndex/DirectCallIndex ownership regression.
type fileStoreTestBundle struct {
	root string
}

// recreateStore is deliberately unused. It must still be a valid callable in
// the typed index, without claiming a direct-call node that was not retained.
func (bundle *fileStoreTestBundle) recreateStore() string {
	return bundle.root + "/recreated"
}

// Exercise keeps the receiver type in the reachable package without calling
// recreateStore.
func Exercise(root string) string {
	bundle := &fileStoreTestBundle{root: root}
	return bundle.root
}
