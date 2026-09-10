# Cumulative JavaScript and TypeScript repository

This workspace retains four source-owning packages: the root application,
two storage packages, and documentation tools. Shared compiler
settings do not merge those package boundaries. Each package has its own
manifest and source declarations, even without a start or bin command.

The manifests deliberately do not declare TypeScript. Analysis can use a
prepared local compiler or one from the selected Node environment; that choice
does not supply missing repository imports or change the observed calls.

The documentation-tools config includes a sibling's sources for documentation.
Its own JavaScript and TypeScript tools remain explicit manifest script inputs;
the sibling sources and an unlisted script do not join that tool package.
