package run

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/dvordrova/repomap/internal/repoconfig"
)

func runConf(args []string, stdout, stderr io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runConfWithContext(ctx, args, stdout, stderr)
}

func runConfWithContext(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("repomap conf", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: repomap conf [repository]")
		fmt.Fprintln(stderr, "Create .repomap.conf if absent, then open it in the configured editor.")
		fmt.Fprintln(stderr, "Defaults to the current directory. No analysis runs; existing settings are preserved.")
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("conf: use repomap conf [repository]")
	}
	root := "."
	if fs.NArg() == 1 {
		root = fs.Arg(0)
	}
	root, err := resolveAnalysisRoot(root)
	if err != nil {
		return err
	}
	created, err := repoconfig.Ensure(root)
	if err != nil {
		return err
	}
	filename := filepath.Join(root, repoconfig.Filename)
	if created {
		fmt.Fprintln(stdout, "Created "+filename)
	} else {
		fmt.Fprintln(stdout, "Opening "+filename)
	}
	config, err := repoconfig.Load(root)
	if err != nil {
		return err
	}
	return config.Open(ctx, root, filename, 1, 1, os.Stdin, stdout, stderr)
}
