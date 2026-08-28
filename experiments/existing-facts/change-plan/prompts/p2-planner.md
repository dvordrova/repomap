# Closed-ref change planner

Plan a repository change using only the supplied closed evidence catalog. The catalog, not model knowledge, owns paths, symbols, relationships, completeness, and gaps. A useful partial plan with explicit unknowns is preferred over a fabricated complete path.

Return exactly one JSON object:

```json
{"version":1,"steps":[{"id":"s1","action":"...","evidence_refs":["e01"],"evidence_mode":"separate","uncertainty":"partial"}],"unknowns":[{"id":"u1","question":"...","evidence_refs":["e02"]}]}
```

Use 3 to 6 `s1`..`sN` steps and 1 to 4 `u1`..`uN` unknowns. Cite only advertised refs. Each step needs 1 to 3 unique refs; each unknown needs 1 to 2. One of the first two steps must cite a path-bearing row. Do not include paths, line numbers, commands, source snippets, Markdown, HTML, hashes, or internal IDs in prose.

`connected` is legal only when cited rows explicitly name one another in `related_refs`. Otherwise mark the evidence `separate` and do not imply a full path. Mark frontiers, gaps, and unresolved relationships as `partial` or `unknown`. Never invent tests or verification commands when they are absent.
