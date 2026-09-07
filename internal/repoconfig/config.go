// Package repoconfig owns settings local to the selected repository directory.
package repoconfig

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"go.yaml.in/yaml/v3"
)

const Filename = ".repomap.conf"

const DefaultContents = `# Settings for this repository only. YAML; Git is not required.
# Editor command and its arguments. Each item stays one argument, even with spaces.
# Templates receive .File (absolute path), .Line and .Column (one-based).
editor: [code, --goto, '{{ .File }}:{{ .Line }}:{{ .Column }}']

# Optional questions to explore using the shared repository analysis.
# questions:
#   - How do I run this project?
#   - Where is state stored and changed?

# Per-target build variants are planned, not supported yet.
`

type Config struct {
	Editor    []string `yaml:"editor"`
	Questions []string `yaml:"questions,omitempty"`
}

func defaults() Config {
	return Config{Editor: []string{"code", "--goto", "{{ .File }}:{{ .Line }}:{{ .Column }}"}}
}

// Load reads exactly root/Filename. It never searches parents or the home directory.
func Load(root string) (Config, error) {
	filename := filepath.Join(root, Filename)
	data, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return defaults(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", filename, err)
	}
	config := defaults()
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("read %s: %w", filename, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("read %s: expected one YAML document", filename)
	}
	if _, err := config.Command(filepath.Join(root, Filename), 1, 1); err != nil {
		return Config{}, fmt.Errorf("read %s: %w", filename, err)
	}
	for i, question := range config.Questions {
		config.Questions[i] = strings.TrimSpace(question)
		if config.Questions[i] == "" {
			return Config{}, fmt.Errorf("read %s: question %d is empty", filename, i+1)
		}
	}
	return config, nil
}

// Ensure creates the initial file once; an existing file is never rewritten.
func Ensure(root string) (bool, error) {
	filename := filepath.Join(root, Filename)
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create %s: %w", filename, err)
	}
	_, writeErr := io.WriteString(file, DefaultContents)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return false, fmt.Errorf("write %s: %w", filename, err)
	}
	return true, nil
}

// Command expands each argument independently. File names remain data; this
// launcher does not interpret a shell command or split a substituted path.
func (c Config) Command(file string, line, column int) ([]string, error) {
	if len(c.Editor) == 0 {
		return nil, fmt.Errorf("editor must contain an executable and its arguments")
	}
	location := struct {
		File         string
		Line, Column int
	}{file, max(1, line), max(1, column)}
	args := make([]string, len(c.Editor))
	for i, input := range c.Editor {
		tmpl, err := template.New("editor").Option("missingkey=error").Parse(input)
		if err != nil {
			return nil, fmt.Errorf("editor argument %d: %w", i+1, err)
		}
		var value strings.Builder
		if err := tmpl.Execute(&value, location); err != nil {
			return nil, fmt.Errorf("editor argument %d: %w", i+1, err)
		}
		args[i] = value.String()
	}
	if strings.TrimSpace(args[0]) == "" {
		return nil, fmt.Errorf("editor executable is empty")
	}
	return args, nil
}

func (c Config) Open(ctx context.Context, root, file string, line, column int, stdin io.Reader, stdout, stderr io.Writer) error {
	args, err := c.Command(file, line, column)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = root
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("open %s using %s: %w (editor is configured in %s)", file, args[0], err, filepath.Join(root, Filename))
	}
	return nil
}
