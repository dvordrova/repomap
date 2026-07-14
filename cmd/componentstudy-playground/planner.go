package main

import (
	"context"
	"fmt"
	"time"

	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/deepseek"
)

type plannerExecution struct {
	Client         *deepseek.Client
	RequestJSON    []byte
	RawResponse    []byte
	Result         componentstudy.Result
	DurationMillis int64
}

func preparePlanner(live bool, bundleJSON []byte) (plannerExecution, error) {
	var (
		client *deepseek.Client
		err    error
	)
	if live {
		client, err = deepseek.NewFromEnv()
	} else {
		client, err = deepseek.NewPromptFromEnv()
	}
	if err != nil {
		return plannerExecution{}, fmt.Errorf("componentstudy-playground: configure planner: %w", err)
	}
	requestJSON, err := client.ComponentPlanPromptJSON(bundleJSON)
	if err != nil {
		return plannerExecution{}, fmt.Errorf("componentstudy-playground: build planner request: %w", err)
	}
	return plannerExecution{Client: client, RequestJSON: requestJSON}, nil
}

func executePlanner(
	ctx context.Context,
	execution plannerExecution,
	bundle componentstudy.Bundle,
) (plannerExecution, error) {
	recorder := &recordingPlanner{inner: deepseek.NewComponentPlanner(execution.Client)}
	service := componentstudy.NewService(recorder)
	started := time.Now()
	result, err := service.Plan(ctx, bundle)
	execution.DurationMillis = time.Since(started).Milliseconds()
	execution.RawResponse = recorder.raw
	if err != nil {
		return execution, fmt.Errorf("componentstudy-playground: plan component study: %w", err)
	}
	execution.Result = result
	return execution, nil
}

func replayPlanner(
	execution plannerExecution,
	bundle componentstudy.Bundle,
	raw []byte,
) (plannerExecution, error) {
	started := time.Now()
	result, err := componentstudy.ParsePlan(bundle, raw)
	execution.DurationMillis = time.Since(started).Milliseconds()
	execution.RawResponse = append(execution.RawResponse[:0], raw...)
	if err != nil {
		return execution, fmt.Errorf("componentstudy-playground: replay component study: %w", err)
	}
	execution.Result = result
	return execution, nil
}

type recordingPlanner struct {
	inner componentstudy.Planner
	raw   []byte
}

func (p *recordingPlanner) Plan(ctx context.Context, bundleJSON []byte) ([]byte, error) {
	raw, err := p.inner.Plan(ctx, bundleJSON)
	p.raw = append(p.raw[:0], raw...)
	return raw, err
}
