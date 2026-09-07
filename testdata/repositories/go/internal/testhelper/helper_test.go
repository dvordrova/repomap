package testhelper

import "testing"

func TestPrivateHelper(t *testing.T) {
	if preparedValue() != "ready" {
		t.Fatal("helper did not prepare a value")
	}
}
