# Add knowledge from your own tool

An extractor returns nodes and labeled links. It does not assign architecture
roles, create report sections, or implement another LLM pipeline. Built-in
sqlc, source database observations and external commands use the same contract
and normalization.

Put `.repomap.json` at the analyzed repository root:

```json
{
  "version": 1,
  "extractors": [
    {"name": "company", "command": ["python3", "tools/describe_project.py"]}
  ]
}
```

The command runs once per analysis in the repository directory. Repomap
writes one JSON object to stdin:

```json
{"version":1,"root":"/local/project","files":["main.go","schema.sql"],"options":{}}
```

`files` is the current corpus inventory. Your code can read the project's
own configuration using `root`. Optional `options` in the command config is
passed through unchanged; its meaning belongs entirely to your code.

Return one JSON object on stdout; write diagnostic logs to stderr:

```json
{
  "version": 1,
  "nodes": [
    {"id": "generator", "path": "sqlc.yaml", "line": 3},
    {"id": "queries", "path": "database/queries.sql"},
    {"id": "output", "path": "database/sqlc"}
  ],
  "links": [
    {"from": "generator", "to": "queries", "label": "configured query input"},
    {"from": "generator", "to": "output", "label": "configured Go output"}
  ]
}
```

That is the public data model:

| Object | Required | Optional |
|---|---|---|
| node | `id`, plus `path` or `name` | `line`, the other of `path` / `name`, source `data` |
| link | `from`, `to`, `label` | `path`, `line` for more precise evidence |

Node IDs belong to this one extractor. Links use those IDs. Paths are
repository-relative; a node may instead have only a name, for an external
reference or a concept. A link uses its origin node as the source location
unless it supplies its own path. A link from a node without a path must
supply a source path. Omitted lines mean the beginning of the file.

Repomap assigns internal identities and target membership, attaches the
producer's name, and lists current corpus files beneath a directory. An
output directory can be absent. No model classifications, confidence
numbers, symbol IDs, target IDs, file lists or special generator types are
required from the plugin.

An optional `data` object describes source database evidence. It requires
`kind` (`table` or `query`), `origin` (`ddl`, `orm` or `query`), a source `scope`
and `name`. A query also carries its original `sql` and `statement` kind.
Optional fields include `schema`, `connection`, `expression`, `partial`,
mentioned `tables`, and `columns`. Each column has `name` and an `anchor`
(`path`, `line`, optional `column`), with optional written `type`, `primary_key`
and `foreign_key`. An optional `owner` source anchor associates a declaration
with its native code owner. Unknown connection identity is left absent.
Scopes distinguish independently declared schemas; a query mention does not
prove schema ownership, and a join does not establish a foreign key.

Only protocol version 1 is accepted. Unknown fields, duplicate node IDs,
missing link endpoints, invalid paths and command failures produce explicit
errors. There are no compatibility readers or replacement results. Exact
stdin, stdout and stderr are saved in `extractions.json`, including failed
exchanges; normalized nodes and links enter `facts.json` as `entity` and
`relation` rows. A syntactically valid label remains the producer's statement,
not a compiler-proved call or an LLM architecture classification.

## Working examples

`internal/extractors/sqlc.go` is the built-in example. It reads sqlc config
version 2, returns one node per config block and referenced path, and labels
the declared schema/query inputs and outputs. The same response decoder and
fact normalizer handle it and command plugins. It does not run sqlc, apply
migrations, or claim generated files are current.

The built-in database extractor reads source SQL and supported SQLAlchemy model
declarations, reusing sqlc's configured source scopes. It preserves table/column
declarations, SQL text and table mentions. Interpolated or concatenated SQL keeps
its original expression and partial status. It never connects to a database or
establishes that a statement executed. These source observations feed the
ordinary Data catalogue and question evidence through the same facts graph.

`examples/extractors/generator.py` is a small standalone example using only
Python's standard library. It reads a company-specific `codegen.json`:

```json
{"input":"schema.sql","output":"generated"}
```

Copy it to `tools/describe_project.py` in the repository and use the config
above. To choose another configuration file, add
`"options":{"config":"another-codegen.json"}` to the command entry.
Input and output paths in this example are relative to the repository root.

## Current integration boundary

The ordinary command runs extractors after language indexing, before fact
normalization. Their results enter the shared fact layer and the same places
graph used by the atlas reader. The question table reads entity declarations,
relationships and every corpus member beneath referenced paths. Related code
files receive the producer's source context too. Model-selected anchors are
restored locally, including a configuration anchor for an output that was not
collected. Producer observations and corpus membership keep their own edge
kinds; neither becomes a compiler-proved call.

This does not create language targets, execute a generator, apply migrations,
or automatically build a change recipe. Saved places use graph v11 and reading
input v12; incompatible analysis inputs must be regenerated. The external plugin
protocol remains v1 with the nodes and links above.
