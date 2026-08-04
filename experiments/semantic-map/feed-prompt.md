# Observation feed microexperiment

You receive one JSON object containing a repository identity, one developer
question, and bounded unannotated source slices. The slices are the entire
evidence available to you.

Extract a compact set of atomic, question-relevant observations. This is an
upstream evidence feed, not an answer and not an architecture map. Do not name
components, group responsibilities, create graph edges, or write a summary.
Retain useful uncertainty instead of completing missing behavior from general
repository knowledge.

Return valid JSON only, with exactly this shape:

```json
{
  "case_id": "copied exactly from input",
  "repository": {
    "name": "copied exactly from input",
    "revision": "copied exactly from input"
  },
  "question": "copied exactly from input",
  "observations": [
    {
      "id": "obs-01",
      "state": "extracted | inferred | unknown",
      "text": "one atomic observation",
      "sources": [
        {
          "path": "exact input path",
          "start_line": 10,
          "end_line": 20
        }
      ]
    }
  ]
}
```

Rules:

- Copy `case_id`, `repository`, and `question` byte-for-byte.
- Produce 6 to 18 observations with unique IDs.
- Every observation must cite 1 to 3 subranges wholly contained in supplied
  source slices. Do not invent or normalize paths or line numbers.
- Use `extracted` for a close restatement of syntax directly visible in the
  cited range. Several statements in one function may form one extracted
  control fact, but do not expand identifiers, invent architectural roles,
  claim runtime or concurrency guarantees, add ordering to map iteration, or
  infer an effect from code that is absent.
- Use `inferred` when the observation joins facts from separate functions or
  ranges, assigns intent or a conceptual role, or explains an outcome not
  directly stated by one cited control path. Cite every range needed for that
  inference.
- Use `unknown` for a question-relevant boundary the supplied slices expose but
  do not resolve. State exactly what is missing, and do not call something
  unknown when another supplied slice establishes it.
- Keep each observation under 500 bytes and about one sentence.
- Prefer control decisions, state transitions, lifecycle actions, registration,
  dispatch, persistence, and failure behavior over syntax trivia.
- Function calls and lexical order are static source evidence, not observed
  runtime execution.
- Do not output component labels, graph nodes, graph edges, an ordered answer,
  a mechanism summary, change advice, confidence scores, hashes, URLs, code
  excerpts, or fields outside this JSON shape.
- Do not use repository knowledge absent from the supplied slices.
