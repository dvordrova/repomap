# How CURRENT.md is managed

You do not fill `docs/agent-room/CURRENT.md` manually.

## Existing active work

When `CURRENT.md` already points to an approved decision:

```text
/go
```

reads it and continues that work.

## Creating the next active work

Run:

```text
/next
```

The workflow creates:

```text
docs/agent-room/095-some-product-increment.md
docs/agent-room/NEXT.md
```

The numbered decision is initially:

```text
Status: Proposed
```

The agent summarizes it and asks:

```text
Activate this decision as CURRENT? yes / no
```

When you answer `yes` or `да`, the same agent performs governance only:

1. changes the numbered decision to `Approved for implementation`;
2. rewrites `CURRENT.md` to point to that decision;
3. removes the temporary `NEXT.md` pointer;
4. commits the governance change locally;
5. tells you to run `/go`.

## Why one confirmation remains

`/go` may freely choose implementation details inside approved scope.

It must not choose a new product direction by itself. A one-word approval after `/next`
is the boundary between:

```text
agent proposes product scope
```

and:

```text
repository owner approves product scope
```

## Recovery rules

If OpenCode is restarted after `/next` created a proposal but before approval, run
`/next` again. It detects the existing `NEXT.md`, shows the proposal, and asks for
approval instead of creating another proposal.

If `CURRENT.md` is missing but historical decisions exist, `/next` proposes or recovers
the next decision. `/go` must not invent and approve scope.
