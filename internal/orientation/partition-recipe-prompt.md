# Requested final decision

Return only this response shape, using actual advertised choices:

```json
{"run_recipe":[{"target":"t1","command":"source-supported command","cwd":".","note":"Required inputs.","refs":["a1"]}]}
```

- `run_recipe`: supported commands with target, command, cwd, note, refs.
Give the steps a newcomer needs to build or start/use this target, in order.
Include supported prerequisite build steps. Preserve source-declared
configuration or generation of required build inputs before the applicable
build, including shared prerequisites and their stated conditions; do not
turn an optional reconfiguration task into a mandatory step.
A manifest's task catalogue is not a recipe: unrelated maintenance, cleanup
and diagnostic commands are not preparation or launch steps.
Every step must cite a manifest or entrypoint source; exports never support
launch. Keep required arguments, placeholders, usage literals and config
prerequisites. A launch point alone is no complete usable invocation.
Omit an invocation the supplied evidence cannot support rather than presenting
its bare entrypoint as sufficient. Preserve source usage arguments exactly;
use explicit named placeholders only for values the user must supply.
Build variables, compiler flags and output suffixes describe settings, not a
complete invocation. Never combine them into a compiler or linker command
with missing input/output operands or an invented consumer. Prefer the
source-supported build task; its settings can appear in its note. An ellipsis
that abbreviates missing command operands is not an explicit user placeholder.
For a library, stop after supported preparation/build/install unless the
supplied source shows a usable consumer or example invocation; API exports
alone do not establish one.
Command paths are relative to cwd; source directory is not automatically cwd.
