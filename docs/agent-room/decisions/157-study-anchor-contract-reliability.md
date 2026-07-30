# Decision 157: Make the Study anchor contract self-consistent

## Status

Active product corrective, authorized by the repository owner after an ordinary
run rejected ten of twelve Study drafts as `invalid_anchor_selection`.

## Attributable failure

Complete Study directions require three to five unique exact code anchors and
one reading instruction for each selected anchor. The provider-facing prose
states that contract, but its JSON example shows only one placeholder anchor
and one reading object. DeepSeek can therefore follow the example rather than
the prose.

This is not theoretical. A preserved Chatto response had twenty-four available
anchors, while ten of twelve otherwise useful drafts selected only two. The
same ten-of-twelve signature has now appeared in another ordinary product run.
Russian output and cross-run caches do not control anchor cardinality.

A second distinct case exists when the bounded bundle itself contains fewer
than three anchors. In that case a complete reading pack is impossible and the
product must not describe the result as a model-selection failure.

## Corrective contract

- The complete-direction JSON example visibly contains three distinct supplied
  anchor placeholders and three matching reading objects.
- The Study candidates prompt version changes so no prior response can be
  mistaken for output from the corrected contract.
- Candidate diagnostics distinguish an out-of-range anchor count, duplicate
  anchors, and malformed anchor IDs. The broad historical
  `invalid_anchor_selection` code remains accepted when reading old artifacts.
- Opaque IDs must already be canonical: surrounding whitespace is rejected on
  the individual candidate instead of failing later reference resolution and
  discarding valid siblings.
- Before asking for complete Study directions, orchestration checks the exact
  bounded anchor count. Fewer than three anchors produce an explicit
  `insufficient_code_anchors` stage outcome and no impossible complete-pack
  provider call.
- Existing incomplete topics and exact starting points remain available. The
  preflight does not manufacture a Mechanism, runtime order, anchors, or source.
- Three-to-five-anchor validation, one-reading-per-anchor validation, provider
  bounds, IDs, source authority, report format, and HTTP behavior remain
  unchanged.

## Acceptance

- prompt tests prove that the example contains three distinct anchors and three
  matching reading entries;
- a twelve-candidate provider fixture following the example retains all twelve
  structurally valid siblings;
- sparse bundles skip the impossible candidates call with a stable bounded
  reason and zero provider calls for that stage;
- diagnostics identify count, duplication, and malformed-ID failures
  separately, including whitespace-padded IDs;
- English and Russian runs use the same anchor contract;
- focused tests, `./scripts/check.sh`,
  `./scripts/etcd_check.sh /Users/dvordrova/git/etcd`, and
  `git diff --check` pass.

No Cobra discovery, lifecycle pairing, Mechanism publication, Search,
provider-client change, source snapshot, report page, or compatibility
framework is part of this corrective.
