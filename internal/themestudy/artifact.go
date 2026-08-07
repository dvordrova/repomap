package themestudy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Artifact filenames (Decision 213 §6). They are persisted in the run
// directory and bound by RunManifest v12 SHA-256 fields.
const (
	ScoutRequestArtifactFilename        = "theme_scout_request.v1.json"
	ScoutResultArtifactFilename         = "theme_scout_result.v1.json"
	ScoutStatusArtifactFilename         = "theme_scout_status.v1.json"
	ExpansionArtifactFilename           = "theme_source_expansion.v1.json"
	AdjudicationRequestArtifactFilename = "theme_adjudication_request.v1.json"
	AdjudicationResultArtifactFilename  = "theme_adjudication_result.v1.json"
	AdjudicationStatusArtifactFilename  = "theme_adjudication_status.v1.json"
	StudyThemesArtifactFilename         = "study_themes.v1.json"
)

// The eight theme artifacts in canonical binding order (RunManifest v12).
var ThemeArtifactFilenames = []string{
	ScoutRequestArtifactFilename,
	ScoutResultArtifactFilename,
	ScoutStatusArtifactFilename,
	ExpansionArtifactFilename,
	AdjudicationRequestArtifactFilename,
	AdjudicationResultArtifactFilename,
	AdjudicationStatusArtifactFilename,
	StudyThemesArtifactFilename,
}

// encodeArtifact canonicalizes a value into the bounded artifact form.
func encodeArtifact(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	// Canonicalize: one JSON value, no trailing whitespace, key order as
	// marshaled. Round-tripping through the decoder is intentionally not
	// applied here; artifacts are written once and verified by SHA-256.
	return encoded, nil
}

func encodeBoundedArtifact(name string, limit int, value any) ([]byte, error) {
	encoded, err := encodeArtifact(value)
	if err != nil {
		return nil, err
	}
	if len(encoded) > limit {
		return nil, fmt.Errorf("%s artifact exceeds %d bytes", name, limit)
	}
	return encoded, nil
}

func decodeArtifact(name string, data []byte, limit int, target any) error {
	if len(data) == 0 {
		return fmt.Errorf("%s artifact: empty", name)
	}
	if len(data) > limit {
		return fmt.Errorf("%s artifact exceeds %d bytes", name, limit)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%s artifact: decode: %w", name, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s artifact: trailing data", name)
	}
	return nil
}

func requireCanonicalArtifact(name string, data []byte, value any) error {
	encoded, err := encodeArtifact(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, encoded) {
		return fmt.Errorf("%s artifact: not canonical", name)
	}
	return nil
}

// validateScoutRequestIdentity verifies the request is self-consistent: the
// wire digest matches the exact wire JSON and the catalog digest matches the
// embedded vocabulary + seed packs. A tampered identity therefore fails closed
// at encode/decode time, not only at replay time.
func validateScoutRequestIdentity(request ScoutRequest) error {
	if request.Version != ScoutRequestVersion || request.PromptVersion != ScoutPromptVersion ||
		!request.Language.Valid() || request.WireSHA256 == "" || request.WireJSON == "" ||
		request.CatalogSHA256 == "" || len(request.Vocabulary.Files) == 0 ||
		len(request.SeedPacks.Packs) == 0 {
		return fmt.Errorf("theme scout request artifact: invalid identity")
	}
	if contentSHA256Bytes([]byte(request.WireJSON)) != request.WireSHA256 {
		return fmt.Errorf("theme scout request artifact: wire digest mismatch")
	}
	want, err := scoutCatalogDigest(request.Vocabulary, request.SeedPacks)
	if err != nil {
		return err
	}
	if want != request.CatalogSHA256 {
		return fmt.Errorf("theme scout request artifact: catalog digest mismatch")
	}
	return nil
}

// EncodeScoutRequest encodes the bounded Theme Scout request artifact.
func EncodeScoutRequest(request ScoutRequest) ([]byte, error) {
	if err := validateScoutRequestIdentity(request); err != nil {
		return nil, err
	}
	return encodeBoundedArtifact("theme scout request", MaxScoutRequestArtifactBytes, request)
}

// DecodeScoutRequest decodes and validates one bounded Theme Scout request.
func DecodeScoutRequest(data []byte) (ScoutRequest, error) {
	var request ScoutRequest
	if err := decodeArtifact("theme scout request", data, MaxScoutRequestArtifactBytes, &request); err != nil {
		return ScoutRequest{}, err
	}
	if err := validateScoutRequestIdentity(request); err != nil {
		return ScoutRequest{}, err
	}
	return request, requireCanonicalArtifact("theme scout request", data, request)
}

// EncodeScoutResult encodes the validated Theme Scout result artifact.
func EncodeScoutResult(result ScoutResult) ([]byte, error) {
	if result.Version != ScoutResultVersion || result.PromptVersion != ScoutPromptVersion ||
		!result.Language.Valid() || result.CatalogSHA256 == "" || result.WireSHA256 == "" ||
		(result.State != "accepted" && result.State != "accepted_partial") ||
		len(result.Candidates) == 0 || result.Status.Accepted != len(result.Candidates) ||
		result.Status.Accepted == 0 {
		return nil, fmt.Errorf("theme scout result artifact: invalid identity")
	}
	return encodeBoundedArtifact("theme scout result", MaxScoutResultArtifactBytes, result)
}

// DecodeScoutResult decodes and validates one bounded Theme Scout result.
func DecodeScoutResult(data []byte) (ScoutResult, error) {
	var result ScoutResult
	if err := decodeArtifact("theme scout result", data, MaxScoutResultArtifactBytes, &result); err != nil {
		return ScoutResult{}, err
	}
	if result.Version != ScoutResultVersion || result.PromptVersion != ScoutPromptVersion ||
		!result.Language.Valid() || result.CatalogSHA256 == "" || result.WireSHA256 == "" ||
		(result.State != "accepted" && result.State != "accepted_partial") ||
		len(result.Candidates) == 0 || result.Status.Accepted != len(result.Candidates) ||
		result.Status.Accepted == 0 {
		return ScoutResult{}, fmt.Errorf("theme scout result artifact: invalid identity")
	}
	return result, requireCanonicalArtifact("theme scout result", data, result)
}

// EncodeScoutStatus encodes the persisted Theme Scout status artifact.
func EncodeScoutStatus(record ScoutStatusRecord) ([]byte, error) {
	if record.Version != ScoutResultVersion || record.PromptVersion != ScoutPromptVersion ||
		!record.Language.Valid() || record.CatalogSHA256 == "" {
		return nil, fmt.Errorf("theme scout status artifact: invalid identity")
	}
	return encodeBoundedArtifact("theme scout status", MaxScoutStatusArtifactBytes, record)
}

// DecodeScoutStatus decodes and validates one bounded Theme Scout status.
func DecodeScoutStatus(data []byte) (ScoutStatusRecord, error) {
	var record ScoutStatusRecord
	if err := decodeArtifact("theme scout status", data, MaxScoutStatusArtifactBytes, &record); err != nil {
		return ScoutStatusRecord{}, err
	}
	if record.Version != ScoutResultVersion || record.PromptVersion != ScoutPromptVersion ||
		!record.Language.Valid() || record.CatalogSHA256 == "" {
		return ScoutStatusRecord{}, fmt.Errorf("theme scout status artifact: invalid identity")
	}
	return record, requireCanonicalArtifact("theme scout status", data, record)
}

// EncodeExpansion encodes the persisted provider-free source expansion
// artifact (contract D).
func EncodeExpansion(expansion SourceExpansion) ([]byte, error) {
	if expansion.Version != expansionVersion || expansion.CandidateSHA256 == "" {
		return nil, fmt.Errorf("theme source expansion artifact: invalid identity")
	}
	return encodeBoundedArtifact("theme source expansion", MaxExpansionArtifactBytes, expansion)
}

// DecodeExpansion decodes and validates one bounded source expansion artifact.
func DecodeExpansion(data []byte) (SourceExpansion, error) {
	var expansion SourceExpansion
	if err := decodeArtifact("theme source expansion", data, MaxExpansionArtifactBytes, &expansion); err != nil {
		return SourceExpansion{}, err
	}
	if expansion.Version != expansionVersion || expansion.CandidateSHA256 == "" {
		return SourceExpansion{}, fmt.Errorf("theme source expansion artifact: invalid identity")
	}
	return expansion, requireCanonicalArtifact("theme source expansion", data, expansion)
}

// validateAdjudicationRequestIdentity verifies the request is self-consistent:
// the wire digest matches the exact wire JSON and the catalog digest matches
// the embedded candidates + expansion + anchors.
func validateAdjudicationRequestIdentity(request AdjudicationRequest) error {
	if request.Version != AdjudicationRequestVersion || request.PromptVersion != AdjudicationPromptVersion ||
		!request.Language.Valid() || request.WireSHA256 == "" || request.WireJSON == "" ||
		request.CatalogSHA256 == "" || len(request.Candidates) == 0 ||
		request.Expansion.CandidateSHA256 == "" || len(request.Anchors) == 0 {
		return fmt.Errorf("theme adjudication request artifact: invalid identity")
	}
	if contentSHA256Bytes([]byte(request.WireJSON)) != request.WireSHA256 {
		return fmt.Errorf("theme adjudication request artifact: wire digest mismatch")
	}
	want, err := adjudicationCatalogDigest(request.Candidates, request.Expansion, request.Anchors)
	if err != nil {
		return err
	}
	if want != request.CatalogSHA256 {
		return fmt.Errorf("theme adjudication request artifact: catalog digest mismatch")
	}
	return nil
}

// EncodeAdjudicationRequest encodes the bounded Theme Adjudication request.
func EncodeAdjudicationRequest(request AdjudicationRequest) ([]byte, error) {
	if err := validateAdjudicationRequestIdentity(request); err != nil {
		return nil, err
	}
	return encodeBoundedArtifact("theme adjudication request", MaxAdjRequestArtifactBytes, request)
}

// DecodeAdjudicationRequest decodes and validates one bounded Theme
// Adjudication request.
func DecodeAdjudicationRequest(data []byte) (AdjudicationRequest, error) {
	var request AdjudicationRequest
	if err := decodeArtifact("theme adjudication request", data, MaxAdjRequestArtifactBytes, &request); err != nil {
		return AdjudicationRequest{}, err
	}
	if err := validateAdjudicationRequestIdentity(request); err != nil {
		return AdjudicationRequest{}, err
	}
	return request, requireCanonicalArtifact("theme adjudication request", data, request)
}

// EncodeAdjudicationResult encodes the validated Theme Adjudication result.
func EncodeAdjudicationResult(result AdjudicationResult) ([]byte, error) {
	if result.Version != AdjudicationResultVersion || result.PromptVersion != AdjudicationPromptVersion ||
		!result.Language.Valid() || result.CatalogSHA256 == "" || result.WireSHA256 == "" ||
		(result.State != "accepted" && result.State != "accepted_partial") ||
		len(result.Themes) == 0 || result.Status.Accepted != len(result.Themes) ||
		result.Status.Accepted == 0 {
		return nil, fmt.Errorf("theme adjudication result artifact: invalid identity")
	}
	return encodeBoundedArtifact("theme adjudication result", MaxAdjResultArtifactBytes, result)
}

// DecodeAdjudicationResult decodes and validates one bounded Theme
// Adjudication result.
func DecodeAdjudicationResult(data []byte) (AdjudicationResult, error) {
	var result AdjudicationResult
	if err := decodeArtifact("theme adjudication result", data, MaxAdjResultArtifactBytes, &result); err != nil {
		return AdjudicationResult{}, err
	}
	if result.Version != AdjudicationResultVersion || result.PromptVersion != AdjudicationPromptVersion ||
		!result.Language.Valid() || result.CatalogSHA256 == "" || result.WireSHA256 == "" ||
		(result.State != "accepted" && result.State != "accepted_partial") ||
		len(result.Themes) == 0 || result.Status.Accepted != len(result.Themes) ||
		result.Status.Accepted == 0 {
		return AdjudicationResult{}, fmt.Errorf("theme adjudication result artifact: invalid identity")
	}
	return result, requireCanonicalArtifact("theme adjudication result", data, result)
}

// EncodeAdjudicationStatus encodes the persisted Theme Adjudication status.
func EncodeAdjudicationStatus(record AdjudicationStatusRecord) ([]byte, error) {
	if record.Version != AdjudicationResultVersion || record.PromptVersion != AdjudicationPromptVersion ||
		!record.Language.Valid() || record.CatalogSHA256 == "" {
		return nil, fmt.Errorf("theme adjudication status artifact: invalid identity")
	}
	return encodeBoundedArtifact("theme adjudication status", MaxAdjStatusArtifactBytes, record)
}

// DecodeAdjudicationStatus decodes and validates one bounded Theme
// Adjudication status.
func DecodeAdjudicationStatus(data []byte) (AdjudicationStatusRecord, error) {
	var record AdjudicationStatusRecord
	if err := decodeArtifact("theme adjudication status", data, MaxAdjStatusArtifactBytes, &record); err != nil {
		return AdjudicationStatusRecord{}, err
	}
	if record.Version != AdjudicationResultVersion || record.PromptVersion != AdjudicationPromptVersion ||
		!record.Language.Valid() || record.CatalogSHA256 == "" {
		return AdjudicationStatusRecord{}, fmt.Errorf("theme adjudication status artifact: invalid identity")
	}
	return record, requireCanonicalArtifact("theme adjudication status", data, record)
}

// EncodeStudyThemes encodes the reduced study_themes artifact (contract F).
func EncodeStudyThemes(themes StudyThemes) ([]byte, error) {
	// Decision 233: StudyThemesVersion 2 (alternate co-projection +
	// concentration diagnostic); Decision 235: version 3 (theme
	// equivalence accounting). The artifact version advances with the
	// constant — no literal drift (D233 defect closed).
	if themes.Version != "v3" {
		return nil, fmt.Errorf("study themes artifact: invalid version")
	}
	return encodeBoundedArtifact("study themes", MaxStudyThemesArtifactBytes, themes)
}

// DecodeStudyThemes decodes and validates one bounded study_themes artifact.
func DecodeStudyThemes(data []byte) (StudyThemes, error) {
	var themes StudyThemes
	if err := decodeArtifact("study themes", data, MaxStudyThemesArtifactBytes, &themes); err != nil {
		return StudyThemes{}, err
	}
	if themes.Version != "v3" {
		return StudyThemes{}, fmt.Errorf("study themes artifact: invalid version")
	}
	return themes, requireCanonicalArtifact("study themes", data, themes)
}
