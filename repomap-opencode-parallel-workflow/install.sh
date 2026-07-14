#!/usr/bin/env bash
set -euo pipefail

if [[ ! -d .git ]]; then
  echo "Run this script from the repomap repository root." >&2
  exit 1
fi

if ! command -v opencode >/dev/null 2>&1; then
  echo "OpenCode is not installed or is not on PATH." >&2
  exit 1
fi

src_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
src_dir="$src_root/.opencode"

strip_ansi() {
  sed $'s/\033\[[0-9;]*[[:alpha:]]//g'
}

models_output=""
if models_output="$(opencode models openai --refresh 2>/dev/null)"; then
  :
elif models_output="$(opencode models openai 2>/dev/null)"; then
  :
else
  echo "Could not list OpenAI models." >&2
  echo "In OpenCode run /connect -> OpenAI -> ChatGPT Plus/Pro, then rerun." >&2
  exit 1
fi

model_file="$(mktemp)"
trap 'rm -f "$model_file"' EXIT

printf '%s\n' "$models_output" \
  | strip_ansi \
  | grep -Eo 'openai/[A-Za-z0-9._:/+-]+' \
  | awk '!seen[$0]++' > "$model_file" || true

if [[ ! -s "$model_file" ]]; then
  echo "No openai/... models were returned." >&2
  echo "Run /connect -> OpenAI -> ChatGPT Plus/Pro in OpenCode, then rerun." >&2
  exit 1
fi

has_model() { grep -Fxq "$1" "$model_file"; }
first_match() { grep -Ei "$1" "$model_file" | head -n 1 || true; }

strongest_fallback() {
  local value
  value="$(first_match '^openai/.*gpt-5\.6.*(sol|codex|pro|thinking|max)')"
  [[ -n "$value" ]] || value="$(first_match '^openai/.*gpt-5\.6')"
  [[ -n "$value" ]] || value="$(first_match '^openai/.*gpt-5\.[0-9]+.*(codex|pro|thinking|max)')"
  [[ -n "$value" ]] || value="$(first_match '^openai/.*gpt-5')"
  [[ -n "$value" ]] || value="$(head -n 1 "$model_file")"
  printf '%s' "$value"
}

resolve_named() {
  local override="$1" exact="$2" pattern="$3" fallback="$4" value=""
  if [[ -n "$override" ]]; then
    has_model "$override" || { echo "Requested model unavailable: $override" >&2; exit 1; }
    printf '%s' "$override"; return
  fi
  if has_model "$exact"; then printf '%s' "$exact"; return; fi
  value="$(first_match "$pattern")"
  [[ -n "$value" ]] && { printf '%s' "$value"; return; }
  printf '%s' "$fallback"
}

strongest="$(strongest_fallback)"
sol="$(resolve_named "${REPOMAP_SOL_MODEL:-}" 'openai/gpt-5.6-sol' '^openai/.*gpt-5\.6.*sol' "$strongest")"
terra="$(resolve_named "${REPOMAP_TERRA_MODEL:-}" 'openai/gpt-5.6-terra' '^openai/.*gpt-5\.6.*terra' "$sol")"
luna="$(resolve_named "${REPOMAP_LUNA_MODEL:-}" 'openai/gpt-5.6-luna' '^openai/.*gpt-5\.6.*luna' "$terra")"

mkdir -p .opencode/agents .opencode/commands

# Remove agents and commands owned by older versions of this workflow.
for old in \
  blocker-diagnoser decision-planner feature-builder product-acceptance-reviewer \
  workflow-manager repo-fact-oracle fixture-impact-selector fixture-runner \
  fixture-auditor semantic-contract-auditor performance-auditor \
  cross-fixture-synthesizer browser-fixture-reviewer
do
  rm -f ".opencode/agents/${old}.md"
done

for old in \
  where-am-i continue-current accept-current fix-current diagnose-blocker \
  publish-current propose-next approve-next checkpoint-current feature-council \
  ship-feature go ship next
do
  rm -f ".opencode/commands/${old}.md"
done

cp -f "$src_dir"/agents/*.md .opencode/agents/
cp -f "$src_dir"/commands/*.md .opencode/commands/

for file in .opencode/agents/*.md; do
  tmp="$(mktemp)"
  sed \
    -e "s|__MODEL_SOL__|$sol|g" \
    -e "s|__MODEL_TERRA__|$terra|g" \
    -e "s|__MODEL_LUNA__|$luna|g" \
    "$file" > "$tmp"
  mv "$tmp" "$file"
done

if grep -R "__MODEL_" .opencode/agents >/dev/null 2>&1; then
  echo "Internal error: unresolved model placeholder." >&2
  exit 1
fi

cat > .opencode/MODELS.md <<MODELS
# Installed repomap workflow models

- Implementation/workflow management: \`$terra\` · high
- Repository oracles and fixture/browser audits: \`$terra\` · medium/high
- Fixture running, impact selection, performance: \`$luna\` · medium
- Cross-fixture synthesis, final acceptance, decision planning: \`$sol\` · high
- Focused blocker diagnosis: \`$sol\` · xhigh

All are explicit per-agent models. TUI model selection does not change them.
MODELS

cat > .opencode/PARALLEL-WORKFLOW.md <<'DOC'
# Installed parallel workflow

Normal commands: `/go`, `/ship`, `/next`.

`/go` may run up to three isolated fixture-scoped subagents concurrently. Only
`feature-builder` edits production code. Internal subagents are hidden from normal
autocomplete.
DOC

printf '\nInstalled repomap parallel workflow:\n'
printf '  /go    parallel evidence + one writer + product acceptance\n'
printf '  /ship  accepted commit + normal push\n'
printf '  /next  propose + explicit approval + update CURRENT.md\n'
printf '\nExplicit models:\n'
printf '  implementation: %s (high)\n' "$terra"
printf '  lightweight:    %s (medium)\n' "$luna"
printf '  synthesis:      %s (high/xhigh)\n' "$sol"
printf '\nWorkflow agents already allow:\n'
printf '  ~/Library/Caches/repomap/**\n'
printf '  ~/git/**\n'
printf '\nFor all OpenCode agents globally, optionally run:\n'
printf '  %s/install-global-permissions.sh\n' "$src_root"
printf '\nRestart OpenCode, then run /go\n'
