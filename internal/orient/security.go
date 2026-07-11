package orient

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/secretscan"
)

func validateOrientationBundleForRemote(bundle llmbundle.Bundle) error {
	if kind, found := secretscan.Detect(bundle.ReadmeExcerpt); found {
		return fmt.Errorf("orientation: %s detected in readme excerpt; refusing remote use", kind)
	}
	for _, signal := range bundle.SourceSignals {
		if kind, found := secretscan.Detect(signal.Snippet); found {
			return fmt.Errorf(
				"orientation: %s detected in source signal at %s:%d; refusing remote use",
				kind,
				signal.Path,
				signal.Line,
			)
		}
	}
	return nil
}

func validateFlowBundleForRemote(bundle flowexplain.FlowBundle) error {
	for _, signal := range bundle.SourceSignals {
		if kind, found := secretscan.Detect(signal.Snippet); found {
			return fmt.Errorf(
				"flow %q: %s detected in source signal at %s:%d; refusing remote use",
				bundle.FlowSeed.Name,
				kind,
				signal.Path,
				signal.Line,
			)
		}
	}
	return nil
}

func validateProviderOutputForStorage(scope string, data []byte) error {
	if kind, found := secretscan.Detect(string(data)); found {
		return fmt.Errorf("%s: %s detected in model output; refusing to retain or present it", scope, kind)
	}
	return nil
}
