package programindex

import (
	"bytes"
	"fmt"
	"os"

	"github.com/dvordrova/repomap/internal/debugdump"
)

// SharedInput owns one immutable adapter projection while a language adapter
// builds several targets from the same native parse. It is an in-memory
// optimization only; persisted artifacts contain the already sealed Index.
type SharedInput struct {
	input Input
}

func ShareInput(input Input) *SharedInput {
	input.Target = TargetInput{}
	return &SharedInput{input: input}
}

func (shared *SharedInput) ForTarget(target TargetInput) Input {
	input := shared.input
	input.Target = target
	return input
}

// ArtifactStore keeps persistence behind the orchestration seam.
type ArtifactStore struct{}

func NewArtifactStore() *ArtifactStore { return &ArtifactStore{} }

// Persist writes the one authoritative sealed graph. The adapter Input is a
// construction value and is deliberately not serialized.
func (*ArtifactStore) Persist(runDir string, index Index, _ Input) error {
	encoded, err := Encode(index)
	if err != nil {
		return fmt.Errorf("program index: encode: %w", err)
	}
	writer, err := debugdump.OpenWriter(runDir)
	if err != nil {
		return err
	}
	defer writer.Close()
	return writer.WriteValidatedFile(ArtifactFilename, encoded, func(saved []byte) error {
		if !bytes.Equal(saved, encoded) {
			return fmt.Errorf("program index: saved bytes differ")
		}
		decoded, err := Decode(saved)
		if err != nil {
			return err
		}
		if decoded.SHA256 != index.SHA256 || decoded.Target.ID != index.Target.ID {
			return fmt.Errorf("program index: persisted authority mismatch")
		}
		return nil
	})
}

func ReadFile(filename string) (Index, error) {
	return new(FileReader).ReadFile(filename)
}

// FileReader remains a sequential-consumer convenience, but no longer owns a
// reconstruction cache: reading an Index never invokes New.
type FileReader struct{}

func (*FileReader) Release() {}

func (*FileReader) ReadFile(filename string) (Index, error) {
	wire, err := os.ReadFile(filename)
	if err != nil {
		return Index{}, err
	}
	return Decode(wire)
}
