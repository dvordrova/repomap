package report

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	standaloneBundleTransportVersion                 = 4
	standaloneBundleOverflowProbeBytes         int64 = 1
	maxStandaloneBundleTargetRawBytesV4              = int64(MaxReportJSONBytes)
	maxStandaloneBundleTargetCompressedBytesV4       = maxStandaloneBundleTargetRawBytesV4 + int64(1<<20)

	standaloneBundleRepositoryEncoding = "identity-json"
	standaloneBundleTargetEncoding     = "gzip+base64"

	standaloneBundleRepositoryElementID = "rm-repository-payload"
	standaloneBundleTargetElementPrefix = "rm-target-chunk-"
)

type standaloneBundleTransportTargetState string

const (
	standaloneBundleTransportTargetAnalyzed    standaloneBundleTransportTargetState = "analyzed"
	standaloneBundleTransportTargetNotAnalyzed standaloneBundleTransportTargetState = "not_analyzed"
)

// standaloneBundleTransportInputV4 is deliberately feature-blind. Its payload
// bytes have already crossed the typed browser-projection validation boundary;
// this layer only binds opaque bytes to exact target metadata and transport.
type standaloneBundleTransportInputV4 struct {
	RepositoryPayload      []byte
	LogicalDefaultTargetID string
	Targets                []standaloneBundleTransportTargetInputV4
}

type standaloneBundleTransportTargetInputV4 struct {
	TargetID        string
	ProgramTargetID string
	State           standaloneBundleTransportTargetState
	Payload         []byte
}

type standaloneBundleIndexV4 struct {
	Version                int                             `json:"version"`
	Repository             standaloneBundlePayloadRefV4    `json:"repository"`
	LogicalDefaultTargetID string                          `json:"logical_default_target_id"`
	Targets                []standaloneBundleTargetIndexV4 `json:"targets"`
}

type standaloneBundleTargetIndexV4 struct {
	TargetID        string                               `json:"target_id"`
	ProgramTargetID string                               `json:"program_target_id,omitempty"`
	State           standaloneBundleTransportTargetState `json:"state"`
	Chunk           *standaloneBundlePayloadRefV4        `json:"chunk,omitempty"`
}

type standaloneBundlePayloadRefV4 struct {
	ElementID            string `json:"element_id"`
	Encoding             string `json:"encoding"`
	RawByteLength        int64  `json:"raw_byte_length"`
	CompressedByteLength int64  `json:"compressed_byte_length,omitempty"`
	SHA256               string `json:"sha256"`
}

type standaloneBundleEncodedTargetChunkV4 struct {
	TargetID string
	Ref      standaloneBundlePayloadRefV4
	Base64   string
}

type standaloneBundleTransportV4 struct {
	Index             standaloneBundleIndexV4
	IndexJSON         []byte
	RepositoryPayload []byte
	TargetChunks      []standaloneBundleEncodedTargetChunkV4
}

// prepareStandaloneBundleTransportV4 applies only transport concerns: bounds,
// deterministic gzip, base64, exact lengths and digests. Every encoded target
// is decoded immediately and compared with its opaque input before the result
// can be handed to an HTML writer.
func prepareStandaloneBundleTransportV4(
	input standaloneBundleTransportInputV4,
) (standaloneBundleTransportV4, error) {
	if err := validateStandaloneBundleTransportInputV4(input); err != nil {
		return standaloneBundleTransportV4{}, err
	}

	aggregate, err := standaloneTargetAggregateBytes(0, int64(len(input.RepositoryPayload)))
	if err != nil {
		return standaloneBundleTransportV4{}, err
	}
	repositoryPayload := append([]byte(nil), input.RepositoryPayload...)
	repositoryRef := standaloneBundleIdentityPayloadRefV4(
		standaloneBundleRepositoryElementID,
		standaloneBundleRepositoryEncoding,
		repositoryPayload,
	)
	if err := verifyStandaloneBundleIdentityPayloadV4(repositoryRef, repositoryPayload); err != nil {
		return standaloneBundleTransportV4{}, fmt.Errorf("report: standalone bundle v4 repository round-trip: %w", err)
	}

	index := standaloneBundleIndexV4{
		Version:                standaloneBundleTransportVersion,
		Repository:             repositoryRef,
		LogicalDefaultTargetID: input.LogicalDefaultTargetID,
		Targets:                make([]standaloneBundleTargetIndexV4, 0, len(input.Targets)),
	}
	chunks := make([]standaloneBundleEncodedTargetChunkV4, 0, len(input.Targets))
	for position, target := range input.Targets {
		row := standaloneBundleTargetIndexV4{
			TargetID:        target.TargetID,
			ProgramTargetID: target.ProgramTargetID,
			State:           target.State,
		}
		if target.State == standaloneBundleTransportTargetAnalyzed {
			aggregate, err = standaloneTargetAggregateBytes(aggregate, int64(len(target.Payload)))
			if err != nil {
				return standaloneBundleTransportV4{}, err
			}
			elementID := fmt.Sprintf("%s%d", standaloneBundleTargetElementPrefix, position)
			chunk, encodeErr := encodeStandaloneBundleTargetChunkV4(target.TargetID, elementID, target.Payload)
			if encodeErr != nil {
				return standaloneBundleTransportV4{}, fmt.Errorf(
					"report: standalone bundle v4 target %d: %w", position, encodeErr,
				)
			}
			restored, decodeErr := decodeStandaloneBundleTargetChunkV4(chunk.Ref, chunk.Base64)
			if decodeErr != nil {
				return standaloneBundleTransportV4{}, fmt.Errorf(
					"report: standalone bundle v4 target %d generation round-trip: %w", position, decodeErr,
				)
			}
			if !bytes.Equal(restored, target.Payload) {
				return standaloneBundleTransportV4{}, fmt.Errorf(
					"report: standalone bundle v4 target %d generation round-trip changed payload bytes", position,
				)
			}
			ref := chunk.Ref
			row.Chunk = &ref
			chunks = append(chunks, chunk)
		}
		index.Targets = append(index.Targets, row)
	}
	_ = aggregate

	if err := validateStandaloneBundleIndexV4(index); err != nil {
		return standaloneBundleTransportV4{}, err
	}
	indexJSON, err := json.Marshal(index)
	if err != nil {
		return standaloneBundleTransportV4{}, fmt.Errorf("report: encode standalone bundle v4 index: %w", err)
	}
	if _, err := decodeStandaloneBundleIndexV4(indexJSON); err != nil {
		return standaloneBundleTransportV4{}, fmt.Errorf("report: verify standalone bundle v4 index: %w", err)
	}

	return standaloneBundleTransportV4{
		Index:             index,
		IndexJSON:         indexJSON,
		RepositoryPayload: repositoryPayload,
		TargetChunks:      chunks,
	}, nil
}

func validateStandaloneBundleTransportInputV4(input standaloneBundleTransportInputV4) error {
	if len(input.RepositoryPayload) == 0 {
		return fmt.Errorf("report: standalone bundle v4 repository payload is empty")
	}
	if !validStandaloneBundleTransportText(input.LogicalDefaultTargetID) {
		return fmt.Errorf("report: standalone bundle v4 logical default target is invalid")
	}
	if len(input.Targets) == 0 {
		return fmt.Errorf("report: standalone bundle v4 target index is empty")
	}
	seenTargets := make(map[string]struct{}, len(input.Targets))
	seenProgramTargets := make(map[string]struct{}, len(input.Targets))
	for position, target := range input.Targets {
		if !validStandaloneBundleTransportText(target.TargetID) {
			return fmt.Errorf("report: standalone bundle v4 target %d identity is invalid", position)
		}
		if _, duplicate := seenTargets[target.TargetID]; duplicate {
			return fmt.Errorf("report: standalone bundle v4 contains duplicate target identity")
		}
		seenTargets[target.TargetID] = struct{}{}
		switch target.State {
		case standaloneBundleTransportTargetAnalyzed:
			if !validStandaloneBundleTransportText(target.ProgramTargetID) || len(target.Payload) == 0 ||
				int64(len(target.Payload)) > maxStandaloneBundleTargetRawBytesV4 {
				return fmt.Errorf("report: standalone bundle v4 analyzed target %d is incomplete", position)
			}
			if _, duplicate := seenProgramTargets[target.ProgramTargetID]; duplicate {
				return fmt.Errorf("report: standalone bundle v4 contains duplicate program target identity")
			}
			seenProgramTargets[target.ProgramTargetID] = struct{}{}
		case standaloneBundleTransportTargetNotAnalyzed:
			if target.ProgramTargetID != "" || len(target.Payload) != 0 {
				return fmt.Errorf("report: standalone bundle v4 failed target %d carries analyzed payload authority", position)
			}
		default:
			return fmt.Errorf("report: standalone bundle v4 target %d state is unsupported", position)
		}
	}
	return nil
}

func validateStandaloneBundleIndexV4(index standaloneBundleIndexV4) error {
	if index.Version != standaloneBundleTransportVersion {
		return fmt.Errorf("report: standalone bundle v4 index version is unsupported")
	}
	if err := validateStandaloneBundlePayloadRefV4(
		index.Repository,
		standaloneBundleRepositoryElementID,
		standaloneBundleRepositoryEncoding,
	); err != nil {
		return fmt.Errorf("report: standalone bundle v4 repository reference: %w", err)
	}
	if !validStandaloneBundleTransportText(index.LogicalDefaultTargetID) || len(index.Targets) == 0 {
		return fmt.Errorf("report: standalone bundle v4 index identity is invalid")
	}
	aggregate, err := standaloneTargetAggregateBytes(0, index.Repository.RawByteLength)
	if err != nil {
		return fmt.Errorf("report: standalone bundle v4 index aggregate: %w", err)
	}
	seenTargets := make(map[string]struct{}, len(index.Targets))
	seenProgramTargets := make(map[string]struct{}, len(index.Targets))
	seenElements := map[string]struct{}{index.Repository.ElementID: {}}
	for position, target := range index.Targets {
		if !validStandaloneBundleTransportText(target.TargetID) {
			return fmt.Errorf("report: standalone bundle v4 target reference %d identity is invalid", position)
		}
		if _, duplicate := seenTargets[target.TargetID]; duplicate {
			return fmt.Errorf("report: standalone bundle v4 index contains duplicate target identity")
		}
		seenTargets[target.TargetID] = struct{}{}
		switch target.State {
		case standaloneBundleTransportTargetAnalyzed:
			if !validStandaloneBundleTransportText(target.ProgramTargetID) || target.Chunk == nil {
				return fmt.Errorf("report: standalone bundle v4 analyzed target reference %d is incomplete", position)
			}
			if _, duplicate := seenProgramTargets[target.ProgramTargetID]; duplicate {
				return fmt.Errorf("report: standalone bundle v4 index contains duplicate program target identity")
			}
			seenProgramTargets[target.ProgramTargetID] = struct{}{}
			wantElementID := fmt.Sprintf("%s%d", standaloneBundleTargetElementPrefix, position)
			if err := validateStandaloneBundlePayloadRefV4(
				*target.Chunk, wantElementID, standaloneBundleTargetEncoding,
			); err != nil {
				return fmt.Errorf("report: standalone bundle v4 target reference %d: %w", position, err)
			}
			if _, duplicate := seenElements[target.Chunk.ElementID]; duplicate {
				return fmt.Errorf("report: standalone bundle v4 index contains duplicate payload element")
			}
			seenElements[target.Chunk.ElementID] = struct{}{}
			aggregate, err = standaloneTargetAggregateBytes(aggregate, target.Chunk.RawByteLength)
			if err != nil {
				return fmt.Errorf("report: standalone bundle v4 index aggregate: %w", err)
			}
		case standaloneBundleTransportTargetNotAnalyzed:
			if target.ProgramTargetID != "" || target.Chunk != nil {
				return fmt.Errorf("report: standalone bundle v4 failed target reference %d carries a chunk", position)
			}
		default:
			return fmt.Errorf("report: standalone bundle v4 target reference %d state is unsupported", position)
		}
	}
	return nil
}

func encodeStandaloneBundleTargetChunkV4(
	targetID string,
	elementID string,
	raw []byte,
) (standaloneBundleEncodedTargetChunkV4, error) {
	if !validStandaloneBundleTransportText(targetID) || elementID == "" || len(raw) == 0 {
		return standaloneBundleEncodedTargetChunkV4{}, fmt.Errorf("target chunk input is invalid")
	}
	var compressed bytes.Buffer
	ref, err := encodeStandaloneBundleTargetChunkToV4(targetID, elementID, raw, &compressed)
	if err != nil {
		return standaloneBundleEncodedTargetChunkV4{}, err
	}
	compressedBytes := compressed.Bytes()
	return standaloneBundleEncodedTargetChunkV4{
		TargetID: targetID,
		Ref:      ref,
		Base64:   base64.StdEncoding.EncodeToString(compressedBytes),
	}, nil
}

type standaloneBundleCountingWriterV4 struct {
	writer io.Writer
	count  int64
}

func (writer *standaloneBundleCountingWriterV4) Write(value []byte) (int, error) {
	written, err := writer.writer.Write(value)
	writer.count += int64(written)
	return written, err
}

// encodeStandaloneBundleTargetChunkToV4 writes one deterministic gzip member
// directly to its destination. It retains no compressed or base64 copy.
func encodeStandaloneBundleTargetChunkToV4(
	targetID string,
	elementID string,
	raw []byte,
	destination io.Writer,
) (standaloneBundlePayloadRefV4, error) {
	if !validStandaloneBundleTransportText(targetID) || elementID == "" || len(raw) == 0 ||
		int64(len(raw)) > maxStandaloneBundleTargetRawBytesV4 || destination == nil {
		return standaloneBundlePayloadRefV4{}, fmt.Errorf("target chunk input is invalid")
	}
	counting := &standaloneBundleCountingWriterV4{writer: destination}
	writer, err := gzip.NewWriterLevel(counting, gzip.BestCompression)
	if err != nil {
		return standaloneBundlePayloadRefV4{}, fmt.Errorf("create deterministic gzip writer: %w", err)
	}
	writer.Header = gzip.Header{ModTime: time.Unix(0, 0).UTC(), OS: 255}
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return standaloneBundlePayloadRefV4{}, fmt.Errorf("compress target payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return standaloneBundlePayloadRefV4{}, fmt.Errorf("finish target payload compression: %w", err)
	}
	if counting.count > maxStandaloneBundleTargetCompressedBytesV4 {
		return standaloneBundlePayloadRefV4{}, fmt.Errorf("compressed target payload exceeds the per-target byte bound")
	}
	return standaloneBundlePayloadRefV4{
		ElementID:            elementID,
		Encoding:             standaloneBundleTargetEncoding,
		RawByteLength:        int64(len(raw)),
		CompressedByteLength: counting.count,
		SHA256:               standaloneBundleTransportSHA256(raw),
	}, nil
}

// verifyStandaloneBundleTargetChunkStreamV4 performs the required generation
// round-trip against one bounded compressed member without allocating its
// decompressed or encoded representation.
func verifyStandaloneBundleTargetChunkStreamV4(
	ref standaloneBundlePayloadRefV4,
	compressed io.Reader,
	raw []byte,
) error {
	return verifyStandaloneBundleTargetChunkContentStreamV4(ref, compressed, raw, true)
}

func verifyStandaloneBundleTargetChunkDigestStreamV4(
	ref standaloneBundlePayloadRefV4,
	compressed io.Reader,
) error {
	return verifyStandaloneBundleTargetChunkContentStreamV4(ref, compressed, nil, false)
}

func verifyStandaloneBundleTargetChunkContentStreamV4(
	ref standaloneBundlePayloadRefV4,
	compressed io.Reader,
	raw []byte,
	compareRaw bool,
) error {
	if err := validateStandaloneBundlePayloadRefV4(ref, ref.ElementID, standaloneBundleTargetEncoding); err != nil {
		return err
	}
	if compressed == nil || compareRaw && int64(len(raw)) != ref.RawByteLength {
		return fmt.Errorf("target chunk raw length mismatch")
	}
	limited := &io.LimitedReader{R: compressed, N: ref.CompressedByteLength}
	buffered := bufio.NewReaderSize(limited, 64<<10)
	reader, err := gzip.NewReader(buffered)
	if err != nil {
		return fmt.Errorf("open target chunk gzip: %w", err)
	}
	reader.Multistream(false)
	digest := sha256.New()
	buffer := make([]byte, 64<<10)
	offset := 0
	for {
		count, readErr := reader.Read(buffer)
		if count > 0 {
			if compareRaw && (offset > len(raw)-count || !bytes.Equal(buffer[:count], raw[offset:offset+count])) {
				_ = reader.Close()
				return fmt.Errorf("target chunk generation round-trip changed payload bytes")
			}
			_, _ = digest.Write(buffer[:count])
			offset += count
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			_ = reader.Close()
			return fmt.Errorf("decompress target chunk: %w", readErr)
		}
	}
	if err := reader.Close(); err != nil {
		return fmt.Errorf("close target chunk gzip: %w", err)
	}
	if limited.N != 0 || buffered.Buffered() != 0 {
		return fmt.Errorf("target chunk contains trailing or concatenated gzip data")
	}
	if int64(offset) != ref.RawByteLength || compareRaw && offset != len(raw) ||
		hex.EncodeToString(digest.Sum(nil)) != ref.SHA256 {
		return fmt.Errorf("target chunk raw length or sha256 mismatch")
	}
	return nil
}

func decodeStandaloneBundleTargetChunkV4(
	ref standaloneBundlePayloadRefV4,
	encoded string,
) ([]byte, error) {
	if err := validateStandaloneBundlePayloadRefV4(ref, ref.ElementID, standaloneBundleTargetEncoding); err != nil {
		return nil, err
	}
	compressed, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode target chunk base64: %w", err)
	}
	if int64(len(compressed)) != ref.CompressedByteLength ||
		base64.StdEncoding.EncodeToString(compressed) != encoded {
		return nil, fmt.Errorf("target chunk compressed length or base64 representation mismatch")
	}
	compressedReader := bytes.NewReader(compressed)
	reader, err := gzip.NewReader(compressedReader)
	if err != nil {
		return nil, fmt.Errorf("open target chunk gzip: %w", err)
	}
	reader.Multistream(false)
	raw, readErr := io.ReadAll(io.LimitReader(
		reader, ref.RawByteLength+standaloneBundleOverflowProbeBytes,
	))
	closeErr := reader.Close()
	if readErr != nil {
		return nil, fmt.Errorf("decompress target chunk: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close target chunk gzip: %w", closeErr)
	}
	if compressedReader.Len() != 0 {
		return nil, fmt.Errorf("target chunk contains trailing or concatenated gzip data")
	}
	if int64(len(raw)) != ref.RawByteLength {
		return nil, fmt.Errorf("target chunk raw length mismatch")
	}
	if standaloneBundleTransportSHA256(raw) != ref.SHA256 {
		return nil, fmt.Errorf("target chunk sha256 mismatch")
	}
	return raw, nil
}

func standaloneBundleIdentityPayloadRefV4(
	elementID string,
	encoding string,
	raw []byte,
) standaloneBundlePayloadRefV4 {
	return standaloneBundlePayloadRefV4{
		ElementID:     elementID,
		Encoding:      encoding,
		RawByteLength: int64(len(raw)),
		SHA256:        standaloneBundleTransportSHA256(raw),
	}
}

func verifyStandaloneBundleIdentityPayloadV4(
	ref standaloneBundlePayloadRefV4,
	raw []byte,
) error {
	if err := validateStandaloneBundlePayloadRefV4(
		ref, standaloneBundleRepositoryElementID, standaloneBundleRepositoryEncoding,
	); err != nil {
		return err
	}
	if int64(len(raw)) != ref.RawByteLength {
		return fmt.Errorf("repository payload raw length mismatch")
	}
	if standaloneBundleTransportSHA256(raw) != ref.SHA256 {
		return fmt.Errorf("repository payload sha256 mismatch")
	}
	return nil
}

func validateStandaloneBundlePayloadRefV4(
	ref standaloneBundlePayloadRefV4,
	wantElementID string,
	wantEncoding string,
) error {
	if ref.ElementID != wantElementID || ref.Encoding != wantEncoding ||
		ref.RawByteLength <= 0 || ref.RawByteLength > MaxStandaloneTargetBundlePayloadBytes ||
		!validStandaloneBundleTransportDigest(ref.SHA256) {
		return fmt.Errorf("payload reference identity is invalid")
	}
	switch wantEncoding {
	case standaloneBundleRepositoryEncoding:
		if ref.CompressedByteLength != 0 {
			return fmt.Errorf("identity payload reference has a compressed length")
		}
	case standaloneBundleTargetEncoding:
		if ref.RawByteLength > maxStandaloneBundleTargetRawBytesV4 ||
			ref.CompressedByteLength <= 0 ||
			ref.CompressedByteLength > maxStandaloneBundleTargetCompressedBytesV4 {
			return fmt.Errorf("compressed payload reference is outside the per-target byte bounds")
		}
	default:
		return fmt.Errorf("payload reference encoding is unsupported")
	}
	return nil
}

func decodeStandaloneBundleIndexV4(raw []byte) (standaloneBundleIndexV4, error) {
	if len(raw) == 0 {
		return standaloneBundleIndexV4{}, fmt.Errorf("report: standalone bundle v4 index is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var index standaloneBundleIndexV4
	if err := decoder.Decode(&index); err != nil {
		return standaloneBundleIndexV4{}, fmt.Errorf("report: decode standalone bundle v4 index: %w", err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return standaloneBundleIndexV4{}, fmt.Errorf("report: standalone bundle v4 index has multiple JSON values")
		}
		return standaloneBundleIndexV4{}, fmt.Errorf("report: standalone bundle v4 index has trailing data: %w", err)
	}
	if err := validateStandaloneBundleIndexV4(index); err != nil {
		return standaloneBundleIndexV4{}, err
	}
	canonical, err := json.Marshal(index)
	if err != nil {
		return standaloneBundleIndexV4{}, fmt.Errorf("report: re-encode standalone bundle v4 index: %w", err)
	}
	if !bytes.Equal(raw, canonical) {
		return standaloneBundleIndexV4{}, fmt.Errorf("report: standalone bundle v4 index is not canonical")
	}
	return index, nil
}

func standaloneBundleTransportSHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func validStandaloneBundleTransportDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validStandaloneBundleTransportText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
