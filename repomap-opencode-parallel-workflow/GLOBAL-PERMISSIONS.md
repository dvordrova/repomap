# Always allow the repomap cache and fixture repositories

OpenCode treats paths outside the directory where it was started as external. To remove
the external-directory approval prompt globally, add this to:

```text
~/.config/opencode/opencode.json
```

```json
{
  "$schema": "https://opencode.ai/config.json",
  "permission": {
    "external_directory": {
      "~/Library/Caches/repomap/**": "allow",
      "~/git/**": "allow"
    }
  }
}
```

Use `**`, not a single `*`, for the recursive directory tree.

OpenCode merges global and project configuration. The global config applies to all
projects; a project or managed config may still override conflicting rules.

## Important security meaning

`external_directory: allow` removes the extra “this path is outside the workspace”
approval gate. It does not turn a tool that is explicitly denied into an allowed tool.

However, for an agent whose `edit` and `bash` permissions are allowed, this also means it
can potentially modify files under `~/git/**` without an external-directory prompt.

The repomap workflow's fixture/oracle/audit agents are separately constrained by their
agent contracts and edit permissions. Global permission is broader and should only be
used because these are trusted local directories.

## Automatic helper

Run:

```bash
./repomap-opencode-parallel-workflow/install-global-permissions.sh
```

The helper safely creates or merges a strict JSON global config. It refuses to rewrite an
existing JSONC file so it does not destroy your comments; in that case, copy the snippet
manually.
