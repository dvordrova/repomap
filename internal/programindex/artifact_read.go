package programindex

import "os"

// ReadFile reads one persisted sealed Index; reading never invokes New.
func ReadFile(filename string) (Index, error) {
	wire, err := os.ReadFile(filename)
	if err != nil {
		return Index{}, err
	}
	return Decode(wire)
}
