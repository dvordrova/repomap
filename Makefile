SHELL := /usr/bin/env bash

-include .env
export

BIN_DIR  ?= .bin
TMP_DIR  ?= tmp
ETCD_REPO ?= ../etcd

.PHONY: help test vet check build clean smoke etcd-check symbol-check symbol-prompt-experiment gopls-examples gopls-examples-fetch debug-last run run-json run-offline

help: ## Print available targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

test: ## Run go tests
	go test ./...

vet: ## Run go vet
	go vet ./...

check: ## Run tests and vet via reusable script
	./scripts/check.sh

build: ## Build binary into .bin/
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/repomap ./cmd/repomap

clean: ## Remove build/tmp/debug artifacts
	./scripts/clean.sh

smoke: ## Smoke test via reusable script (no network)
	./scripts/smoke.sh

etcd-check: ## Validate against etcd clone via reusable script
	./scripts/etcd_check.sh $(ETCD_REPO)

symbol-check: ## Build and inspect an offline DeepSeek prompt for etcd kvServer.Put
	./scripts/symbol_check.sh $(ETCD_REPO) kvServer.Put

symbol-prompt-experiment: ## Call DeepSeek for a versioned symbol prompt (LABEL=x FORMAT=json|tagged)
	./scripts/symbol_prompt_experiment.sh "$(LABEL)" "$(ETCD_REPO)" kvServer.Put "$(or $(FORMAT),tagged)"

gopls-examples: ## Analyze available example repos with the isolated gopls playground
	./scripts/gopls_examples.sh

gopls-examples-fetch: ## Fetch missing example repos, then generate gopls evidence artifacts
	./scripts/gopls_examples.sh --fetch

# --- Primary UX targets ---

run: ## Run full pipeline against ETCD_REPO (needs DEEPSEEK_API_KEY)
	go run ./cmd/repomap $(ETCD_REPO) --flows 10

run-json: ## Run full pipeline with JSON output
	go run ./cmd/repomap $(ETCD_REPO) --json | jq .

run-offline: ## Run offline (no API key, local bundles only)
	go run ./cmd/repomap $(ETCD_REPO) --offline

run-flows2: ## Run with 2 explained flows
	go run ./cmd/repomap $(ETCD_REPO) --flows 2

debug-last: ## Inspect last debug run
	./scripts/debug_last_run.sh

# --- Legacy compat (kept for internal dev) ---

deepseek-check: ## Full DeepSeek call via reusable script
	./scripts/deepseek_check.sh $(ETCD_REPO)
