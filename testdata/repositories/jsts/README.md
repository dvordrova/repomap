# Cumulative JavaScript and TypeScript repository

This workspace retains five source-owning packages: the root application,
two storage packages, documentation tools and a canvas UI. Shared compiler
settings do not merge those package boundaries. Each package has its own
manifest and source declarations, even without a start or bin command.

The manifests deliberately do not declare TypeScript. Analysis can use a
prepared local compiler or one from the selected Node environment; that choice
does not supply missing repository imports or change the observed calls.

The documentation-tools config includes a sibling's sources for documentation.
Its own JavaScript and TypeScript tools remain explicit manifest script inputs;
the sibling sources and an unlisted script do not join that tool package.

The canvas UI's manifest exports `canvas.mjs` alone: a program importing the
package reaches `drawCanvas`, and `layout`, which `layout.mjs` exports to its
sibling module, is no part of the package's API. The canvas UI tests itself
the way the repomap report UI does. Its test
script runs the unit test beside its module with the Node test runner, and
Playwright runs the checks in `visual/` with a reporter module. Those files,
the Playwright config and the stub API that only Playwright's `webServer`
starts are test code. The application server, which the `start` script runs
and `webServer` starts too, the build script and a draft test below the
package root are not.
