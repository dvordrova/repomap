package run

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/report"
)

func runRender(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("repomap render", flag.ContinueOnError)
	fs.SetOutput(stdout)
	output := fs.String("output", "", "HTML file to write using the current report templates")
	fs.Usage = func() {
		fmt.Fprintln(stdout, "Usage: repomap render RUN_DIR --output FILE.html")
		fmt.Fprintln(stdout, "Render a saved report and its saved translations. No analysis, cache or provider calls.")
		fs.PrintDefaults()
	}
	// Match the ordinary command's repository-first form; flags may also precede it.
	var runDir string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		runDir, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if runDir == "" && fs.NArg() == 1 {
		runDir = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return fmt.Errorf("render: use repomap render RUN_DIR --output FILE.html")
	}
	if runDir == "" || *output == "" || strings.ToLower(filepath.Ext(*output)) != ".html" {
		return fmt.Errorf("render: a saved run directory and --output FILE.html are required")
	}
	filename, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	rendered, err := report.RenderSavedHTML(runDir)
	if err != nil {
		return err
	}
	// Publish only the completed HTML. A render or write failure leaves the
	// previous page, canonical report and all other run artifacts untouched.
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".repomap-render-*.html")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(rendered); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary.Name(), filename); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Rendered "+filename+" (saved analysis; 0 provider requests)")
	return nil
}
