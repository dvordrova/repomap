# Open product and research questions

This document tracks questions that must remain visible while repomap evolves.
They are not roadmap commitments or reasons to add abstractions immediately.
When a question is resolved, record the decision, evidence, rejected options,
and consequences in this document or a dedicated decision record.

## 1. Model provider portability

### Question

Must a repomap installation use DeepSeek, or can it use any sufficiently
compatible hosted or local model API?

The likely term is **OpenAI-compatible API**: an HTTP chat-completions protocol
implemented by multiple providers. This is different from **OpenAPI**, which is
an API-description specification.

### What is open

- What is the smallest provider capability contract repomap needs?
- Are endpoint, model, authentication, timeouts, and token limits sufficient
  configuration, or do providers require different request adapters?
- Must a provider support JSON response mode? The tagged `KEY: VALUE` symbol
  protocol intentionally does not require it.
- How are context limits, pricing, retries, rate limits, and unsupported options
  represented without leaking provider details into the core pipeline?
- Should local model servers be a first-class supported case?

### Current constraint

DeepSeek is the current implementation and calibration target, not a fact that
the core product should depend on. Runtime endpoint/model/bearer-or-none auth/
timeout configuration is now provider-neutral and was exercised against Ollama.
Prompt capability negotiation, non-compatible transports, and consumer-owned
model interfaces remain open. New core types should describe model input,
output, and evidence requirements rather than DeepSeek request fields.

### Evidence needed for a decision

- A fixed HTTP contract test shared by provider adapters.
- One company-hosted compatibility/calibration run using the bounded bundle and
  the same evaluator; the prior Ollama run proves transport compatibility but
  not acceptable repository-analysis quality.
- A comparison of output quality, latency, token use, and failure behavior.

## 2. Vendored code and embedded dependencies

### Question

Should repository analysis enter `vendor/` and other embedded dependency trees?
Some projects vendor ordinary third-party dependencies; others keep patched,
forked, generated, or organization-owned projects there. Excluding everything
can hide behavior that matters, while indexing everything can overwhelm the
repository's own architecture.

### Policies to compare

1. Exclude vendored code completely.
2. Include it as ordinary repository code.
3. Keep it as a collapsed dependency boundary by default, then expand a specific
   dependency only when a selected flow, symbol, ticket, or bug crosses into it.

The third policy currently looks most compatible with progressive exploration,
but it is not yet a decision.

### What must stay explicit

- Whether a fact comes from project-owned code, generated code, vendored code,
  a fork, or an external module.
- Whether the active Go build actually resolves imports through `vendor/`.
- Whether generated and vendored files are eligible for LLM input, local-only
  structural analysis, or neither.
- Whether an organization-owned vendored project should be treated differently
  from an unmodified third-party dependency, and how that ownership is detected.

### Evidence needed for a decision

- Experiments on repositories that make materially different use of vendoring,
  including Grafana projects.
- Measurements of graph size, analysis time, bundle size, and useful paths found
  under each policy.
- At least one task whose correct explanation requires crossing the dependency
  boundary.

## 3. Verifying claims about what a function does

### Question

If a repository contains thousands of functions, how do we check a model claim
such as "this function validates a request" when the available evidence is only
its name and the symbols it calls? Do we verify every claim, verify selected
claims on demand, or present some claims only as navigation hypotheses?

### Non-negotiable distinction

A name or static call edge can support an **inference**. It cannot prove source
semantics or runtime behavior. The prompt evaluator measures contract adherence;
it does not establish semantic truth.

Possible evidence levels are:

1. **Navigation hypothesis** — inferred from names, paths, signatures, and graph
   neighborhood.
2. **Source-supported** — corroborated by a bounded source body, documentation,
   constants, types, or conditions.
3. **Test-supported** — exercised or asserted by a relevant test.
4. **Runtime-observed** — seen in a named trace, profile, log, or instrumented
   scenario.
5. **Verified for a scenario** — corroborated by multiple appropriate sources;
   still not a universal claim for every build and runtime condition.

### What is open

- Which claims require automatic source inspection before they are shown?
- Should verification be lazy and driven by the user's selected question rather
  than attempted for all functions?
- How should contradictory source, tests, documentation, and runtime evidence be
  displayed?
- What sampling or golden-task suite can estimate hallucination and omission
  rates without manually reviewing an entire repository?
- When should repomap say "unknown" instead of spending more analysis or model
  budget?

### Candidate experiment

Choose a small set of symbols across etcd, k6, Prometheus, NATS Server, and
golangci-lint. For each symbol, compare:

- names and depth-one static edges only;
- bounded source bodies and type/signature evidence;
- related tests;
- an executable or observable scenario where practical.

Have a human label which claims survived each evidence level. Use the result to
decide when source reading and tests are worth their cost.

## 4. Onboarding as a product mode

### Question

Can repomap become a repeatable onboarding tool rather than only an ad hoc
repository explorer?

An onboarding mode needs a standard question scope, while still allowing a user
to choose where to go deeper. Candidate question groups are:

- What does the system exist to do, and what does it explicitly not do?
- What are the major runtime components and their boundaries?
- How does the process start, stop, and receive work?
- What are the important request, event, background, and data flows?
- Where is state stored, cached, replicated, or discarded?
- Which architectural and Go patterns recur in project-owned code?
- Which external libraries and internal shared projects materially shape the
  design, and where are their integration boundaries?
- How are configuration, feature flags, errors, logging, metrics, tracing, and
  security handled?
- How are changes tested locally and in CI?
- Where are extension points, generated code, compatibility constraints, and
  operational hazards?
- What remains unknown, and which files or experiments would answer it?

### What is open

- Which questions belong in every onboarding report versus optional prompt packs?
- Should the output be one report, an interactive curriculum, or a saved trail
  of progressively verified investigations?
- How should a team add organization-specific questions without forking core
  prompts?
- What outcome measures onboarding quality: time to first correct change, ability
  to explain one flow, fewer reviewer corrections, or something else?

## 5. Progressive exploration like a human reader

### Question

Can repomap build understanding in layers, starting with a high-level map and
expanding only where the user's goal or curiosity leads, instead of exhaustively
understanding every dependency from the bottom up?

### Candidate interaction model

1. **Survey** — identify repository purpose, entrypoints, components, docs, and
   candidate flows from cheap deterministic facts.
2. **Choose** — let the user select a flow, component, symbol, ticket, bug, or
   onboarding question.
3. **Focus** — collect a bounded neighborhood and distinguish local code from
   dependencies and generated code.
4. **Verify** — read source, tests, or runtime evidence only for claims that
   matter to the selected goal.
5. **Update the map** — retain conclusions, evidence, contradictions, and open
   questions so later exploration does not start over.

### What is open

- What state should survive between investigations, and how is it invalidated
  after repository changes?
- How does the UI reveal alternative directions without turning into a giant
  static diagram?
- What is the correct expansion budget per step?
- How does a user inspect why a component, file, or next question was suggested?

The core bias is progressive disclosure: understanding an application should
not require first explaining all of `net/http` or every transitive dependency.

## 6. Reusable knowledge for standard and popular libraries

### Question

Should repomap reuse precomputed, reviewed knowledge about the Go standard
library and common dependencies such as `slog` or `zap`, instead of asking a
model to rediscover their general purpose for every repository?

### Required separation

Reusable knowledge can explain a library's general contract. It cannot explain
how a particular repository configures, wraps, or misuses that library. The
integration boundary must still be derived from the current repository.

### What is open

- Is the knowledge generated from package docs and source, manually curated, or
  both?
- How is it keyed and invalidated: Go toolchain version, module path and version,
  build tags, fork identity, or content hash?
- How is provenance shown, and what confidence is assigned to curated versus
  generated summaries?
- Can the cache remain local-first and work offline?
- How do licensing, redistribution, storage size, and update policy affect a
  distributable catalog?
- When should repomap expand library internals despite having a cached summary?

### Candidate first slice

Precompute a small, versioned catalog for the Go packages most often encountered
in the example repositories. Measure whether it reduces tokens and latency
without hiding repository-specific wrappers or control flow.

## 7. Architectural pattern signals

### Reference note: `alexandergrom/go-patterns`

[`alexandergrom/go-patterns`](https://github.com/alexandergrom/go-patterns) is
useful as a small vocabulary of recognizable code shapes, not as a dependency
or a source of current Go architecture. It is an educational, class-oriented
catalog with no module or releases, and its last recorded changes are from 2021.

For repomap's own implementation, the useful ideas are deliberately narrower
than the named patterns:

- adapters fit gopls, model APIs, and future presentation boundaries, but should
  remain small consumer-owned interfaces rather than a plugin registry;
- the CLI/application service can act as a facade over capability cubes without
  becoming another domain abstraction;
- strategy is justified only after a second real implementation exists, such as
  a future dumb-model implementation of the same typed assessment capability;
- investigation state and commands should remain data plus a pure reducer, not
  mutable objects with `Execute` methods.

For analysing other applications, treat patterns as **candidate architecture
signals**, never semantic truth. Useful static shapes include wrappers,
middleware chains, subscriber notification loops, recursive composites,
constructor-selected implementations, and visitor-style double dispatch. Some
names are structurally ambiguous: adapter, decorator, and proxy may all look
like a wrapper that delegates; strategy and state may both look like an
interface field with interchangeable implementations. Control flow, source,
tests, or runtime evidence is needed before assigning intent.

A future optional output can therefore look like:

```text
kind: wrapper | policy | state_transition | middleware_chain | pubsub | tree | factory
status: candidate
evidence_ids: [...]
confidence: ...
```

This is a later `architecture_signal` facet, not a new subsystem and not part of
M1. Do not add detectors until source/test-grounded symbol investigation works
and the signals can be evaluated on etcd, k6, Prometheus, NATS Server, and
golangci-lint fixtures.

Do not copy the catalog's singleton/global state, fatal exits in library code,
OO-heavy builders, `interface{}` iterators, broad interfaces that force no-op
methods, or observer/chain examples without cancellation, synchronization,
backpressure, and explicit unhandled/error results.

## Development checklist

Until these questions are resolved, review material changes against this list:

- Does this hard-code DeepSeek behavior into a core model or pipeline type?
- Does this silently include or exclude vendored/generated code?
- Does this promote an inference to a source-supported or runtime claim?
- Does this analyze the whole repository when an on-demand expansion would work?
- Does this ask a model to restate stable library knowledge without examining
  the repository-specific integration?
- Does this label a familiar code shape as a named design pattern without
  evidence for the author's intent or its runtime role?
- Does this preserve provenance, build scenario, prompt version, provider/model,
  and evaluator version so an experiment can be reproduced?
