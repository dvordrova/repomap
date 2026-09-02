package contracttest

import "testing"

func TestCumulativeJSTSRepositoryFileInventory(t *testing.T) {
	_, repository := materializeFixtureRepository(t, "jsts")
	if len(repository.Entries()) != 6 {
		t.Fatalf("JSTS fixture tracked-file count = %d, want exact inventory of 6", len(repository.Entries()))
	}
}
