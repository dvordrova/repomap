# Independent source oracle: Syncthing (decision 094)

## Scope and provenance

This oracle was established from source inspection only, before inspecting any
generated repomap report. It is an expected-truth fixture for exactly this
repository and revision.

| Field | Exact fact |
| --- | --- |
| Fixture | `syncthing` |
| Absolute repository path | `/Users/dvordrova/git/syncthing` |
| Required and observed revision | `d4cffd848eb13d65f3caca5ef6da9a3fd25a2d6a` |
| Module | `github.com/syncthing/syncthing` (`go.mod:1`) |
| Go version declared | `go 1.25.0` (`go.mod:3`) |
| Dirty state | clean: `git status --porcelain=v1 --untracked-files=all` produced zero paths; staged and unstaged diffs were also clean |
| Build-output state | no top-level `bin/` directory was present before this inspection; source was not built or changed by this oracle task |
| Generated-source state | clean tracked generated source is present, including protobuf code under `internal/gen/`, mock code under `lib/*/mocks/`, `lib/build/runtimeos.gen.go`, and generated GUI asset sources; examples of generation declarations are `lib/api/auto/doc.go:7` and `lib/relay/protocol/packets.go:3` |

## Executables and user-facing surfaces

### Primary executable

`syncthing` is the primary end-user executable. The build target maps it to
`cmd/syncthing` at `build.go:88-95`; the program parses the top-level Kong CLI
and dispatches its command at `cmd/syncthing/main.go:212-241`.

Exact primary user surfaces:

- Default `syncthing serve` runs the synchronizer (`cmd/syncthing/main.go:129`, `:255-313`).
- The top-level CLI exposes `cli`, `browser`, `decrypt`, `device-id`,
  `generate`, `paths`, `upgrade`, `version`, `debug`, and shell-completion
  commands (`cmd/syncthing/main.go:119-142`).
- `serve` accepts configuration/data/home locations plus GUI, logging,
  auditing, pause, upgrade, and diagnostic options (`cmd/syncthing/main.go:124-183`).
- The browser GUI and REST API are user-facing only when GUI configuration is
  enabled; the application creates the API service in
  `lib/syncthing/syncthing.go:404-424`, and it registers REST routes in
  `lib/api/api.go:219-279`.
- `syncthing cli` is a separate local command client for the running GUI/API,
  with show/debug/operations/errors/config groups
  (`cmd/syncthing/cli/main.go:20-49`).
- `syncthing generate` creates keys/configuration and can set GUI
  authentication (`cmd/syncthing/generate/generate.go:26-46`, `:49-99`).
- `syncthing decrypt` decrypts or verifies a receive-encrypted folder; it
  requires `--to` or `--verify-only` (`cmd/syncthing/decrypt/decrypt.go:30-78`).

### Secondary executables

The explicit non-primary build targets are exact executable targets, not
subcommands of `syncthing`:

| Binary | Source package | Role evidenced by build target |
| --- | --- | --- |
| `stdiscosrv` | `cmd/stdiscosrv` | Discovery server (`build.go:133-157`) |
| `strelaysrv` | `cmd/strelaysrv` | Relay server (`build.go:159-185`) |
| `strelaypoolsrv` | `cmd/infra/strelaypoolsrv` | Relay-pool server (`build.go:187-192`) |
| `stupgrades` | `cmd/infra/stupgrades` | Upgrade-check server (`build.go:193-198`) |
| `stcrashreceiver` | `cmd/infra/stcrashreceiver` | Crash server (`build.go:199-204`) |
| `ursrv` | `cmd/infra/ursrv` | Usage-reporting server (`build.go:205-210`) |

`stdiscosrv` starts an in-memory discovery store and API service
(`cmd/stdiscosrv/main.go:121-153`). `strelaysrv` is a separately configured
relay daemon with protocol/status listeners (`cmd/strelaysrv/main.go:82-107`).
`cmd/dev/` and `script/` also contain `main` packages, but these are developer
utilities or repository-maintenance tools, not equivalent user-facing product
entry surfaces.

### Important runtime activities that are **not** entry surfaces

Do not promote the following internal activities to user-facing entrypoints:

- `(*serveCmd).syncthingMain` is the inner startup routine, not a command;
  it loads/generates identity material, locks the instance, loads config,
  migrates/opens the database, and constructs/starts `syncthing.App`
  (`cmd/syncthing/main.go:420-474`, `:493-606`).
- `syncthing.New` and `(*App).Start` construct and supervise runtime services;
  they are application wiring APIs, not independent launch surfaces
  (`lib/syncthing/syncthing.go:81-115`).
- `model.NewModel`, `(*model).serve`, and folder `Serve` methods are internal
  synchronization services (`lib/model/model.go:212-293`; `lib/model/folder.go:149-265`).
- `connections.NewService`, discovery manager configuration, protocol
  connection loops, scanners, index receipt, and pull scheduling are runtime
  mechanisms (`lib/connections/service.go:186-238`; `lib/discover/manager.go:231-279`; `lib/protocol/protocol.go:271-313`; `lib/scanner/walk.go:89-140`; `lib/model/model.go:1151-1193`).
- Config-wrapper `Serve` serializes modification, notification, and saving;
  it is not a config-file command (`lib/config/wrapper.go:221-299`).

## Expected onboarding flows

These are source-anchored flows a new engineer should be able to follow. The
responsibility labels are informed expectations; arrows and cited operations
are exact.

1. **Launch and construct the application.**
   `main` parses the CLI and calls the selected command
   (`cmd/syncthing/main.go:212-241`) -> default `serveCmd.Run` establishes
   locations/logging and chooses monitor versus inner process
   (`cmd/syncthing/main.go:255-313`) -> `syncthingMain` loads config, migrates
   and opens the DB, calls `syncthing.New`, then `app.Start`
   (`cmd/syncthing/main.go:456-606`) -> `App.Start` creates the main suture
   supervisor and runs `startup` (`lib/syncthing/syncthing.go:94-115`).

2. **Wire services, discovery, peer connections, and GUI.**
   `App.startup` creates the model (`lib/syncthing/syncthing.go:254-258`) ->
   creates the mutually dependent discovery manager and connection service,
   then wires the late address lister (`lib/syncthing/syncthing.go:260-284`) ->
   conditionally adds usage reporting and GUI/API (`lib/syncthing/syncthing.go:297-305`, `:404-424`) -> API `Serve` obtains its listener and installs REST
   routes (`lib/api/api.go:219-279`).

3. **Discover/connect and exchange synchronization protocol.**
   Discovery configuration adds global/local finders
   (`lib/discover/manager.go:231-279`) -> connection service starts listener,
   dial, connection-handling, hello-handling, and NAT services
   (`lib/connections/service.go:214-232`) -> `handleConns` validates TLS peer
   identity and exchanges Hello (`lib/connections/service.go:241-300`) ->
   `rawConnection.Start` starts reader/dispatcher/writer/ping goroutines
   (`lib/protocol/protocol.go:271-313`) -> incoming full/incremental indexes
   enter `model.handleIndex` (`lib/model/model.go:1151-1193`).

4. **Scan local files and reconcile a folder.**
   `NewModel` starts in read-only mode and `serve` initializes configured
   folders (`lib/model/model.go:212-215`, `:268-315`) -> folder service
   immediately scans, optionally watches, handles scheduled scans/pulls, and
   cleans versions (`lib/model/folder.go:149-265`) -> scanner `Walk` walks the
   filesystem and feeds parallel hashing (`lib/scanner/walk.go:89-140`) -> a
   received index is validated and delivered to an index handler
   (`lib/model/model.go:1163-1193`).

## Conceptual architecture (informed expectations)

- `cmd/syncthing` is process/CLI/monitor startup orchestration; `lib/syncthing`
  owns composition and lifecycle of the running application.
- `lib/config`, `lib/locations`, and `internal/db` respectively govern
  persistent configuration, path selection, and durable synchronization
  metadata.
- `lib/model` is the synchronization state machine: folders, indexes,
  connection state, and file requests.
- `lib/scanner`, `lib/fs`, `lib/ignore`, and `lib/versioner` provide local
  filesystem observation, filtering, and version handling.
- `lib/discover`, `lib/connections`, `lib/nat`, `lib/relay`, `lib/protocol`,
  and TLS utilities form peer discovery, transport establishment, and BEP
  exchange. `bep/1.0` is the application’s declared ALPN protocol
  (`lib/syncthing/syncthing.go:45-52`, `:260-279`).
- `lib/api`, `gui/`, `lib/events`, and `lib/ur` expose/manage operator UI/API,
  event streams, and reporting.

## Analysis frontiers

### Dynamic frontiers (exact facts)

- Behavior depends on CLI flags and `ST*` environment variables, including
  config/data locations and GUI override values (`cmd/syncthing/main.go:119-183`, `:255-264`).
- Runtime state depends on the configured GUI, folders, devices, discovery
  options, listener addresses, local filesystem, persisted database, and
  certificate-derived device identity (`cmd/syncthing/main.go:435-474`,
  `lib/syncthing/syncthing.go:150-159`, `:200-210`, `:404-424`).
- Concurrent service supervision and goroutines are central: suture supervisors
  run application/model/connections services, while protocol connections start
  five goroutines (`lib/syncthing/syncthing.go:97-108`; `lib/model/model.go:216-265`; `lib/connections/service.go:186-238`; `lib/protocol/protocol.go:288-309`).
- Networking crosses GUI/REST listener, discovery services, TCP/QUIC/relay
  transports, TLS, NAT, and remote peers; source alone cannot establish a
  particular configured topology or successful peer exchange.

### Static-analysis frontiers (exact facts)

- Build tags and platform-specific files alter compilation; `build.go` itself
  requires the `tools` build tag (`build.go:7-10`) and the build driver accepts
  OS/architecture/tags (`build.go:365-383`).
- Generated protobuf, mocks, runtime OS information, and GUI assets are part
  of the checked-out source boundary; generation directives and generated-file
  headers identify them (`lib/api/auto/doc.go:7`; `internal/gen/bep/bep.pb.go:1`; `lib/build/runtimeos.gen.go:1`).
- The build driver can rebuild assets, generate protobuf sources, generate
  mocks, and run integration tests (`build.go:282-315`, `:386-436`), so a
  static source map must not imply those commands were executed.
- The REST route table is statically visible, but route authentication,
  configuration, and actual request outcomes require runtime context.

## Claims that would be misleading

- “Syncthing has only one executable.” It has the primary client plus the six
  explicit server build targets above, as well as developer/tool `main`
  packages.
- “Every `main` or `Serve` method is a user entry surface.” Most such symbols
  are build tooling, developers’ utilities, or supervised internal services.
- “The GUI/API is always running.” `setupGUI` returns without adding it when
  GUI is disabled (`lib/syncthing/syncthing.go:404-409`).
- “A file scan itself performs all synchronization.” Scanning/hashing is one
  subsystem; discovery, TLS connection, BEP index exchange, model/index
  handling, and folder pull scheduling are separate stages.
- “The source tree contains no generated code because the worktree is clean.”
  Generated code is checked in and clean; clean status does not mean absent.
- “`go run build.go` necessarily produces only `syncthing`.” With no command,
  the driver installs the dynamically collected `cmd/*` packages
  (`build.go:213-223`, `:254-257`).

## Minimum useful onboarding journey

1. Read `README.md:9-80` for product goals and source build expectation.
2. Read `cmd/syncthing/main.go:119-313` to learn the real CLI and default
   process boundary.
3. Trace `cmd/syncthing/main.go:420-606` into
   `lib/syncthing/syncthing.go:94-329` for startup/wiring.
4. Trace peer lifecycle through `lib/discover/manager.go:231-279`,
   `lib/connections/service.go:186-300`, and
   `lib/protocol/protocol.go:271-313`.
5. Trace synchronization state through `lib/model/model.go:212-315`,
   `lib/model/folder.go:149-265`, and `lib/scanner/walk.go:89-140`; only then
   use `lib/api/api.go:219-279` to connect UI/API operations to runtime state.
