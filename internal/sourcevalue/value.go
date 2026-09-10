// Package sourcevalue records source expressions without evaluating repository
// code. It is shared syntax evidence, not a runtime value or a semantic graph.
package sourcevalue

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

type Anchor struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

// Parameter positions exclude the receiver and start at one. Owner identifies
// the declaring callable, including an outer callable captured by a closure.
// CallResult anchors the observed call; it does not assert what that call returns.
type Value struct {
	// Initializer is an observed field assignment when the receiver instance
	// cannot be followed. It remains a possible source, not a runtime value.
	Initializer *Value  `json:"initializer,omitempty"`
	Kind        string  `json:"kind"`
	Text        string  `json:"text,omitempty"`
	Position    int     `json:"position,omitempty"`
	Anchor      *Anchor `json:"anchor,omitempty"`
	Owner       *Anchor `json:"owner,omitempty"`
	Parts       []Value `json:"parts,omitempty"`
}

func Validate(value *Value) error {
	if value == nil {
		return nil
	}
	if !utf8.ValidString(value.Text) {
		return fmt.Errorf("source value: invalid text")
	}
	if value.Initializer != nil {
		if value.Kind != "field" {
			return fmt.Errorf("source value: initializer outside field")
		}
		if err := Validate(value.Initializer); err != nil {
			return err
		}
	}
	for _, anchor := range []*Anchor{value.Anchor, value.Owner} {
		if anchor != nil && (anchor.Path == "" || path.IsAbs(anchor.Path) || path.Clean(anchor.Path) != anchor.Path || strings.HasPrefix(anchor.Path, "../") || anchor.Line < 1 || anchor.Column < 0) {
			return fmt.Errorf("source value: invalid source anchor")
		}
	}
	switch value.Kind {
	case "literal", "unknown":
		if len(value.Parts) != 0 {
			return fmt.Errorf("source value: leaf has parts")
		}
	case "parameter":
		if value.Position < 1 || value.Owner == nil || len(value.Parts) != 0 {
			return fmt.Errorf("source value: invalid parameter")
		}
	case "receiver":
		if value.Owner == nil || value.Position != 0 || len(value.Parts) != 0 {
			return fmt.Errorf("source value: invalid receiver")
		}
	case "record":
		for _, part := range value.Parts {
			if part.Kind != "field_value" {
				return fmt.Errorf("source value: invalid record field")
			}
		}
	case "field_value":
		if value.Text == "" || len(value.Parts) != 1 {
			return fmt.Errorf("source value: invalid field value")
		}
	case "call_result":
		if value.Anchor == nil || len(value.Parts) != 0 {
			return fmt.Errorf("source value: invalid call result")
		}
	case "concat", "alternatives":
		if len(value.Parts) < 2 {
			return fmt.Errorf("source value: incomplete composite")
		}
	case "field":
		if value.Text == "" || len(value.Parts) != 1 {
			return fmt.Errorf("source value: invalid field")
		}
	case "index":
		if len(value.Parts) != 2 {
			return fmt.Errorf("source value: invalid index")
		}
	default:
		return fmt.Errorf("source value: unsupported kind %q", value.Kind)
	}
	for i := range value.Parts {
		if err := Validate(&value.Parts[i]); err != nil {
			return err
		}
	}
	return nil
}

func Clone(value *Value) *Value {
	if value == nil {
		return nil
	}
	result := *value
	result.Initializer = Clone(value.Initializer)
	if value.Anchor != nil {
		anchor := *value.Anchor
		result.Anchor = &anchor
	}
	if value.Owner != nil {
		owner := *value.Owner
		result.Owner = &owner
	}
	if value.Parts != nil {
		result.Parts = make([]Value, len(value.Parts))
		for i := range value.Parts {
			result.Parts[i] = *Clone(&value.Parts[i])
		}
	}
	return &result
}
