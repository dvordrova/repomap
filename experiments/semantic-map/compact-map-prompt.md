# Compact state-free semantic map experiment

Input contains one question and compact `facts`, `hypotheses`, and `unknowns`.
They are the entire evidence available. Build a small mechanism map and return
JSON only. Do not assign trust states; the consumer derives them from reference
types.

Exact output shape:

```json
{
  "nodes": [
    {"id": "n1", "label": "short label", "responsibility": "one sentence", "refs": ["f1"]}
  ],
  "edges": [
    {"id": "e1", "from": "n1", "to": "n2", "verb": "concrete action", "refs": ["h1"]}
  ],
  "overlay": {
    "summary": "two to four short sentences answering the question",
    "node_ids": ["n1"],
    "edge_ids": ["e1"],
    "unknown_ids": ["u1"]
  }
}
```

Rules:

- Return 3 to 6 nodes and 2 to 8 edges. Keep the response under 6 KiB, each
  label under 60 bytes, each responsibility under 240 bytes, each verb under
  120 bytes, and the summary under 600 bytes.
- Every node and edge cites 1 to 4 returned evidence IDs. Every endpoint and
  overlay ID must resolve.
- A direct close restatement may cite facts. A conceptual role, grouping,
  cross-fact relation, multi-step sequence, lifecycle outcome, preservation
  claim, or rollback claim must cite at least one hypothesis.
- Include every returned unknown ID exactly once in `overlay.unknown_ids`.
- Keep verbs concrete and directional. Distinguish aborting a candidate while
  current state remains unchanged from reverting already changed state.
- Never use package imports or a generic dependency graph as map edges.
- Do not output states, confidence, paths, line ranges, code excerpts, hashes,
  free-form unknown prose, or fields outside the exact shape.
- Do not use repository knowledge absent from the supplied evidence.
