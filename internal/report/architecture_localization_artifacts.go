package report

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	architectureLocalizationArtifactDir = "localization"

	architectureLocalizationCanonicalFile  = "architecture.canonical.v1.json"
	architectureLocalizationInputFile      = "architecture.en.input.v1.json"
	architectureLocalizationProjectionFile = "architecture.en.projection.v1.json"

	maxArchitectureLocalizationStatusBytes    = 64 << 10
	maxArchitectureLocalizationSynthesisBytes = 512 << 10
	maxArchitectureLocalizationArtifactBytes  = 1 << 20
)

type architectureLocalizationArtifactPayload struct {
	name string
	data []byte
}

type architectureLocalizationIdentityContext struct {
	runDir   string
	canvas   ArchitectureCanvas
	payloads []architectureLocalizationArtifactPayload
}

// MaterializeArchitectureLocalizationIdentity is an explicit provider-free
// developer check. It writes three non-consumable Architecture identity
// sidecars only when the current saved run proves an explicit English,
// accepted model synthesis against the current locally rebuilt facts.
func MaterializeArchitectureLocalizationIdentity(runDir string) (string, error) {
	context, err := prepareArchitectureLocalizationIdentity(runDir)
	if err != nil {
		return "", err
	}
	if err := writeArchitectureLocalizationIdentityPayloads(
		context.runDir,
		context.payloads,
	); err != nil {
		return "", err
	}
	return filepath.Join(context.runDir, architectureLocalizationArtifactDir), nil
}

func prepareArchitectureLocalizationIdentity(
	runDir string,
) (architectureLocalizationIdentityContext, error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return architectureLocalizationIdentityContext{}, fmt.Errorf(
			"architecture localization artifacts: resolve run dir: %w",
			err,
		)
	}
	info, err := os.Stat(absDir)
	if err != nil {
		return architectureLocalizationIdentityContext{}, fmt.Errorf(
			"architecture localization artifacts: inspect run dir: %w",
			err,
		)
	}
	if !info.IsDir() {
		return architectureLocalizationIdentityContext{}, fmt.Errorf(
			"architecture localization artifacts: run path is not a directory",
		)
	}

	root, err := os.OpenRoot(absDir)
	if err != nil {
		return architectureLocalizationIdentityContext{}, fmt.Errorf(
			"architecture localization artifacts: open run root: %w",
			err,
		)
	}
	defer root.Close()
	statusJSON, err := readArchitectureLocalizationArtifactInput(
		root,
		ArchitectureSynthesisStatusFile,
		maxArchitectureLocalizationStatusBytes,
	)
	if err != nil {
		return architectureLocalizationIdentityContext{}, err
	}
	synthesisJSON, err := readArchitectureLocalizationArtifactInput(
		root,
		ArchitectureSynthesisFile,
		maxArchitectureLocalizationSynthesisBytes,
	)
	if err != nil {
		return architectureLocalizationIdentityContext{}, err
	}
	var status ArchitectureSynthesisStatus
	if err := decodeArchitectureLocalizationJSON(statusJSON, &status); err != nil {
		return architectureLocalizationIdentityContext{}, fmt.Errorf(
			"architecture localization artifacts: decode synthesis status: %w",
			err,
		)
	}
	if err := status.Validate(); err != nil {
		return architectureLocalizationIdentityContext{}, fmt.Errorf(
			"architecture localization artifacts: validate synthesis status: %w",
			err,
		)
	}
	data, err := readRunDir(absDir, "", nil, &savedArchitectureArtifacts{
		status:    status,
		synthesis: append([]byte(nil), synthesisJSON...),
	})
	if err != nil {
		return architectureLocalizationIdentityContext{}, fmt.Errorf(
			"architecture localization artifacts: read run: %w",
			err,
		)
	}
	payloads, err := buildArchitectureLocalizationIdentityPayloads(
		data,
		statusJSON,
		synthesisJSON,
	)
	if err != nil {
		return architectureLocalizationIdentityContext{}, err
	}
	if data.ArchitectureCanvas == nil {
		return architectureLocalizationIdentityContext{}, fmt.Errorf(
			"architecture localization artifacts: current run has no accepted Architecture Canvas",
		)
	}
	return architectureLocalizationIdentityContext{
		runDir:   absDir,
		canvas:   *data.ArchitectureCanvas,
		payloads: payloads,
	}, nil
}

func readArchitectureLocalizationArtifactInput(
	root *os.Root,
	name string,
	limit int64,
) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: inspect %s: %w",
			name,
			err,
		)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return nil, fmt.Errorf(
			"architecture localization artifacts: %s is not a bounded regular file",
			name,
		)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: open %s: %w",
			name,
			err,
		)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: read %s: %w",
			name,
			err,
		)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf(
			"architecture localization artifacts: %s exceeds its byte limit",
			name,
		)
	}
	return data, nil
}

func buildArchitectureLocalizationIdentityPayloads(
	data *ReportData,
	statusJSON,
	synthesisJSON []byte,
) ([]architectureLocalizationArtifactPayload, error) {
	if data == nil || data.ArchitectureCanvas == nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: current run has no accepted Architecture Canvas",
		)
	}
	canvas := *data.ArchitectureCanvas
	if canvas.Fallback || canvas.FallbackReason != "" {
		return nil, fmt.Errorf(
			"architecture localization artifacts: fallback Architecture Canvas is ineligible",
		)
	}

	var status ArchitectureSynthesisStatus
	if err := decodeArchitectureLocalizationJSON(statusJSON, &status); err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: decode synthesis status: %w",
			err,
		)
	}
	if err := status.Validate(); err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: validate synthesis status: %w",
			err,
		)
	}
	if status.Version != ArchitectureSynthesisStatusVersion ||
		(status.State != ArchitectureSynthesisSucceeded &&
			status.State != ArchitectureSynthesisCached) ||
		!status.ProviderCallSucceeded ||
		!status.ResponseParsed ||
		!status.ProposalAccepted ||
		status.ProposalRejected ||
		status.FallbackSelected {
		return nil, fmt.Errorf(
			"architecture localization artifacts: synthesis status is not an accepted current v2 result",
		)
	}
	if data.ArchitectureSynthesis == nil ||
		!reflect.DeepEqual(*data.ArchitectureSynthesis, status) {
		return nil, fmt.Errorf(
			"architecture localization artifacts: synthesis status does not match the current report",
		)
	}

	var record componentmap.SynthesisRecord
	if err := decodeArchitectureLocalizationJSON(synthesisJSON, &record); err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: decode synthesis record: %w",
			err,
		)
	}
	if record.Version != componentmap.SynthesisRecordVersion ||
		record.Call == nil ||
		record.Call.Metadata.OutputLanguage != localization.LocaleEnglish {
		return nil, fmt.Errorf(
			"architecture localization artifacts: synthesis record is not explicit current English",
		)
	}
	metadata := record.Call.Metadata
	if metadata.FallbackReason != "" {
		return nil, fmt.Errorf(
			"architecture localization artifacts: synthesis record selected a fallback",
		)
	}
	switch metadata.ValidationOutcome {
	case componentmap.ValidationAccepted:
		if metadata.ArchitectureSource != componentmap.SourceValidatedModel ||
			len(metadata.Normalizations) != 0 ||
			status.ProposalNormalized {
			return nil, fmt.Errorf(
				"architecture localization artifacts: accepted synthesis metadata is inconsistent",
			)
		}
	case componentmap.ValidationAcceptedNormalized:
		if metadata.ArchitectureSource != componentmap.SourceNormalizedModel ||
			len(metadata.Normalizations) == 0 ||
			!status.ProposalNormalized {
			return nil, fmt.Errorf(
				"architecture localization artifacts: normalized synthesis metadata is inconsistent",
			)
		}
	default:
		return nil, fmt.Errorf(
			"architecture localization artifacts: rejected synthesis is ineligible",
		)
	}
	if status.ArchitectureSource != string(metadata.ArchitectureSource) ||
		status.ArchitectureLevel != metadata.ArchitectureLevel ||
		status.NormalizationCount != len(metadata.Normalizations) ||
		canvas.ValidationOutcome != metadata.ValidationOutcome ||
		canvas.ArchitectureSource != metadata.ArchitectureSource ||
		canvas.ArchitectureLevel != metadata.ArchitectureLevel ||
		!reflect.DeepEqual(canvas.Normalizations, metadata.Normalizations) {
		return nil, fmt.Errorf(
			"architecture localization artifacts: synthesis outcome metadata does not match the current canvas",
		)
	}

	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: rebuild current canvas input: %w",
			err,
		)
	}
	replayed, err := ReplayArchitectureSynthesis(input, synthesisJSON)
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: replay current synthesis: %w",
			err,
		)
	}
	replayedCanvas, err := ProjectArchitectureCanvas(replayed)
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: project replayed canvas: %w",
			err,
		)
	}
	canvasJSON, err := json.Marshal(canvas)
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: encode current canvas: %w",
			err,
		)
	}
	replayedCanvasJSON, err := json.Marshal(replayedCanvas)
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: encode replayed canvas: %w",
			err,
		)
	}
	if !bytes.Equal(canvasJSON, replayedCanvasJSON) {
		return nil, fmt.Errorf(
			"architecture localization artifacts: saved synthesis does not reproduce the current canvas",
		)
	}

	canonical, localizationInput, err := buildArchitectureLocalization(
		canvas,
		localization.LocaleEnglish,
	)
	if err != nil {
		return nil, err
	}
	projection, err := localization.IdentityProjection(canonical)
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: build English identity projection: %w",
			err,
		)
	}
	canonicalJSON, err := localization.MarshalCanonical(canonical)
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: encode canonical artifact: %w",
			err,
		)
	}
	inputJSON, err := json.Marshal(localizationInput)
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: encode English input: %w",
			err,
		)
	}
	projectionJSON, err := json.Marshal(projection)
	if err != nil {
		return nil, fmt.Errorf(
			"architecture localization artifacts: encode English projection: %w",
			err,
		)
	}
	payloads := []architectureLocalizationArtifactPayload{
		{name: architectureLocalizationCanonicalFile, data: canonicalJSON},
		{name: architectureLocalizationInputFile, data: inputJSON},
		{name: architectureLocalizationProjectionFile, data: projectionJSON},
	}
	if err := validateArchitectureLocalizationIdentityPayloads(canvasJSON, payloads); err != nil {
		return nil, err
	}
	return payloads, nil
}

func validateArchitectureLocalizationIdentityPayloads(
	canvasJSON []byte,
	payloads []architectureLocalizationArtifactPayload,
) error {
	if err := validateArchitectureLocalizationArtifactNames(payloads); err != nil {
		return err
	}
	for _, payload := range payloads {
		if len(payload.data) == 0 || len(payload.data) > maxArchitectureLocalizationArtifactBytes {
			return fmt.Errorf(
				"architecture localization artifacts: %s exceeds its byte limit",
				payload.name,
			)
		}
		if kind, found := secretscan.DetectAlways(string(payload.data)); found {
			return fmt.Errorf(
				"architecture localization artifacts: %s contains an obvious %s",
				payload.name,
				kind,
			)
		}
	}

	var canonical localization.CanonicalArtifact
	var input localization.Input
	var projection localization.Projection
	if err := decodeArchitectureLocalizationJSON(payloads[0].data, &canonical); err != nil {
		return fmt.Errorf(
			"architecture localization artifacts: decode canonical payload: %w",
			err,
		)
	}
	if err := decodeArchitectureLocalizationJSON(payloads[1].data, &input); err != nil {
		return fmt.Errorf(
			"architecture localization artifacts: decode input payload: %w",
			err,
		)
	}
	if err := decodeArchitectureLocalizationJSON(payloads[2].data, &projection); err != nil {
		return fmt.Errorf(
			"architecture localization artifacts: decode projection payload: %w",
			err,
		)
	}
	if kind, found := architectureLocalizationCredential(
		canonical,
		input,
		projection,
	); found {
		return fmt.Errorf(
			"architecture localization artifacts: decoded identity payload contains an obvious %s",
			kind,
		)
	}
	roundTripCanonical, err := localization.MarshalCanonical(canonical)
	if err != nil || !bytes.Equal(roundTripCanonical, payloads[0].data) {
		return fmt.Errorf(
			"architecture localization artifacts: canonical payload is not stable",
		)
	}
	var canvas ArchitectureCanvas
	if err := decodeArchitectureLocalizationJSON(canvasJSON, &canvas); err != nil {
		return fmt.Errorf(
			"architecture localization artifacts: decode current canvas: %w",
			err,
		)
	}
	projected, result, err := applyArchitectureLocalization(
		canvas,
		canonical,
		input,
		projection,
	)
	if err != nil {
		return fmt.Errorf(
			"architecture localization artifacts: replay English identity: %w",
			err,
		)
	}
	if result.Fallback || len(result.Diagnostics) != 0 {
		return fmt.Errorf(
			"architecture localization artifacts: English identity replay produced diagnostics",
		)
	}
	projectedJSON, err := json.Marshal(projected)
	if err != nil || !bytes.Equal(projectedJSON, canvasJSON) {
		return fmt.Errorf(
			"architecture localization artifacts: English identity replay changed the canvas",
		)
	}
	return nil
}

func architectureLocalizationCredential(
	canonical localization.CanonicalArtifact,
	input localization.Input,
	projection localization.Projection,
) (string, bool) {
	scan := func(values ...string) (string, bool) {
		for _, value := range values {
			if kind, found := secretscan.DetectAlways(value); found {
				return kind, true
			}
		}
		return "", false
	}
	if kind, found := scan(
		canonical.Locale,
		canonical.SHA256,
		input.CanonicalSHA256,
		input.SourceLocale,
		input.TargetLocale,
		projection.CanonicalSHA256,
		projection.Locale,
	); found {
		return kind, true
	}
	for _, field := range canonical.Fields {
		if kind, found := scan(
			field.ID,
			string(field.OwnerKind),
			field.OwnerID,
			string(field.Name),
			field.Text,
		); found {
			return kind, true
		}
		for _, term := range field.ProtectedTerms {
			if kind, found := scan(term.Token, string(term.Kind), term.Value); found {
				return kind, true
			}
		}
	}
	for _, field := range input.Fields {
		if kind, found := scan(field.ID, field.Text); found {
			return kind, true
		}
		for _, placeholder := range field.Placeholders {
			if kind, found := scan(
				placeholder.Token,
				string(placeholder.Kind),
			); found {
				return kind, true
			}
		}
	}
	if len(projection.Translations) > len(input.Fields) {
		// Both callers raw-scan the bounded JSON first. Apply rejects this
		// collection atomically without forwarding any untrusted ID or value.
		return "", false
	}
	translationIDs := make([]string, 0, len(input.Fields))
	for id := range projection.Translations {
		translationIDs = append(translationIDs, id)
	}
	sort.Strings(translationIDs)
	for _, id := range translationIDs {
		translated := projection.Translations[id]
		if kind, found := scan(id, translated); found {
			return kind, true
		}
	}
	return "", false
}

func decodeArchitectureLocalizationJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func writeArchitectureLocalizationIdentityPayloads(
	runDir string,
	payloads []architectureLocalizationArtifactPayload,
) error {
	if err := validateArchitectureLocalizationArtifactNames(payloads); err != nil {
		return err
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("architecture localization artifacts: open run root: %w", err)
	}
	defer root.Close()

	info, err := root.Lstat(architectureLocalizationArtifactDir)
	switch {
	case os.IsNotExist(err):
		if err := root.Mkdir(architectureLocalizationArtifactDir, 0o700); err != nil {
			return fmt.Errorf(
				"architecture localization artifacts: create localization directory: %w",
				err,
			)
		}
	case err != nil:
		return fmt.Errorf(
			"architecture localization artifacts: inspect localization directory: %w",
			err,
		)
	case info.Mode()&os.ModeSymlink != 0 || !info.IsDir():
		return fmt.Errorf(
			"architecture localization artifacts: localization path is not a real directory",
		)
	}
	if err := root.Chmod(architectureLocalizationArtifactDir, 0o700); err != nil {
		return fmt.Errorf(
			"architecture localization artifacts: protect localization directory: %w",
			err,
		)
	}

	existing := 0
	for _, payload := range payloads {
		name := filepath.Join(architectureLocalizationArtifactDir, payload.name)
		info, err := root.Lstat(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf(
				"architecture localization artifacts: inspect existing %s: %w",
				payload.name,
				err,
			)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
			info.Size() < 0 || info.Size() > maxArchitectureLocalizationArtifactBytes {
			return fmt.Errorf(
				"architecture localization artifacts: existing %s is not a bounded regular file",
				payload.name,
			)
		}
		current, err := root.ReadFile(name)
		if err != nil {
			return fmt.Errorf(
				"architecture localization artifacts: read existing %s: %w",
				payload.name,
				err,
			)
		}
		if !bytes.Equal(current, payload.data) {
			return fmt.Errorf(
				"architecture localization artifacts: existing %s conflicts with current identity",
				payload.name,
			)
		}
		existing++
	}
	if existing != 0 {
		if existing != len(payloads) {
			return fmt.Errorf(
				"architecture localization artifacts: existing identity set is incomplete",
			)
		}
		for _, payload := range payloads {
			if err := root.Chmod(
				filepath.Join(architectureLocalizationArtifactDir, payload.name),
				0o600,
			); err != nil {
				return fmt.Errorf(
					"architecture localization artifacts: protect existing %s: %w",
					payload.name,
					err,
				)
			}
		}
		return nil
	}

	return installArchitectureLocalizationIdentityPayloads(
		root,
		payloads,
		root.Rename,
	)
}

func validateArchitectureLocalizationArtifactNames(
	payloads []architectureLocalizationArtifactPayload,
) error {
	expected := []string{
		architectureLocalizationCanonicalFile,
		architectureLocalizationInputFile,
		architectureLocalizationProjectionFile,
	}
	if len(payloads) != len(expected) {
		return fmt.Errorf("architecture localization artifacts: incomplete payload set")
	}
	for index, name := range expected {
		if payloads[index].name != name {
			return fmt.Errorf("architecture localization artifacts: invalid fixed payload set")
		}
	}
	return nil
}

func installArchitectureLocalizationIdentityPayloads(
	root *os.Root,
	payloads []architectureLocalizationArtifactPayload,
	rename func(string, string) error,
) (resultErr error) {
	temporary := make([]string, 0, len(payloads))
	installed := make([]string, 0, len(payloads))
	defer func() {
		if resultErr == nil {
			return
		}
		for _, name := range temporary {
			_ = root.Remove(name)
		}
		for _, name := range installed {
			_ = root.Remove(name)
		}
	}()

	for _, payload := range payloads {
		tempName, file, err := createArchitectureLocalizationTemp(root, payload.name)
		if err != nil {
			return err
		}
		temporary = append(temporary, tempName)
		if _, err := file.Write(payload.data); err != nil {
			_ = file.Close()
			return fmt.Errorf(
				"architecture localization artifacts: write %s: %w",
				payload.name,
				err,
			)
		}
		if err := file.Chmod(0o600); err != nil {
			_ = file.Close()
			return fmt.Errorf(
				"architecture localization artifacts: protect %s: %w",
				payload.name,
				err,
			)
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return fmt.Errorf(
				"architecture localization artifacts: sync %s: %w",
				payload.name,
				err,
			)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf(
				"architecture localization artifacts: close %s: %w",
				payload.name,
				err,
			)
		}
	}

	for index, payload := range payloads {
		finalName := filepath.Join(architectureLocalizationArtifactDir, payload.name)
		if err := rename(temporary[index], finalName); err != nil {
			return fmt.Errorf(
				"architecture localization artifacts: install %s: %w",
				payload.name,
				err,
			)
		}
		installed = append(installed, finalName)
		temporary[index] = ""
	}
	dir, err := root.Open(architectureLocalizationArtifactDir)
	if err != nil {
		return fmt.Errorf(
			"architecture localization artifacts: open localization directory: %w",
			err,
		)
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return fmt.Errorf(
			"architecture localization artifacts: sync localization directory: %w",
			err,
		)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf(
			"architecture localization artifacts: close localization directory: %w",
			err,
		)
	}
	return nil
}

func createArchitectureLocalizationTemp(
	root *os.Root,
	finalName string,
) (string, *os.File, error) {
	for attempt := 0; attempt < 8; attempt++ {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return "", nil, fmt.Errorf(
				"architecture localization artifacts: create random temp name: %w",
				err,
			)
		}
		name := filepath.Join(
			architectureLocalizationArtifactDir,
			"."+finalName+"."+hex.EncodeToString(nonce[:])+".tmp",
		)
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return name, file, nil
		}
		if !os.IsExist(err) {
			return "", nil, fmt.Errorf(
				"architecture localization artifacts: create %s temp file: %w",
				finalName,
				err,
			)
		}
	}
	return "", nil, fmt.Errorf(
		"architecture localization artifacts: create %s temp file: name collision",
		finalName,
	)
}
