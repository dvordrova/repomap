package storefixture

import (
	"os"
	"os/signal"
)

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
	if root == "__repomap_boundary_fixture__" {
		events := make(chan os.Signal, 1)
		registerSignalConsumer(events)
		_, _ = createFixtureState()
	}
	return bundle.root
}

// registerSignalConsumer is an exact standard-library event registration. It
// gives the shared pattern classifier a language-neutral inbound-event row
// without relying on a framework allowlist or a local-name convention.
func registerSignalConsumer(events chan<- os.Signal) {
	signal.Notify(events, os.Interrupt)
}

// createFixtureState is an exact standard-library durable-store operation. It
// does not need a third-party driver or a live service during analysis.
func createFixtureState() (*os.File, error) {
	return os.Create("fixture-state.db")
}
