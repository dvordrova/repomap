package clientrecipe

import (
	"fmt"
	"path"
	"strings"
)

type Oracle struct {
	Version            int              `json:"version"`
	Target             OracleTarget     `json:"target"`
	Bootstrap          []SourceLocator  `json:"bootstrap"`
	Entrypoints        []SourceLocator  `json:"entrypoints"`
	Instances          []OracleInstance `json:"instances"`
	Excluded           []OracleExcluded `json:"excluded"`
	ExpectedRoles      []OracleRole     `json:"expected_roles"`
	AllowedBest        []string         `json:"allowed_best_instance_ids"`
	MandatoryTaskSlots []string         `json:"mandatory_task_slots"`
}

type OracleTarget struct {
	Module string `json:"module"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
}

type SourceLocator struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol"`
}

type OracleInstance struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Complete         bool         `json:"complete"`
	Family           string       `json:"family"`
	VerificationKind string       `json:"verification_kind"`
	Slots            []OracleSlot `json:"slots"`
	Missing          []string     `json:"missing"`
}

type OracleSlot struct {
	Role     string          `json:"role"`
	Evidence []SourceLocator `json:"evidence"`
}

type OracleExcluded struct {
	ID       string          `json:"id"`
	Reason   string          `json:"reason"`
	Evidence []SourceLocator `json:"evidence"`
}

type OracleRole struct {
	Role                      string `json:"role"`
	ObservedCompleteInstances int    `json:"observed_complete_instances"`
	Necessity                 string `json:"necessity"`
}

func DecodeOracle(raw []byte) (Oracle, error) {
	var value Oracle
	if err := decodeStrict(raw, &value, "oracle"); err != nil {
		return Oracle{}, err
	}
	if err := value.Validate(); err != nil {
		return Oracle{}, err
	}
	return value, nil
}

func (value Oracle) Validate() error {
	if value.Version != OracleVersion || !validText(value.Target.Module) ||
		value.Target.Kind != "service" || !validText(value.Target.Name) || len(value.Bootstrap) == 0 ||
		len(value.Entrypoints) == 0 || len(value.Instances) == 0 || value.Excluded == nil ||
		len(value.ExpectedRoles) == 0 || len(value.AllowedBest) == 0 || len(value.MandatoryTaskSlots) == 0 {
		return fmt.Errorf("client recipe oracle: invalid identity or coverage")
	}
	for _, locator := range append(append([]SourceLocator{}, value.Bootstrap...), value.Entrypoints...) {
		if err := locator.Validate(); err != nil {
			return err
		}
	}
	instances := make(map[string]OracleInstance, len(value.Instances))
	completeInstances := 0
	completeRoleCounts := make(map[string]int)
	previousInstance := ""
	for _, instance := range value.Instances {
		if !validID(instance.ID) || !validText(instance.Name) || instance.Family != "outbound_client" ||
			!validVerificationKind(instance.VerificationKind) ||
			instance.Slots == nil || instance.Missing == nil || (previousInstance != "" && previousInstance >= instance.ID) {
			return fmt.Errorf("client recipe oracle: invalid instance %q", instance.ID)
		}
		previousInstance = instance.ID
		if _, duplicate := instances[instance.ID]; duplicate {
			return fmt.Errorf("client recipe oracle: duplicate instance %q", instance.ID)
		}
		seenRoles := make(map[string]struct{}, len(instance.Slots))
		for _, slot := range instance.Slots {
			if !validRole(slot.Role) || len(slot.Evidence) == 0 {
				return fmt.Errorf("client recipe oracle: invalid slot %q", slot.Role)
			}
			if _, duplicate := seenRoles[slot.Role]; duplicate {
				return fmt.Errorf("client recipe oracle: duplicate slot %q", slot.Role)
			}
			seenRoles[slot.Role] = struct{}{}
			for _, locator := range slot.Evidence {
				if err := locator.Validate(); err != nil {
					return err
				}
			}
		}
		if !sortedUnique(instance.Missing) {
			return fmt.Errorf("client recipe oracle: missing roles are not canonical")
		}
		if instance.Complete && len(instance.Missing) != 0 {
			return fmt.Errorf("client recipe oracle: complete instance has missing roles")
		}
		if instance.Complete {
			completeInstances++
			if instance.VerificationKind == "none" {
				return fmt.Errorf("client recipe oracle: complete instance has no verification kind")
			}
			for role := range seenRoles {
				completeRoleCounts[role]++
			}
			for _, mandatory := range value.MandatoryTaskSlots {
				if _, found := seenRoles[mandatory]; !found {
					return fmt.Errorf("client recipe oracle: complete instance %q lacks mandatory role %q", instance.ID, mandatory)
				}
			}
		}
		instances[instance.ID] = instance
	}
	previousExcluded := ""
	for _, excluded := range value.Excluded {
		if !validID(excluded.ID) || !validExclusionReason(excluded.Reason) || len(excluded.Evidence) == 0 ||
			(previousExcluded != "" && previousExcluded >= excluded.ID) {
			return fmt.Errorf("client recipe oracle: invalid excluded candidate %q", excluded.ID)
		}
		previousExcluded = excluded.ID
		for _, locator := range excluded.Evidence {
			if err := locator.Validate(); err != nil {
				return err
			}
		}
	}
	seenExpected := make(map[string]struct{}, len(value.ExpectedRoles))
	for _, expected := range value.ExpectedRoles {
		if !validRole(expected.Role) || !validNecessity(expected.Necessity) ||
			expected.ObservedCompleteInstances < 0 || expected.ObservedCompleteInstances > completeInstances ||
			expected.ObservedCompleteInstances != completeRoleCounts[expected.Role] ||
			expected.Necessity != roleNecessity(expected.ObservedCompleteInstances, completeInstances) {
			return fmt.Errorf("client recipe oracle: invalid expected role")
		}
		if _, duplicate := seenExpected[expected.Role]; duplicate {
			return fmt.Errorf("client recipe oracle: duplicate expected role")
		}
		seenExpected[expected.Role] = struct{}{}
	}
	for _, role := range allRoles() {
		if _, found := seenExpected[role]; !found {
			return fmt.Errorf("client recipe oracle: missing expected role %q", role)
		}
	}
	if !sortedUnique(value.AllowedBest) || !sortedUnique(value.MandatoryTaskSlots) {
		return fmt.Errorf("client recipe oracle: closed lists are not canonical")
	}
	for _, instanceID := range value.AllowedBest {
		instance, ok := instances[instanceID]
		if !ok || !instance.Complete {
			return fmt.Errorf("client recipe oracle: best example %q is not complete", instanceID)
		}
	}
	for _, role := range value.MandatoryTaskSlots {
		if !validRole(role) {
			return fmt.Errorf("client recipe oracle: invalid mandatory task slot %q", role)
		}
	}
	return nil
}

func (value SourceLocator) Validate() error {
	if value.Path == "" || value.Path != path.Clean(value.Path) || path.IsAbs(value.Path) || value.Path == "." ||
		value.Path == ".." || strings.HasPrefix(value.Path, "../") || strings.Contains(value.Path, "\\") ||
		value.Line <= 0 || !validText(value.Symbol) {
		return fmt.Errorf("client recipe oracle: invalid source locator")
	}
	return nil
}

func validRole(role string) bool {
	switch role {
	case "configuration", "construction", "local_wrapper", "consumer_boundary", "application_wiring", "production_operation",
		"verification", "observability", "failure_policy":
		return true
	default:
		return false
	}
}

func allRoles() []string {
	return []string{
		"application_wiring", "configuration", "construction", "consumer_boundary", "failure_policy",
		"local_wrapper", "observability", "production_operation", "verification",
	}
}

func validVerificationKind(value string) bool {
	return value == "unit_test" || value == "integration_test" || value == "none"
}

func roleNecessity(observed, complete int) string {
	if complete > 0 && observed == complete {
		return "required"
	}
	if observed >= 2 && observed*3 >= complete*2 {
		return "common"
	}
	if observed >= 1 {
		return "optional"
	}
	return "unknown"
}

func validNecessity(value string) bool {
	return value == "required" || value == "common" || value == "optional" || value == "unknown"
}

func validExclusionReason(value string) bool {
	switch value {
	case "generated", "test_only", "not_production_reachable", "not_external_boundary":
		return true
	default:
		return false
	}
}
