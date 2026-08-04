# go-git one-call shelf review

Repository: `go-git/go-git@9b81545704e806f3e745a18617e240ce01a1d3b8`

Sampler checkpoint: `8da56da`

Provider: `deepseek-v4-flash`, thinking disabled, exactly one attempt

Call: 116,005 request bytes; 28,792 input / 1,580 output tokens; 12,429 ms

Raw response SHA-256:
`837880944203ee63dc028161d3790de6ee8c6db03ddcb6dbd4c33eea459f5241`

## Shelf outcome

PASS WITH A WEAK FETCH CLAIM AND ONE FAILED CHANGE TRAIL.

- 12 distinct concerns; remote operations and worktree behavior are both
  present.
- Plumbing/storage concerns are five of twelve and do not erase the public API
  shelf.
- The raw shelf is unchanged. Example-heavy or shallow support remains visible
  in the review.

## Claim → observations → verification → review status

| Topic claim | Exact observed support | Verification | Status |
|---|---|---|---|
| t1 Clone repository from remote | `CloneOptions.Validate` plus five `_examples/clone...:main` anchors | Both t1 trails reach production `repository.clone`; the change trail also reaches `CloneContext` and `PlainCloneContext` | corroborated, with example-heavy initial support |
| t2 Push commits to remote | `PushOptions.Validate`, `Remote.Push`, `PushContext`, `sendPack` | t2/how retains all four production anchors | corroborated |
| t3 Fetch objects from remote | `FetchSupports` and `LsRefs` in internal transport v2 plus an options validator | No selected verification trail; no `Remote.Fetch` or fetch integration anchor | unknown |
| t4 Worktree status computation | `Status`, `StatusWithOptions`, internal status diff, ignore-pattern collection | not selected | corroborated |
| t5 Commit creation in worktree | `Commit`, `updateHEAD`, `buildCommitObject`, signature sanitization | not selected | corroborated |
| t6 Reference resolution and expansion | `ExpandRef`, `WriteInfoRefs`, filesystem reference storage, filtered reference iterator | not selected | corroborated |
| t7 Object storage and retrieval | filesystem object/storage constructors, object cache, memory `EncodedObject` | not selected | corroborated |
| t8 Packfile scanning and indexing | pack scanner construction, offset/hash lookup, object retrieval | not selected | corroborated |
| t9 Remote protocol handshake | file/git/SSH handshakes plus v2 client capability negotiation | not selected | corroborated |
| t10 Configuration loading and merging | core read/load/path functions plus plugin auto-merge | not selected | corroborated |
| t11 Submodule initialization and status | init/status/repository/default-remote functions | not selected | corroborated |
| t12 Blame annotation computation | public blame entry plus line/needs propagation helpers | not selected | corroborated |

Totals: 11 corroborated, 1 unknown, 0 conflicted.

## Anchor-seeded trail review

Every support ID survives as a distance-zero source anchor. Provenance
operations are only `ast_anchor_resolution`, `call_hierarchy`, and
`document_symbols`; there is no `workspace_symbol` rediscovery.

1. `t1/how` — useful. Example-heavy seeds reach the production
   `Repository.clone` body, which exposes remote creation, fetch/update,
   checkout, and submodule setup. One options test is bounded noise.
2. `t1/change` — not a reliable edit trail for the question as written. It
   reaches clone entrypoints and existing authentication examples, but not the
   transport/client registration seam needed for a genuinely new protocol or
   authentication implementation. It is useful for configuring existing auth,
   not for the proposed change.
3. `t2/how` — useful with heavy test expansion. Production seeds include
   `Remote.Push`, `PushContext`, and `sendPack`; seven of the eight secondary
   choices are push tests.

Result: 2/3 useful for their exact questions; 2/3 have visible example/test
noise.

## Remaining limitations

- This is one provider sample, not repeatability evidence.
- The useful clone concern was inferred from example paths because exact
  `Clone`/`PlainClone` declarations were absent from the model inventory.
- Fetch remains a plausible weak topic rather than a verified flow.
- Manual review statuses are development annotations, not a runtime
  classifier.
- Exact call expansion can spend most secondary slots on tests even when
  production anchors are preserved.
