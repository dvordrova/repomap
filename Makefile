SHELL := /usr/bin/env bash

export

BIN_DIR  ?= .bin
TMP_DIR  ?= tmp
ETCD_REPO ?= ../etcd

.PHONY: help test vet check quality-check build clean smoke etcd-check symbol-check symbol-prompt-experiment gopls-examples gopls-examples-fetch doctor debug-last run run-json run-offline run-flows2 deepseek-check

help: ## Print available targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

test: ## Run go tests
	go test ./...

vet: ## Run go vet
	go vet ./...

check: ## Run tests and vet via reusable script
	./scripts/check.sh

quality-check: ## Replay saved quality tasks without a model call
	./scripts/quality_check.sh

build: ## Build binary into .bin/
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/repomap ./cmd/repomap

clean: ## Remove project-local build, tmp, and debug artifacts
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

doctor: ## Validate configured OpenAI-compatible LLM (no network request)
	go run ./cmd/repomap doctor llm

run: ## Orient ETCD_REPO with the configured OpenAI-compatible LLM
	go run ./cmd/repomap $(ETCD_REPO)

run-json: ## Run full pipeline with JSON output
	go run ./cmd/repomap $(ETCD_REPO) --json | jq .

run-offline: ## Run local extraction only (no model call)
	go run ./cmd/repomap $(ETCD_REPO) --offline

run-flows2: ## Run with 2 opt-in explained flows
	go run ./cmd/repomap $(ETCD_REPO) --flows 2

debug-last: ## Inspect last debug run
	./scripts/debug_last_run.sh

# --- Legacy compat (kept for internal dev) ---

deepseek-check: ## Live configured-LLM call (legacy target name)
	./scripts/deepseek_check.sh $(ETCD_REPO)
