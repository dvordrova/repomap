package orientation

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// These catalogues belong to one actual prepared leaf. Sharing an identical
// view changes only its representation, never a record's uses or provenance.
type contextViewCatalogue struct {
	Layer string            `json:"layer"`
	Views []json.RawMessage `json:"views"`
}

type contextReadingWire struct {
	Scope       orientationScope      `json:"scope"`
	Task        string                `json:"task"`
	Target      targetWire            `json:"target"`
	Records     []orientationRecord   `json:"records"`
	Groups      *contextViewCatalogue `json:"group_views,omitempty"`
	Targets     *contextViewCatalogue `json:"target_views,omitempty"`
	SeedHeaders *contextViewCatalogue `json:"seed_headers,omitempty"`
}

type contextViewBuilder struct {
	catalogue contextViewCatalogue
	byRef     map[string]json.RawMessage
}

type contextViewField struct {
	name    string
	builder *contextViewBuilder
}

func (builder *contextViewBuilder) add(kind string, value json.RawMessage) (string, error) {
	var identity struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal(value, &identity); err != nil || identity.Ref == "" {
		return "", fmt.Errorf("orientation context: %s view has no closed ref", kind)
	}
	if previous, found := builder.byRef[identity.Ref]; found {
		if !bytes.Equal(previous, value) {
			return "", fmt.Errorf("orientation context: conflicting %s view %q", kind, identity.Ref)
		}
		return identity.Ref, nil
	}
	if builder.byRef == nil {
		builder.byRef = make(map[string]json.RawMessage)
	}
	builder.byRef[identity.Ref] = value
	builder.catalogue.Views = append(builder.catalogue.Views, value)
	return identity.Ref, nil
}

func (builder *contextViewBuilder) wire() *contextViewCatalogue {
	if len(builder.catalogue.Views) == 0 {
		return nil
	}
	return &builder.catalogue
}

// encodeContextReading runs after lossless adaptive partitioning. Every leaf
// carries all definitions its records use, including endpoints outside that
// leaf and declaration headers whose seed row occurred in another leaf.
func encodeContextReading(item orientationReading) ([]byte, error) {
	if len(item.Records) == 0 {
		return nil, fmt.Errorf("orientation context: empty input")
	}
	groups := contextViewBuilder{catalogue: contextViewCatalogue{Layer: "model_interpretation"}}
	targets := contextViewBuilder{catalogue: contextViewCatalogue{Layer: "native"}}
	seeds := contextViewBuilder{catalogue: contextViewCatalogue{Layer: "native"}}
	wire := contextReadingWire{
		Scope: orientationScope{"w1", "all_supplied_records", item.Records[0].Ref, item.Records[len(item.Records)-1].Ref},
		Task:  item.Task, Target: item.Target,
		Records: make([]orientationRecord, 0, len(item.Records)),
	}
	for _, record := range item.Records {
		var fields []contextViewField
		switch record.Kind {
		case "connection":
			fields = append(fields,
				contextViewField{"from_group", &groups},
				contextViewField{"to_group", &groups},
				contextViewField{"from_target", &targets},
				contextViewField{"to_target", &targets},
			)
		case "seed_call", "seed_evidence":
			fields = append(fields, contextViewField{"seed", &seeds})
		}
		if len(fields) > 0 {
			encoded, err := encodeWire(record.Value)
			if err != nil {
				return nil, err
			}
			var value map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &value); err != nil || value == nil {
				return nil, fmt.Errorf("orientation context: %s record %q has no object value", record.Kind, record.Ref)
			}
			for _, field := range fields {
				ref, err := field.builder.add(field.name, value[field.name])
				if err != nil {
					return nil, err
				}
				refField := field.name + "_ref"
				if _, exists := value[refField]; exists {
					return nil, fmt.Errorf("orientation context: %s record %q already has %s", record.Kind, record.Ref, refField)
				}
				value[refField], _ = json.Marshal(ref)
				delete(value, field.name)
			}
			record.Value = value
		}
		wire.Records = append(wire.Records, record)
	}
	wire.Groups, wire.Targets, wire.SeedHeaders = groups.wire(), targets.wire(), seeds.wire()
	return encodeWire(wire)
}
