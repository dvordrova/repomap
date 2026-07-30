# Decision 147: Russian report language

## Status

Approved by the repository owner with the explicit request to try
`repomap --lang ru`.

## Product contract

`--lang` accepts `en` and `ru`; English remains the default. Russian mode is
one product-wide choice rather than separate per-stage switches:

- every ordinary provider stage is instructed to write human-readable prose
  in Russian;
- the saved report records the selected language;
- the static report shell and dynamically rendered report UI use the same
  language;
- repository paths, code identifiers, package and module names, API and
  protocol names, product and library names, JSON keys, enum values, opaque
  IDs, and quoted source remain exact.

The English provider request and report behavior remain unchanged when the
flag is absent. The report format gains only one optional
`report_language` field.

## Acceptance

Provider-free tests must prove that:

- `--lang ru` reaches ordinary provider clients and report metadata;
- Russian instructions change only system prompts, not structured repository
  input;
- the rendered HTML declares `lang="ru"` and localizes UI labels;
- `runServer`, `NATS JetStream`, and repository paths remain unchanged;
- default English output does not serialize the optional language field.
