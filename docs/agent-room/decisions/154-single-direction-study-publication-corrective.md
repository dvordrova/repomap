# Decision 154: Preserve valid Study directions in localized runs

## Status

Active corrective, authorized by the repository owner after a real product run
exposed the retained `reviewed selection has 1 directions; need at least 3`
failure.

## Attributable failure

Decision 134 changed Study candidate decoding from whole-response rejection to
independent candidate acceptance. Decision 150 made one-area small-library
Study briefs valid. Neither decision changed the original canonical
`MinDirections = 3` publication floor.

The review stage can therefore retain one useful, source-reviewed direction and
then discard the complete canonical Study solely because two unrelated sibling
directions did not survive. The report subsequently shows retained candidate
starts as incomplete even though one direction passed the complete review
contract.

The same run also exposed a schema/presentation mismatch. `--lang ru` asks for
all human-readable prose in Russian, while `reading_anchors.label` looks like
prose but is validated against five exact English UI literals. Real English
runs have also returned noncanonical labels such as `Platform-specific` and
`Core operation`. Those values remain invalid, but one malformed candidate
must not erase independently reviewed siblings.

The Study rejection is not a cache or provider-transport failure. A broader
audit did find a separate localization defect in active cache identity:
orientation and Guided Tour shared one cache slot across languages, while
architecture synthesis could actively replay English prose into a Russian run.
Historical saved artifacts did not record enough information to infer their
language safely.

## Corrective contract

- One through seven reviewed Study directions form a valid canonical
  selection.
- Zero reviewed directions still fail closed.
- Candidate validation, three-to-five anchor review, exact source authority,
  reduction ordering, IDs, status format, report format, and HTTP behavior stay
  unchanged.
- The request may continue asking for a useful set of directions; diversity is
  a selection preference, not a reason to erase a single surviving result.
- Existing saved failures remain readable diagnostics.
- Product-wide Russian provider instructions explicitly preserve closed schema
  literals and exact protocol/format tags even when their words look
  human-readable.
- Study prompts explicitly identify `reading_anchors.label` as a closed
  English schema value localized later by the report.
- Canonical English reading labels and the report-owned Russian equivalents are
  normalized to the canonical five labels before validation. Unknown
  model-authored labels remain invalid; there is no positional or arbitrary
  copy fallback. Raw localized label text never reaches canonical output, and
  anchors, reading instructions, IDs, and authority remain strictly validated.
- Active orientation, targeted research, Guided Tour, and architecture
  synthesis caches isolate Russian output from the default English identity.
- Default-English orientation, targeted research, and Guided Tour cache keys
  remain byte-for-byte unchanged. The default-English architecture cache path
  also stays unchanged, but an active legacy synthesis record with unknown
  language is treated as a cache miss and replaced once.
- Historical architecture record replay remains valid; missing language is
  rejected only when selecting an artifact as an active cache hit.
- Study question/evidence fit ignores localized prose that cannot match source
  identifiers, while preserving technical ASCII terms and the existing
  English score.
- Existing unsupported-runtime-order checks recognize direct Russian
  equivalents instead of becoming weaker under localization.
- Targeted-research impact selection, runtime-only gating, and unsupported
  certainty checks recognize a small fixed set of direct Russian equivalents.

## Acceptance

- a provider-free review fixture with one accepted direction composes and
  validates a canonical record;
- an empty selection remains invalid;
- a published status containing one direction is accepted;
- English and report-owned Russian labels normalize to canonical labels in
  complete and incomplete Study decoding;
- normalized saved artifacts still require canonical labels;
- Russian provider instructions preserve all closed schema literals;
- English and Russian active cache entries cannot replay across languages;
- existing default-English model-research cache keys stay unchanged;
- legacy language-unknown architecture records replay historically but miss
  active cache selection;
- Russian prose is neutral in local question-fit ranking while preserved
  technical terms still discriminate relevant anchors;
- unsupported runtime-order wording is rejected in English and Russian;
- targeted-research impact, runtime-only, and certainty guards retain their
  English behavior and cover direct Russian equivalents;
- existing multi-direction fixtures remain unchanged;
- focused tests, `./scripts/check.sh`, `./scripts/etcd_check.sh ../etcd`, and
  `git diff --check` pass.

No provider call, repository rerun, retry, Search surface, analysis framework,
or UI redesign is part of this corrective.
