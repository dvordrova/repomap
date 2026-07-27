# How an etcd `Put` becomes visible—and what “success” does not prove

`episode_id: etcd-put-recoverability`

Pinned source: [`etcd-io/etcd@58f45a9ff1c083130830eb02b0cc7d9783609095`](https://github.com/etcd-io/etcd/tree/58f45a9ff1c083130830eb02b0cc7d9783609095)

This is a source-code trace of one question: after a client sends `Put`, what has
happened when the call succeeds? The short answer is: the request has completed
local state-machine application and is readable through etcd's MVCC/backend
buffering path. That is not the same assertion as “the matching bbolt transaction
has committed,” and the inspected code does not establish a simple before/after
ordering between the client response and the matching WAL save.

Claim labels:

- `EXTRACTED` — directly visible in one local code path.
- `CORROBORATED` — the conclusion joins two or more matching code paths or tests.
- `INFERRED` — useful interpretation that is not mechanically complete.
- `UNKNOWN` — the inspected evidence is insufficient to order or identify events.

## The causal path

### 1. The caller parks on a request ID

`claim-success-waits-for-local-apply` — **CORROBORATED**
<!-- episode-claim {"id":"claim-success-waits-for-local-apply","state":"corroborated","anchor_ids":["anchor-rpc-put-return","anchor-request-bytes-and-waiter","anchor-apply-result-to-waiter"],"support_fact_ids":["fact-rpc-put-returns-kv-result","fact-request-bytes-proposed-and-waited","fact-apply-result-triggers-waiter"]} -->

`EtcdServer.Put` wraps the public request in `InternalRaftRequest`. The proposal
path assigns an ID, marshals the envelope, registers a waiter under that ID,
proposes the bytes to Raft, and blocks on the waiter channel. During apply, etcd
decodes the committed entry, recovers the same ID, executes the operation, and
triggers the waiter with the apply result. Therefore a successful return is tied
to local application, not merely to proposal admission.

Evidence IDs: `fact-rpc-put-returns-kv-result`,
`fact-put-wraps-raft-request`, `fact-request-bytes-proposed-and-waited`,
`fact-apply-result-triggers-waiter`.

Sources:

- [`key.go`: return the delegated KV Put result](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/api/v3rpc/key.go#L90-L101)
- [`v3_server.go`: marshal, propose, and wait](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/v3_server.go#L1093-L1129)
- [`server.go`: apply result triggers that waiter](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/server.go#L1968-L1981)

### 2. A `Ready` starts apply before its WAL save finishes

`claim-ready-entry-sets-have-distinct-roles` — **EXTRACTED**
<!-- episode-claim {"id":"claim-ready-entry-sets-have-distinct-roles","state":"extracted","anchor_ids":["anchor-committed-entries-to-apply","anchor-unstable-entries-to-wal"],"support_fact_ids":["fact-committed-entries-feed-apply","fact-unstable-entries-feed-wal"]} -->

The Raft loop builds `toApply.entries` from `rd.CommittedEntries` and sends that
work to the apply scheduler. It then calls
`storage.Save(rd.HardState, rd.Entries)`. Because the apply scheduler runs
separately, state-machine apply and the Raft loop's WAL work can overlap.

The important distinction is literal in the code:

> `rd.CommittedEntries` feeds state-machine apply.
> `rd.Entries` feeds WAL persistence.

Do not collapse them into one list: **`rd.Entries != rd.CommittedEntries`**.

Evidence IDs: `fact-committed-entries-feed-apply`,
`fact-unstable-entries-feed-wal`.

Sources:

- [`raft.go`: construct and enqueue `toApply`](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/raft.go#L218-L231)
- [`raft.go`: persist `HardState` and `rd.Entries`](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/raft.go#L243-L260)

### 3. The committed request becomes an MVCC revision

`claim-applied-put-is-mvcc-visible` — **CORROBORATED**
<!-- episode-claim {"id":"claim-applied-put-is-mvcc-visible","state":"corroborated","anchor_ids":["anchor-mvcc-put-transaction","anchor-mvcc-revision-publication","anchor-put-read-buffer-before-commit"],"support_fact_ids":["fact-mvcc-put-ends-write-transaction","fact-mvcc-end-publishes-revision","fact-put-buffer-readable-before-commit"]} -->

The committed entry is unmarshaled and dispatched to the backend applier's
`Put`. The transaction layer opens an MVCC write transaction and defers `End`.
`TxnWrite.Put` builds a versioned `mvccpb.KeyValue`, serializes it under the new
revision, updates the in-memory key index, and records the change. `End`
publishes the new store revision and unlocks the backend transaction before the
apply result can wake the request waiter.

Evidence IDs: `fact-mvcc-put-ends-write-transaction`,
`fact-mvcc-end-publishes-revision`,
`fact-put-buffer-readable-before-commit`.

Sources:

- [`txn/put.go`: open and end the MVCC write transaction](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/txn/put.go#L30-L45)
- [`kvstore_txn.go`: publish the revision and unlock](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/mvcc/kvstore_txn.go#L185-L197)
- [`batch_tx.go`: expose the buffered Put before commit](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/backend/batch_tx.go#L308-L339)

### 4. Visible does not mean “physically committed to bbolt now”

`claim-small-put-does-not-wait-for-backend-commit` — **CORROBORATED**
<!-- episode-claim {"id":"claim-small-put-does-not-wait-for-backend-commit","state":"corroborated","anchor_ids":["anchor-put-read-buffer-before-commit","anchor-backend-periodic-commit"],"support_fact_ids":["fact-put-buffer-readable-before-commit","fact-backend-commits-periodically"]} -->

For a below-limit Put, unlocking the buffered batch transaction writes pending
updates into the read buffer but does not force a bbolt commit. Reads merge that
buffer with the current bbolt read transaction, so the newly applied value is
visible before the periodic or batch-limit commit. The client path has no
physical-backend-commit barrier after apply; it waits for the request-ID result.

This says the response does not *depend on* an immediate bbolt commit. Scheduling
may still allow a background commit to happen first in a particular run.

Evidence IDs: `fact-put-buffer-readable-before-commit`,
`fact-backend-commits-periodically`.

Sources:

- [`batch_tx.go`: write back the buffer without committing a small Put](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/backend/batch_tx.go#L308-L339)
- [`backend.go`: periodic backend commit](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/backend/backend.go#L441-L455)

### 5. WAL and backend preserve different forms of the operation

`claim-wal-carries-replayable-request-bytes` — **INFERRED**
<!-- episode-claim {"id":"claim-wal-carries-replayable-request-bytes","state":"inferred","anchor_ids":["anchor-request-bytes-and-waiter","anchor-wal-entry-record-and-sync","anchor-restart-reads-wal"],"support_fact_ids":["fact-request-bytes-proposed-and-waited","fact-wal-records-entry-and-state","fact-restart-reads-committed-wal"]} -->

The local etcd code shows a serialized `InternalRaftRequest` passed to
`Propose(data)`, `raftpb.Entry` values written to the WAL, and those entries read
back on restart. The connecting step—`Propose(data)` becoming the matching
`raftpb.Entry.Data`—remains inferred until the external Raft implementation is
pinned and anchored. The useful working interpretation is that the WAL carries
the replayable command envelope and consensus position, not the final MVCC
key/value layout.

Evidence IDs: `fact-request-bytes-proposed-and-waited`,
`fact-wal-records-entry-and-state`, `fact-restart-reads-committed-wal`.

Sources:

- [`v3_server.go`: marshal the request bytes and propose them](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/v3_server.go#L1093-L1129)
- [`wal.go`: encode entries and synchronize the WAL](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/wal/wal.go#L936-L992)
- [`bootstrap.go`: read WAL state and entries on restart](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/bootstrap.go#L624-L662)

`claim-backend-carries-materialized-keyvalue-bytes` — **CORROBORATED**
<!-- episode-claim {"id":"claim-backend-carries-materialized-keyvalue-bytes","state":"corroborated","anchor_ids":["anchor-mvcc-keyvalue-bytes","anchor-backend-saves-consistent-index","anchor-backend-bbolt-commit"],"support_fact_ids":["fact-mvcc-encodes-revision-and-keyvalue","fact-backend-commit-saves-consistent-index"]} -->

The backend side stores the serialized `mvccpb.KeyValue` under a revision key.
At bbolt commit, the pre-commit hook saves the consistent index in the same
backend transaction. This materialized MVCC state is what reads and recovery
reconstruct, rather than the original `PutRequest` envelope.

Evidence IDs: `fact-mvcc-encodes-revision-and-keyvalue`,
`fact-backend-commit-saves-consistent-index`.

Sources:

- [`kvstore_txn.go`: materialize and store `mvccpb.KeyValue`](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/mvcc/kvstore_txn.go#L212-L238)
- [`hooks.go`: save consistent index during backend pre-commit](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/hooks.go#L40-L52)
- [`batch_tx.go`: commit the bbolt transaction](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/backend/batch_tx.go#L261-L287)

### 6. The consistent index makes backend replay idempotent

`claim-consistent-index-bounds-replay` — **CORROBORATED**
<!-- episode-claim {"id":"claim-consistent-index-bounds-replay","state":"corroborated","anchor_ids":["anchor-restart-filters-uncommitted","anchor-apply-uses-consistent-index","anchor-backend-saves-consistent-index"],"support_fact_ids":["fact-restart-reads-committed-wal","fact-backend-commit-saves-consistent-index"]} -->

While applying entries, etcd compares each Raft index with the backend
consistent index. An entry above it applies to the v3 backend and advances the
applying index; an entry at or below it skips v3 reapplication. The next backend
commit persists the consistent index with the materialized MVCC writes.

The recoverability idea is atomic at the backend boundary: if that bbolt
transaction did not commit, replay can apply the entry again; if it did commit,
the persisted index tells replay not to duplicate the MVCC mutation.

Evidence IDs: `fact-restart-reads-committed-wal`,
`fact-backend-commit-saves-consistent-index`.

Sources:

- [`bootstrap.go`: discard uncommitted WAL entries on restart](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/bootstrap.go#L714-L727)
- [`server.go`: compare entry index and choose whether to apply v3](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/server.go#L1905-L1917)
- [`hooks.go`: persist that index at backend commit](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/storage/hooks.go#L40-L52)

### 7. The exact client-ack/WAL ordering remains open

`claim-ack-versus-wal-save-order-is-unknown` — **UNKNOWN**
<!-- episode-claim {"id":"claim-ack-versus-wal-save-order-is-unknown","state":"unknown","anchor_ids":["anchor-committed-entries-to-apply","anchor-unstable-entries-to-wal","anchor-apply-result-to-waiter"],"support_fact_ids":["fact-committed-entries-feed-apply","fact-unstable-entries-feed-wal","fact-apply-result-triggers-waiter"]} -->

The inspected loop proves that apply work is enqueued before the current
`Ready` saves `rd.Entries`, and the waiter is triggered during apply. It does
**not** prove that the committed entry carrying this Put is one of the unstable
entries saved by that same `Ready`. A committed entry can have been persisted
in an earlier `Ready`.

Therefore this episode does not claim either “ack always precedes WAL save” or
“ack always follows WAL save” for the same Put. **Same-`Ready` identity is not
proven**, and `rd.Entries != rd.CommittedEntries`.

Evidence IDs: `fact-committed-entries-feed-apply`,
`fact-unstable-entries-feed-wal`, `fact-apply-result-triggers-waiter`.

Sources:

- [`raft.go`: committed entries feed apply](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/raft.go#L218-L231)
- [`raft.go`: unstable entries feed WAL storage](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/raft.go#L243-L260)
- [`server.go`: waiter trigger occurs inside entry application](https://github.com/etcd-io/etcd/blob/58f45a9ff1c083130830eb02b0cc7d9783609095/server/etcdserver/server.go#L1968-L1981)

## Boundaries

- This episode starts inside the v3 Put handler; it does not explain client-side
  endpoint selection, forwarding, authentication policy, or Raft quorum math.
- It follows one normal Put, not transactions, deletes, leases, snapshots, or
  no-space alarm recovery.
- It distinguishes logical visibility, WAL persistence, and physical backend
  commit. It does not claim that those are three globally serialized moments.
- It intentionally leaves the cross-`Ready` identity needed to order client ack
  against the matching WAL save as unknown.

## Focused checks

These commands were run successfully against the pinned revision:

```sh
go test ./server/etcdserver \
  -run '^(TestApplyRepeat|TestConfigChangeBlocksApply)$' \
  -count=1

go test ./server/storage/backend \
  -run '^(TestBackendBatchIntervalCommit|TestBackendWriteback)$' \
  -count=1
```
