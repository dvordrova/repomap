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
