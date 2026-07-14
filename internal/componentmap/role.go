// Package componentmap contains the presentation-neutral vocabulary used by
// the repository component landscape.
package componentmap

import (
	"encoding/json"
	"strings"
)

// Role is a coarse orientation hint, not a fact derived from static imports.
// It exists to arrange components into a readable landscape while keeping the
// evidence-backed relations between them separate.
type Role string

const (
	RoleUnknown      Role = "unknown"
	RoleEntry        Role = "entry"
	RoleBoundary     Role = "boundary"
	RoleCoordination Role = "coordination"
	RoleDomain       Role = "domain"
	RoleState        Role = "state"
	RoleSupport      Role = "support"
)

// UnmarshalJSON keeps a weak provider's malformed optional role from making an
// otherwise usable orientation report unreadable. Normalize will turn the
// sentinel into unknown and let the caller record a warning.
func (r *Role) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		*r = Role("invalid_json_type")
		return nil
	}
	*r = Role(value)
	return nil
}

// Normalize accepts the bounded role vocabulary and maps missing values from
// older artifacts to unknown. The boolean is false only for a non-empty value
// outside the vocabulary.
func Normalize(value string) (Role, bool) {
	role := Role(strings.ToLower(strings.TrimSpace(value)))
	if role == "" {
		return RoleUnknown, true
	}
	switch role {
	case RoleUnknown, RoleEntry, RoleBoundary, RoleCoordination, RoleDomain, RoleState, RoleSupport:
		return role, true
	default:
		return RoleUnknown, false
	}
}
