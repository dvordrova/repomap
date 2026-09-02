// Package clientrecipe owns the test-only client-recipe experiment.
//
// Nothing in this package is imported by the ordinary repomap product path.
package clientrecipe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	TaskContractVersion = 1
	OracleVersion       = 1
	MatrixVersion       = 1
	maxContractBytes    = 1 << 20
	maxIDBytes          = 120
	maxTextBytes        = 2048
)

type TaskContract struct {
	Version            int                   `json:"version"`
	Feature            string                `json:"feature"`
	Lens               string                `json:"lens"`
	UserSituation      string                `json:"user_situation"`
	Intent             string                `json:"intent"`
	UserNeeds          []UserNeed            `json:"user_needs"`
	CompletionCriteria []CompletionCriterion `json:"completion_criteria"`
	UIBounds           UIBounds              `json:"ui_bounds"`
	StateMachine       []TaskState           `json:"state_machine"`
}

type UserNeed struct {
	ID       string `json:"id"`
	Question string `json:"question"`
}

type CompletionCriterion struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type UIBounds struct {
	LandingChoices       int  `json:"landing_choices"`
	PrimaryRecipeSteps   int  `json:"primary_recipe_steps"`
	VisibleExamples      int  `json:"visible_examples"`
	VisibleEvidence      int  `json:"visible_evidence_per_step"`
	ExactSourceActions   int  `json:"exact_source_actions"`
	AuditInFirstViewport bool `json:"audit_in_first_viewport"`
}

type TaskState struct {
	State            string   `json:"state"`
	QuestionAnswered string   `json:"question_answered"`
	Objects          []string `json:"objects"`
	PrimaryAction    string   `json:"primary_action"`
	SecondaryActions []string `json:"secondary_actions"`
	Hidden           []string `json:"hidden"`
	Disclosure       string   `json:"disclosure"`
	ReturnState      string   `json:"return_state"`
}

type Traceability struct {
	Version int               `json:"version"`
	Rows    []TraceabilityRow `json:"rows"`
}

type TraceabilityRow struct {
	UserNeed       string `json:"user_need"`
	UIElement      string `json:"ui_element"`
	ResultField    string `json:"result_field"`
	RequiredSignal string `json:"required_signal"`
	SignalSource   string `json:"signal_source"`
	Authority      string `json:"authority"`
	Test           string `json:"test"`
}

type SignalClass string

const (
	SignalExact          SignalClass = "exact"
	SignalDerived        SignalClass = "derived"
	SignalSmallExtractor SignalClass = "small_extractor"
	SignalLLM            SignalClass = "llm"
	SignalUnavailable    SignalClass = "not_available"
)

type SignalMatrix struct {
	Version int               `json:"version"`
	Rows    []SignalMatrixRow `json:"rows"`
}

type SignalMatrixRow struct {
	Signal                        string      `json:"signal"`
	Classification                SignalClass `json:"classification"`
	NeededFor                     []string    `json:"needed_for"`
	AvailableInExistingAuthority  bool        `json:"available_in_existing_authority"`
	ExistingOwner                 string      `json:"existing_owner"`
	MissingDetail                 string      `json:"missing_detail"`
	ExperimentSource              string      `json:"experiment_source"`
	PossibleFutureProductionOwner string      `json:"possible_future_production_owner"`
}

func DecodeTaskContract(raw []byte) (TaskContract, error) {
	var value TaskContract
	if err := decodeStrict(raw, &value, "task contract"); err != nil {
		return TaskContract{}, err
	}
	if err := value.Validate(); err != nil {
		return TaskContract{}, err
	}
	return value, nil
}

func DecodeTraceability(raw []byte) (Traceability, error) {
	var value Traceability
	if err := decodeStrict(raw, &value, "traceability"); err != nil {
		return Traceability{}, err
	}
	if err := value.Validate(); err != nil {
		return Traceability{}, err
	}
	return value, nil
}

func DecodeSignalMatrix(raw []byte) (SignalMatrix, error) {
	var value SignalMatrix
	if err := decodeStrict(raw, &value, "signal matrix"); err != nil {
		return SignalMatrix{}, err
	}
	if err := value.Validate(); err != nil {
		return SignalMatrix{}, err
	}
	return value, nil
}

func (value TaskContract) Validate() error {
	if value.Version != TaskContractVersion || value.Feature != "add_external_client" ||
		value.Lens != "service_outbound_client_recipe" || !validText(value.UserSituation) ||
		!validText(value.Intent) || len(value.UserNeeds) == 0 || len(value.CompletionCriteria) == 0 {
		return fmt.Errorf("client recipe task contract: invalid identity or task definition")
	}
	if value.UIBounds != (UIBounds{
		LandingChoices: 1, PrimaryRecipeSteps: 6, VisibleExamples: 3,
		VisibleEvidence: 3, ExactSourceActions: 2, AuditInFirstViewport: false,
	}) {
		return fmt.Errorf("client recipe task contract: UI bounds do not match the experiment contract")
	}
	needIDs := make(map[string]struct{}, len(value.UserNeeds))
	for _, need := range value.UserNeeds {
		if !validID(need.ID) || !validText(need.Question) {
			return fmt.Errorf("client recipe task contract: invalid user need")
		}
		if _, duplicate := needIDs[need.ID]; duplicate {
			return fmt.Errorf("client recipe task contract: duplicate user need %q", need.ID)
		}
		needIDs[need.ID] = struct{}{}
	}
	criterionIDs := make(map[string]struct{}, len(value.CompletionCriteria))
	for _, criterion := range value.CompletionCriteria {
		if !validID(criterion.ID) || !validText(criterion.Description) {
			return fmt.Errorf("client recipe task contract: invalid completion criterion")
		}
		if _, duplicate := criterionIDs[criterion.ID]; duplicate {
			return fmt.Errorf("client recipe task contract: duplicate completion criterion %q", criterion.ID)
		}
		criterionIDs[criterion.ID] = struct{}{}
	}
	wantStates := []string{"target_landing", "recipe_overview", "recipe_step", "example_instance", "evidence"}
	if len(value.StateMachine) != len(wantStates) {
		return fmt.Errorf("client recipe task contract: state machine is incomplete")
	}
	for index, state := range value.StateMachine {
		if state.State != wantStates[index] || !validText(state.QuestionAnswered) || state.Objects == nil ||
			!validID(state.PrimaryAction) || state.SecondaryActions == nil || state.Hidden == nil ||
			!validID(state.Disclosure) || !validID(state.ReturnState) {
			return fmt.Errorf("client recipe task contract: invalid state %q", state.State)
		}
	}
	return nil
}

func (value Traceability) Validate() error {
	if value.Version != MatrixVersion || len(value.Rows) == 0 {
		return fmt.Errorf("client recipe traceability: invalid identity")
	}
	seen := make(map[string]struct{}, len(value.Rows))
	previous := ""
	for _, row := range value.Rows {
		if !validID(row.UserNeed) || !validID(row.UIElement) || !validID(row.ResultField) ||
			!validID(row.RequiredSignal) || !validID(row.SignalSource) || !validID(row.Authority) || !validID(row.Test) {
			return fmt.Errorf("client recipe traceability: invalid row")
		}
		key := row.UserNeed + "\x00" + row.UIElement + "\x00" + row.ResultField
		if _, duplicate := seen[key]; duplicate || (previous != "" && previous >= key) {
			return fmt.Errorf("client recipe traceability: rows are not canonical")
		}
		seen[key] = struct{}{}
		previous = key
	}
	return nil
}

func (value SignalMatrix) Validate() error {
	if value.Version != MatrixVersion || len(value.Rows) == 0 {
		return fmt.Errorf("client recipe signal matrix: invalid identity")
	}
	seen := make(map[string]struct{}, len(value.Rows))
	previous := ""
	for _, row := range value.Rows {
		if !validID(row.Signal) || !row.Classification.Valid() || len(row.NeededFor) == 0 ||
			!sortedUnique(row.NeededFor) || !validID(row.ExistingOwner) || !validText(row.MissingDetail) ||
			!validID(row.ExperimentSource) || !validID(row.PossibleFutureProductionOwner) {
			return fmt.Errorf("client recipe signal matrix: invalid row %q", row.Signal)
		}
		if _, duplicate := seen[row.Signal]; duplicate || (previous != "" && previous >= row.Signal) {
			return fmt.Errorf("client recipe signal matrix: rows are not canonical at %q after %q", row.Signal, previous)
		}
		seen[row.Signal] = struct{}{}
		previous = row.Signal
	}
	return nil
}

func (class SignalClass) Valid() bool {
	switch class {
	case SignalExact, SignalDerived, SignalSmallExtractor, SignalLLM, SignalUnavailable:
		return true
	default:
		return false
	}
}

func decodeStrict(raw []byte, destination any, label string) error {
	if len(raw) == 0 || len(raw) > maxContractBytes {
		return fmt.Errorf("client recipe %s: invalid byte size", label)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("client recipe %s: decode: %w", label, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("client recipe %s: trailing JSON value", label)
		}
		return fmt.Errorf("client recipe %s: trailing data: %w", label, err)
	}
	return nil
}

func validID(value string) bool {
	if value == "" || len(value) > maxIDBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func validText(value string) bool {
	return value != "" && len(value) <= maxTextBytes && utf8.ValidString(value) && strings.TrimSpace(value) == value
}

func sortedUnique(values []string) bool {
	return sort.StringsAreSorted(values) && !hasDuplicate(values)
}

func hasDuplicate(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index-1] == values[index] {
			return true
		}
	}
	return false
}
