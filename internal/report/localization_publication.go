package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

// DisplayPublication records the selected presentation artifact. It does not
// change any English analysis data, entity identity or source location.
type DisplayPublication struct {
	Language             DisplayLanguage `json:"language"`
	HTMLFilename         string          `json:"html_filename"`
	TranslationsFilename string          `json:"translations_filename"`
	NoModel              bool            `json:"no_model,omitempty"`
}

func ReportHTMLFilename(repository string, language DisplayLanguage) (string, error) {
	language, err := NormalizeDisplayLanguage(string(language))
	if err != nil {
		return "", err
	}
	if language == English {
		return "report.html", nil
	}
	name := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, path.Base(strings.TrimSuffix(repository, "/")))
	name = strings.Trim(name, "-")
	if name == "" {
		name = "repository"
	}
	return "report." + name + "." + string(language) + ".html", nil
}

func (display *DisplayPublication) validate() error {
	if display == nil {
		return nil
	}
	language, err := NormalizeDisplayLanguage(string(display.Language))
	if err != nil || language == English || language != display.Language {
		return fmt.Errorf("report: invalid translated publication language")
	}
	if filepath.Base(display.HTMLFilename) != display.HTMLFilename ||
		validateManifestPath(display.HTMLFilename) != nil ||
		!strings.HasPrefix(display.HTMLFilename, "report.") ||
		!strings.HasSuffix(display.HTMLFilename, "."+string(language)+".html") ||
		display.TranslationsFilename != "report-translations."+string(language)+".json" {
		return fmt.Errorf("report: invalid translated publication filenames")
	}
	return nil
}

func prepareDisplayPublication(repository string, options RenderOptions) (*DisplayPublication, []byte, error) {
	language, err := NormalizeDisplayLanguage(string(options.Language))
	if err != nil {
		return nil, nil, err
	}
	if language == English {
		if options.Translations != nil {
			return nil, nil, fmt.Errorf("report: English publication cannot carry translations")
		}
		return nil, nil, nil
	}
	if options.Translations == nil || options.Translations.Language != language {
		return nil, nil, fmt.Errorf("report: translated publication requires its completed translations")
	}
	filename, err := ReportHTMLFilename(repository, language)
	if err != nil {
		return nil, nil, err
	}
	display := &DisplayPublication{Language: language, HTMLFilename: filename,
		TranslationsFilename: "report-translations." + string(language) + ".json", NoModel: options.NoModel}
	raw, err := json.MarshalIndent(options.Translations, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return display, append(raw, '\n'), nil
}

func (receipt RunReceipt) HTMLFilename() string {
	if receipt.manifest.Display != nil {
		return receipt.manifest.Display.HTMLFilename
	}
	return "report.html"
}

// RenderOptions restores presentation independently of the canonical report.
// The server adds its transient source links without changing these values.
func (receipt RunReceipt) RenderOptions() RenderOptions {
	return receipt.renderOptions
}

func loadDisplayOptions(runDir string, display *DisplayPublication) (RenderOptions, error) {
	if display == nil {
		return RenderOptions{}, nil
	}
	if err := display.validate(); err != nil {
		return RenderOptions{}, err
	}
	raw, err := os.ReadFile(filepath.Join(runDir, display.TranslationsFilename))
	if err != nil {
		return RenderOptions{}, fmt.Errorf("report: read translations: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var translations DisplayTranslations
	if err := decoder.Decode(&translations); err != nil {
		return RenderOptions{}, fmt.Errorf("report: decode translations: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return RenderOptions{}, fmt.Errorf("report: trailing translation data")
	}
	if translations.Version != DisplayTextVersion || translations.Language != display.Language {
		return RenderOptions{}, fmt.Errorf("report: translation artifact does not match the publication")
	}
	return RenderOptions{Language: display.Language, Translations: &translations, NoModel: display.NoModel}, nil
}
