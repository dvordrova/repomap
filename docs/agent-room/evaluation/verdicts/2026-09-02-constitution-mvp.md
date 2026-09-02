# Constitution MVP — dogfood receipt

Date: 2026-09-02
Fixture: `github.com/dvordrova/python-tutorial-game` at `78714d34ee`
Runs: `.bin/repomap <repo> --no-serve --no-open --debug-dir ...` against the
live DeepSeek endpoint, twice (cold with `--no-cache`, then warm).

## Before the change

The published page was the Canvas: a repository screen offering only "Choose a
target to explore its final grouped program graph", then three lanes of model
group cards whose members were `path:line` anchors printed twice. 318 KB of
HTML over 177 KB of template JavaScript and CSS.

Of the seven acceptance questions the reader could answer **one** (roughly
"which responsibilities exist"). What the repository is, how to run it, which
port the frontend talks to, the `exec` of user code, the dead files, the
missing tests/CI/Dockerfile, and the click-to-animation flow were all absent.

## After the change

Same repository, same provider. 92 KB of HTML over 5.9 KB of stylesheet and
script. 86 anchored facts, 28 quoted claims, 0 rejected model rows.

| Question | Answer on the page |
|---|---|
| What is this? | Model summary plus one role per target, each with anchors; the root README quoted with its date and age. |
| How do I run it? | `pipenv run python main.py` in `backend` anchored to `backend/main.py:14` and `backend/Pipfile:11`; `npm start` in `front` anchored to `front/package.json:29`. |
| Where do the parts talk, on which port? | Portals table: `GET /api/levels`, `GET /api/level/{level_id}` (possible), `POST /api/level/run`, each `front/src/service/http.ts:N → backend/app/app.py:N`. Port from `proxy = http://localhost:8080` at `front/package.json:52` and the `APP_PORT` read at `backend/app/settings.py:6`. |
| What is dangerous? | `exec` in `make_step` at `backend/app/field.py:98`, with the source line. |
| What is dead? | Four unreachable front-end files, including `front/src/utils/retry.ts` and `front/src/utils/instructions.ts`. |
| What is missing? | No CI, no Dockerfile, no test files, README is 22 bytes. |
| Main flow? | Seven ordered steps from the level fetch through `POST /api/level/run` to the returned simulation, each anchored and explained in one sentence. |

## Notes

- Provenance is visible: facts plain, model sentences badged, claims quoted
  with source and age.
- `repomap cache clear --debug-dir <dir>` verified against a real run
  directory: 32 cached responses removed.
- Two adjustments came out of the first read: manifest settings now appear in
  each target's configuration (this is what made the port visible), and the
  overview quotes only the shallowest README so Create React App boilerplate
  no longer buries the summary.

## Transfer: chi (Go)

`github.com/go-chi/chi/v5` at `8b258c7bb28f`, live run, 4 of 4 targets
analyzed: 53 anchored facts, 250 quoted claims, a correct summary ("the chi
HTTP router library for Go, with examples demonstrating REST APIs and API
versioning"), and 25 HTTP routes with their methods, path literals and
resolved handlers, including `GET /{articleSlug:[a-z-]+}` at
`_examples/rest/main.go:92`.

Two defects surfaced by reading the page, both fixed:

- the first README quote was a raw `<img>` tag and the third was a wall of
  unrendered Markdown, so claims now quote prose;
- two targets both rendered as "versions", so a repeated name now carries the
  detail that separates it.

### Known limitation: nested route prefixes

A route registered inside a `Route`/`Mount` closure keeps only its own path
literal, so `ListArticles` reads `GET /` rather than `GET /articles`. The
anchor is exact and the handler is right, but the path is incomplete wherever
a router nests. The fact layer already holds what composition needs: the
mounting call retains its own literal, and `passes_callback` links it to the
closure the inner routes are registered on. Composing them is the next
correctness change to the fact layer. The canonical fixture does not nest
routers, so acceptance is unaffected.

Group-mounting calls also report the method `ANY`, which is honest for a
subrouter mount but reads oddly beside real verbs.
