SHELL := /usr/bin/env bash

-include .env
export

BIN_DIR  ?= .bin
TMP_DIR  ?= tmp
ETCD_REPO ?= ../etcd
RUN_ARGS ?=

.PHONY: help test vet check quality-check build clean smoke etcd-check friend-check symbol-check symbol-prompt-experiment source-prompt-experiment component-study-preview component-study-live component-study-replay component-probe component-probe-frontier component-teach-preview component-teach-live component-teach-replay research-trail-replay flowproof-replay research-budget-check pyright-fixture gopls-examples gopls-examples-fetch doctor doctor-check generic-deepseek-doctor debug-last serve run run-json run-offline run-flows2 deepseek-check

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

SURFACE_REPO ?= internal/experiment/surfacediscovery/testdata/direct
SURFACE_OUT ?= $(TMP_DIR)/surface-discovery

.PHONY: surface-check surface-playground caddy-surface-check
surface-check: ## Run config-driven Go surface discovery fixtures
	go test ./internal/semantics/catalog ./internal/experiment/surfacediscovery ./internal/surfacebridge

surface-playground: ## Emit local surface JSON and Markdown for SURFACE_REPO
	go run ./cmd/surface-discovery-playground --repo "$(SURFACE_REPO)" --out "$(SURFACE_OUT)"

caddy-surface-check: ## Compare deterministic surface discovery with a nearby Caddy checkout
	./scripts/caddy_surface_check.sh "$(CADDY_REPO)"

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

friend-check: ## Replay the one-request browser onboarding journey
	./scripts/friend_check.sh

symbol-check: ## Build and inspect an offline DeepSeek prompt for etcd kvServer.Put
	./scripts/symbol_check.sh $(ETCD_REPO) kvServer.Put

symbol-prompt-experiment: ## Call DeepSeek for a versioned symbol prompt (LABEL=x FORMAT=json|tagged)
	./scripts/symbol_prompt_experiment.sh "$(LABEL)" "$(ETCD_REPO)" kvServer.Put "$(or $(FORMAT),tagged)"

source-prompt-experiment: ## Call DeepSeek for a source-stage prompt (LABEL=x SYMBOL=kvServer.Put)
	./scripts/source_prompt_experiment.sh "$(LABEL)" "$(ETCD_REPO)" "$(or $(SYMBOL),kvServer.Put)"

COMPONENT_STUDY_RUN ?= $(HOME)/Library/Caches/repomap/runs/20260711-011750-soft-serve
COMPONENT_STUDY_COMPONENT ?= SSH server
COMPONENT_STUDY_ANCHOR ?= cmd/soft/serve/serve.go
COMPONENT_STUDY_GOAL ?= After soft serve, how are configuration and backend initialized, which long-running services start, how do failures converge, and how is shutdown performed?
COMPONENT_STUDY_OUT ?= $(TMP_DIR)/deeper/soft-serve-startup
COMPONENT_STUDY_RESPONSE ?=
COMPONENT_STUDY_RESPONSE_PROMPT ?= unknown

component-study-preview: ## Build the bounded Soft Serve deeper-planner artifacts without a model call
	go run ./cmd/componentstudy-playground --run-dir "$(COMPONENT_STUDY_RUN)" --component "$(COMPONENT_STUDY_COMPONENT)" --anchor "$(COMPONENT_STUDY_ANCHOR)" --goal "$(COMPONENT_STUDY_GOAL)" --out-dir "$(COMPONENT_STUDY_OUT)"

component-study-live: ## Run one configured model call over the bounded deeper-planner bundle
	go run ./cmd/componentstudy-playground --run-dir "$(COMPONENT_STUDY_RUN)" --component "$(COMPONENT_STUDY_COMPONENT)" --anchor "$(COMPONENT_STUDY_ANCHOR)" --goal "$(COMPONENT_STUDY_GOAL)" --out-dir "$(COMPONENT_STUDY_OUT)" --live

component-study-replay: ## Replay COMPONENT_STUDY_RESPONSE through the current local planner parser
	@test -n "$(COMPONENT_STUDY_RESPONSE)" || (echo "COMPONENT_STUDY_RESPONSE is required" >&2; exit 2)
	go run ./cmd/componentstudy-playground --run-dir "$(COMPONENT_STUDY_RUN)" --component "$(COMPONENT_STUDY_COMPONENT)" --anchor "$(COMPONENT_STUDY_ANCHOR)" --goal "$(COMPONENT_STUDY_GOAL)" --out-dir "$(COMPONENT_STUDY_OUT)" --response-file "$(COMPONENT_STUDY_RESPONSE)" --response-prompt-version "$(COMPONENT_STUDY_RESPONSE_PROMPT)"

COMPONENT_PROBE_RUN ?= $(HOME)/Library/Caches/repomap/runs/20260711-011750-soft-serve
COMPONENT_PROBE_STUDY_BUNDLE ?= $(TMP_DIR)/deeper/soft-serve-startup-v3-replay/planner/bundle.json
COMPONENT_PROBE_PLAN ?= $(TMP_DIR)/deeper/soft-serve-startup-v3-replay/planner/plan.json
COMPONENT_PROBE_OUT ?= $(TMP_DIR)/deeper/soft-serve-startup-probe-round1
COMPONENT_PROBE_PREVIOUS ?= $(COMPONENT_PROBE_OUT)/probe/bundle.json
COMPONENT_PROBE_FRONTIER_ID ?=
COMPONENT_PROBE_FRONTIER_OUT ?= $(TMP_DIR)/deeper/soft-serve-startup-probe-round2

component-probe: ## Probe the saved Soft Serve primary question with bounded local gopls evidence
	go run ./cmd/componentprobe-playground --run-dir "$(COMPONENT_PROBE_RUN)" --study-bundle "$(COMPONENT_PROBE_STUDY_BUNDLE)" --plan "$(COMPONENT_PROBE_PLAN)" --out-dir "$(COMPONENT_PROBE_OUT)"

component-probe-frontier: ## Follow one opaque frontier ID from a saved round-1 component probe
	@test -n "$(COMPONENT_PROBE_FRONTIER_ID)" || (echo "COMPONENT_PROBE_FRONTIER_ID is required" >&2; exit 2)
	go run ./cmd/componentprobe-playground --run-dir "$(COMPONENT_PROBE_RUN)" --previous-probe "$(COMPONENT_PROBE_PREVIOUS)" --frontier-id "$(COMPONENT_PROBE_FRONTIER_ID)" --out-dir "$(COMPONENT_PROBE_FRONTIER_OUT)"

COMPONENT_TEACH_RUN ?= $(HOME)/Library/Caches/repomap/runs/20260711-072351-pebble
COMPONENT_TEACH_ROUND1 ?= $(TMP_DIR)/deeper/pebble-batch-commit-probe-round1/probe/bundle.json
COMPONENT_TEACH_ROUND2 ?= $(TMP_DIR)/deeper/pebble-batch-commit-probe-round2/probe/bundle.json
COMPONENT_TEACH_OUT ?= $(TMP_DIR)/deeper/pebble-batch-commit-teacher
COMPONENT_TEACH_RESPONSE ?=
COMPONENT_TEACH_RESPONSE_PROMPT ?= unknown

component-teach-preview: ## Build the bounded Pebble teacher request without a model call
	go run ./cmd/componentteach-playground --run-dir "$(COMPONENT_TEACH_RUN)" --probe-round1 "$(COMPONENT_TEACH_ROUND1)" --probe-round2 "$(COMPONENT_TEACH_ROUND2)" --out-dir "$(COMPONENT_TEACH_OUT)"

component-teach-live: ## Make one configured grounded teacher call for the Pebble probe chain
	go run ./cmd/componentteach-playground --run-dir "$(COMPONENT_TEACH_RUN)" --probe-round1 "$(COMPONENT_TEACH_ROUND1)" --probe-round2 "$(COMPONENT_TEACH_ROUND2)" --out-dir "$(COMPONENT_TEACH_OUT)" --live

component-teach-replay: ## Replay COMPONENT_TEACH_RESPONSE through the current grounded parser
	@test -n "$(COMPONENT_TEACH_RESPONSE)" || (echo "COMPONENT_TEACH_RESPONSE is required" >&2; exit 2)
	go run ./cmd/componentteach-playground --run-dir "$(COMPONENT_TEACH_RUN)" --probe-round1 "$(COMPONENT_TEACH_ROUND1)" --probe-round2 "$(COMPONENT_TEACH_ROUND2)" --out-dir "$(COMPONENT_TEACH_OUT)" --response-file "$(COMPONENT_TEACH_RESPONSE)" --response-prompt-version "$(COMPONENT_TEACH_RESPONSE_PROMPT)"

RESEARCH_TRAIL_CASE ?= $(TMP_DIR)/deeper/pebble-batch-commit-teacher/case.json
RESEARCH_TRAIL_STUDY_BUNDLE ?= $(TMP_DIR)/deeper/pebble-batch-commit-v3-replay/planner/bundle.json
RESEARCH_TRAIL_PLAN ?= $(TMP_DIR)/deeper/pebble-batch-commit-v3-replay/planner/plan.json
RESEARCH_TRAIL_PLAN_DIAGNOSTICS ?= $(TMP_DIR)/deeper/pebble-batch-commit-v3-replay/planner/parse_warnings.json
RESEARCH_TRAIL_ROUND1 ?= $(TMP_DIR)/deeper/pebble-batch-commit-probe-round1/probe/bundle.json
RESEARCH_TRAIL_ROUND2 ?= $(TMP_DIR)/deeper/pebble-batch-commit-probe-round2/probe/bundle.json
RESEARCH_TRAIL_TEACH_BUNDLE ?= $(TMP_DIR)/deeper/pebble-batch-commit-teacher/teacher/bundle.json
RESEARCH_TRAIL_TEACH_INDEX ?= $(TMP_DIR)/deeper/pebble-batch-commit-teacher/teacher/index.json
RESEARCH_TRAIL_TEACH_REPORT ?= $(TMP_DIR)/deeper/pebble-batch-commit-teacher/teacher/report.json
RESEARCH_TRAIL_TEACH_DIAGNOSTICS ?= $(TMP_DIR)/deeper/pebble-batch-commit-teacher/teacher/parse_warnings.json
RESEARCH_TRAIL_OUT ?= $(TMP_DIR)/deeper/pebble-batch-commit-trail

research-trail-replay: ## Compose the saved Pebble planner/probe/teacher chain without model or gopls calls
	go run ./cmd/researchtrail-playground \
		--case "$(RESEARCH_TRAIL_CASE)" \
		--study-bundle "$(RESEARCH_TRAIL_STUDY_BUNDLE)" \
		--plan "$(RESEARCH_TRAIL_PLAN)" \
		--plan-diagnostics "$(RESEARCH_TRAIL_PLAN_DIAGNOSTICS)" \
		--probe-round1 "$(RESEARCH_TRAIL_ROUND1)" \
		--probe-round2 "$(RESEARCH_TRAIL_ROUND2)" \
		--teacher-bundle "$(RESEARCH_TRAIL_TEACH_BUNDLE)" \
		--teacher-index "$(RESEARCH_TRAIL_TEACH_INDEX)" \
		--teacher-report "$(RESEARCH_TRAIL_TEACH_REPORT)" \
		--teacher-diagnostics "$(RESEARCH_TRAIL_TEACH_DIAGNOSTICS)" \
		--out-dir "$(RESEARCH_TRAIL_OUT)"

FLOWPROOF_RUN ?=
FLOWPROOF_REPO ?= ../restic

flowproof-replay: ## Rebuild a saved orientation's local FlowProof without a model call
	@test -n "$(FLOWPROOF_RUN)" || (echo "FLOWPROOF_RUN is required" >&2; exit 2)
	go run ./cmd/flowproof-playground \
		--repo "$(FLOWPROOF_REPO)" \
		--orientation "$(FLOWPROOF_RUN)/orientation_report.json" \
		--snapshot "$(FLOWPROOF_RUN)/snapshot.json"

RESTIC_REPO ?= ../restic
CADDY_REPO ?= ../caddy

research-budget-check: ## Measure adaptive orientation budgets on Restic and Caddy
	./scripts/research_budget_check.sh "$(RESTIC_REPO)" "$(CADDY_REPO)"

PYRIGHT_REPO ?= internal/analyzer/python/pyright/testdata/fixture
PYRIGHT_PATH ?= app/service.py
PYRIGHT_LINE ?= 8
PYRIGHT_COLUMN ?= 0
PYRIGHT_LANGSERVER ?=

pyright-fixture: ## Analyze one exact symbol in the tracked Python fixture (requires Pyright)
	@go run ./cmd/pyright-playground \
		--repo "$(PYRIGHT_REPO)" \
		--path "$(PYRIGHT_PATH)" \
		--line "$(PYRIGHT_LINE)" \
		--column "$(PYRIGHT_COLUMN)" \
		$(if $(PYRIGHT_LANGSERVER),--pyright-langserver "$(PYRIGHT_LANGSERVER)")

gopls-examples: ## Analyze available example repos with the isolated gopls playground
	./scripts/gopls_examples.sh

gopls-examples-fetch: ## Fetch missing example repos, then generate gopls evidence artifacts
	./scripts/gopls_examples.sh --fetch

# --- Primary UX targets ---

serve: build ## Serve the latest saved report without a model call
	$(BIN_DIR)/repomap serve

doctor: ## Validate configured OpenAI-compatible LLM (no network request)
	go run ./cmd/repomap doctor llm

doctor-check: ## Validate configured OpenAI-compatible LLM with one small request
	go run ./cmd/repomap doctor llm --check

generic-deepseek-doctor: export REPOMAP_LLM_ENDPOINT = https://api.deepseek.com/chat/completions
generic-deepseek-doctor: export REPOMAP_LLM_MODEL = deepseek-v4-flash
generic-deepseek-doctor: export REPOMAP_LLM_API_KEY = $(DEEPSEEK_API_KEY)
generic-deepseek-doctor: export REPOMAP_LLM_AUTH = bearer
generic-deepseek-doctor: ## Calibrate generic provider config against DeepSeek
	go run ./cmd/repomap doctor llm --check

run: ## Orient ETCD_REPO with the configured OpenAI-compatible LLM
	go run ./cmd/repomap $(ETCD_REPO) $(RUN_ARGS)

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
