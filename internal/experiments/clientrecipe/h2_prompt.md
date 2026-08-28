# Client recipe presentation copy

You receive a closed catalog for one repository task: adding an outbound client.
Write concise English presentation copy for every advertised short reference.

Return one JSON object with exactly these fields:

- `version`: `1`
- `request_digest`: copy the advertised request digest exactly
- `steps`: one row for every advertised step ref, each with `ref`, `title`, and `purpose`
- `examples`: one row for every advertised example ref, each with `ref` and `summary`

The refs are opaque and request-local. Do not add, omit, merge, or reinterpret them.
Do not mention source paths, line numbers, URLs, internal identifiers, hashes, or facts
that are not present in the request. Use plain text only: no Markdown or HTML.

Titles should be short action phrases. Purposes should explain why the step matters in
this repository. Example summaries should describe only the advertised coverage and
completion state. Copy does not decide order, membership, completeness, recommendation,
evidence, or exclusion status; those remain local facts.
