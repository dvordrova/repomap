// Package catalog owns declarative meanings for terminal semantic operations.
package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

const CurrentVersion = 1

type Origin string

const (
	OriginCatalogStatic         Origin = "catalog_static"
	OriginWrapperStatic         Origin = "wrapper_static"
	OriginUserDeclaredSemantics Origin = "user_declared_semantics"
)

type EffectKind string

const (
	EffectHTTPRouteRegistration EffectKind = "http_route_registration"
	EffectHTTPServerStart       EffectKind = "http_server_start"
	EffectHTTPHandlerAssignment EffectKind = "http_handler_assignment"
	EffectHTTPRouteProvider     EffectKind = "http_route_provider"
	EffectHTTPRouteAssembly     EffectKind = "http_route_assembly"
	EffectAsyncTaskStart        EffectKind = "async_task_start"
)

type Operation string

const (
	OperationCall       Operation = "call"
	OperationFieldStore Operation = "field_store"
)

type ProjectionSource string

const (
	ProjectionArgument      ProjectionSource = "argument"
	ProjectionReceiver      ProjectionSource = "receiver"
	ProjectionReceiverField ProjectionSource = "receiver_field"
	ProjectionReturnField   ProjectionSource = "return_field"
	ProjectionConstant      ProjectionSource = "constant"
)

type Catalog struct {
	Version int    `json:"version"`
	Seeds   []Seed `json:"seeds"`
}

type Seed struct {
	ID              string                `json:"id"`
	Operation       Operation             `json:"operation"`
	Symbol          Symbol                `json:"symbol"`
	Effect          Effect                `json:"effect"`
	Projections     map[string]Projection `json:"projections"`
	Scenario        Scenario              `json:"scenario"`
	Reference       string                `json:"reference"`
	FixtureCoverage []string              `json:"fixture_coverage"`
}

type Symbol struct {
	PackagePath string `json:"package"`
	Receiver    string `json:"receiver,omitempty"`
	Name        string `json:"name"`
	MinArgs     int    `json:"min_args,omitempty"`
	Variadic    *bool  `json:"variadic,omitempty"`
}

type Effect struct {
	Kind      EffectKind `json:"kind"`
	Transport string     `json:"transport"`
	Framework string     `json:"framework"`
}

type Projection struct {
	Source  ProjectionSource `json:"source"`
	Index   *int             `json:"index,omitempty"`
	Value   string           `json:"value,omitempty"`
	Default string           `json:"default,omitempty"`
	Field   string           `json:"field,omitempty"`
}

type Scenario struct {
	GOOS   []string `json:"goos,omitempty"`
	GOARCH []string `json:"goarch,omitempty"`
	Tags   []string `json:"tags,omitempty"`
}

//go:embed *.json
var files embed.FS

func Builtin() (Catalog, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return Catalog{}, fmt.Errorf("semantic catalog: list embedded files: %w", err)
	}
	combined := Catalog{Version: CurrentVersion, Seeds: []Seed{}}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := files.ReadFile(entry.Name())
		if err != nil {
			return Catalog{}, fmt.Errorf("semantic catalog: read %s: %w", entry.Name(), err)
		}
		part, err := Decode(strings.NewReader(string(data)))
		if err != nil {
			return Catalog{}, fmt.Errorf("semantic catalog: %s: %w", entry.Name(), err)
		}
		combined.Seeds = append(combined.Seeds, part.Seeds...)
	}
	sort.Slice(combined.Seeds, func(i, j int) bool {
		return combined.Seeds[i].ID < combined.Seeds[j].ID
	})
	if err := combined.Validate(); err != nil {
		return Catalog{}, err
	}
	return combined, nil
}

func Decode(input io.Reader) (Catalog, error) {
	decoder := json.NewDecoder(input)
	decoder.DisallowUnknownFields()
	var result Catalog
	if err := decoder.Decode(&result); err != nil {
		return Catalog{}, fmt.Errorf("decode: %w", err)
	}
	if err := result.Validate(); err != nil {
		return Catalog{}, err
	}
	return result, nil
}

func (c Catalog) Validate() error {
	if c.Version != CurrentVersion {
		return fmt.Errorf("semantic catalog: unsupported version %d; need %d", c.Version, CurrentVersion)
	}
	if len(c.Seeds) == 0 {
		return fmt.Errorf("semantic catalog: seeds must not be empty")
	}
	seen := make(map[string]struct{}, len(c.Seeds))
	for index, seed := range c.Seeds {
		if err := seed.validate(); err != nil {
			return fmt.Errorf("semantic catalog: seed %d: %w", index, err)
		}
		if _, exists := seen[seed.ID]; exists {
			return fmt.Errorf("semantic catalog: duplicate seed id %q", seed.ID)
		}
		seen[seed.ID] = struct{}{}
	}
	return nil
}

func (s Seed) validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if s.Operation != OperationCall && s.Operation != OperationFieldStore {
		return fmt.Errorf("%s: unsupported operation %q", s.ID, s.Operation)
	}
	if s.Symbol.PackagePath == "" || s.Symbol.Name == "" {
		return fmt.Errorf("%s: exact package and symbol name are required", s.ID)
	}
	if s.Effect.Kind == "" || s.Effect.Transport == "" || s.Effect.Framework == "" {
		return fmt.Errorf("%s: effect kind, transport, and framework are required", s.ID)
	}
	switch s.Effect.Kind {
	case EffectHTTPRouteRegistration,
		EffectHTTPServerStart,
		EffectHTTPHandlerAssignment,
		EffectHTTPRouteProvider,
		EffectHTTPRouteAssembly,
		EffectAsyncTaskStart:
	default:
		return fmt.Errorf("%s: unsupported effect kind %q", s.ID, s.Effect.Kind)
	}
	if len(s.Projections) == 0 {
		return fmt.Errorf("%s: projections must not be empty", s.ID)
	}
	for name, projection := range s.Projections {
		switch projection.Source {
		case ProjectionArgument:
			if projection.Index == nil || *projection.Index < 0 {
				return fmt.Errorf("%s: projection %q needs a non-negative argument index", s.ID, name)
			}
		case ProjectionReceiver:
			if s.Symbol.Receiver == "" {
				return fmt.Errorf("%s: projection %q uses a receiver for a package function", s.ID, name)
			}
		case ProjectionReceiverField:
			if s.Symbol.Receiver == "" || strings.TrimSpace(projection.Field) == "" {
				return fmt.Errorf("%s: projection %q needs a receiver and field", s.ID, name)
			}
			if s.Operation != OperationCall || s.Effect.Kind != EffectHTTPRouteRegistration {
				return fmt.Errorf("%s: projection %q uses receiver_field outside route registration call semantics", s.ID, name)
			}
		case ProjectionReturnField:
			if strings.TrimSpace(projection.Field) == "" {
				return fmt.Errorf("%s: projection %q needs a return field", s.ID, name)
			}
			if s.Operation != OperationCall || s.Effect.Kind != EffectHTTPRouteProvider {
				return fmt.Errorf("%s: projection %q uses return_field outside route provider call semantics", s.ID, name)
			}
		case ProjectionConstant:
			if projection.Value == "" {
				return fmt.Errorf("%s: projection %q needs a constant value", s.ID, name)
			}
		default:
			return fmt.Errorf("%s: projection %q has unsupported source %q", s.ID, name, projection.Source)
		}
	}
	if s.Effect.Kind == EffectHTTPRouteProvider {
		for _, name := range []string{"path", "handler"} {
			projection, ok := s.Projections[name]
			if !ok || projection.Source != ProjectionReturnField || strings.TrimSpace(projection.Field) == "" {
				return fmt.Errorf("%s: route provider requires %q return_field projection", s.ID, name)
			}
		}
	}
	return nil
}
