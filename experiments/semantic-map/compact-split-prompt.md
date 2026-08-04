# Compact fact/claim split experiment

Input is one developer question plus at most 12 unannotated source slices. The
slices are the entire evidence available. Return compact JSON only; do not copy
the repository identity or question into the output.

Exact output shape:

```json
{
  "facts": [
    {"id": "f1", "text": "one literal fact", "at": [0, 10, 20]}
  ],
  "hypotheses": [
    {"id": "h1", "text": "one atomic interpretation", "refs": ["f1", "f2"]}
  ],
  "unknowns": [
    {"id": "u1", "text": "one unresolved boundary", "refs": ["f2"]}
  ]
}
```

Rules:

- Return 4 to 8 facts, 1 to 4 hypotheses, and 1 to 3 unknowns. Keep the entire
  response under 8 KiB and every text under 240 bytes.
- `at` is `[zero-based source_slices index, start_line, end_line]`. The line
  range must be inside that one input slice.
- A fact is a close restatement of syntax in its one cited range. Directly
  visible restoration or cleanup may be a fact. Do not expand identifiers,
  assign architectural roles, join another range, add intent, claim runtime or
  concurrency guarantees, add deterministic order to map iteration, or infer
  an outcome from absent code.
- A hypothesis is one atomic relation or interpretation backed by 1 to 4 fact
  IDs. Put cross-range sequences, conceptual roles, lifecycle outcomes,
  preservation claims, and rollback claims here. Do not write a full answer,
  component map, or mechanism summary.
- An unknown names one question-relevant boundary unresolved by the slices and
  cites 1 to 4 fact IDs. Do not invent a gap already answered by another slice.
- Function calls and lexical order are static source evidence, not observed
  runtime execution.
- Do not output states, confidence, paths, code excerpts, components, graph
  nodes or edges, hashes, URLs, or fields outside the exact shape.
- Do not use repository knowledge absent from the supplied slices.
