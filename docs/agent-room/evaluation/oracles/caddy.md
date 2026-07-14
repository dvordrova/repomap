# Independent oracle: Caddy (decision 094, regression fixture)

## Scope and evidence boundary

This is an independently collected source oracle for **only** `/Users/dvordrova/git/caddy`. It was prepared without reading a generated repomap report. Paths below are absolute only where they identify the fixture; source anchors are repository-relative and include symbols/lines at the recorded revision.

## Exact facts

### Fixture state

| Item | Value |
|---|---|
| Fixture name | `caddy` |
| Absolute path | `/Users/dvordrova/git/caddy` |
| Git revision | `873fac5fc094fe538d0c477509127bb321d51a32` |
| Revision subject | `caddyfile: treat quoted braces as literal arguments (#7875)` |
| Worktree/index state | clean (`git status --porcelain=v1` had zero entries; both index and worktree diffs were quiet) |
| Build/generated-source state | no untracked or ignored paths were present at observation; `.gitignore` identifies local Caddy binaries under `cmd/caddy/`, `dist`, `caddy-build`, and `caddy-dist` as ignored build artifacts (`.gitignore:12-27`). No build was run for this oracle. |
| Go module | `github.com/caddyserver/caddy/v2` (`go.mod:1`), Go `1.25.1` (`go.mod:3`) |

### Executables and entry surfaces

* The only main package reported by `go list ./...` is `github.com/caddyserver/caddy/v2/cmd/caddy`, rooted at `cmd/caddy`.
* **Primary executable:** the `caddy` command. Its executable `main` calls `caddycmd.Main`; it blank-imports `modules/standard`, making the standard module set available: `cmd/caddy/main.go:15-42`.
* **Secondary executables:** none found by `go list ./...` at this revision. `cmd/main.go` is package `caddycmd`, not a second main package; it supplies `Main`: `cmd/main.go:15,64-79`.
* **CLI surface:** Cobra-backed commands include `start`, `run`, `stop`, `reload`, `version`, `list-modules`, `build-info`, `environ`, `adapt`, `validate`, `storage`, `fmt`, `upgrade`, `add-package`, and `remove-package`: `cmd/commands.go:95-603`. The operational core is `run` (`cmd/commandfuncs.go:188-354`); `start` respawns it in the background (`cmd/commandfuncs.go:45-170`).
* **Configuration input surface:** native JSON is the runtime configuration; adapters are registered modules. `LoadConfig` reads a supplied/default Caddyfile or JSON and invokes the selected adapter: `cmd/main.go:95-241`. The Caddyfile adapter parses then emits JSON: `caddyconfig/caddyfile/adapter.go:26-64`; its HTTP server type evaluates directives into a `caddy.Config`: `caddyconfig/httpcaddyfile/httptype.go:53-180`.
* **Administrative HTTP surface:** the running instance exposes config endpoints at `/config/`, `/id/`, and `/stop`; it also exposes pprof and expvar debug endpoints: `admin.go:219-292`. The default local admin listen address is `localhost:2019` unless `CADDY_ADMIN` changes it: `admin.go:58-67,69-88`.
* **Data-plane HTTP(S) surface:** configured HTTP servers serve requests through `(*caddyhttp.Server).ServeHTTP`: `modules/caddyhttp/server.go:420-580`; the HTTP app binds configured listeners and starts `net/http` serving loops: `modules/caddyhttp/app.go:463-670`.
* **Library/programmatic surface:** callers can use `caddy.Run(*Config)` or `caddy.Load([]byte, forceReload)` to apply configuration: `caddy.go:103-142`.

### Internal runtime activities — do not promote these to user entry surfaces

* Module registration is an import-time implementation mechanism (`RegisterModule`): `modules.go:130-161`; the standard binary’s blank-import set is `modules/standard/imports.go:1-18`.
* Configuration decoding, context creation, module provisioning, app start/stop, and autosave are lifecycle internals: `unsyncedDecodeAndRun` (`caddy.go:329-403`), `run` (`caddy.go:405-477`), and `provisionContext` (`caddy.go:479-581`).
* `Context.LoadModule` uses struct tags plus reflection to instantiate, provision, validate, and type-check configured modules: `context.go:140-287`. It is a runtime extension mechanism, not a direct user entrypoint.
* TLS certificate-cache setup, certificate automation, ECH publication, and storage cleaning are internal app activities: `modules/caddytls/tls.go:162-260,400-475`.
* Request preparation, ACME-challenge interception, error routing, and handler-chain dispatch are internals of the configured HTTP listener rather than separate externally selected surfaces: `modules/caddyhttp/server.go:420-580`.

## New-engineer flows

### 1. Launch from a Caddyfile/JSON config to running listeners

`cmd/caddy/main.go:40-42` `main` -> `cmd/main.go:64-79` `caddycmd.Main` -> registered `run` command at `cmd/commands.go:121-168` -> `cmd/commandfuncs.go:188-287` `cmdRun` -> `cmd/main.go:95-241` `LoadConfig` -> (when Caddyfile) `caddyconfig/caddyfile/adapter.go:31-64` `Adapter.Adapt` -> `caddy.Load` at `caddy.go:112-142` -> `unsyncedDecodeAndRun` at `caddy.go:337-402` -> `run`/`provisionContext` at `caddy.go:419-581` -> `(*caddyhttp.App).Start` at `modules/caddyhttp/app.go:465-670`.

### 2. Reload a live instance through CLI or admin API

CLI: `cmd/commands.go:189-209` registers `reload` -> `cmd/commandfuncs.go:376-412` `cmdReload` loads/adapts the source config and POSTs `/load`.

Admin API: `admin.go:262-273` registers `/config/` and `/id/` -> config mutation goes through `changeConfig` (`caddy.go:144-275`) -> it calls `unsyncedDecodeAndRun` (`caddy.go:245-275`) -> the new context is installed before `unsyncedStop(oldCtx)` (`caddy.go:362-375`).

### 3. Serve an HTTP request through the configured route chain

`(*caddyhttp.App).Start` constructs `http.Server{Handler: srv}` and calls `srv.server.Serve` (`modules/caddyhttp/app.go:472-492,550-620`) -> `(*Server).ServeHTTP` (`modules/caddyhttp/server.go:420-580`) prepares the request, handles ACME challenges, and calls `serveHTTP` (`modules/caddyhttp/server.go:454-505,582`) -> the configured primary handler chain is reached immediately after the visible `serveHTTP` prechecks (`modules/caddyhttp/server.go:649` onward).

### 4. Understand why a module or Caddyfile directive is available

The primary binary blank-imports `modules/standard` (`cmd/caddy/main.go:34-38`), whose imports cause module/adapter `init` registration (`modules/standard/imports.go:3-18`). A module registers via `caddy.RegisterModule` (`modules.go:130-161`); the Caddyfile HTTP adapter itself registers as `caddyfile` in `caddyconfig/httpcaddyfile/httptype.go:38-40`. During config provision, `Context.LoadModule` interprets JSON fields and `caddy` tags to load the registered implementation (`context.go:140-287`).

## Informed expectations (not direct exhaustive facts)

### Conceptual responsibilities

* **Core (`caddy.go`, `context.go`, `modules.go`, `admin.go`):** own raw configuration, reload transactions, app lifecycle/context, module registry/loading, and administrative control.
* **Command package (`cmd/`):** expose operating commands, acquire/adapt config, and bridge local CLI actions to the runtime/admin API.
* **Configuration packages (`caddyconfig/`, especially `caddyfile/` and `httpcaddyfile/`):** parse/adapt user-facing configuration into the native JSON graph.
* **Apps/modules (`modules/`):** implement pluggable capabilities. `caddyhttp` owns listeners, routing, and request dispatch; `caddytls` owns TLS policy and certificate automation.

### Dynamic/static-analysis frontiers

* The compiled module set is build-dependent: imports in `cmd/caddy/main.go` and `modules/standard/imports.go` register behavior via `init`; a custom binary can change that set by changing blank imports. Static source alone cannot state the complete module universe of every Caddy binary.
* Config uses `json.RawMessage`, `ModuleMap`, module namespaces, struct tags, reflection, and type assertions (`caddy.go:72-82`, `context.go:140-287`). Exact handler/issuer/storage/plugin edges depend on the supplied JSON/Caddyfile and on registered modules.
* Caddyfile directives are registered dynamically and dispatched through the `registeredDirectives` map (`caddyconfig/httpcaddyfile/httptype.go:119-159`); a simple import graph cannot recover all source-config-to-runtime edges.
* Runtime listeners, protocol selection, TLS, routes, certificate issuance, dynamic configuration loads, and remote administration depend on configuration and environment. The code creates goroutines for serving and background TLS work (`modules/caddyhttp/app.go:618-620`; `modules/caddytls/tls.go:421-460`).

### Claims to avoid

* Do not describe Caddy only as a static web server: its README calls it an extensible server platform, and the core runs configured apps/modules (`README.md:177-185`; `caddy.go:79-101`).
* Do not call every exported function or module lifecycle method an entry surface; `LoadModule`, provisioning, TLS maintenance, and handler dispatch are implementation activities.
* Do not claim Caddyfile is the sole/native configuration format. JSON is native; Caddyfile is an adapter (`README.md:78-81,181-183`; `cmd/main.go:202-241`).
* Do not claim all configuration changes require process restart. The admin API and `caddy reload` apply replacements to a running instance (`cmd/commandfuncs.go:376-412`; `caddy.go:144-275`).
* Do not claim a source checkout defines all possible functionality: custom builds can blank-import additional modules (`cmd/caddy/main.go:19-28`).

## Minimum useful onboarding journey

1. Read `cmd/caddy/main.go` and `cmd/main.go` to locate the sole executable and the CLI handoff.
2. Trace `cmdRun` and `LoadConfig` to distinguish source configuration/adaptation from native runtime JSON.
3. Read `caddy.go` (`Load`, `changeConfig`, `run`, `provisionContext`) and `context.go` (`LoadModule`) before modifying lifecycle or module behavior.
4. Read `caddyconfig/caddyfile/adapter.go` plus `caddyconfig/httpcaddyfile/httptype.go` before changing Caddyfile parsing/directives.
5. For serving behavior, continue with `modules/caddyhttp/app.go` then `modules/caddyhttp/server.go`; for automatic HTTPS, continue with `modules/caddytls/tls.go`.
