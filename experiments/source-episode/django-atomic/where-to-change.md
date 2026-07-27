# Where to place a transactional outbox handoff

Question: **Where should a Django application place an outbox write and its dispatch trigger so nested transaction.atomic() rollback semantics are respected, dispatch starts only after the outer commit, and the remaining delivery gap is explicit?**

Accepted input: `django-nested-atomic` at `django/django@3e389b7ddaf08109900da5415ddaac5a355a170f`.

## Short answer — INFERRED

- **Place the row and registration inside the owning rollback scope** — Create the outbox row at the application-service boundary that owns the business mutation, using the same active atomic(using=alias) scope and database alias. Before that rollback scope exits, register transaction.on_commit(lambda: wake(outbox_id), using=alias). An inner or outer rollback then removes both the row and the callback registration; only a successful outer commit allows the callback to run. The callback is a latency optimization. Independently scanning pending rows is the recovery path.

## What the accepted episode establishes

- **CORROBORATED** — Inner success remains provisional until outer success. Rows written and callbacks registered by a successful inner block can still be removed by a later rollback of an enclosing savepoint or the outer transaction.
- **CORROBORATED** — `on_commit()` callbacks follow their rollback scope. Registration captures active savepoint IDs, so rollback prunes callbacks from that scope while callbacks from surviving sibling or outer scopes remain queued.
- **CORROBORATED** — Surviving callbacks run after commit and autocommit restoration. After the outer body exits without error and commit succeeds, __exit__ restores autocommit and drains surviving callbacks in registration order before returning; callback code can therefore query or start a new transaction.
- **INFERRED** — Treat `on_commit()` as a post-commit handoff, not a delivery guarantee. The useful design signal is to place side effects after transaction success, while recognizing that this in-process queue supplies no durable retry or crash-recovery contract.

## Where to change — INFERRED

1. **Application-service mutation boundary.** Insert the outbox row beside the business write inside the same atomic(using=alias) block and on the same alias, so both writes share every enclosing savepoint and the outer commit.
2. **Post-commit wake-up.** After the row has an ID and before the current transaction or savepoint scope exits, register transaction.on_commit(lambda: wake(outbox_id), using=alias). If registration happens after the owning transaction has exited, on_commit() executes immediately and no longer provides ordering against that commit.
3. **Recovery dispatcher.** Independently scan pending outbox rows and retry unfinished publication. Use the outbox ID as a stable deduplication key, and define explicitly when a row becomes delivered. Treat on_commit() only as a low-latency wake-up for this recovery path.

## What behavior changes — INFERRED

- **Inner rollback** — The outbox row is rolled back and the callback registration from that savepoint scope is pruned.
- **Inner success followed by outer rollback** — The provisional outbox row is rolled back later and the queued callback never runs.
- **Successful outer commit** — The transaction commits the outbox row, Django restores autocommit, and the surviving wake-up callback then runs.
- **Process exit after commit but before callback completion** — The committed pending row remains, the fast wake-up may be lost, and independent scanning is required to resume publication.

## What remains unknown — UNKNOWN

- **Commit-to-callback gap** — Django does not guarantee that an in-process callback completes after commit. In this proposed design, process exit can therefore leave a committed pending outbox row without the fast wake-up; only independent scanning can recover it.
- **Publish-to-record gap** — If the dispatcher publishes an event and exits before recording completion, a retry may publish it again. Broker acknowledgement, mark-delivered ordering, and consumer deduplication were not inspected.
- **Exact application edit points were not inspected** — This framework episode contains no application service, outbox schema, broker adapter, or dispatcher, so it cannot name a project file or establish retry and idempotency behavior.

## Smallest useful proof — INFERRED

1. **Rollback scopes.** Prove that inner rollback and outer rollback after inner success leave neither the outbox row nor a runnable callback.
2. **Commit order.** Prove that outer commit makes the row visible before the wake-up callback runs.
3. **Lost wake-up.** Suppress the callback and prove that independent scanning still finds and publishes the pending row.
4. **Duplicate publication.** Crash after publish but before completion recording and prove that retry does not create an unintended duplicate effect.

## Source navigation — context, not automatic edit points

These accepted locations explain the selected behavior. Treat them as navigation unless the change section explicitly names one as an edit point.

- [`django/db/transaction.py:242`](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/transaction.py#L242-L271)
- [`django/db/backends/base/base.py:323`](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/backends/base/base.py#L323-L341)
- [`django/db/backends/base/base.py:405`](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/backends/base/base.py#L405-L431)
- [`django/db/backends/base/base.py:470`](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/backends/base/base.py#L470-L492)
- [`django/db/backends/base/base.py:727`](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/backends/base/base.py#L727-L750)
- [`django/db/backends/base/base.py:752`](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/backends/base/base.py#L752-L769)
- [`tests/transaction_hooks/tests.py:87`](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/tests/transaction_hooks/tests.py#L87-L109)
- [`tests/transaction_hooks/tests.py:111`](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/tests/transaction_hooks/tests.py#L111-L177)
- [`tests/transaction_hooks/tests.py:234`](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/tests/transaction_hooks/tests.py#L234-L266)
