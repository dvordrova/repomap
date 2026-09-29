"""Ground-truth inventory of othello for the claim audit (internal/audit).

Run from anywhere: python3 testdata/audit/othello/generate.py
It reads the checkout ~/git/othello at the pinned revision (PINNED below)
only, refuses any other HEAD, and rewrites inventory.json beside this script. After the freeze a
change is a dated erratum with a code citation, never a silent edit.
"""
import json, subprocess, os
REPO = os.path.expanduser("~/git/othello")
PINNED = "b8130da8608698667dc78b0c2f7cf4120f65111e"
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "inventory.json")
os.chdir(REPO)
rev = subprocess.check_output(["git", "rev-parse", "HEAD"]).decode().strip()
assert rev == PINNED, f"{REPO} is at {rev}, the inventory is pinned to {PINNED}"
items = []


def add(component, inventory, kind, name, anchor, handler, must, why, **extra):
    d = {"component": component, "inventory": inventory, "kind": kind, "name": name,
         "anchor": anchor, "handler": handler or None, "must": must, "why": why}
    d.update(extra)
    items.append(d)


DESK, WEB, TOOL = "othello-desktop", "othello-web", "dev-tooling"

# ---------- commands: launch ----------
add(DESK, "commands", "alias", "clj -M:run", "deps.edn:25", "othello.core/-main", True,
    "deps.edn alias that adds Quil and runs -m othello.core; the only way to start the desktop game.")
add(WEB, "commands", "alias", "clj -M:web watch app", "deps.edn:27", "othello.ui.web/init", True,
    "Runs shadow-cljs; build :app (shadow-cljs.edn:6) compiles to public/js with init-fn othello.ui.web/init; page at localhost:8080/index.html.")

# ---------- commands: in-game input, same shared handlers wired into both programs ----------
wiring = {DESK: ("src/othello/ui/sketch.clj", 43, 44, 45), WEB: ("src/othello/ui/web.cljs", 46, 47, 48)}
for comp, (f, press, move, key) in wiring.items():
    for k, cmd, l, w in [(":n", ":new-game", 211, "new game with the same colour"),
                         (":u", ":undo", 212, "undo the last turn (you + computer), or cancel an animation"),
                         (":h", ":hints", 213, "toggle legal-move hints"),
                         (":1", ":play-black", 214, "restart playing Black (you move first)"),
                         (":2", ":play-white", 215, "restart playing White (computer opens)")]:
        add(comp, "commands", "key", f"{k} -> {cmd}", f"src/othello/ui/events.cljc:{l}", "handle-button", True,
            f"Key binding in key->command: {w}; wired via :key-pressed host/on-key ({f}:{key}).")
    add(comp, "commands", "mouse", "click a board square", "src/othello/ui/events.cljc:174", "handle-board-click", True,
        f"Main move input: a legal square starts the move animation, an illegal one flashes; wired via :mouse-pressed ({f}:{press}).")
    for bid, l, w in [(":new-game", 47, "New Game"), (":undo", 48, "Undo"), (":play-black", 49, "Play Black"),
                      (":play-white", 50, "Play White"), (":hints", 51, "Hints")]:
        add(comp, "commands", "button", f"sidebar button {bid} ({w})", f"src/othello/ui/layout.cljc:{l}", "handle-button",
            False, "Sidebar button hit-tested by button-at; same command as its key.")
    add(comp, "commands", "mouse", "mouse move (hover ghost disc)", "src/othello/ui/events.cljc:231", "on-mouse-move", False,
        f"Stores the pointer; the view shows a ghost disc and hand cursor on legal squares; wired via :mouse-moved ({f}:{move}).")

# ---------- workers ----------
add(DESK, "workers", "future", "AI move search in a future", "src/othello/ui/sketch.clj:29", "launch-ai", True,
    "Runs ai/move (negamax, alpha-beta, iterative deepening, 1200 ms budget) off the sketch thread; polled with realized?.")
add(DESK, "workers", "frame-loop", "Quil :update frame loop (60 fps)", "src/othello/ui/sketch.clj:41", "update-state", True,
    "Every frame: host/update-state -> events/on-frame ticks animations, pass notice and AI job; launches the future when needed.")
add(DESK, "workers", "frame-loop", "Quil :draw per frame", "src/othello/ui/sketch.clj:42", "othello.ui.draw/draw-state", False,
    "Paints the view model each frame.")
add(WEB, "workers", "timer", "AI move search via js/setTimeout (20 ms)", "src/othello/ui/web.cljs:29", "launch-ai", True,
    "Defers ai/move (400 ms budget) to a timeout on the browser main thread; the result lands in an atom polled each frame.")
add(WEB, "workers", "frame-loop", "Quil :update frame loop (60 fps)", "src/othello/ui/web.cljs:44", "update-state", True,
    "Every frame: host/update-state -> events/on-frame ticks animations and launches the AI timeout when needed.")
add(WEB, "workers", "frame-loop", "Quil :draw per frame", "src/othello/ui/web.cljs:45", "othello.ui.draw/draw-state", False,
    "Paints the view model each frame on the p5 canvas.")

# ---------- external ----------
add(WEB, "external", "cdn-script", "https://cdn.jsdelivr.net/npm/p5@1.7.0/lib/p5.min.js", "public/index.html:26", None, True,
    "The browser fetches p5 from jsDelivr; the bundle resolves \"p5\" to this global (shadow-cljs.edn:10).")

# ---------- data ----------
for comp in (DESK, WEB):
    add(comp, "data", "in-memory-store", "UI state map (Quil fun-mode state)", "src/othello/ui/events.cljc:8", "fresh-ui", True,
        "Single state map: :game, :phase, :pointer, :animation, :hints?, frame counters, :ai and :ai-job; threaded through every handler.",
        struct="map built by othello.ui.events/fresh-ui (events.cljc:10-23); :game from othello.game/new-game")
    add(comp, "data", "in-memory", "game map with :board, :moves and :history", "src/othello/game.cljc:5", "new-game", False,
        "Game record; :history holds snapshots (game.cljc:34) used by undo.")
    add(comp, "data", "in-memory", "board vector of 64 cells", "src/othello/board.cljc:50", "initial-board", False,
        "Flat vector of :empty/:black/:white indexed row*8+col.")
    add(comp, "data", "constant-table", "positional weights table", "src/othello/ai/eval.cljc:5", None, False,
        "Static 64-entry evaluation weights used by the AI.")

# ---------- dev tooling (deps.edn aliases running third-party tools) ----------
for a, l, w in [("clj -M:spec", 4, "runs the Speclj suite in spec/"),
                ("clj -M:cov", 7, "runs Speclj under Cloverage (lcov); not in the README"),
                ("clj -M:crap", 14, "runs crap4clj (coverage + complexity report)"),
                ("clj -M:mutate <file>", 18, "runs clj-mutate mutation testing on one source file")]:
    add(TOOL, "commands", "alias", a, f"deps.edn:{l}", None, False, f"Developer alias: {w}.")

nots = [
    (WEB, "dev-http 8080 (index.html)", "shadow-cljs.edn:3",
     "shadow-cljs's static dev server for public/; the repository defines no request handler."),
    (WEB, "nREPL port 7888", "shadow-cljs.edn:2", "shadow-cljs development REPL port, not a product endpoint."),
    (WEB, ":after-load othello.ui.web/init", "shadow-cljs.edn:11", "Hot-reload hook of the dev build, not a worker."),
    (WEB, "/js/main.js", "public/index.html:27", "The app's own compiled bundle from the same origin, not an external service."),
    (WEB, "p5 npm dependency", "package.json:5",
     "Not bundled: shadow-cljs resolves \"p5\" to the global loaded from the CDN script."),
    (WEB, "public/js output dir", "shadow-cljs.edn:7", "Build output written by shadow-cljs, not runtime data."),
    (DESK, "-main arguments", "src/othello/core.clj:5", "-main ignores its arguments; the desktop game has no flags or subcommands."),
    (DESK, "right mouse button", "src/othello/ui/host.cljc:18", "Right clicks are explicitly ignored; not a command."),
    (DESK, "q/create-font \"SansSerif\"", "src/othello/ui/sketch.clj:16",
     "Java logical font resolved by the local JVM; not an external load, file or service."),
    (DESK, "\"N new   U undo   H hints\" / \"1 play black   2 play white\"", "src/othello/ui/draw.cljc:149",
     "Help text painted in the sidebar; the bindings live in key->command."),
    (DESK, "button labels \"New Game\", \"Undo\", ...", "src/othello/ui/layout.cljc:47",
     "Display labels; commands are dispatched by :id in handle-button."),
    (DESK, "events/tick-ai", "src/othello/ui/events.cljc:156",
     "Synchronous AI path used only by specs (spec/othello/spec_helper.clj:54), not a worker."),
    (DESK, "timed-deepen deadline", "src/othello/ai/search.cljc:84",
     "Time budget checked inside the synchronous search, not a timer."),
    (DESK, "anim frame constants", "src/othello/ui/anim.cljc:3", "Frame counts consumed by the frame loop, not timers."),
    (TOOL, "--max-workers 3", "README.md:49", "clj-mutate option in the README, not a worker or flag of the game."),
    (TOOL, "git dependencies on github.com", "deps.edn:15", "Build-time dependency resolution for dev aliases, not runtime communication."),
    (TOOL, "clj-mutate manifest comments (:test-command \"clj -M:spec --tag ~no-mutate\")", "src/othello/ui/events.cljc:238",
     "Tool metadata comments in each source file, not code or commands of the game."),
    ("test", "spec/**", "spec/othello/architecture_spec.clj:7",
     "Test-only code (it reads src/ files to check dependencies); not product data or behaviour."),
]
not_list = [{"component": c, "name": n, "anchor": a, "reason": r} for c, n, a, r in nots]

components = [
    {"name": DESK, "kind": "program (JVM, Quil/Processing window)", "built_by": "deps.edn:25 (:run)",
     "entry": "src/othello/core.clj:5 -main -> src/othello/ui/sketch.clj:36 start!",
     "sources": "src/othello/core.clj, src/othello/ui/sketch.clj + shared .cljc (board, rules, game, ai, ui/*)"},
    {"name": WEB, "kind": "program (ClojureScript browser bundle, p5 canvas)", "built_by": "shadow-cljs.edn:6 (:app) via deps.edn:27 (:web)",
     "entry": "src/othello/ui/web.cljs:40 defsketch, init at src/othello/ui/web.cljs:51",
     "sources": "src/othello/ui/web.cljs, public/index.html + shared .cljc"},
    {"name": TOOL, "kind": "developer aliases (not programs of the repository)", "built_by": "deps.edn:4-24"},
]
notes = [
    "Two programs share the .cljc domain and UI logic (board, rules, game, ai, ui/events, ui/host, ui/view, ui/draw); no separate library artifact is built. Shared inputs and the state map are listed once per program with the same anchor.",
    "No requests: neither program serves a connection. The browser page is served by shadow-cljs dev-http (port 8080), which has no handler in this repository.",
    "No environment variables, config files, databases or files are read or written at runtime; the desktop program opens no network connection. The only runtime network access is the browser loading p5 from jsDelivr.",
    "Key bindings are Quil keywords :n :u :h :1 :2 (lower case); the README shows N/U/H. Shift or Caps Lock would produce different keywords and do nothing (inference from Quil key-as-keyword, not tested here).",
    "The web AI job is not parallel: js/setTimeout runs the search on the browser main thread after 20 ms, which is why the web budget is smaller (400 ms, depth 4, endgame 8 vs 1200 ms, depth 6, endgame 12).",
    "Sidebar buttons duplicate the key commands and are marked must=false; board click and the five keys are must.",
    "dev-tooling aliases run third-party tools (Speclj, Cloverage, crap4clj, clj-mutate); listed as must=false commands only.",
]
review = [
    {"change": "added trap 'q/create-font \"SansSerif\"'", "anchor": "src/othello/ui/sketch.clj:16",
     "why": "The only resource the desktop program asks for by name at runtime; it is a JVM logical font, so a claim of an external load or font file would be wrong."},
]
review_summary = ("Reviewer (read-only, code only): opened every must=true anchor (2 launch aliases, 5 keys + board click per program, "
                  "future/setTimeout and :update loop per program, p5 CDN script, UI state map per program); all land on the named "
                  "code with the right handler and component. Independent greps (future/Thread/setTimeout/setInterval/agent/promise, "
                  "slurp/spit/io/, js/, getenv, fetch/http, key->command, Quil handler wiring in sketch.clj and web.cljs, index.html, "
                  "deps.edn, shadow-cljs.edn) found no missing must item. All 17 original traps verified at their anchors. "
                  "Edits were made in _gen/gen_othello.py and the JSON regenerated.")
doc = {"repository": "othello", "path": "~/git/othello", "revision": rev, "components": components,
       "items": items, "not": not_list, "notes": notes, "review_summary": review_summary, "review": review}
json.dump(doc, open(OUT, "w"), indent=1, ensure_ascii=False)
print("wrote", OUT, len(items), "items", len(not_list), "not")
