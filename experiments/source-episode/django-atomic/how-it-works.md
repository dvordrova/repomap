# Nested `atomic()` blocks do not nest database commits

`episode_id: django-nested-atomic`

Pinned source: [`django/django@3e389b7ddaf08109900da5415ddaac5a355a170f`](https://github.com/django/django/tree/3e389b7ddaf08109900da5415ddaac5a355a170f)

## Short answer

On the ordinary path where autocommit was initially enabled, a successful
inner `transaction.atomic()` usually releases a savepoint. It does **not**
commit the database transaction. Its writes can still be rolled back by an
enclosing block, and its `on_commit()` callbacks stay queued.

When the outer block body exits without error and `connection.commit()`
succeeds, `Atomic.__exit__` restores autocommit and runs the callbacks that
survived every rollback boundary before returning. A non-robust callback can
therefore make the `with` statement raise even though the database commit has
already succeeded.

The labels below describe confidence, not permission to read a result:

- `EXTRACTED` — directly visible in one pinned code path.
- `CORROBORATED` — code and focused tests tell the same story.
- `INFERRED` — a useful engineering interpretation with a stated limit.
- `UNKNOWN` — the inspected boundary cannot answer it.

## Keep these moments separate

| Moment | Transaction state | `on_commit()` state |
| --- | --- | --- |
| Successful nested `atomic()` | Savepoint released; inner and outer writes are still provisional | Still queued |
| Nested savepoint rollback | Inner writes removed; earlier outer writes can continue | Callbacks from that savepoint discarded |
| Successful `savepoint=False` block | No local checkpoint and no commit | Still queued |
| Failed `savepoint=False` block | No rollback occurs at this inner exit; `needs_rollback` propagates until the nearest real boundary rolls back | May remain queued meanwhile, then is pruned or cleared by that rollback |
| `commit()` returned inside outer `__exit__` | Database commit has succeeded | Queue is armed but not yet drained |
| Autocommit restored inside `__exit__` | Post-transaction context, before `__exit__` returns | Surviving callbacks run in registration order |

## The causal branches

### The ordinary outer block owns the transaction commit — **EXTRACTED**
<!-- episode-claim {"id":"claim-outermost-owns-commit-boundary","state":"extracted","anchor_ids":["anchor-atomic-enter","anchor-atomic-clean-exit"],"support_fact_ids":["fact-entry-chooses-transaction-or-savepoint","fact-clean-exit-releases-or-commits"]} -->

With autocommit initially enabled, the outer block resets rollback state and
disables autocommit. When its body exits without error, the corresponding
`__exit__` branch calls `connection.commit()`. Manual transaction management is
deliberately outside this episode because the ownership of that boundary is
different.

Sources:

- [`Atomic.__enter__`: start the outer boundary or create a nested savepoint](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/transaction.py#L194-L219)
- [`Atomic.__exit__`: release a savepoint or commit the transaction](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/transaction.py#L242-L271)

### A nested atomic block is usually a savepoint, not a nested commit — **CORROBORATED**
<!-- episode-claim {"id":"claim-nested-block-is-savepoint-not-commit","state":"corroborated","anchor_ids":["anchor-atomic-enter","anchor-atomic-clean-exit","anchor-test-delayed-through-nesting"],"support_fact_ids":["fact-entry-chooses-transaction-or-savepoint","fact-clean-exit-releases-or-commits","fact-tests-delay-until-outer-commit"]} -->

A default nested block creates a savepoint. Clean exit calls
`savepoint_commit(sid)`, which releases that savepoint; it does not call the
connection's transaction commit. The name is easy to overread. Django's tests
make the distinction concrete: a callback remains silent after inner success
and runs only after the final outer exit.

Sources:

- [`Atomic.__enter__`: nested savepoint selection](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/transaction.py#L194-L219)
- [`Atomic.__exit__`: mutually exclusive savepoint-release and transaction-commit branches](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/transaction.py#L242-L271)
- [Tests: callbacks remain delayed through nested success](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/tests/transaction_hooks/tests.py#L87-L109)

### Inner success remains provisional until outer success — **CORROBORATED**
<!-- episode-claim {"id":"claim-inner-success-remains-rollbackable","state":"corroborated","anchor_ids":["anchor-atomic-clean-exit","anchor-test-delayed-through-nesting","anchor-test-rollback-scopes"],"support_fact_ids":["fact-clean-exit-releases-or-commits","fact-tests-delay-until-outer-commit","fact-tests-separate-rollback-scopes"]} -->

Releasing an inner savepoint removes a local checkpoint; it does not make the
inner writes independent. A later outer failure rolls back both outer and
already-successful inner work. The same is true of callbacks registered inside
that inner block: they still depend on the enclosing transaction.

Sources:

- [`Atomic.__exit__`: inner release versus outer commit](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/transaction.py#L242-L271)
- [Tests: nested callbacks wait for the outer transaction](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/tests/transaction_hooks/tests.py#L87-L109)
- [Tests: outer rollback removes successful inner rows and callbacks](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/tests/transaction_hooks/tests.py#L111-L177)

### A failed inner savepoint can be isolated — **CORROBORATED**
<!-- episode-claim {"id":"claim-inner-failure-uses-nearest-real-boundary","state":"corroborated","anchor_ids":["anchor-atomic-rollback-exit","anchor-savepoint-callback-discard","anchor-test-rollback-scopes"],"support_fact_ids":["fact-error-uses-nearest-real-boundary","fact-savepoint-rollback-prunes-hooks","fact-tests-separate-rollback-scopes"]} -->

When a real savepoint exists, an error rolls back that savepoint. If the
exception is handled outside the failed inner block, earlier outer work can
continue. Django also removes callbacks registered while the failed savepoint
was active, while callbacks from surviving sibling or outer scopes remain.

Sources:

- [`Atomic.__exit__`: roll back the nearest real savepoint](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/transaction.py#L272-L299)
- [`BaseDatabaseWrapper`: prune callbacks registered under that savepoint](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/backends/base/base.py#L405-L431)
- [Tests: failed inner scope disappears while successful siblings survive](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/tests/transaction_hooks/tests.py#L111-L177)

### `savepoint=False` removes a recovery boundary, not just overhead — **CORROBORATED**
<!-- episode-claim {"id":"claim-savepoint-false-removes-local-recovery","state":"corroborated","anchor_ids":["anchor-atomic-enter","anchor-atomic-rollback-exit","anchor-test-rollback-scopes"],"support_fact_ids":["fact-entry-chooses-transaction-or-savepoint","fact-error-uses-nearest-real-boundary","fact-tests-separate-rollback-scopes"]} -->

The nested block records `None` instead of a savepoint ID. Clean exit performs
no local commit. On failure there is no local point to roll back to, and no
rollback occurs at that inner exit: `needs_rollback` propagates until the
nearest real enclosing savepoint or outer transaction rolls back. A callback
need not be removed at the inner exit itself, but it cannot run if that
enclosing scope rolls back.

Sources:

- [`Atomic.__enter__`: record the absence of a savepoint](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/transaction.py#L194-L219)
- [`Atomic.__exit__`: propagate rollback when the savepoint ID is `None`](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/transaction.py#L272-L299)
- [Tests: a `savepoint=False` failure rolls back its enclosing transaction scope](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/tests/transaction_hooks/tests.py#L111-L177)

### `on_commit()` callbacks follow their rollback scope — **CORROBORATED**
<!-- episode-claim {"id":"claim-oncommit-follows-savepoint-scope","state":"corroborated","anchor_ids":["anchor-oncommit-queue-or-immediate","anchor-savepoint-callback-discard","anchor-test-rollback-scopes"],"support_fact_ids":["fact-oncommit-captures-savepoint-scope","fact-savepoint-rollback-prunes-hooks","fact-tests-separate-rollback-scopes"]} -->

Inside `atomic()`, registration stores the callback together with the active
savepoint IDs. Savepoint rollback uses that snapshot to discard exactly the
callbacks registered under the failed scope. This queue is local to one
database connection; separate aliases are not one atomic boundary.

Sources:

- [`on_commit`: capture the active savepoint set](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/backends/base/base.py#L727-L750)
- [Savepoint rollback: discard callbacks from that scope](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/backends/base/base.py#L405-L431)
- [Tests: successful and rolled-back callback scopes](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/tests/transaction_hooks/tests.py#L111-L177)

### Surviving callbacks run after commit and autocommit restoration — **CORROBORATED**
<!-- episode-claim {"id":"claim-callbacks-run-after-commit-and-autocommit","state":"corroborated","anchor_ids":["anchor-connection-commit-rollback","anchor-autocommit-runs-hooks","anchor-test-postcommit-context"],"support_fact_ids":["fact-commit-arms-hooks-rollback-clears","fact-outer-exit-restores-autocommit","fact-hooks-run-after-autocommit-restored","fact-tests-postcommit-order-and-context"]} -->

A successful connection commit arms the queue. Still inside `Atomic.__exit__`,
the outer cleanup restores autocommit, and `set_autocommit(True)` then drains
surviving callbacks before `__exit__` returns. Tests show callbacks issuing
queries and opening a fresh atomic block, and show survivors running in
registration order.

A callback exception happens after the commit. It cannot roll that transaction
back. A non-robust exception can still interrupt the remaining local callback
drain.

Sources:

- [`commit` arms hooks; `rollback` clears them](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/backends/base/base.py#L323-L341)
- [`set_autocommit(True)` restores state before draining hooks](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/backends/base/base.py#L470-L492)
- [Tests: callbacks query and start fresh transactions after commit](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/tests/transaction_hooks/tests.py#L234-L266)

## A useful weak signal

### Treat `on_commit()` as a post-commit handoff, not a delivery guarantee — **INFERRED**
<!-- episode-claim {"id":"claim-oncommit-is-handoff-not-delivery","state":"inferred","anchor_ids":["anchor-connection-commit-rollback","anchor-hook-runner","anchor-test-postcommit-context"],"support_fact_ids":["fact-commit-arms-hooks-rollback-clears","fact-hooks-run-after-autocommit-restored","fact-tests-postcommit-order-and-context"]} -->

For application design, `on_commit()` is a useful place to move work that must
not happen before transaction success. But the inspected mechanism is an
in-process callback list, cleared before invocation. It is not itself a durable
outbox, retry queue, or proof of delivery after a process crash.

That “handoff” framing is an interpretation, not a Django type or named
abstraction. It is shown because it is useful, with its uncertainty attached.

Sources:

- [`commit` marks the in-memory queue ready](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/backends/base/base.py#L323-L341)
- [The callback runner detaches and drains the queue](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/django/db/backends/base/base.py#L752-L769)
- [Tests: callbacks execute in post-commit application context](https://github.com/django/django/blob/3e389b7ddaf08109900da5415ddaac5a355a170f/tests/transaction_hooks/tests.py#L234-L266)

## What remains unknown

**UNKNOWN — physical durability.** This episode reaches Django's successful
backend `connection.commit()` return. It does not establish when a particular
database, replica set, filesystem, or device makes those bytes durable.

**UNKNOWN — delivery after a process crash.** A process can exit after the
database commit and before or during callback execution. Nothing in this
in-memory queue establishes retry or eventual delivery.

Those unknowns do not erase the useful control-flow result: inner success is
still provisional, the outer commit is the transaction boundary, and surviving
callbacks are post-commit work.
