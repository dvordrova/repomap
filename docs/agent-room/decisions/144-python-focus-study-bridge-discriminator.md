# Decision: Python Focus-to-Study Bridge Discriminator

Status: Active. Approved by the repository owner for the real Go/Python product
acceptance matrix and narrowed by two product-supervisor reviews.

## Observed failure

The ordinary pinned Beets run proposed ten Study directions and rejected all ten
because its Study bundle contained only the deprecated
`beets/ui/commands/__init__.py:22 __getattr__` declaration. Targeted research
read `beets/plugins.py:1-80`, before the plugin-loading mechanism, although the
recorded Pyright selector had already located `_get_plugin` at line 406.

## Approved scope

Add one provider-free, test-only bridge discriminator. It routes the recorded
Beets focus through the existing targeted research window planner and the real
Study bundle assembler.

## Acceptance

- the planner emits the exact `beets/plugins.py:366-445` window containing
  `_get_plugin` and `issubclass(obj, BeetsPlugin)`;
- Study exposes a validated exact `_get_plugin:406` anchor;
- no complete Mechanism is invented;
- no production, provider, prompt, UI, Search, report, or repository-run change.

## Stop condition

Stop after the discriminator and return its exact result to both supervisors
before integrating the selector into ordinary Python execution.
