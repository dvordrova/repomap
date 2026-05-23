package orient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/snapshot"
)

type Options struct {
	RepoPath            string
	SnapshotOnly        bool
	MaxReadmeBytes      int
	MaxTreeLines        int
	MaxInterestingFiles int
}

func Run(ctx context.Context, opts Options) ([]byte, error) {
	s, err := snapshot.Build(snapshot.Options{
		RepoPath:            opts.RepoPath,
		MaxReadmeBytes:      opts.MaxReadmeBytes,
		MaxTreeLines:        opts.MaxTreeLines,
		MaxInterestingFiles: opts.MaxInterestingFiles,
	})
	if err != nil {
		return nil, err
	}

	snapshotJSON, err := s.JSON()
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	if opts.SnapshotOnly {
		return append(snapshotJSON, '\n'), nil
	}

	client, err := deepseek.NewFromEnv()
	if err != nil {
		return nil, err
	}
	raw, err := client.Orient(ctx, snapshotJSON)
	if err != nil {
		return nil, err
	}

	var validate json.RawMessage
	if err := json.Unmarshal(raw, &validate); err != nil {
		return nil, fmt.Errorf("deepseek response is not valid JSON:\n%s", string(raw))
	}

	var pretty json.RawMessage
	if err := json.Unmarshal(raw, &pretty); err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("pretty print json: %w", err)
	}
	return append(out, '\n'), nil
}
