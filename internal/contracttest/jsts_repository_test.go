package contracttest

import "testing"

func TestCumulativeJSTSRepositoryFileInventory(t *testing.T) {
	_, repository := materializeFixtureRepository(t, "jsts")
	if len(repository.Entries()) != 3 {
		t.Fatalf("JSTS fixture tracked-file count = %d, want exact inventory of 3", len(repository.Entries()))
	}
}
