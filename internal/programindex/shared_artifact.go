package programindex

import (
	"os"
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

// Persist writes the one authoritative sealed graph exactly as Persist does.
// The adapter Input is a construction value and is deliberately not
// serialized.
func (*ArtifactStore) Persist(runDir string, index Index, _ Input) error {
	return Persist(runDir, ArtifactFilename, index)
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
