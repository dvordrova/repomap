# Debugging execution-story edit

Rewrite the supplied factual spine for an engineer debugging TLS setup during server startup.

Lead with the observed chain and the first uncertain handoff. Describe every catalog edge exactly once and in catalog order. Copy each edge's `ref`, `from_ref`, `to_ref`, and `certainty` unchanged. Bind the story to the supplied `start_ref` and `end_ref`. Explicitly distinguish exact calls from the `possible` callback transfer. Surface every frontier and say what the catalog cannot establish. Use exactly the supplied `recommended_code_jump_refs`; do not propose other files or fixes.

Return this exact shape and no extra fields:

```json
{
  "schema": "repomap.experiment.execution-story.result.v1",
  "audience": "debugger",
  "title": "short title",
  "summary": {"text": "one compact debugging summary", "support_refs": ["closed refs only"]},
  "start_ref": "copy supplied start_ref",
  "end_ref": "copy supplied end_ref",
  "steps": [
    {"edge_ref": "e1", "from_ref": "n1", "to_ref": "n2", "certainty": "exact", "text": "catalog-grounded sentence"}
  ],
  "uncertainty": [
    {"frontier_ref": "u1", "text": "what is uncertain or not represented"}
  ],
  "code_jump_refs": ["copy the two supplied refs"],
  "not_represented": {"text": "one sentence about the external boundary and omitted facts", "support_refs": ["frontier refs only"]}
}
```
