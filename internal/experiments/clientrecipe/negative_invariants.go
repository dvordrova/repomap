package clientrecipe

import "fmt"

type NegativeInvariants struct {
	Version    int                 `json:"version"`
	Invariants []NegativeInvariant `json:"invariants"`
}

type NegativeInvariant struct {
	ID         string `json:"id"`
	Scope      string `json:"scope"`
	Statement  string `json:"statement"`
	Prohibited string `json:"prohibited"`
	Test       string `json:"test"`
}

func DecodeNegativeInvariants(raw []byte) (NegativeInvariants, error) {
	var value NegativeInvariants
	if err := decodeStrict(raw, &value, "negative invariants"); err != nil {
		return NegativeInvariants{}, err
	}
	if err := value.Validate(); err != nil {
		return NegativeInvariants{}, err
	}
	return value, nil
}

func (value NegativeInvariants) Validate() error {
	if value.Version != MatrixVersion || len(value.Invariants) == 0 {
		return fmt.Errorf("client recipe negative invariants: invalid identity")
	}
	previous := ""
	for _, invariant := range value.Invariants {
		if !validID(invariant.ID) || !validInvariantScope(invariant.Scope) || !validText(invariant.Statement) ||
			!validID(invariant.Prohibited) || !validID(invariant.Test) || (previous != "" && previous >= invariant.ID) {
			return fmt.Errorf("client recipe negative invariants: invalid or non-canonical invariant %q", invariant.ID)
		}
		previous = invariant.ID
	}
	return nil
}

func validInvariantScope(value string) bool {
	return value == "analysis" || value == "synthesis" || value == "ui" || value == "transport"
}
