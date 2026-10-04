package reading

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const InputFilename = "reading-input.json"
const InputVersion = 21

// Input is the complete deterministic boundary before the first atlas call.
// No repository files, manifests, report schema or compiler are needed to read it.
type Input struct {
	Version    int          `json:"version"`
	Repository string       `json:"repository"`
	Revision   string       `json:"revision"`
	Graph      atlas.Graph  `json:"graph"`
	Targets    []TargetMeta `json:"targets"`
	Budget     bool         `json:"budget"`
}

func (input Input) Options() Options {
	return Options{Graph: input.Graph, Targets: input.Targets, Repository: input.Repository, Revision: input.Revision, Budget: input.Budget}
}

// savedInput is Input as reading-input.json holds it: the sealed graph as
// the bytes it was sealed to or, when the places.json beside it holds exactly
// those bytes, as that file named by the SHA-256 of its bytes.
type savedInput struct {
	Version    int             `json:"version"`
	Repository string          `json:"repository"`
	Revision   string          `json:"revision"`
	Graph      json.RawMessage `json:"graph,omitempty"`
	GraphFile  *graphFile      `json:"graph_file,omitempty"`
	Targets    []TargetMeta    `json:"targets"`
	Budget     bool            `json:"budget"`
}

// graphFile is the places.json a reading input's graph is read from.
type graphFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// SaveInput runs before the provider and returns the same sealed graph it saves.
// Ordinary reading and reading a saved input therefore carry the same identity.
// The graph is sealed once: SealedGraph, when the caller already sealed it,
// or EncodeGraph here. The reader still decodes and checks those bytes.
func SaveInput(opts Options) (atlas.Graph, error) {
	graphJSON := opts.SealedGraph
	if graphJSON == nil {
		var err error
		if graphJSON, err = atlas.EncodeGraph(opts.Graph); err != nil {
			return atlas.Graph{}, err
		}
	}
	graph, err := atlas.DecodeGraph(graphJSON)
	if err != nil {
		return atlas.Graph{}, err
	}
	input := savedInput{Version: InputVersion, Repository: opts.Repository, Revision: opts.Revision, Graph: graphJSON, Targets: opts.Targets, Budget: opts.Budget}
	places, err := os.ReadFile(filepath.Join(opts.OwnerRunDir, atlas.GraphFilename))
	switch {
	case err == nil && bytes.Equal(bytes.TrimSuffix(places, []byte("\n")), graphJSON):
		digest := sha256.Sum256(places)
		input.Graph, input.GraphFile = nil, &graphFile{Name: atlas.GraphFilename, SHA256: hex.EncodeToString(digest[:])}
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return atlas.Graph{}, err
	}
	data, err := json.Marshal(input)
	if err != nil {
		return atlas.Graph{}, err
	}
	if err := os.WriteFile(filepath.Join(opts.OwnerRunDir, InputFilename), data, 0o600); err != nil {
		return atlas.Graph{}, err
	}
	return graph, nil
}

// LoadInput accepts exactly the current format. Old runs need a new analysis.
func LoadInput(filename string) (Input, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return Input{}, fmt.Errorf("reading input: %w", err)
	}
	var saved savedInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&saved); err != nil {
		return Input{}, fmt.Errorf("reading input: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Input{}, fmt.Errorf("reading input: expected one JSON object")
	}
	if saved.Version != InputVersion {
		return Input{}, fmt.Errorf("reading input: version %d, want %d; generate a new analysis", saved.Version, InputVersion)
	}
	graphJSON := []byte(saved.Graph)
	if saved.GraphFile != nil {
		if len(saved.Graph) != 0 || saved.GraphFile.Name != atlas.GraphFilename {
			return Input{}, fmt.Errorf("reading input: the graph is written and named, or names another file")
		}
		places, err := os.ReadFile(filepath.Join(filepath.Dir(filename), atlas.GraphFilename))
		if err != nil {
			return Input{}, fmt.Errorf("reading input: %w", err)
		}
		if digest := sha256.Sum256(places); hex.EncodeToString(digest[:]) != saved.GraphFile.SHA256 {
			return Input{}, fmt.Errorf("reading input: %s beside it is not the graph it was saved with; generate a new analysis", atlas.GraphFilename)
		}
		graphJSON = places
	}
	input := Input{Version: saved.Version, Repository: saved.Repository, Revision: saved.Revision, Targets: saved.Targets, Budget: saved.Budget}
	decoder = json.NewDecoder(bytes.NewReader(graphJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input.Graph); err != nil {
		return Input{}, fmt.Errorf("reading input: graph: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Input{}, fmt.Errorf("reading input: graph: expected one JSON object")
	}
	graphJSON, err = json.Marshal(input.Graph)
	if err != nil {
		return Input{}, err
	}
	input.Graph, err = atlas.DecodeGraph(graphJSON)
	if err != nil {
		return Input{}, err
	}
	if input.Repository == "" || input.Revision != input.Graph.Revision || len(input.Targets) == 0 {
		return Input{}, fmt.Errorf("reading input: repository, revision or targets are incomplete")
	}
	targets := make(map[string]bool)
	for _, target := range input.Targets {
		if target.ID == "" || targets[target.ID] || target.Language == "" || target.Kind == "" || target.Root == "" {
			return Input{}, fmt.Errorf("reading input: incomplete or duplicate target %q", target.ID)
		}
		if target.SelectedRole != "" && !atlas.ValidRole(target.SelectedRole) {
			return Input{}, fmt.Errorf("reading input: invalid selected role for target %q", target.ID)
		}
		targets[target.ID] = true
	}
	roles := make(map[string]string, len(input.Targets))
	for _, target := range input.Targets {
		roles[target.ID] = target.SelectedRole
	}
	for _, target := range input.Targets {
		seen := make(map[string]bool)
		for _, id := range target.SharedCode {
			if id == target.ID || seen[id] || roles[id] != atlas.RoleSharedCode {
				return Input{}, fmt.Errorf("reading input: invalid shared code for target %q", target.ID)
			}
			seen[id] = true
		}
	}
	for _, place := range input.Graph.Places {
		for _, id := range place.TargetIDs {
			if !targets[id] {
				return Input{}, fmt.Errorf("reading input: place %s has unknown target %s", place.ID, id)
			}
		}
	}
	return input, nil
}

// StageName resolves a human-sized name to the table stage used by the reader.
func StageName(name string) (string, error) {
	switch name = strings.TrimPrefix(name, "atlas_"); name {
	case "":
		return "", nil
	case "route":
		return "", fmt.Errorf("reading stage route was removed; use --through answer for answers with ordered supporting sources")
	case "directories", "files", "symbols", "api", "operations", "boundaries", "zones", "arrows", "targets", "joints", "learn", "question", "answer":
		return "atlas_" + name, nil
	default:
		return "", fmt.Errorf("unknown reading stage %q; use directories, files, symbols, api, operations, boundaries, zones, arrows, targets, joints, learn, question or answer", name)
	}
}

func validateControls(opts Options) error {
	stage, err := StageName(opts.Through)
	if err != nil {
		return err
	}
	if stage != opts.Through {
		return fmt.Errorf("reading: use canonical stage %s", stage)
	}
	if opts.Prompt != "" && opts.Through == "" {
		return fmt.Errorf("reading: a prompt override needs an explicit through stage")
	}
	if opts.WindowRows < 0 || opts.InputBytes < 0 {
		return fmt.Errorf("reading: table budgets cannot be negative")
	}
	seen := make(map[string]bool)
	for _, question := range opts.Questions {
		if question == "" || question != strings.TrimSpace(question) || seen[question] {
			return fmt.Errorf("reading: questions must be nonempty, trimmed and distinct")
		}
		seen[question] = true
	}
	if (opts.Through == lines.StageQuestion || opts.Through == lines.StageAnswer) && len(opts.Questions) == 0 {
		return fmt.Errorf("reading: question stage requires a question")
	}
	if len(opts.Questions) > 0 && opts.Through != "" && opts.Through != lines.StageQuestion && opts.Through != lines.StageAnswer {
		return fmt.Errorf("reading: question cannot be combined with an earlier through stage")
	}
	return nil
}

// writeWindowResult binds validated model cells to their source IDs before
// graph folding (box cancellation, ranking, membership). Invalid windows
// retain their reason and never acquire model cells.
func (r *reader) writeWindowResult(window table.Window, answers table.Answers, source, reason string) error {
	type row struct {
		ID    string       `json:"id"`
		Path  string       `json:"path,omitempty"`
		Line  int          `json:"line,omitempty"`
		Cells table.Answer `json:"cells,omitempty"`
	}
	result := struct {
		Stage  string `json:"stage"`
		Round  int    `json:"round"`
		Source string `json:"source"`
		Reason string `json:"reason,omitempty"`
		Rows   []row  `json:"rows"`
	}{Stage: window.Stage, Round: window.Round, Source: source, Reason: reason}
	for i, item := range window.Rows {
		value := row{ID: item.ID}
		if place, ok := r.places[item.ID]; ok {
			value.Path, value.Line = place.Path, place.LineNo
		}
		if window.Stage == lines.StageQuestion {
			for _, field := range item.Fields {
				if field.Name == "path" {
					if path, ok := field.Value.(string); ok {
						value.Path = path
					}
				}
			}
		}
		if answers != nil {
			value.Cells = answers[i]
		}
		result.Rows = append(result.Rows, value)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return r.writeWindowFile(window, "result.json", data)
}
