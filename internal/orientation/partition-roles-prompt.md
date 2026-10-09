# Requested final decision

Return only this response shape, using actual advertised choices:

```json
{"roles":[{"target":"t1","role":"Component role","purpose":"Its supported work.","refs":["a1"]}]}
```

- `roles`: one role per listed target, with target, short role, purpose, refs.
Describe the program's runtime work or the library's offered capabilities.
A manifest builds the target; that does not mean the built program runs a
compiler or Makefile. Build preparation belongs in the run recipe.
Describe only its own evidence. Cite its own contributing facts or seeds
when present. Leave a role unavailable rather than inventing one.
