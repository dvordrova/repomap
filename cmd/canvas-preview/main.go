// Command canvas-preview serves one saved report fixture without repository
// analysis, editor authority, or provider access. It is a development inner
// loop for the architecture canvas rather than a product entrypoint.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/report"
)

const defaultFixture = "internal/report/testdata/canvas/restic-backup-v2.json"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("canvas-preview", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	fixturePath := flags.String("fixture", defaultFixture, "saved report fixture")
	port := flags.Int("port", 0, "local preview port (default: random)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("canvas preview: unexpected arguments: %v", flags.Args())
	}
	if *port < 0 || *port > 65535 {
		return fmt.Errorf("canvas preview: port must be between 0 and 65535")
	}

	html, err := loadPreviewHTML(*fixturePath)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(*port))
	if err != nil {
		return fmt.Errorf("canvas preview: listen: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		_, _ = io.Copy(writer, bytes.NewReader(html))
	})
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	address := listener.Addr().(*net.TCPAddr)
	fmt.Fprintf(stdout, "Canvas preview: http://127.0.0.1:%d/\n", address.Port)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("canvas preview: serve: %w", err)
	}
	return nil
}

func loadPreviewHTML(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("canvas preview: read fixture: %w", err)
	}
	var saved report.ReportData
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("canvas preview: decode fixture: %w", err)
	}
	if saved.ArchitectureCanvas == nil {
		return nil, fmt.Errorf("canvas preview: fixture has no architecture_canvas")
	}
	if saved.ArchitectureCanvas.Version != report.ArchitectureCanvasVersion {
		return nil, fmt.Errorf(
			"canvas preview: architecture_canvas version %d, want %d",
			saved.ArchitectureCanvas.Version,
			report.ArchitectureCanvasVersion,
		)
	}
	html, err := report.RenderHTML(&saved)
	if err != nil {
		return nil, fmt.Errorf("canvas preview: render fixture: %w", err)
	}
	return html, nil
}
