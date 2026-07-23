# Decision: Golden Mechanism v1 cold generalization

**Status**: Approved for implementation

**Approved by**: Repository owner in the current implementation session

## Product question

Can the existing evidence-to-publication contract produce a second
onboarding-quality mechanism for Caddyfile syntax errors, whose shape is error
creation, contextual enrichment, bounded cross-layer propagation, and a
user-visible result, without response-specific validator or pipeline changes?

## Cold boundary

Before the first and only synthesis response, the experiment must freeze:

- one exact question and its bounded mechanism scope;
- required, key, supporting, and explicitly optional answer aspects;
- seed files and symbols;
- probe ceilings and stop conditions;
- available and missing capabilities;
- fact IDs, evidence IDs, fact hashes, and the complete fixture hash;
- acceptance rubric and maximum provider call count.

The question is selected by a cheap read-only source survey over the three
owner-provided scopes. No opportunity scan participates in selection. The
broadest scope that is directly supportable within 3–8 files, at most 15
functions, three local expansion levels, 96 KiB retained evidence, and five
seconds is chosen. If even the narrowest scope is not supportable, the
experiment stops before synthesis.

## Approved implementation

1. Reuse the current bounded local syntax/evidence approach, stable opaque IDs,
   capability-labelled facts, common Golden synthesis prompt, claim support,
   aspect coverage, canonical publication, replay, report, evidence, and Super
   Search paths.
2. Add only deterministic facts that the bounded source proves. Explicit error
   wrapping, returns, context attachment, layer handoff, output, tests, and
   limitations may be represented using the existing fact contract when
   present; no universal error-flow analyzer or relation ontology is approved.
3. Preserve the v0.3 separation between available evidence, required answer
   aspects, and claim-required support. Editorial step order is not runtime
   order. Temporal prose still requires sequence-capable support.
4. Perform exactly one synthesis call, with no retry. The common prompt and
   validators may receive mechanism-specific question, aspects, facts,
   aliases, and explicit gaps, but may not be rewritten for Caddyfile errors.
5. After the response, do not edit prose, change the fixture or aspects, expand
   the probe, require a particular fact, add response-word rules, weaken a
   validator, or make a second synthesis call.
6. If the unchanged common contract rejects the response, save diagnostics and
   stop. A generally valid structure rejected by a validator is recorded as a
   finding and is not fixed during the cold evaluation.
7. On acceptance only, publish through the existing canonical semantic record
   and report path, prove replay without model output or analyzers, validate
   HTML/evidence and five fixed searches, preserve exact-path precedence, and
   verify that the directory-listing artifact remains available.
8. Save an ignored supervisor report comparing both Golden mechanisms and
   stating whether a reusable production pipeline is justified.

## Hard exclusions

- no opportunity scan, whole-repository package loading, SSA, pointer analysis,
  global call graph, runtime-surface discovery, or new repository-wide
  analysis;
- no second model call, retry, alternate model/provider, output post-processing,
  or handwritten prose repair;
- no candidate-specific pipeline, prompt, renderer, search algorithm, or
  validator patch;
- no full error-flow analyzer, universal relation ontology, MCP, Foglamp,
  Bonsai, Ollama, third mechanism, or directory-listing improvement;
- no fixture, aspect, scope, budget, or rubric change after the response.

## Success

Full success requires an accepted second artifact with score at least 3/4, all
key aspects covered, explicit unknowns, no-model replay, working HTML/evidence,
search discoverability, exact-path precedence, and no regression of the first
Golden artifact. A precise rejection caused by insufficient bounded
propagation evidence, a missing generic fact capability, model evidence loss,
a generally invalid validator decision, or an over-broad boundary remains a
useful cold result, but it is not generalization success.
