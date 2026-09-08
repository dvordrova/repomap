package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// RenderSavedHTML applies the current ordinary templates to a completed saved
// report and its saved display translation. It reads no repository, cache or
// provider configuration and never regenerates missing analysis or translation.
func RenderSavedHTML(runDir string) ([]byte, error) {
	receipt, err := ReadRunReceipt(runDir)
	if err != nil {
		return nil, err
	}
	manifest := receipt.Manifest()
	data := *receipt.Data()
	data.GitHubSourceLinks, data.GitLabSourceLinks, err = ordinaryHTMLSourceLinks(
		data.CapturedRevision, OrdinaryReportHTMLAuthority{
			StandaloneSource: manifest.StandaloneSource,
			RepositoryRoot:   manifest.RepositoryState.Identity,
			AnalysisRoot:     manifest.AnalysisRoot,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("report: restore source links: %w", err)
	}
	if manifest.StandaloneSource != nil {
		data.UnavailableSourcePaths = manifest.StandaloneSource.UnavailablePaths
	}
	options := receipt.RenderOptions()
	options.ReportSHA256 = receipt.savedReportSHA256
	options.LocalRoots = []string{receipt.RunDir(), manifest.AnalysisRoot, manifest.RepositoryState.Identity}
	if options.Translations != nil {
		page, err := PreparePage(&data, options)
		if err != nil {
			return nil, err
		}
		catalog := page.TextCatalog()
		if options.Translations.CatalogSHA256 != catalog.SHA256 {
			// A new copy of existing prose can change traversal order. Reuse
			// only a complete, validated catalogue with the exact same entries.
			raw, err := os.ReadFile(filepath.Join(receipt.RunDir(), "report-display-texts.json"))
			if err != nil {
				return nil, fmt.Errorf("report: read saved display text catalogue: %w", err)
			}
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			var saved DisplayTextCatalog
			if err := decoder.Decode(&saved); err != nil {
				return nil, fmt.Errorf("report: decode saved display text catalogue: %w", err)
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				return nil, fmt.Errorf("report: trailing saved display text catalogue data")
			}
			translations, err := rebindDisplayTranslations(saved, catalog, *options.Translations)
			if err != nil {
				return nil, err
			}
			options.Translations = &translations
		}
	}
	return RenderHTMLWithOptions(&data, options)
}
