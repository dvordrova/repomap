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

CANVAS_FIXTURE ?= internal/report/testdata/canvas/restic-backup-v2.json
CANVAS_PORT ?= 0

.PHONY: canvas-preview
canvas-preview: ## Serve a saved architecture canvas fixture without analysis or provider access
	go run ./cmd/canvas-preview --fixture "$(CANVAS_FIXTURE)" --port "$(CANVAS_PORT)"

.PHONY: elkjs-asset-check elkjs-asset-refresh
elkjs-asset-check: ## Verify the pinned vendored ELK.js browser asset (offline)
	@printf '%s  %s\n' '20dd2114d683ce758b3ce19bcc56e28a504a617b0d280f760407c37314631d0e' 'internal/report/assets/elkjs/elk.bundled.js' | shasum -a 256 -c -
	@printf '%s  %s\n' '89591d4578fb1ebd91501312a3d25f021bd865a2e436641c1cf7b1bc7e3c1617' 'internal/report/assets/elkjs/LICENSE.md' | shasum -a 256 -c -

elkjs-asset-refresh: ## Refresh pinned ELK.js from the verified npm tarball (no npm required)
	@set -euo pipefail; \
		tmp_dir="$$(mktemp -d)"; \
		trap 'rm -rf "$$tmp_dir"' EXIT; \
		curl -fsSL 'https://registry.npmjs.org/elkjs/-/elkjs-0.11.1.tgz' -o "$$tmp_dir/elkjs.tgz"; \
		printf '%s  %s\n' '83973e243b44842353717427ee8ea1880d688ebe79634d4017e3cc30f3214a4a' "$$tmp_dir/elkjs.tgz" | shasum -a 256 -c -; \
		tar -xzf "$$tmp_dir/elkjs.tgz" -C "$$tmp_dir" package/lib/elk.bundled.js package/LICENSE.md; \
		mkdir -p 'internal/report/assets/elkjs'; \
		install -m 0644 "$$tmp_dir/package/lib/elk.bundled.js" 'internal/report/assets/elkjs/elk.bundled.js'; \
		install -m 0644 "$$tmp_dir/package/LICENSE.md" 'internal/report/assets/elkjs/LICENSE.md'
	@$(MAKE) --no-print-directory elkjs-asset-check

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
