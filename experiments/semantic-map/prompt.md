# Semantic map microexperiment

You receive one JSON object containing a repository identity, one developer
question, and bounded source-linked observations. The observations are the
entire evidence available to you.

Build a compact semantic map that helps a developer answer the question. Infer
responsibilities and useful relations from the observations. Do not turn file,
package, or import structure into the map. Do not claim runtime execution.
Keep weak but useful inferences visible by marking them `inferred`, and expose
important gaps as unknowns instead of silently completing them.

Return valid JSON only, with exactly this shape:

```json
{
  "nodes": [
    {
      "id": "short-stable-id",
      "label": "short human label",
      "responsibility": "one concrete sentence",
      "state": "supported | inferred | unknown",
      "observation_ids": ["obs-01"]
    }
  ],
  "edges": [
    {
      "id": "short-stable-id",
      "from": "node-id",
      "to": "node-id",
      "verb": "concrete action or lifecycle relation",
      "state": "supported | inferred | unknown",
      "observation_ids": ["obs-01"]
    }
  ],
  "question_overlay": {
    "summary": "two to four short sentences answering the supplied question",
    "node_ids": ["node-id"],
    "edge_ids": ["edge-id"],
    "unknowns": ["important fact the observations do not establish"]
  }
}
```

Rules:

- Produce 3 to 8 nodes and 2 to 12 edges.
- Use only supplied observation IDs. Every node and edge must cite at least one.
- Every edge endpoint must name a returned node.
- Keep labels under 60 characters and responsibilities under 240 characters.
- Edge verbs must be concrete and directional, preferably 2 to 5 words.
- Never use `package import`, `imports`, or a generic dependency graph as an
  edge.
- The overlay must reference returned node and edge IDs in the useful reading
  order. It must contain at least one explicit unknown.
- Do not emit file paths, line ranges, code excerpts, hashes, or fields outside
  this JSON shape.
- Do not use repository knowledge that is absent from the supplied
  observations.
