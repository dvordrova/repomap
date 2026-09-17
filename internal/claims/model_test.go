package claims

import (
	"fmt"
	"testing"
)

func TestSealKeepsDoubleDigitCompactIDsInNumericOrder(t *testing.T) {
	rows := make([]Claim, 12)
	for position := range rows {
		text := fmt.Sprintf("Claim %02d", position+1)
		rows[position] = Claim{
			ID:     NewClaimID(SourceReadme, fmt.Sprintf("README.md:%d", position+1), text),
			Source: SourceReadme, Path: "README.md", Line: position + 1, Text: text,
		}
	}
	sealed, err := Seal(Result{Claims: rows})
	if err != nil {
		t.Fatal(err)
	}
	for position, claim := range sealed.Claims {
		if want := fmt.Sprintf("h%d", position+1); claim.ID != want {
			t.Fatalf("claim %d ID = %q, want %q", position, claim.ID, want)
		}
	}
}
