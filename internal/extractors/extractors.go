// Package extractors runs built-in and external producers through one JSON
// contract. Producers know a tool or framework; facts owns repository identity.
package extractors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/facts"
)

const Version = 1
const ConfigFilename = ".repomap.json"
const ArtifactFilename = "extractions.json"

// Request is written to a command's stdin. Root is a local filesystem path,
// not model input. Commands run in Root and can be written in any language.
type Request struct {
	Version int             `json:"version"`
	Root    string          `json:"root"`
	Files   []string        `json:"files"`
	Options json.RawMessage `json:"options"`
}

type Response struct {
	Version     int                    `json:"version"`
	Nodes       []facts.ExtractionNode `json:"nodes"`
	Links       []facts.ExtractionLink `json:"links"`
	Diagnostics []facts.Diagnostic     `json:"diagnostics"`
}

type Command struct {
	Name    string          `json:"name"`
	Command []string        `json:"command"`
	Options json.RawMessage `json:"options,omitempty"`
}

type config struct {
	Version    int       `json:"version"`
	Extractors []Command `json:"extractors"`
}

// Exchange is the exact replayable evidence before normalization. Keeping
// the command's stderr here lets plugin authors debug without mixing logs
// into the JSON protocol on stdout.
type Exchange struct {
	Name    string   `json:"name"`
	Command []string `json:"command,omitempty"`
	Request Request  `json:"request"`
	Stdout  string   `json:"stdout"`
	Stderr  string   `json:"stderr,omitempty"`
}

type Result struct {
	Extractions []facts.Extraction `json:"extractions"`
	Exchanges   []Exchange         `json:"exchanges"`
}

func Run(ctx context.Context, root string, repository *corpus.Corpus) (Result, error) {
	result := Result{Extractions: []facts.Extraction{}, Exchanges: []Exchange{}}
	if repository == nil {
		return result, nil
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return result, err
	}
	request := Request{Version: Version, Root: root, Files: []string{}, Options: json.RawMessage(`{}`)}
	for _, entry := range repository.Entries() {
		request.Files = append(request.Files, entry.Path)
	}
	configuration := config{Version: Version}
	if id, ok := repository.ID(ConfigFilename); ok {
		content, err := repository.ReadFileAll(id)
		if err != nil {
			return result, err
		}
		if err := decode(content.Bytes, &configuration); err != nil {
			return result, fmt.Errorf("%s: %w", ConfigFilename, err)
		}
		if configuration.Version != Version {
			return result, fmt.Errorf("%s: unsupported version %d", ConfigFilename, configuration.Version)
		}
	}
	names := map[string]bool{"sqlc": true}
	for _, command := range configuration.Extractors {
		if strings.TrimSpace(command.Name) == "" || names[command.Name] || len(command.Command) == 0 || command.Command[0] == "" {
			return result, fmt.Errorf("%s: each extractor needs a unique name and nonempty command", ConfigFilename)
		}
		names[command.Name] = true
	}
	// Built-ins produce the same bytes and cross the same decoding seam as an
	// external command. sqlc has no privileged way to append facts.
	response, err := SQLC(ctx, request)
	if err != nil {
		return result, err
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return result, err
	}
	if err := result.accept(Exchange{Name: "sqlc", Request: request, Stdout: string(raw)}); err != nil {
		return result, err
	}
	for _, command := range configuration.Extractors {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		input := request
		if len(command.Options) > 0 {
			input.Options = command.Options
		}
		data, err := json.Marshal(input)
		if err != nil {
			return result, err
		}
		process := exec.CommandContext(ctx, command.Command[0], command.Command[1:]...)
		process.Dir, process.Stdin = root, bytes.NewReader(data)
		var stderr bytes.Buffer
		process.Stderr = &stderr
		output, err := process.Output()
		exchange := Exchange{Name: command.Name, Command: command.Command, Request: input, Stdout: string(output), Stderr: stderr.String()}
		if err != nil {
			result.Exchanges = append(result.Exchanges, exchange)
			return result, fmt.Errorf("extractor %s: command failed: %w; %s", command.Name, err, strings.TrimSpace(stderr.String()))
		}
		if err := result.accept(exchange); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (result *Result) accept(exchange Exchange) error {
	result.Exchanges = append(result.Exchanges, exchange)
	var response Response
	if err := decode([]byte(exchange.Stdout), &response); err != nil {
		return fmt.Errorf("extractor %s: invalid response: %w", exchange.Name, err)
	}
	if response.Version != Version || response.Nodes == nil || response.Links == nil {
		return fmt.Errorf("extractor %s: expected version %d, nodes and links arrays", exchange.Name, Version)
	}
	result.Extractions = append(result.Extractions, facts.Extraction{Name: exchange.Name, Nodes: response.Nodes, Links: response.Links, Diagnostics: response.Diagnostics})
	return nil
}

func decode(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected one JSON object")
	}
	return nil
}
