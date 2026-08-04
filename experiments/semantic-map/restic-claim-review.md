# Restic one-call shelf review

Repository: `restic/restic@8baffc40273bb3aa4f6c7826d582ebe576f4c90c`

Sampler checkpoint: `8da56da`

Provider: `deepseek-v4-flash`, thinking disabled, exactly one attempt

Call: 114,350 request bytes; 28,797 input / 1,444 output tokens; 13,105 ms

Raw response SHA-256:
`fe3bb0ba59e07ac7c5beeedfbf51581237a25d873b34dd387cd6ef1ae4dadc5d`

## Shelf outcome

PASS WITH VISIBLE UNCERTAINTY.

- 12 topics, at least nine clearly distinct and directly supported concerns.
- Backup, repository initialization, index/pack, and restore are present.
- Backend/cache/retry topics are three of twelve; command wrappers did not
  dominate despite occupying 44.5% of the sampled declarations.
- The raw shelf is unchanged. Unsupported detail is marked below, not filtered
  or silently repaired.

## Claim → observations → review status

| Topic claim | Exact observed support | Status | Review note |
|---|---|---|---|
| t1 Backup operation lifecycle | `runBackup`, `collectTargets`, `collectRejectByNameFuncs`, `findParentSnapshot`, `filterExisting` | corroborated | Strong, coherent command-to-snapshot entry set. |
| t2 Repository initialization and configuration | `runInit`, `CreateConfig`, `maybeReadChunkerPolynomial`, `addKey`, `switchToNewKeyAndRemoveIfBroken` | corroborated | Config and key setup are both directly observed. |
| t3 Snapshot management and retention | `runForget`, `changeTags`, `rewriteSnapshot`, `filterAndReplaceSnapshot`, `PrintSnapshots` | corroborated | Retention and metadata lifecycle are directly observed. |
| t4 Backend abstraction and storage operations | backend cast, backend registry, location parse/register, only `mem.Save` | unknown | Registry/one implementation are visible; generic Save/Load/Remove dispatch is not established by these anchors. |
| t5 Data encryption and integrity | `poly1305MAC`, blob buffers, hashing reader/writer | unknown | Integrity/MAC are observed; the claimed encryption flow is not established by these anchors. |
| t6 Index and pack management | `NewPacker`, `NewIndex`, `ForAllIndexes`, `AllIndexBlobs`, index-map add | corroborated | Pack construction and index traversal are both visible. |
| t7 Restore operation and file reconstruction | `runRestore`, `newFileRestorer`, `newFilesWriter`, xattr filter, restorer option `Set` | corroborated | Command-to-restorer bridge and metadata hook are visible. |
| t8 Locking and concurrent access control | read/append/exclusive lock openers plus unlock command | corroborated | Distinct lock modes and release path are visible. |
| t9 Cache management and optimization | cache backend constructor, version/dir/file helpers, test helper | unknown | Cache existence is observed, but the five support anchors do not establish the proposed “size or eviction policy” edit. Separately observed `MaxAge`/`OlderThan` cleanup is age-based, so this is missing support rather than a direct contradiction. |
| t10 Error handling and retry logic | retry wrapper, notify/success loop, backoff wrapper | corroborated | Concrete retry mechanics are visible. |
| t11 Progress reporting and terminal output | backup/restore progress, JSON progress, terminal setup | corroborated | Multiple presentation modes are directly observed. |
| t12 Self-update mechanism | `runSelfUpdate`, hash lookup, Unix/Windows replacement, GitHub source | corroborated | Download verification and replacement anchors are visible. |

Totals: 9 corroborated, 3 unknown, 0 conflicted.

## Anchor-seeded trail review

All five support IDs were preserved as distance-zero source anchors in every
run. Provenance operations were only `ast_anchor_resolution`,
`call_hierarchy`, and `document_symbols`; there was no `workspace_symbol`
rediscovery.

1. `t1/how` — useful. Reaches `runBackup`, target/exclusion/parent-snapshot
   helpers, then `internal/archiver.Snapshot`. Two integration-test neighbors
   appear, but production reading choices remain clear.
2. `t1/change` — useful with noise. Exact edit candidate
   `collectRejectByNameFuncs` remains first-class; expansion becomes test-heavy
   after the production anchors, and `filterExisting` is adjacent backup input
   handling rather than the exclusion-policy edit itself.
3. `t2/how` — useful. Reaches `runInit`, `CreateConfig`, key setup, and
   `internal/repository.Init`; later bounded neighbors include testing/debug
   helpers.

Result: 3/3 useful; 2/3 contain visible bounded noise.

## Remaining limitations

- This is one provider sample, not repeatability evidence.
- Review statuses are manual experiment judgments, not a new runtime
  classifier.
- The sampler gives only 4–5 slots to several central Restic directories, so a
  valid topic may still rely on shallow anchors.
- Test neighbors enter exact call expansion even though `_test.go` files are
  excluded from the model inventory.
