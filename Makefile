SHELL := /usr/bin/env bash

-include .env
export

BIN_DIR  ?= .bin
TMP_DIR  ?= tmp
ETCD_REPO ?= ../etcd
RUN_ARGS ?=

.PHONY: help test vet check quality-check localization-check localization-replay localization-stage build clean smoke etcd-check friend-check symbol-check symbol-prompt-experiment source-prompt-experiment component-study-preview component-study-live component-study-replay component-probe component-probe-frontier component-teach-preview component-teach-live component-teach-replay research-trail-replay flowproof-replay research-budget-check pyright-fixture gopls-examples gopls-examples-fetch doctor doctor-check generic-deepseek-doctor debug-last guided-tour-run guided-tour-fanout guided-tour-experiment semantic-discovery semantic-discovery-experiment fresh-repo-onboarding fresh-repo-onboarding-replan fresh-repo-onboarding-replay golden-mechanism golden-mechanism-v01 golden-mechanism-v02 golden-mechanism-v02-prepare golden-mechanism-v02-replay golden-mechanism-v03 golden-mechanism-v03-replay golden-mechanism-v1 golden-mechanism-v1-prepare golden-mechanism-v1-replay mechanism-v1 mechanism-v1-replay chi-request-dispatch chi-request-dispatch-prepare chi-request-dispatch-response-replay chi-request-dispatch-replay review-cockpit review-serve serve run run-json run-offline run-flows2 deepseek-check

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

localization-check: ## Validate provider-free canonical and locale projection contracts
	go test ./internal/localization -count=1
	go test ./internal/report -run 'ArchitectureLocalization' -count=1
	go test ./cmd/repomap -run 'Localization(Stage|Replay|Check)|PrintPromptVersions' -count=1
	@if [[ -n "$(RUN)" ]]; then \
		go run ./cmd/repomap dev localization-check "$(RUN)"; \
	fi

localization-replay: ## Replay one provider-free Russian Architecture projection
	@if [[ -z "$(RUN)" ]]; then echo "RUN is required" >&2; exit 2; fi
	@if [[ -z "$(PROJECTION)" ]]; then echo "PROJECTION is required" >&2; exit 2; fi
	@go run ./cmd/repomap dev localization-replay "$(RUN)" "$(PROJECTION)"

localization-stage: ## Preview or replay the provider-free Architecture localization stage
	@if [[ -z "$(RUN)" ]]; then echo "RUN is required" >&2; exit 2; fi
	@if [[ -n "$(RESPONSE)" ]]; then \
		go run ./cmd/repomap dev localization-stage "$(RUN)" "$(RESPONSE)"; \
	else \
		go run ./cmd/repomap dev localization-stage "$(RUN)"; \
	fi

SURFACE_REPO ?= internal/experiment/surfacediscovery/testdata/direct
SURFACE_OUT ?= $(TMP_DIR)/surface-discovery

.PHONY: surface-check surface-playground caddy-surface-check syncthing-surface-check
surface-check: ## Run config-driven Go surface discovery fixtures
	go test ./internal/semantics/catalog ./internal/experiment/surfacediscovery ./internal/surfacebridge

surface-playground: ## Emit local surface JSON and Markdown for SURFACE_REPO
	go run ./cmd/surface-discovery-playground --repo "$(SURFACE_REPO)" --out "$(SURFACE_OUT)"

caddy-surface-check: ## Compare deterministic surface discovery with a nearby Caddy checkout
	./scripts/caddy_surface_check.sh "$(CADDY_REPO)"

syncthing-surface-check: ## Validate partial package recovery and process surfaces against nearby Syncthing
	./scripts/syncthing_surface_check.sh "$(SYNCTHING_REPO)"

build: ## Build binary into .bin/
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/repomap ./cmd/repomap

CANVAS_FIXTURE ?= internal/report/testdata/canvas/restic-backup-v2.json
CANVAS_PORT ?= 0

.PHONY: canvas-preview v31-product-gate-check
canvas-preview: ## Serve a saved architecture canvas fixture without analysis or provider access
	go run ./cmd/canvas-preview --fixture "$(CANVAS_FIXTURE)" --port "$(CANVAS_PORT)"

V31_RUN ?=
V31_SPEC ?= scripts/testdata/v31_litestream_gate.json
v31-product-gate-check: ## Check the fixed Litestream v3.1 product result without model or repository analysis
	@test -n "$(V31_RUN)" || (echo "V31_RUN is required" >&2; exit 2)
	@test -n "$(V31_SPEC)" || (echo "V31_SPEC is required" >&2; exit 2)
	node scripts/v31_product_gate_check.js "$(V31_RUN)" "$(V31_SPEC)"

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
SYNCTHING_REPO ?= ../syncthing

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

GUIDED_TOUR_RUN ?=

guided-tour-run: ## Add or replay a guided tour for an existing saved run
	@test -n "$(GUIDED_TOUR_RUN)" || (echo "GUIDED_TOUR_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev guided-tour "$(GUIDED_TOUR_RUN)"

guided-tour-fanout: ## Run only the bounded fan-out/fan-in strategy for one saved run
	@test -n "$(GUIDED_TOUR_RUN)" || (echo "GUIDED_TOUR_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev guided-tour-fanout "$(GUIDED_TOUR_RUN)"

guided-tour-experiment: ## Compare monolithic and fan-out tours for one saved run
	@test -n "$(GUIDED_TOUR_RUN)" || (echo "GUIDED_TOUR_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev guided-tour-experiment "$(GUIDED_TOUR_RUN)"

SEMANTIC_DISCOVERY_RUN ?=
FRESH_ONBOARDING_RUN ?=
FRESH_ONBOARDING_REPO ?=

semantic-discovery: ## Build semantic artifacts from one existing saved run
	@test -n "$(SEMANTIC_DISCOVERY_RUN)" || (echo "SEMANTIC_DISCOVERY_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev semantic-discovery "$(SEMANTIC_DISCOVERY_RUN)"

semantic-discovery-experiment: ## Compare monolithic and fan-out semantic synthesis on a saved run
	@test -n "$(SEMANTIC_DISCOVERY_RUN)" || (echo "SEMANTIC_DISCOVERY_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev semantic-discovery-experiment "$(SEMANTIC_DISCOVERY_RUN)"

fresh-repo-onboarding: ## Add bounded central onboarding paths to one existing saved run
	@test -n "$(FRESH_ONBOARDING_RUN)" || (echo "FRESH_ONBOARDING_RUN is required" >&2; exit 2)
	@test -n "$(FRESH_ONBOARDING_REPO)" || (echo "FRESH_ONBOARDING_REPO is required" >&2; exit 2)
	go run ./cmd/repomap dev fresh-repo-onboarding --run-dir "$(FRESH_ONBOARDING_RUN)" --repo "$(FRESH_ONBOARDING_REPO)"

fresh-repo-onboarding-replan: ## Reuse saved questions and run bounded primary-path planning before synthesis
	@test -n "$(FRESH_ONBOARDING_RUN)" || (echo "FRESH_ONBOARDING_RUN is required" >&2; exit 2)
	@test -n "$(FRESH_ONBOARDING_REPO)" || (echo "FRESH_ONBOARDING_REPO is required" >&2; exit 2)
	go run ./cmd/repomap dev fresh-repo-onboarding --run-dir "$(FRESH_ONBOARDING_RUN)" --repo "$(FRESH_ONBOARDING_REPO)" --replan-saved

fresh-repo-onboarding-replay: ## Revalidate saved bounded candidate responses without model, probe, or repository analysis
	@test -n "$(FRESH_ONBOARDING_RUN)" || (echo "FRESH_ONBOARDING_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev fresh-repo-onboarding --run-dir "$(FRESH_ONBOARDING_RUN)" --replay-saved

GOLDEN_MECHANISM_RUN ?=
GOLDEN_MECHANISM_ARGS ?=
MECHANISM_V1_RUN ?=
CHI_DISPATCH_RUN ?= $(TMP_DIR)/chi-llm-runs/20260715-193912-chi

golden-mechanism: ## Enrich one saved Caddy run with the bounded golden mechanism experiment
	@test -n "$(GOLDEN_MECHANISM_RUN)" || (echo "GOLDEN_MECHANISM_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev golden-mechanism "$(GOLDEN_MECHANISM_RUN)" $(GOLDEN_MECHANISM_ARGS)

golden-mechanism-v01: ## Replay or publish the fixed six-fact Caddy Golden Mechanism fixture
	@test -n "$(GOLDEN_MECHANISM_RUN)" || (echo "GOLDEN_MECHANISM_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev golden-mechanism-v01 "$(GOLDEN_MECHANISM_RUN)" $(GOLDEN_MECHANISM_ARGS)

golden-mechanism-v02: ## Make the single v3 synthesis call over the prepared seven-fact fixture
	@test -n "$(GOLDEN_MECHANISM_RUN)" || (echo "GOLDEN_MECHANISM_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev golden-mechanism-v02 "$(GOLDEN_MECHANISM_RUN)"

golden-mechanism-v02-prepare: ## Prove and save the local sequence fixture without model or analyzers
	@test -n "$(GOLDEN_MECHANISM_RUN)" || (echo "GOLDEN_MECHANISM_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev golden-mechanism-v02 "$(GOLDEN_MECHANISM_RUN)" --prepare

golden-mechanism-v02-replay: ## Rebuild a copied canonical run without model, probe, or analyzers
	@test -n "$(GOLDEN_MECHANISM_RUN)" || (echo "GOLDEN_MECHANISM_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev golden-mechanism-v02 "$(GOLDEN_MECHANISM_RUN)" --replay

golden-mechanism-v03: ## Revalidate and publish the saved v0.2 response without model or probe
	@test -n "$(GOLDEN_MECHANISM_RUN)" || (echo "GOLDEN_MECHANISM_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev golden-mechanism-v03 "$(GOLDEN_MECHANISM_RUN)"

golden-mechanism-v03-replay: ## Replay a manifest-free run copy without raw model output
	@test -n "$(GOLDEN_MECHANISM_RUN)" || (echo "GOLDEN_MECHANISM_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev golden-mechanism-v03 "$(GOLDEN_MECHANISM_RUN)" --replay

golden-mechanism-v1: ## Make the one cold-generalization synthesis call over the frozen Caddyfile fixture
	@test -n "$(GOLDEN_MECHANISM_RUN)" || (echo "GOLDEN_MECHANISM_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev golden-mechanism-v1 "$(GOLDEN_MECHANISM_RUN)"

golden-mechanism-v1-prepare: ## Freeze the Caddyfile error fixture with one bounded probe and no model call
	@test -n "$(GOLDEN_MECHANISM_RUN)" || (echo "GOLDEN_MECHANISM_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev golden-mechanism-v1 "$(GOLDEN_MECHANISM_RUN)" --prepare

golden-mechanism-v1-replay: ## Replay a copied two-artifact Golden run without model, probe, or analyzers
	@test -n "$(GOLDEN_MECHANISM_RUN)" || (echo "GOLDEN_MECHANISM_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev golden-mechanism-v1 "$(GOLDEN_MECHANISM_RUN)" --replay

mechanism-v1: ## Materialize the accepted Caddy Mechanism object without model or analyzers
	@test -n "$(MECHANISM_V1_RUN)" || (echo "MECHANISM_V1_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev mechanism-v1 "$(MECHANISM_V1_RUN)"

mechanism-v1-replay: ## Replay the saved Mechanism object into Start Here, Search, and the map
	@test -n "$(MECHANISM_V1_RUN)" || (echo "MECHANISM_V1_RUN is required" >&2; exit 2)
	go run ./cmd/repomap dev mechanism-v1 "$(MECHANISM_V1_RUN)" --replay

chi-request-dispatch: ## Make the one bounded chi request-dispatch synthesis call
	go run ./cmd/repomap dev chi-request-dispatch "$(CHI_DISPATCH_RUN)"

chi-request-dispatch-prepare: ## Freeze the chi dispatch facts and prompt without a model call
	go run ./cmd/repomap dev chi-request-dispatch "$(CHI_DISPATCH_RUN)" --prepare

chi-request-dispatch-response-replay: ## Revalidate and publish the fixed chi response without model, probe, or analyzers
	go run ./cmd/repomap dev chi-request-dispatch "$(CHI_DISPATCH_RUN)" --replay-response

chi-request-dispatch-replay: ## Replay the saved chi Mechanism without a model call or repository analysis
	go run ./cmd/repomap dev chi-request-dispatch "$(CHI_DISPATCH_RUN)" --replay

REVIEW_CADDY_RUN ?= $(TMP_DIR)/golden-mechanism-caddy-20260715/run
REVIEW_CHI_RUN ?= $(TMP_DIR)/chi-llm-runs/20260715-193912-chi
REVIEW_OUT ?= $(TMP_DIR)/repomap-review
REVIEW_PORT ?= 8765

review-cockpit: ## Generate the saved-artifact Morning Review without model or analyzers
	go run ./cmd/repomap dev review-cockpit --caddy-run "$(REVIEW_CADDY_RUN)" --chi-run "$(REVIEW_CHI_RUN)" --out "$(REVIEW_OUT)"

review-serve: ## Serve the generated Morning Review over localhost
	python3 -m http.server "$(REVIEW_PORT)" --bind 127.0.0.1 --directory "$(abspath $(REVIEW_OUT))"

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

# --- Task Lens v0 experiment harness ---

TASK_LENS_ROOT ?= $(TMP_DIR)/task-lens-v0
TASK_LENS_SOURCE_REPO ?= ../fuego
TASK_LENS_BINARY ?= $(BIN_DIR)/repomap
TASK_LENS_DEV_SET ?= scripts/testdata/task_lens_v0/DEV_SET.json
TASK_LENS_HOLDOUT_SET ?= scripts/testdata/task_lens_v0/HOLDOUT_SET.json
TASK_LENS_BUDGETS ?= scripts/testdata/task_lens_v0/BUDGETS.json
TASK_LENS_DEV_TASKS_DIR ?= $(TMP_DIR)/fuego-historical-benchmark-v0
TASK_LENS_OWNER_PROMPT ?= scripts/testdata/task_lens_v0/CODEX_TASK_LENS_V0_PROMPT.md
TASK_LENS_OWNER_CHECKSUMS ?= scripts/testdata/task_lens_v0/MANIFEST.sha256
TASK_LENS_CONTRACTS ?= internal/tasklens internal/deepseek
TASK_LENS_EPISODE ?=
TASK_LENS_CHEAP_EXIT_EPISODES ?=
TASK_LENS_GOLD_DIR ?=
TASK_LENS_SCORES ?= $(TASK_LENS_ROOT)/evaluation/SCORECARD.json
TASK_LENS_REVIEW_PORT ?= 8767
TASK_LENS_FROZEN_HARNESS ?= $(TASK_LENS_ROOT)/freeze/harness/task_lens_harness.py
TASK_LENS_FROZEN_EVAL ?= $(TASK_LENS_ROOT)/freeze/harness/task_lens_eval.py

.PHONY: task-lens-init task-lens-harness-check task-lens-dev-prepare task-lens-dev-run task-lens-dev-seal task-lens-freeze task-lens-cheap-exits-declare task-lens-holdout-prepare task-lens-holdout-run task-lens-holdout-seal task-lens-gold-unlock task-lens-evaluate task-lens-review task-lens-review-serve

task-lens-init: ## Initialize the Task Lens v0 review bundle without running product episodes
	python3 scripts/task_lens_harness.py init --root "$(TASK_LENS_ROOT)"

task-lens-harness-check: ## Run the synthetic protocol plus real-binary offline Task Lens smoke
	./scripts/task_lens_harness_check.sh

task-lens-dev-prepare: ## Prepare exact development worktrees and Git-free source exports
	python3 scripts/task_lens_harness.py prepare \
		--root "$(TASK_LENS_ROOT)" \
		--phase dev \
		--source-repo "$(TASK_LENS_SOURCE_REPO)" \
		--manifest "$(TASK_LENS_DEV_SET)" \
		--tasks-dir "$(TASK_LENS_DEV_TASKS_DIR)"

task-lens-dev-run: ## Run and seal one development iteration (TASK_LENS_EPISODE is optional)
	python3 scripts/task_lens_harness.py run \
		--root "$(TASK_LENS_ROOT)" \
		--phase dev \
		--binary "$(TASK_LENS_BINARY)" \
		$(if $(TASK_LENS_EPISODE),--episode "$(TASK_LENS_EPISODE)")

task-lens-dev-seal: ## Revalidate and seal development outputs already on disk
	python3 scripts/task_lens_harness.py seal \
		--root "$(TASK_LENS_ROOT)" \
		--phase dev \
		--binary "$(TASK_LENS_BINARY)" \
		$(if $(TASK_LENS_EPISODE),--episode "$(TASK_LENS_EPISODE)")

task-lens-freeze: ## Freeze code, prompts/schemas, budgets, tasks, diff, and executable SHA
	python3 scripts/task_lens_harness.py freeze \
		--root "$(TASK_LENS_ROOT)" \
		--implementation-repo . \
		--binary "$(TASK_LENS_BINARY)" \
		--owner-prompt "$(TASK_LENS_OWNER_PROMPT)" \
		--owner-checksums "$(TASK_LENS_OWNER_CHECKSUMS)" \
		--budgets "$(TASK_LENS_BUDGETS)" \
		--dev-manifest "$(TASK_LENS_DEV_SET)" \
		--holdout-manifest "$(TASK_LENS_HOLDOUT_SET)" \
		$(foreach contract,$(TASK_LENS_CONTRACTS),--contract "$(contract)")

task-lens-cheap-exits-declare: ## Seal expected cheap-exit episode IDs before holdout preparation
	@test -n "$(TASK_LENS_CHEAP_EXIT_EPISODES)" || (echo "TASK_LENS_CHEAP_EXIT_EPISODES is required" >&2; exit 2)
	python3 "$(TASK_LENS_FROZEN_EVAL)" declare-cheap-exits \
		--root "$(TASK_LENS_ROOT)" \
		$(foreach episode,$(TASK_LENS_CHEAP_EXIT_EPISODES),--episode "$(episode)")

task-lens-holdout-prepare: ## Prepare gold-isolated exact holdout worktrees from the frozen manifest
	python3 "$(TASK_LENS_FROZEN_HARNESS)" prepare \
		--root "$(TASK_LENS_ROOT)" \
		--phase holdout \
		--source-repo "$(TASK_LENS_SOURCE_REPO)" \
		--manifest "$(TASK_LENS_HOLDOUT_SET)"

task-lens-holdout-run: ## Consume and seal the one allowed attempt per holdout episode
	python3 "$(TASK_LENS_FROZEN_HARNESS)" run \
		--root "$(TASK_LENS_ROOT)" \
		--phase holdout \
		$(if $(TASK_LENS_EPISODE),--episode "$(TASK_LENS_EPISODE)")

task-lens-holdout-seal: ## Verify episode seals and write the global holdout seal when complete
	python3 "$(TASK_LENS_FROZEN_HARNESS)" seal \
		--root "$(TASK_LENS_ROOT)" \
		--phase holdout \
		$(if $(TASK_LENS_EPISODE),--episode "$(TASK_LENS_EPISODE)")

task-lens-gold-unlock: ## Unlock historical gold only after every holdout output is globally sealed
	@test -n "$(TASK_LENS_GOLD_DIR)" || (echo "TASK_LENS_GOLD_DIR is required" >&2; exit 2)
	python3 "$(TASK_LENS_FROZEN_EVAL)" unlock-gold \
		--root "$(TASK_LENS_ROOT)" \
		--gold-dir "$(TASK_LENS_GOLD_DIR)"

task-lens-evaluate: ## Validate separate 0-4 supervisor scores and render the review bundle
	python3 "$(TASK_LENS_FROZEN_EVAL)" evaluate \
		--root "$(TASK_LENS_ROOT)" \
		--scores "$(TASK_LENS_SCORES)"

task-lens-review: ## Print the stable supervisor-report and localhost review convention
	python3 scripts/task_lens_harness.py review --root "$(TASK_LENS_ROOT)"

task-lens-review-serve: ## Serve the static Task Lens v0 review (no analysis or model calls)
	python3 -m http.server "$(TASK_LENS_REVIEW_PORT)" --bind 127.0.0.1 --directory "$(abspath $(TASK_LENS_ROOT))"

# --- Legacy compat (kept for internal dev) ---

deepseek-check: ## Live configured-LLM call (legacy target name)
	./scripts/deepseek_check.sh $(ETCD_REPO)
