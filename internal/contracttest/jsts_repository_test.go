package contracttest

import "testing"

// Materializing the fixture checks its exact tracked-file inventory.
func TestCumulativeJSTSRepositoryFileInventory(t *testing.T) {
	materializeFixtureRepository(t, "jsts")
}
