# Where to add a backend-commit acknowledgment barrier

Question: **Where would a backend-commit acknowledgment barrier be inserted, and what behavior would it change?**

Accepted input: `etcd-put-recoverability` at [`58f45a9ff1c083130830eb02b0cc7d9783609095`](https://github.com/etcd-io/etcd/tree/58f45a9ff1c083130830eb02b0cc7d9783609095).

<!-- where-to-change {"episode_id":"etcd-put-recoverability","flow_id":"flow-successful-put-partial","state":"inferred","claim_ids":["claim-success-waits-for-local-apply","claim-applied-put-is-mvcc-visible","claim-small-put-does-not-wait-for-backend-commit","claim-backend-carries-materialized-keyvalue-bytes","claim-consistent-index-bounds-replay"],"fact_ids":["fact-apply-result-triggers-waiter","fact-mvcc-end-publishes-revision","fact-put-buffer-readable-before-commit","fact-backend-commits-periodically","fact-mvcc-encodes-revision-and-keyvalue","fact-backend-commit-saves-consistent-index"],"flow_node_ids":["flow-node-client-ack","flow-node-mvcc-visible","flow-node-backend-committed","flow-node-replay-guard"],"anchor_ids":["anchor-apply-result-to-waiter","anchor-mvcc-revision-publication","anchor-put-read-buffer-before-commit","anchor-backend-periodic-commit","anchor-mvcc-keyvalue-bytes","anchor-backend-saves-consistent-index","anchor-backend-bbolt-commit"]} -->

## Short answer — INFERRED

This is a two-ended barrier, not one safe line move:

1. Keep the already-computed apply result pending at the current request-waiter release seam.
2. Publish a separate completion notification only after the backend transaction commits successfully.
3. Release the waiter only when `committed_consistent_index >= applied_entry_index`.

That would change successful handler completion from “the Put is locally applied and MVCC-visible” to “the Put is included in a successfully committed bbolt transaction.” The accepted episode does not expose an existing request-to-batch correlation primitive, so this is a design seam, not a ready-made edit.

## What happens today

- **CORROBORATED** — Success waits for the local apply result. On this path, a successful unary Put response is downstream of the request-ID result triggered by applying the committed normal entry on the serving member.
- **CORROBORATED** — Apply publishes an MVCC-visible revision before returning its result. The Put apply call returns through a deferred transaction End that advances the MVCC revision and writes the staged Put into the backend read buffer, so later local reads can observe it before the waiter result becomes the RPC response.
- **CORROBORATED** — An ordinary small Put need not wait for a per-request bbolt commit. For a Put that does not hit the batch limit, backend Unlock exposes the write through the read buffer without committing it; a later timer can commit the batch.

## Where to change — INFERRED

### 1. Gate response release at [`server/etcdserver/server.go:1968`](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/server.go#L1968-L1981)

Today `s.w.Trigger(id, ar)` releases the request-ID waiter after local apply. Keep the apply result associated with its applied Raft index instead of triggering the waiter immediately.

### 2. Emit completion after [`server/storage/backend/batch_tx.go:261`](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/backend/batch_tx.go#L261-L287)

The notification must follow a successful `t.tx.Commit()`. A batch commit is shared work, not a per-Put callback, so the commit path should publish a committed watermark rather than wake a request waiter directly.

### 3. Correlate through the consistent index from [`server/storage/hooks.go:40`](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/hooks.go#L40-L52)

The pre-commit hook records the consistent index in the transaction, but it runs before commit and cannot acknowledge success itself. A post-commit signal must carry the committed watermark; pending apply results become releasable only when `committed_consistent_index >= applied_entry_index`.

## What behavior changes — INFERRED

- Local MVCC visibility stays where it is; the response boundary moves later.
- A below-limit Put can wait for the next successful shared batch commit instead of returning as soon as buffered MVCC state is visible.
- Preserving batching requires holding multiple pending results and releasing every result covered by the committed consistent-index watermark.
- Forcing a commit for every Put is a separate policy choice, not a consequence of the episode.
- Commit failure, cancellation, shutdown, and backpressure need explicit behavior before this can become production code.

## What remains unknown

- **UNKNOWN** — Client acknowledgment and this Ready loop's WAL Save completion are not ordered here. The Ready loop hands committed entries to the apply channel before calling storage Save, while the apply path can trigger the request waiter independently. These anchors do not establish whether the Put handler returns before or after the relevant WAL Save call completes.

- **UNKNOWN** — The episode does not identify an existing request ID → applied index → committed batch notification path.
- **UNKNOWN** — Waiting for bbolt commit would not establish filesystem/device persistence or order the response against the relevant WAL save.
- **UNKNOWN** — The latency and batching cost require a focused runtime experiment; they are not derivable from this source slice.

## Smallest useful proof before production

These are proposed checks, not commands claimed to exist:

1. Hold a below-limit Put after MVCC visibility and prove its handler still waits before backend commit.
2. Commit one batch containing several applied indices and release exactly the pending results covered by its persisted watermark.
3. Make backend commit fail and prove no covered Put reports success.
4. Measure the latency and batching delta without changing WAL-order claims.

## Pinned source seams

- [`server/etcdserver/server.go` lines 1968–1981](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/server.go#L1968-L1981)
- [`server/storage/mvcc/kvstore_txn.go` lines 185–197](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/mvcc/kvstore_txn.go#L185-L197)
- [`server/storage/backend/batch_tx.go` lines 308–339](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/backend/batch_tx.go#L308-L339)
- [`server/storage/backend/backend.go` lines 441–455](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/backend/backend.go#L441-L455)
- [`server/storage/mvcc/kvstore_txn.go` lines 212–238](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/mvcc/kvstore_txn.go#L212-L238)
- [`server/storage/hooks.go` lines 40–52](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/hooks.go#L40-L52)
- [`server/storage/backend/batch_tx.go` lines 261–287](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/backend/batch_tx.go#L261-L287)
- [`server/etcdserver/raft.go` lines 218–231](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/raft.go#L218-L231)
- [`server/etcdserver/raft.go` lines 243–260](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/raft.go#L243-L260)
