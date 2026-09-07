package reportserver

import (
	"context"
	"os"
	"time"

	"github.com/dvordrova/repomap/internal/repoconfig"
)

// Bind settings once for this server's lifetime. A configuration error affects
// the source action, not the availability of the generated report.
func configuredEditor(root string, config *repoconfig.Config, logf func(string, ...any)) OpenFileFunc {
	var loadErr error
	if config == nil {
		loaded, err := repoconfig.Load(root)
		config, loadErr = &loaded, err
	}
	return func(ctx context.Context, file string, line, column int) error {
		if loadErr != nil {
			return loadErr
		}
		started := time.Now()
		err := config.Open(ctx, root, file, line, column, os.Stdin, os.Stdout, os.Stderr)
		if logf != nil {
			outcome := "opened"
			if err != nil {
				outcome = "error"
			}
			logf("editor dispatch outcome=%s elapsed_ms=%d", outcome, time.Since(started).Milliseconds())
		}
		return err
	}
}
