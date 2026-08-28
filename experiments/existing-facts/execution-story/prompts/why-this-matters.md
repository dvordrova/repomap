# Why-this-matters edit

You receive one closed execution-story catalog. Write the smallest useful explanation of why a developer would open this route.

The explanation must help with an investigation or decision; do not retell the ordered edge list. Frame uses as questions the route can help investigate, never as incidents that definitely occurred. Do not claim that the callback executed, that the external library succeeded, that a failure exists, or that the route is complete.

Use only closed fact refs from the supplied catalog. Keep the prose plain and
specific. Source destinations are rendered locally and are not part of this
request. Mention the most important uncertainty instead of hiding it.

Return this exact shape and no extra fields:

```json
{
  "schema": "repomap.experiment.why-this-matters.v1",
  "use_case": "one of: investigate | plan_change",
  "value": {"text": "one sentence, 18-32 words", "support_refs": ["closed refs only"]},
  "use_when": [
    {"text": "one concrete developer question, 8-18 words", "support_refs": ["closed refs only"]},
    {"text": "a different concrete developer question, 8-18 words", "support_refs": ["closed refs only"]}
  ],
  "limit": {"text": "one sentence stating the decisive uncertainty", "support_refs": ["closed fact refs only"]}
}
```

The application owns the section heading, code-jump selection, labels, paths,
and purposes. None of those values enter the provider request.
