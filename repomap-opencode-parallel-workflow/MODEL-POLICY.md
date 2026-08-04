# Explicit model policy for parallel work

The installer resolves concrete `openai/...` model IDs available through the connected
ChatGPT Plus/Pro OpenAI provider.

| Role | Preferred model | Effort |
|---|---|---:|
| feature-builder / workflow-manager | GPT-5.6 Terra | high |
| repository oracle / fixture auditor | GPT-5.6 Terra | medium |
| browser fixture reviewer | GPT-5.6 Terra | high |
| fixture runner / impact / performance | GPT-5.6 Luna | medium |
| cross-fixture synthesis / final acceptance / planning | GPT-5.6 Sol | high |
| precise blocker diagnosis | GPT-5.6 Sol | xhigh |

The installer selects the closest available OpenAI fallback when a preferred family is
not exposed by the account and records the actual mapping in `.opencode/MODELS.md`.

Concurrency is intentionally bounded so parallelism does not multiply expensive Sol
calls:

- at most three fixture-scoped Terra/Luna tasks;
- one Sol-high synthesis/reviewer at a time;
- one Sol-xhigh diagnosis at a time.
