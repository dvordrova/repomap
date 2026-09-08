package run

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
)

func runReplay(args []string, stdout, stderr io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	output := newRunOutput(stderr)
	err := runReplayConfigured(ctx, args, stdout, defaultTargetPortfolioProviderFactory, output)
	if err != nil {
		writeRunOutputError(output, err)
	}
	return err
}

func runReplayWithProvider(ctx context.Context, args []string, stdout, stderr io.Writer, factory func() (llm.Provider, error)) error {
	return runReplayConfigured(ctx, args, stdout, factory, newRunOutput(stderr))
}

func runReplayConfigured(ctx context.Context, args []string, stdout io.Writer, factory targetPortfolioProviderFactory, output *runOutput) error {
	fs := flag.NewFlagSet("repomap replay", flag.ContinueOnError)
	fs.SetOutput(stdout)
	filename := fs.String("file", "", "saved .llm-cache/payloads/<request-sha>.json")
	cacheRoot := fs.String("debug-dir", defaultDebugDir(), "shared model cache directory")
	fs.Usage = func() {
		fmt.Fprintln(stdout, "Usage: repomap replay --file REQUEST.json")
		fmt.Fprintln(stdout, "Send the exact saved request through the configured client and refresh its cached answer.")
		fmt.Fprintln(stdout, "Assistant content goes to stdout; timing and token usage go to stderr. No report is generated.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *filename == "" || fs.NArg() != 0 {
		return fmt.Errorf("replay: use repomap replay --file REQUEST.json")
	}
	raw, err := os.ReadFile(*filename)
	if err != nil {
		return fmt.Errorf("replay: read request: %w", err)
	}
	prepared, err := deepseek.PrepareReplay(raw)
	if err != nil {
		return err
	}
	provider, err := providerFactoryWithOutput(factory, output)()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*cacheRoot, 0o700); err != nil {
		return err
	}
	// The configured client still owns all transport policy. No repository
	// analysis or stage-specific schema validation runs here.
	completion, callErr := llm.ReplayJSON(ctx, llm.Executor{RootDir: *cacheRoot, Enabled: true}, provider, prepared)
	if len(completion.Response) > 0 {
		if _, err := stdout.Write(completion.Response); err != nil {
			return fmt.Errorf("replay: write response: %w", err)
		}
		if completion.Response[len(completion.Response)-1] != '\n' {
			if _, err := io.WriteString(stdout, "\n"); err != nil {
				return fmt.Errorf("replay: write response: %w", err)
			}
		}
	}
	metrics := completion.Metrics
	requestPath, err := llm.SavePayload(*cacheRoot, raw)
	if err != nil {
		return err
	}
	output.Stage("Request", requestPath)
	if len(completion.Response) > 0 {
		responsePath, err := llm.SavePayload(*cacheRoot, completion.Response)
		if err != nil {
			return err
		}
		output.Stage("Response", responsePath)
	}
	output.Stage("Replay", fmt.Sprintf("%s; %d attempts; finish=%s; input=%d output=%d reasoning=%d tokens",
		metrics.Latency, metrics.Attempts, completion.FinishReason,
		metrics.InputTokens, metrics.OutputTokens, metrics.ReasoningTokens))
	return callErr
}
