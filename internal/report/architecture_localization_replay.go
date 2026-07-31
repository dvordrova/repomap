package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const ArchitectureLocalizationReplayVersion = 1

const maxArchitectureLocalizationReplayDepth = 64

// ArchitectureLocalizationReplay is non-consumable developer evidence that
// one supplied Russian projection was applied to a freshly verified current
// English Architecture Canvas. It is not a report or cache artifact.
type ArchitectureLocalizationReplay struct {
	Version         int                       `json:"version"`
	CanonicalSHA256 string                    `json:"canonical_sha256"`
	Locale          string                    `json:"locale"`
	Fallback        bool                      `json:"fallback"`
	Diagnostics     []localization.Diagnostic `json:"diagnostics,omitempty"`
	Canvas          ArchitectureCanvas        `json:"architecture_canvas"`
}

// ReplayArchitectureLocalizationRussianFile reads one explicitly supplied,
// bounded regular projection fixture and returns deterministic replay JSON.
// It does not write into the run directory or call a provider.
func ReplayArchitectureLocalizationRussianFile(
	runDir,
	projectionPath string,
) ([]byte, error) {
	projectionJSON, err := readArchitectureLocalizationProjectionFile(projectionPath)
	if err != nil {
		return nil, err
	}
	return ReplayArchitectureLocalizationRussian(runDir, projectionJSON)
}

// ReplayArchitectureLocalizationRussian applies one strict provider-free
// Russian projection to an English Architecture Canvas re-derived from the
// saved run's current facts and accepted synthesis.
func ReplayArchitectureLocalizationRussian(
	runDir string,
	projectionJSON []byte,
) ([]byte, error) {
	projection, err := decodeArchitectureLocalizationProjection(projectionJSON)
	if err != nil {
		return nil, err
	}

	context, err := prepareArchitectureLocalizationIdentity(runDir)
	if err != nil {
		return nil, err
	}
	canonical, input, err := buildArchitectureLocalization(
		context.canvas,
		localization.LocaleRussian,
	)
	if err != nil {
		return nil, err
	}
	if kind, found := architectureLocalizationCredential(
		canonical,
		input,
		projection,
	); found {
		return nil, fmt.Errorf(
			"architecture localization replay: decoded projection contains an obvious %s",
			kind,
		)
	}

	projected, result, err := applyArchitectureLocalization(
		context.canvas,
		canonical,
		input,
		projection,
	)
	if err != nil {
		return nil, fmt.Errorf("architecture localization replay: apply projection: %w", err)
	}
	replay := ArchitectureLocalizationReplay{
		Version:         ArchitectureLocalizationReplayVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          result.Locale,
		Fallback:        result.Fallback,
		Diagnostics:     append([]localization.Diagnostic(nil), result.Diagnostics...),
		Canvas:          projected,
	}
	if kind, found, err := preflightArchitectureLocalizationReplay(replay); err != nil {
		return nil, err
	} else if found {
		return nil, fmt.Errorf(
			"architecture localization replay: result contains an obvious %s",
			kind,
		)
	}
	encoded, err := json.Marshal(replay)
	if err != nil {
		return nil, fmt.Errorf("architecture localization replay: encode result: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > maxArchitectureLocalizationArtifactBytes {
		return nil, fmt.Errorf("architecture localization replay: result exceeds its byte limit")
	}
	if kind, found := secretscan.Detect(string(encoded)); found {
		return nil, fmt.Errorf(
			"architecture localization replay: result contains an obvious %s",
			kind,
		)
	}
	return encoded, nil
}

func readArchitectureLocalizationProjectionFile(path string) ([]byte, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("architecture localization replay: resolve projection: %w", err)
	}
	root, err := os.OpenRoot(filepath.Dir(absPath))
	if err != nil {
		return nil, fmt.Errorf("architecture localization replay: open projection root: %w", err)
	}
	defer root.Close()

	name := filepath.Base(absPath)
	info, err := root.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("architecture localization replay: inspect projection: %w", err)
	}
	if !info.Mode().IsRegular() ||
		info.Size() <= 0 ||
		info.Size() > maxArchitectureLocalizationArtifactBytes {
		return nil, fmt.Errorf(
			"architecture localization replay: projection is not a bounded regular file",
		)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("architecture localization replay: open projection: %w", err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("architecture localization replay: stat projection: %w", err)
	}
	if !openedInfo.Mode().IsRegular() ||
		openedInfo.Size() <= 0 ||
		openedInfo.Size() > maxArchitectureLocalizationArtifactBytes {
		return nil, fmt.Errorf(
			"architecture localization replay: projection is not a bounded regular file",
		)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxArchitectureLocalizationArtifactBytes+1))
	if err != nil {
		return nil, fmt.Errorf("architecture localization replay: read projection: %w", err)
	}
	if len(data) == 0 || len(data) > maxArchitectureLocalizationArtifactBytes {
		return nil, fmt.Errorf(
			"architecture localization replay: projection exceeds its byte limit",
		)
	}
	return data, nil
}

func decodeArchitectureLocalizationProjection(
	data []byte,
) (localization.Projection, error) {
	if len(data) == 0 || len(data) > maxArchitectureLocalizationArtifactBytes {
		return localization.Projection{}, fmt.Errorf(
			"architecture localization replay: projection exceeds its byte limit",
		)
	}
	if !utf8.Valid(data) {
		return localization.Projection{}, fmt.Errorf(
			"architecture localization replay: projection is not valid UTF-8",
		)
	}
	if kind, found := secretscan.Detect(string(data)); found {
		return localization.Projection{}, fmt.Errorf(
			"architecture localization replay: projection contains an obvious %s",
			kind,
		)
	}
	var projection localization.Projection
	if err := decodeArchitectureLocalizationJSON(data, &projection); err != nil {
		return localization.Projection{}, fmt.Errorf(
			"architecture localization replay: decode projection: %w",
			err,
		)
	}
	return projection, nil
}

type architectureLocalizationReplayPreflight struct {
	remaining  int
	secretKind string
}

func preflightArchitectureLocalizationReplay(
	replay ArchitectureLocalizationReplay,
) (string, bool, error) {
	state := architectureLocalizationReplayPreflight{
		remaining: maxArchitectureLocalizationArtifactBytes,
	}
	if err := state.walk(reflect.ValueOf(replay), 0); err != nil {
		return "", false, err
	}
	return state.secretKind, state.secretKind != "", nil
}

func (state *architectureLocalizationReplayPreflight) walk(
	value reflect.Value,
	depth int,
) error {
	if depth > maxArchitectureLocalizationReplayDepth {
		return fmt.Errorf(
			"architecture localization replay: result exceeds its structural depth limit",
		)
	}
	if !value.IsValid() {
		return state.consume(4)
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return state.consume(4)
		}
		return state.walk(value.Elem(), depth+1)
	case reflect.String:
		return state.consumeString(value.String(), true)
	case reflect.Bool:
		return state.consume(5)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64:
		return state.consume(32)
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.IsNil() {
			return state.consume(4)
		}
		if err := state.consume(2); err != nil {
			return err
		}
		for index := 0; index < value.Len(); index++ {
			if err := state.consume(1); err != nil {
				return err
			}
			if err := state.walk(value.Index(index), depth+1); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		if value.IsNil() {
			return state.consume(4)
		}
		if value.Type().Key().Kind() != reflect.String {
			return fmt.Errorf(
				"architecture localization replay: result has unsupported map key type",
			)
		}
		if err := state.consume(2); err != nil {
			return err
		}
		iter := value.MapRange()
		for iter.Next() {
			if err := state.consume(1); err != nil {
				return err
			}
			if err := state.consumeString(iter.Key().String(), true); err != nil {
				return err
			}
			if err := state.consume(1); err != nil {
				return err
			}
			if err := state.walk(iter.Value(), depth+1); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		if err := state.consume(2); err != nil {
			return err
		}
		valueType := value.Type()
		for index := 0; index < value.NumField(); index++ {
			field := valueType.Field(index)
			if field.PkgPath != "" {
				continue
			}
			name := field.Name
			tag := field.Tag.Get("json")
			if comma := strings.IndexByte(tag, ','); comma >= 0 {
				tag = tag[:comma]
			}
			if tag == "-" {
				continue
			}
			if tag != "" {
				name = tag
			}
			if err := state.consume(1); err != nil {
				return err
			}
			if err := state.consumeString(name, false); err != nil {
				return err
			}
			if err := state.consume(1); err != nil {
				return err
			}
			if err := state.walk(value.Field(index), depth+1); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf(
			"architecture localization replay: result has unsupported value type",
		)
	}
}

func (state *architectureLocalizationReplayPreflight) consumeString(
	value string,
	scan bool,
) error {
	if err := state.consume(2); err != nil {
		return err
	}
	for index := 0; index < len(value); {
		next := 1
		encodedBytes := 1
		if value[index] < utf8.RuneSelf {
			switch value[index] {
			case '\\', '"':
				encodedBytes = 2
			case '<', '>', '&':
				encodedBytes = 6
			default:
				if value[index] < 0x20 {
					encodedBytes = 6
				}
			}
		} else {
			runeValue, runeBytes := utf8.DecodeRuneInString(value[index:])
			next = runeBytes
			encodedBytes = runeBytes
			if runeValue == utf8.RuneError && runeBytes == 1 ||
				runeValue == '\u2028' ||
				runeValue == '\u2029' {
				encodedBytes = 6
			}
		}
		if err := state.consume(encodedBytes); err != nil {
			return err
		}
		index += next
	}
	if scan {
		if kind, found := secretscan.Detect(value); found &&
			(state.secretKind == "" || kind < state.secretKind) {
			state.secretKind = kind
		}
	}
	return nil
}

func (state *architectureLocalizationReplayPreflight) consume(count int) error {
	if count < 0 || count > state.remaining {
		return fmt.Errorf(
			"architecture localization replay: result exceeds its byte limit",
		)
	}
	state.remaining -= count
	return nil
}
