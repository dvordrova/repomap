package guidedtour

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const maxRecordBytes = 128 << 10

// EncodeRecord validates a proposal against its exact bundle and records the
// canonical bundle digest needed for deterministic replay.
func EncodeRecord(bundle Bundle, proposal Proposal) ([]byte, error) {
	if err := ValidateProposal(bundle, proposal); err != nil {
		return nil, err
	}
	bundleSHA256, _, err := BundleHash(bundle)
	if err != nil {
		return nil, err
	}
	record := Record{
		Version:      RecordVersion,
		BundleSHA256: bundleSHA256,
		Proposal:     proposal,
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("guided tour: encode record: %w", err)
	}
	if len(encoded) > maxRecordBytes {
		return nil, fmt.Errorf("guided tour: encoded record is too large")
	}
	return encoded, nil
}

// DecodeRecord strictly decodes the persisted record contract. Bundle-bound
// reference validation is repeated by ReplayRecord.
func DecodeRecord(raw []byte) (Record, error) {
	if len(raw) == 0 || len(raw) > maxRecordBytes {
		return Record{}, fmt.Errorf("guided tour: record is empty or too large")
	}
	var record Record
	if err := decodeStrictJSON(raw, &record); err != nil {
		return Record{}, fmt.Errorf("guided tour: invalid record json: %w", err)
	}
	if record.Version != RecordVersion {
		return Record{}, fmt.Errorf("guided tour: unsupported record version %d", record.Version)
	}
	if !validSHA256(record.BundleSHA256) {
		return Record{}, fmt.Errorf("guided tour: record bundle hash is malformed")
	}
	if err := validateProposalShape(record.Proposal); err != nil {
		return Record{}, fmt.Errorf("guided tour: record proposal is invalid: %w", err)
	}
	return record, nil
}

// ReplayRecord verifies the canonical bundle digest, then revalidates and
// materializes the saved proposal. A stale record never selects a fallback.
func ReplayRecord(bundle Bundle, raw []byte) (Story, error) {
	record, err := DecodeRecord(raw)
	if err != nil {
		return Story{}, err
	}
	bundleSHA256, _, err := BundleHash(bundle)
	if err != nil {
		return Story{}, err
	}
	if record.BundleSHA256 != bundleSHA256 {
		return Story{}, fmt.Errorf("guided tour: record bundle hash does not match")
	}
	return MaterializeStory(bundle, record.Proposal)
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
