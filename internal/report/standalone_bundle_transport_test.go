package report

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestStandaloneBundleTransportV4RoundTripsOpaquePayloads(t *testing.T) {
	input := standaloneBundleTransportV4Fixture()
	transport, err := prepareStandaloneBundleTransportV4(input)
	if err != nil {
		t.Fatalf("prepareStandaloneBundleTransportV4: %v", err)
	}
	if transport.Index.Version != standaloneBundleTransportVersion ||
		transport.Index.LogicalDefaultTargetID != "selected-failed" ||
		len(transport.Index.Targets) != 3 || len(transport.TargetChunks) != 2 {
		t.Fatalf("transport index = %#v, chunks = %d", transport.Index, len(transport.TargetChunks))
	}
	if !bytes.Equal(transport.RepositoryPayload, input.RepositoryPayload) {
		t.Fatal("repository payload bytes changed")
	}
	if err := verifyStandaloneBundleIdentityPayloadV4(
		transport.Index.Repository, transport.RepositoryPayload,
	); err != nil {
		t.Fatalf("verify repository payload: %v", err)
	}
	decodedIndex, err := decodeStandaloneBundleIndexV4(transport.IndexJSON)
	if err != nil {
		t.Fatalf("decode generated index: %v", err)
	}
	if decodedIndex.LogicalDefaultTargetID != input.LogicalDefaultTargetID {
		t.Fatalf("decoded logical default = %q", decodedIndex.LogicalDefaultTargetID)
	}

	wantPayloadByTarget := map[string][]byte{
		"selected-go": input.Targets[0].Payload,
		"selected-js": input.Targets[2].Payload,
	}
	for _, chunk := range transport.TargetChunks {
		raw, err := decodeStandaloneBundleTargetChunkV4(chunk.Ref, chunk.Base64)
		if err != nil {
			t.Fatalf("decode chunk %q: %v", chunk.TargetID, err)
		}
		if !bytes.Equal(raw, wantPayloadByTarget[chunk.TargetID]) {
			t.Fatalf("chunk %q payload changed: %q", chunk.TargetID, raw)
		}
	}
	if got := transport.Index.Targets[0].Chunk.ElementID; got != "rm-target-chunk-0" {
		t.Fatalf("first chunk element = %q", got)
	}
	failed := transport.Index.Targets[1]
	if failed.State != standaloneBundleTransportTargetNotAnalyzed ||
		failed.ProgramTargetID != "" || failed.Chunk != nil {
		t.Fatalf("failed target carries a chunk: %#v", failed)
	}
	if got := transport.Index.Targets[2].Chunk.ElementID; got != "rm-target-chunk-2" {
		t.Fatalf("third target chunk element = %q", got)
	}

	input.RepositoryPayload[0] = 'X'
	input.Targets[0].Payload[0] = 'X'
	if bytes.Equal(transport.RepositoryPayload, input.RepositoryPayload) {
		t.Fatal("prepared transport retained caller-owned repository bytes")
	}
	raw, err := decodeStandaloneBundleTargetChunkV4(
		transport.TargetChunks[0].Ref, transport.TargetChunks[0].Base64,
	)
	if err != nil || bytes.Equal(raw, input.Targets[0].Payload) {
		t.Fatalf("prepared target chunk retained caller mutation: %q, %v", raw, err)
	}
}

func TestStandaloneBundleTransportV4GzipIsDeterministic(t *testing.T) {
	raw := []byte(`{"version":1,"target":{"id":"program-go"},"features":{"core":"stable"}}`)
	first, err := encodeStandaloneBundleTargetChunkV4("selected-go", "rm-target-chunk-0", raw)
	if err != nil {
		t.Fatal(err)
	}
	second, err := encodeStandaloneBundleTargetChunkV4("selected-go", "rm-target-chunk-0", raw)
	if err != nil {
		t.Fatal(err)
	}
	if first.Ref != second.Ref || first.Base64 != second.Base64 {
		t.Fatalf("deterministic encoding drifted:\nfirst: %#v\nsecond: %#v", first, second)
	}
	restored, err := decodeStandaloneBundleTargetChunkV4(first.Ref, first.Base64)
	if err != nil || !bytes.Equal(restored, raw) {
		t.Fatalf("deterministic chunk round-trip = %q, %v", restored, err)
	}
}

func TestStandaloneBundleTransportV4StreamsDeterministicGzipRoundTrip(t *testing.T) {
	raw := bytes.Repeat([]byte(`{"target":"streamed"}`), 4096)
	buffer := &bytes.Buffer{}
	ref, err := encodeStandaloneBundleTargetChunkToV4(
		"selected-streamed", "rm-target-chunk-0", raw, buffer,
	)
	if err != nil {
		t.Fatal(err)
	}
	if ref.CompressedByteLength != int64(buffer.Len()) {
		t.Fatalf("streamed compressed length = %d, want %d", ref.CompressedByteLength, buffer.Len())
	}
	if err := verifyStandaloneBundleTargetChunkStreamV4(ref, bytes.NewReader(buffer.Bytes()), raw); err != nil {
		t.Fatalf("verify streamed generation round-trip: %v", err)
	}
	inMemory, err := encodeStandaloneBundleTargetChunkV4(
		"selected-streamed", "rm-target-chunk-0", raw,
	)
	if err != nil {
		t.Fatal(err)
	}
	if ref != inMemory.Ref || base64.StdEncoding.EncodeToString(buffer.Bytes()) != inMemory.Base64 {
		t.Fatal("streaming gzip bytes differ from the deterministic in-memory codec")
	}
}

func TestStandaloneBundleTransportV4RejectsCorruptTargetChunks(t *testing.T) {
	raw := []byte(`{"version":1,"target":{"id":"program-go"},"features":{"core":"payload"}}`)
	chunk, err := encodeStandaloneBundleTargetChunkV4("selected-go", "rm-target-chunk-0", raw)
	if err != nil {
		t.Fatal(err)
	}
	compressed, err := base64.StdEncoding.DecodeString(chunk.Base64)
	if err != nil {
		t.Fatal(err)
	}
	corruptGzip := append([]byte(nil), compressed...)
	corruptGzip[0] ^= 0xff
	trailingGzip := append(append([]byte(nil), compressed...), 0)
	other, err := encodeStandaloneBundleTargetChunkV4(
		"selected-other", "rm-target-chunk-1", []byte(`{"version":1,"target":{"id":"other"}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	otherCompressed, err := base64.StdEncoding.DecodeString(other.Base64)
	if err != nil {
		t.Fatal(err)
	}
	concatenatedGzip := append(append([]byte(nil), compressed...), otherCompressed...)

	tests := map[string]func() (standaloneBundlePayloadRefV4, string){
		"raw length too short": func() (standaloneBundlePayloadRefV4, string) {
			ref := chunk.Ref
			ref.RawByteLength--
			return ref, chunk.Base64
		},
		"raw length too long": func() (standaloneBundlePayloadRefV4, string) {
			ref := chunk.Ref
			ref.RawByteLength++
			return ref, chunk.Base64
		},
		"compressed length": func() (standaloneBundlePayloadRefV4, string) {
			ref := chunk.Ref
			ref.CompressedByteLength++
			return ref, chunk.Base64
		},
		"digest": func() (standaloneBundlePayloadRefV4, string) {
			ref := chunk.Ref
			ref.SHA256 = strings.Repeat("0", 64)
			return ref, chunk.Base64
		},
		"truncated base64": func() (standaloneBundlePayloadRefV4, string) {
			return chunk.Ref, chunk.Base64[:len(chunk.Base64)-3]
		},
		"corrupted gzip": func() (standaloneBundlePayloadRefV4, string) {
			return chunk.Ref, base64.StdEncoding.EncodeToString(corruptGzip)
		},
		"trailing gzip bytes": func() (standaloneBundlePayloadRefV4, string) {
			ref := chunk.Ref
			ref.CompressedByteLength = int64(len(trailingGzip))
			return ref, base64.StdEncoding.EncodeToString(trailingGzip)
		},
		"concatenated gzip member": func() (standaloneBundlePayloadRefV4, string) {
			ref := chunk.Ref
			ref.CompressedByteLength = int64(len(concatenatedGzip))
			return ref, base64.StdEncoding.EncodeToString(concatenatedGzip)
		},
		"unsupported encoding": func() (standaloneBundlePayloadRefV4, string) {
			ref := chunk.Ref
			ref.Encoding = "identity-json"
			return ref, chunk.Base64
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			ref, encoded := mutate()
			if restored, err := decodeStandaloneBundleTargetChunkV4(ref, encoded); err == nil {
				t.Fatalf("corrupt chunk was accepted: %q", restored)
			}
		})
	}
}

func TestStandaloneBundleTransportV4RejectsInvalidInputs(t *testing.T) {
	tests := map[string]func(*standaloneBundleTransportInputV4){
		"empty repository": func(input *standaloneBundleTransportInputV4) {
			input.RepositoryPayload = nil
		},
		"invalid default": func(input *standaloneBundleTransportInputV4) {
			input.LogicalDefaultTargetID = ""
		},
		"duplicate target": func(input *standaloneBundleTransportInputV4) {
			input.Targets[2].TargetID = input.Targets[0].TargetID
		},
		"duplicate program target": func(input *standaloneBundleTransportInputV4) {
			input.Targets[2].ProgramTargetID = input.Targets[0].ProgramTargetID
		},
		"failed target payload": func(input *standaloneBundleTransportInputV4) {
			input.Targets[1].Payload = []byte(`{"unexpected":true}`)
		},
		"failed program target": func(input *standaloneBundleTransportInputV4) {
			input.Targets[1].ProgramTargetID = "program-failed"
		},
		"analyzed target without payload": func(input *standaloneBundleTransportInputV4) {
			input.Targets[0].Payload = nil
		},
		"analyzed target without program identity": func(input *standaloneBundleTransportInputV4) {
			input.Targets[0].ProgramTargetID = ""
		},
		"unsupported state": func(input *standaloneBundleTransportInputV4) {
			input.Targets[0].State = "partial"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := standaloneBundleTransportV4Fixture()
			mutate(&input)
			if transport, err := prepareStandaloneBundleTransportV4(input); err == nil {
				t.Fatalf("invalid input was accepted: %#v", transport.Index)
			}
		})
	}
}

func TestStandaloneBundleTransportV4RejectsInvalidIndexes(t *testing.T) {
	transport, err := prepareStandaloneBundleTransportV4(standaloneBundleTransportV4Fixture())
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*standaloneBundleIndexV4){
		"unsupported version": func(index *standaloneBundleIndexV4) {
			index.Version++
		},
		"repository encoding": func(index *standaloneBundleIndexV4) {
			index.Repository.Encoding = standaloneBundleTargetEncoding
		},
		"invalid default": func(index *standaloneBundleIndexV4) {
			index.LogicalDefaultTargetID = ""
		},
		"duplicate target": func(index *standaloneBundleIndexV4) {
			index.Targets[2].TargetID = index.Targets[0].TargetID
		},
		"duplicate program target": func(index *standaloneBundleIndexV4) {
			index.Targets[2].ProgramTargetID = index.Targets[0].ProgramTargetID
		},
		"failed target chunk": func(index *standaloneBundleIndexV4) {
			ref := *index.Targets[0].Chunk
			index.Targets[1].Chunk = &ref
		},
		"failed program target": func(index *standaloneBundleIndexV4) {
			index.Targets[1].ProgramTargetID = "program-failed"
		},
		"analyzed target without chunk": func(index *standaloneBundleIndexV4) {
			index.Targets[0].Chunk = nil
		},
		"unsupported target encoding": func(index *standaloneBundleIndexV4) {
			index.Targets[0].Chunk.Encoding = "deflate+base64"
		},
		"wrong chunk element": func(index *standaloneBundleIndexV4) {
			index.Targets[2].Chunk.ElementID = "rm-target-chunk-1"
		},
		"invalid target digest": func(index *standaloneBundleIndexV4) {
			index.Targets[0].Chunk.SHA256 = "invalid"
		},
		"target raw length exceeds browser bound": func(index *standaloneBundleIndexV4) {
			index.Targets[0].Chunk.RawByteLength = maxStandaloneBundleTargetRawBytesV4 + 1
		},
		"target compressed length exceeds browser bound": func(index *standaloneBundleIndexV4) {
			index.Targets[0].Chunk.CompressedByteLength = maxStandaloneBundleTargetCompressedBytesV4 + 1
		},
		"aggregate raw length exceeds bundle bound": func(index *standaloneBundleIndexV4) {
			index.Repository.RawByteLength = MaxStandaloneTargetBundlePayloadBytes
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			index := cloneStandaloneBundleIndexV4(t, transport.Index)
			mutate(&index)
			if err := validateStandaloneBundleIndexV4(index); err == nil {
				t.Fatalf("invalid index was accepted: %#v", index)
			}
		})
	}
}

func TestStandaloneBundleTransportV4IndexDecodeIsStrict(t *testing.T) {
	transport, err := prepareStandaloneBundleTransportV4(standaloneBundleTransportV4Fixture())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeStandaloneBundleIndexV4(transport.IndexJSON); err != nil {
		t.Fatalf("canonical index rejected: %v", err)
	}
	unknown := bytes.Replace(
		transport.IndexJSON,
		[]byte(`{"version":4`),
		[]byte(`{"version":4,"future":true`),
		1,
	)
	if bytes.Equal(unknown, transport.IndexJSON) {
		t.Fatal("index version prefix was absent")
	}
	for name, raw := range map[string][]byte{
		"unknown field":        unknown,
		"trailing value":       append(append([]byte(nil), transport.IndexJSON...), []byte(` {}`)...),
		"alternate whitespace": append([]byte(" "), transport.IndexJSON...),
	} {
		t.Run(name, func(t *testing.T) {
			if index, err := decodeStandaloneBundleIndexV4(raw); err == nil {
				t.Fatalf("non-strict index was accepted: %#v", index)
			}
		})
	}
}

func standaloneBundleTransportV4Fixture() standaloneBundleTransportInputV4 {
	return standaloneBundleTransportInputV4{
		RepositoryPayload:      []byte(`{"version":1,"repository":{"name":"fixture"},"runtime":{"roles":[]},"target_outcomes":{"outcomes":[]}}`),
		LogicalDefaultTargetID: "selected-failed",
		Targets: []standaloneBundleTransportTargetInputV4{
			{
				TargetID:        "selected-go",
				ProgramTargetID: "program-go",
				State:           standaloneBundleTransportTargetAnalyzed,
				Payload:         []byte(`{"version":1,"target":{"id":"program-go"},"features":{"core":"go"}}`),
			},
			{
				TargetID: "selected-failed",
				State:    standaloneBundleTransportTargetNotAnalyzed,
			},
			{
				TargetID:        "selected-js",
				ProgramTargetID: "program-js",
				State:           standaloneBundleTransportTargetAnalyzed,
				Payload:         []byte(`{"version":1,"target":{"id":"program-js"},"features":{"surfaces":"js"}}`),
			},
		},
	}
}

func cloneStandaloneBundleIndexV4(
	t *testing.T,
	index standaloneBundleIndexV4,
) standaloneBundleIndexV4 {
	t.Helper()
	raw, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	var cloned standaloneBundleIndexV4
	if err := json.Unmarshal(raw, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
