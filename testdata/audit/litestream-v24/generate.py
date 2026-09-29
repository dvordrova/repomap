#!/usr/bin/env python3
"""Generate and verify the litestream-v24 ground-truth inventory (internal/audit).

Run from anywhere: python3 testdata/audit/litestream-v24/generate.py
It reads ~/git/litestream-v24 at the pinned revision only, refuses any other
HEAD, and rewrites inventory.json beside this script.

Every item carries a `check` substring that must appear on the anchored
line; the script refuses to write the JSON if any anchor does not match.
"""
import json
import os
import subprocess
import sys

REPO = os.path.expanduser("~/git/litestream-v24")
PINNED = "d26cb54ec43b5937a0c8b3bd875696c1375d8cb3"
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "inventory.json")

CLI = "cmd/litestream"
TEST = "cmd/litestream-test"
VFS = "cmd/litestream-vfs"
LIB = "litestream (root library package)"

items = []
traps = []


def I(component, inventory, kind, name, anchor, handler, must, why, check):
    items.append(dict(component=component, inventory=inventory, kind=kind, name=name,
                      anchor=anchor, handler=handler, must=must, why=why, _check=check))


def N(component, name, anchor, reason, check):
    traps.append(dict(component=component, name=name, anchor=anchor, reason=reason, _check=check))


# ---------------------------------------------------------------- cmd/litestream
# requests
I(CLI, "requests", "http", "GET /metrics", "cmd/litestream/replicate.go:362", "promhttp.Handler", True,
  "Prometheus metrics served by an HTTP server on config `addr` started in ReplicateCommand.Run.",
  'http.Handle("/metrics", promhttp.Handler())')
I(CLI, "requests", "http", "GET /debug/pprof/*", "cmd/litestream/replicate.go:11", "net/http/pprof (DefaultServeMux)", False,
  "Blank pprof import registers profiling routes on DefaultServeMux, which the metrics server (ListenAndServe(addr, nil)) serves.",
  '_ "net/http/pprof"')
I(CLI, "requests", "mcp", "MCP Streamable HTTP endpoint /", "cmd/litestream/mcp.go:48", "server.NewStreamableHTTPServer (wrapped by httplog.Logger)", True,
  "MCP server listening on config `mcp-addr`; started from ReplicateCommand.Run.",
  's.mux.Handle("/", httplog.Logger(server.NewStreamableHTTPServer(mcpServer)))')
for name, line, fn in [
    ("litestream_databases", 84, "DatabasesTool"),
    ("litestream_info", 106, "InfoTool"),
    ("litestream_restore", 184, "RestoreTool"),
    ("litestream_version", 245, "VersionTool"),
    ("litestream_ltx", 260, "LTXTool"),
    ("litestream_status", 297, "StatusTool"),
    ("litestream_reset", 323, "ResetTool"),
]:
    I(CLI, "requests", "mcp-tool", name, f"cmd/litestream/mcp.go:{line}", f"{fn} (returned closure)", True,
      "MCP tool served by the MCP server; the handler shells out to the `litestream` CLI.",
      f'mcp.NewTool("{name}"')
I(CLI, "requests", "unix-socket-http", "control socket server (litestream.Server)", "cmd/litestream/replicate.go:302",
  "litestream.NewServer / Server.Start", False,
  "Wiring only: replicate starts the library control server when socket.enabled; its routes are listed under the library component.",
  "c.Server = litestream.NewServer(c.Store)")
I(CLI, "requests", "windows-scm", "Windows service control requests (Stop, Interrogate)", "cmd/litestream/main_windows.go:84",
  "windowsService.Execute", False,
  "When run as a Windows service, the Service Control Manager sends stop/interrogate requests.",
  "case svc.Stop:")

# commands: subcommands
for name, line, handler, must, why, check in [
    ("databases", 138, "DatabasesCommand.Run", True, "Lists databases and replica types from the config file.", 'case "databases":'),
    ("replicate", 140, "ReplicateCommand.ParseFlags / ReplicateCommand.Run", True, "Runs the replication daemon (config file or DB_PATH REPLICA_URL...).", 'case "replicate":'),
    ("start", 196, "StartCommand.Run", True, "Asks the running daemon (control socket) to start replicating a database.", 'case "start":'),
    ("stop", 198, "StopCommand.Run", True, "Asks the running daemon to stop replicating a database.", 'case "stop":'),
    ("register", 200, "RegisterCommand.Run", True, "Registers a new database + replica URL with the running daemon.", 'case "register":'),
    ("unregister", 202, "UnregisterCommand.Run", True, "Removes a database from the running daemon.", 'case "unregister":'),
    ("reset", 204, "ResetCommand.Run", True, "Deletes the local LTX directory of a database to force a fresh snapshot.", 'case "reset":'),
    ("restore", 206, "RestoreCommand.Run", True, "Restores a database from a replica (config DB path or replica URL).", 'case "restore":'),
    ("status", 208, "StatusCommand.Run", True, "Shows local status (local TXID, WAL size) for configured databases.", 'case "status":'),
    ("sync", 210, "SyncCommand.Run", True, "Asks the running daemon to sync a database now.", 'case "sync":'),
    ("list", 212, "ListCommand.Run", True, "Lists databases managed by the running daemon.", 'case "list":'),
    ("info", 214, "InfoCommand.Run", True, "Shows daemon version, PID, uptime, database count.", 'case "info":'),
    ("version", 216, "VersionCommand.Run", True, "Prints the binary version.", 'case "version":'),
    ("ltx", 218, "LTXCommand.Run", True, "Lists LTX files on the replica for a database or replica URL.", 'case "ltx":'),
    ("wal", 220, "LTXCommand.Run", False, "Deprecated alias of ltx that prints a warning.", 'case "wal":'),
    ("help / -h / --help", 225, "Main.Usage", False, "Prints the command list.", 'cmd == "help"'),
]:
    I(CLI, "commands", "subcommand", name, f"cmd/litestream/main.go:{line}", handler, must, why, check)
I(CLI, "commands", "mode", "Windows service mode", "cmd/litestream/main.go:122", "runWindowsService", False,
  "When launched by the Windows SCM the program runs replicate with the default config instead of parsing a subcommand.",
  "isWindowsService()")

# commands: flags
I(CLI, "commands", "flag", "-config", "cmd/litestream/main.go:2041", "registerConfigFlag", True,
  "Config path for replicate, databases, ltx, reset, restore and status; defaults to LITESTREAM_CONFIG or /etc/litestream.yml.",
  'fs.String("config", "", "config path")')
I(CLI, "commands", "flag", "-no-expand-env", "cmd/litestream/main.go:2042", "registerConfigFlag", False,
  "Disables $VAR expansion in the config file for the same commands as -config.",
  'fs.Bool("no-expand-env"')
I(CLI, "commands", "argument", "replicate DB_PATH REPLICA_URL [REPLICA_URL...]", "cmd/litestream/replicate.go:123", "ReplicateCommand.ParseFlags", True,
  "Positional form builds a one-database config without a config file.",
  "dbConfig := &DBConfig{")
flag_rows = [
    ("replicate", "-exec", "replicate.go", 68, True, "Runs a child process; litestream exits when it exits.", 'fs.String("exec"'),
    ("replicate", "-log-level", "replicate.go", 69, True, "Overrides logging level (also sets LOG_LEVEL).", 'fs.String("log-level"'),
    ("replicate", "-restore-if-db-not-exists", "replicate.go", 70, True, "Restores each missing database from its replica before replicating.", 'fs.Bool("restore-if-db-not-exists"'),
    ("replicate", "-once", "replicate.go", 71, True, "Syncs every database once and exits; disables background monitors.", 'fs.Bool("once"'),
    ("replicate", "-force-snapshot", "replicate.go", 72, True, "With -once, takes a snapshot of every database.", 'fs.Bool("force-snapshot"'),
    ("replicate", "-enforce-retention", "replicate.go", 73, True, "With -once, enforces snapshot retention.", 'fs.Bool("enforce-retention"'),
    ("databases", "-json", "databases.go", 19, False, "JSON output.", 'fs.Bool("json"'),
    ("info", "-socket", "info.go", 22, False, "Control socket path (default /var/run/litestream.sock).", 'fs.String("socket"'),
    ("info", "-timeout", "info.go", 23, False, "Request timeout in seconds.", 'fs.Int("timeout"'),
    ("info", "-json", "info.go", 24, False, "JSON output.", 'fs.Bool("json"'),
    ("list", "-socket", "list.go", 22, False, "Control socket path.", 'fs.String("socket"'),
    ("list", "-timeout", "list.go", 23, False, "Request timeout in seconds.", 'fs.Int("timeout"'),
    ("list", "-json", "list.go", 24, False, "JSON output.", 'fs.Bool("json"'),
    ("ltx", "-json", "ltx.go", 23, False, "JSON output.", 'fs.Bool("json"'),
    ("ltx", "-level", "ltx.go", 25, True, "Compaction level to list (0-9 or all).", 'fs.Var(&level, "level"'),
    ("reset", "-dry-run", "reset.go", 21, True, "Prints the local LTX files that would be removed.", 'fs.Bool("dry-run"'),
    ("register", "-timeout", "register.go", 21, False, "Request timeout in seconds.", 'fs.Int("timeout"'),
    ("register", "-socket", "register.go", 22, False, "Control socket path.", 'fs.String("socket"'),
    ("register", "-replica", "register.go", 23, True, "Replica URL to register (required).", 'fs.String("replica"'),
    ("register", "-json", "register.go", 24, False, "JSON output.", 'fs.Bool("json"'),
    ("restore", "-o", "restore.go", 30, True, "Output path of the restored database.", 'fs.StringVar(&opt.OutputPath, "o"'),
    ("restore", "-txid", "restore.go", 31, True, "Restore up to a hex TXID.", '"txid", "transaction ID"'),
    ("restore", "-parallelism", "restore.go", 32, True, "Parallel downloads during restore.", '"parallelism"'),
    ("restore", "-if-db-not-exists", "restore.go", 33, True, "Exit 0 if the output database already exists.", 'fs.Bool("if-db-not-exists"'),
    ("restore", "-if-replica-exists", "restore.go", 34, True, "Exit 0 if no backups are found.", 'fs.Bool("if-replica-exists"'),
    ("restore", "-timestamp", "restore.go", 35, True, "Point-in-time restore (RFC3339).", 'fs.String("timestamp"'),
    ("restore", "-dry-run", "restore.go", 36, True, "Prints the restore plan (LTX files) without writing.", 'fs.Bool("dry-run"'),
    ("restore", "-force", "restore.go", 37, True, "Overwrites an existing output database and its -wal/-shm/-journal.", 'fs.Bool("force"'),
    ("restore", "-json", "restore.go", 38, False, "JSON summary output.", 'fs.Bool("json"'),
    ("restore", "-f", "restore.go", 39, True, "Follow mode: keep polling and applying new LTX files.", 'fs.BoolVar(&opt.Follow, "f"'),
    ("restore", "-follow-interval", "restore.go", 40, True, "Polling interval for follow mode.", '"follow-interval"'),
    ("restore", "-integrity-check", "restore.go", 41, True, "Post-restore PRAGMA quick_check/integrity_check.", 'fs.String("integrity-check"'),
    ("unregister", "-timeout", "unregister.go", 21, False, "Request timeout in seconds.", 'fs.Int("timeout"'),
    ("unregister", "-socket", "unregister.go", 22, False, "Control socket path.", 'fs.String("socket"'),
    ("unregister", "-dry-run", "unregister.go", 23, True, "Prints the request without sending it.", 'fs.Bool("dry-run"'),
    ("unregister", "-json", "unregister.go", 24, False, "JSON output.", 'fs.Bool("json"'),
    ("status", "-json", "status.go", 23, False, "JSON output.", 'fs.Bool("json"'),
    ("stop", "-timeout", "stop.go", 23, False, "Request timeout in seconds (final sync wait).", 'fs.Int("timeout"'),
    ("stop", "-socket", "stop.go", 24, False, "Control socket path.", 'fs.String("socket"'),
    ("stop", "-json", "stop.go", 25, False, "JSON output.", 'fs.Bool("json"'),
    ("sync", "-timeout", "sync.go", 23, False, "Request timeout in seconds.", 'fs.Int("timeout"'),
    ("sync", "-socket", "sync.go", 24, False, "Control socket path.", 'fs.String("socket"'),
    ("sync", "-wait", "sync.go", 25, True, "Blocks until the replica upload completes.", 'fs.Bool("wait"'),
    ("sync", "-json", "sync.go", 26, False, "JSON output.", 'fs.Bool("json"'),
    ("start", "-timeout", "start.go", 23, False, "Request timeout in seconds.", 'fs.Int("timeout"'),
    ("start", "-socket", "start.go", 24, False, "Control socket path.", 'fs.String("socket"'),
    ("start", "-json", "start.go", 25, False, "JSON output.", 'fs.Bool("json"'),
]
handlers = {
    "replicate": "ReplicateCommand.ParseFlags", "databases": "DatabasesCommand.Run", "info": "InfoCommand.Run",
    "list": "ListCommand.Run", "ltx": "LTXCommand.Run", "reset": "ResetCommand.Run", "register": "RegisterCommand.Run",
    "restore": "RestoreCommand.Run", "unregister": "UnregisterCommand.Run", "status": "StatusCommand.Run",
    "stop": "StopCommand.Run", "sync": "SyncCommand.Run", "start": "StartCommand.Run",
}
for cmd, flag, f, line, must, why, check in flag_rows:
    I(CLI, "commands", "flag", f"{cmd} {flag}", f"cmd/litestream/{f}:{line}", handlers[cmd], must, why, check)

# commands: config-file settings
M = "cmd/litestream/main.go"
setting_rows = [
    # top level
    ("addr", M, 270, True, "ReplicateCommand.Run", "Bind address of the Prometheus /metrics HTTP server.", 'yaml:"addr"'),
    ("socket.enabled", "server.go", 19, True, "ReplicateCommand.Run", "Enables the Unix control socket (default false) needed by start/stop/sync/register/unregister/info/list.", 'yaml:"enabled"'),
    ("socket.path", "server.go", 20, True, "ReplicateCommand.Run", "Control socket path (default /var/run/litestream.sock).", 'yaml:"path"'),
    ("socket.permissions", "server.go", 21, False, "ReplicateCommand.Run", "Control socket file mode (default 0600).", 'yaml:"permissions"'),
    ("levels (levels[].interval)", M, 277, True, "Config.CompactionLevels", "Compaction levels L1..Ln and their intervals (default 30s, 5m, 1h).", 'yaml:"levels"'),
    ("snapshot.interval", M, 324, True, "ReplicateCommand.Run", "How often a full snapshot (level 9) is written (default 24h).", 'yaml:"interval"'),
    ("snapshot.retention", M, 325, True, "ReplicateCommand.Run", "How long snapshots are kept before retention deletes them (default 24h).", 'yaml:"retention"'),
    ("validation.interval", M, 335, True, "ReplicateCommand.Run", "Enables the periodic LTX validation monitor.", 'Interval *time.Duration `yaml:"interval"`'),
    ("l0-retention", M, 286, True, "ReplicateCommand.Run", "How long L0 files are kept after compaction into L1 (default 5m).", 'yaml:"l0-retention"'),
    ("l0-retention-check-interval", M, 287, False, "ReplicateCommand.Run", "How often L0 retention runs (default 15s).", 'yaml:"l0-retention-check-interval"'),
    ("verify-compaction", M, 291, False, "ReplicateCommand.Run", "Post-compaction TXID contiguity check.", 'yaml:"verify-compaction"'),
    ("retention.enabled", M, 330, True, "ReplicateCommand.Run", "false stops Litestream deleting remote files (leave it to bucket lifecycle rules).", 'yaml:"enabled"'),
    ("heartbeat-url", M, 297, True, "ReplicateCommand.Run", "URL pinged (HTTP GET) when all databases synced recently.", 'yaml:"heartbeat-url"'),
    ("heartbeat-interval", M, 298, True, "ReplicateCommand.Run", "Heartbeat ping interval (min 1m, default 5m).", 'yaml:"heartbeat-interval"'),
    ("dbs", M, 301, True, "ReplicateCommand.Run", "List of databases (or directories) to replicate.", 'yaml:"dbs"'),
    ("exec", M, 305, True, "ReplicateCommand.Run", "Child command run alongside replication; litestream exits with it.", 'yaml:"exec"'),
    ("logging.level", "internal/log.go", 30, True, "internal.InitLog", "Log level (trace, debug, info, warn, error).", 'yaml:"level"'),
    ("logging.type", "internal/log.go", 31, False, "internal.InitLog", "Log format: text, json or pretty.", 'yaml:"type"'),
    ("logging.stderr", "internal/log.go", 32, False, "ParseConfig", "Log to stderr instead of stdout.", 'yaml:"stderr"'),
    ("logging.source", "internal/log.go", 33, False, "internal.InitLog", "Adds source file:line to log records.", 'yaml:"source"'),
    ("mcp-addr", M, 311, True, "ReplicateCommand.Run", "Enables the MCP HTTP server on this address.", 'yaml:"mcp-addr"'),
    ("shutdown-sync-timeout", M, 314, False, "ReplicateCommand.Run", "Total time to retry the final replica sync at shutdown (default 30s).", 'yaml:"shutdown-sync-timeout"'),
    ("shutdown-sync-interval", M, 315, False, "ReplicateCommand.Run", "Delay between shutdown sync retries (default 500ms).", 'yaml:"shutdown-sync-interval"'),
    # per database
    ("dbs[].path", M, 706, True, "NewDBFromConfig", "Path of one SQLite database to replicate (sqlite:// prefixes stripped).", 'yaml:"path"'),
    ("dbs[].dir", M, 707, True, "NewDBsFromDirectoryConfig", "Directory scanned for SQLite databases instead of a single path.", 'yaml:"dir"'),
    ("dbs[].pattern", M, 708, True, "FindSQLiteDatabases", "Filename glob required with dir.", 'yaml:"pattern"'),
    ("dbs[].recursive", M, 709, False, "FindSQLiteDatabases", "Scan subdirectories of dir.", 'yaml:"recursive"'),
    ("dbs[].watch", M, 710, True, "NewDirectoryMonitor", "Watch dir with fsnotify and add/remove databases at runtime.", 'yaml:"watch"'),
    ("dbs[].snapshot", M, 711, False, "Config.applyDBSnapshotConfig", "Per-db snapshot interval/retention; folded into the single global snapshot setting (conflicts are errors).", 'yaml:"snapshot"'),
    ("dbs[].meta-path", M, 712, True, "NewDBFromConfig", "Overrides the local metadata directory (default .<db>-litestream).", 'yaml:"meta-path"'),
    ("dbs[].meta-dir", M, 713, False, "newDBFromDirectoryEntry", "Base directory for per-database metadata in directory mode.", 'yaml:"meta-dir"'),
    ("dbs[].monitor-interval", M, 714, True, "NewDBFromConfig", "How often the DB monitor syncs WAL to L0 (default 1s).", 'yaml:"monitor-interval"'),
    ("dbs[].checkpoint-interval", M, 715, False, "NewDBFromConfig", "Time-based PASSIVE checkpoint interval (default 1m).", 'yaml:"checkpoint-interval"'),
    ("dbs[].busy-timeout", M, 716, False, "NewDBFromConfig", "SQLite busy timeout (default 1s).", 'yaml:"busy-timeout"'),
    ("dbs[].min-checkpoint-page-count", M, 717, False, "NewDBFromConfig", "WAL pages before a PASSIVE checkpoint (default 1000).", 'yaml:"min-checkpoint-page-count"'),
    ("dbs[].truncate-page-n", M, 718, False, "NewDBFromConfig", "WAL pages before an emergency TRUNCATE checkpoint (default 121359).", 'yaml:"truncate-page-n"'),
    ("dbs[].max-sync-wal-bytes", M, 719, False, "NewDBFromConfig", "Max WAL bytes per sync executor run (default 64MiB).", 'yaml:"max-sync-wal-bytes"'),
    ("dbs[].restore-if-db-not-exists", M, 721, True, "ReplicateCommand.restoreIfNeeded", "Restore a missing database from its replica before replicating.", 'yaml:"restore-if-db-not-exists"'),
    ("dbs[].replica", M, 723, True, "NewReplicaFromConfig", "The single replica destination of the database.", 'yaml:"replica"'),
    ("dbs[].replicas", M, 724, False, "NewDBFromConfig", "Deprecated list form; only one element accepted.", 'yaml:"replicas"'),
    # replica
    ("replica.type", M, 1322, True, "ReplicaConfig.ReplicaType", "Backend type (file, s3, gs, abs, sftp, webdav, nats, oss); inferred from url, defaults to file.", 'yaml:"type"'),
    ("replica.path", M, 1324, True, "NewReplicaFromConfig", "Destination path (file dir or object prefix).", 'Path string `yaml:"path"`'),
    ("replica.url", M, 1325, True, "NewReplicaFromConfig", "Replica URL; scheme selects the backend.", 'yaml:"url"'),
    ("sync-interval", M, 1097, True, "NewReplicaFromConfig", "Replica upload interval (default 1s); global default or per replica.", 'yaml:"sync-interval"'),
    ("max-sync-ltx-files", M, 1102, False, "NewReplicaFromConfig", "Max L0 files uploaded per replica sync run (default 256).", 'yaml:"max-sync-ltx-files"'),
    ("auto-recover", M, 1107, False, "NewReplicaFromConfig", "Reset local state automatically on LTX errors.", 'yaml:"auto-recover"'),
    # backend-specific (global default or per replica)
    ("access-key-id", M, 1110, False, "NewS3ReplicaClientFromConfig / newOSSReplicaClientFromConfig", "S3/OSS access key.", 'yaml:"access-key-id"'),
    ("secret-access-key", M, 1111, False, "NewS3ReplicaClientFromConfig / newOSSReplicaClientFromConfig", "S3/OSS secret key.", 'yaml:"secret-access-key"'),
    ("region", M, 1112, False, "NewS3ReplicaClientFromConfig / newOSSReplicaClientFromConfig", "S3/OSS region.", 'yaml:"region"'),
    ("bucket", M, 1113, False, "NewS3ReplicaClientFromConfig and gs/abs/nats/oss builders", "Bucket/container (S3, GCS, ABS, NATS object store, OSS).", 'yaml:"bucket"'),
    ("endpoint", M, 1114, False, "NewS3ReplicaClientFromConfig / newABSReplicaClientFromConfig / newOSSReplicaClientFromConfig", "Custom endpoint (S3-compatible, ABS, OSS).", 'yaml:"endpoint"'),
    ("force-path-style", M, 1115, False, "NewS3ReplicaClientFromConfig", "S3 path-style addressing.", 'yaml:"force-path-style"'),
    ("sign-payload", M, 1116, False, "NewS3ReplicaClientFromConfig", "S3 payload signing.", 'yaml:"sign-payload"'),
    ("require-content-md5", M, 1117, False, "NewS3ReplicaClientFromConfig", "S3 Content-MD5 on requests.", 'yaml:"require-content-md5"'),
    ("skip-verify", M, 1118, False, "NewS3ReplicaClientFromConfig", "Skip TLS verification for S3.", 'yaml:"skip-verify"'),
    ("storage-class", M, 1119, False, "NewS3ReplicaClientFromConfig", "S3 storage class.", 'yaml:"storage-class"'),
    ("part-size", M, 1120, False, "NewS3ReplicaClientFromConfig / newOSSReplicaClientFromConfig", "Multipart upload part size (S3/OSS).", 'yaml:"part-size"'),
    ("concurrency", M, 1121, False, "NewS3ReplicaClientFromConfig / newOSSReplicaClientFromConfig", "Multipart upload concurrency (S3/OSS).", 'yaml:"concurrency"'),
    ("sse-customer-algorithm", M, 1124, False, "NewS3ReplicaClientFromConfig", "S3 SSE-C algorithm.", 'yaml:"sse-customer-algorithm"'),
    ("sse-customer-key", M, 1125, False, "NewS3ReplicaClientFromConfig", "S3 SSE-C key.", 'yaml:"sse-customer-key"'),
    ("sse-customer-key-path", M, 1126, False, "NewS3ReplicaClientFromConfig", "File holding the S3 SSE-C key.", 'yaml:"sse-customer-key-path"'),
    ("sse-kms-key-id", M, 1129, False, "NewS3ReplicaClientFromConfig", "S3 SSE-KMS key id.", 'yaml:"sse-kms-key-id"'),
    ("account-name", M, 1132, False, "newABSReplicaClientFromConfig", "Azure storage account.", 'yaml:"account-name"'),
    ("account-key", M, 1133, False, "newABSReplicaClientFromConfig", "Azure shared key.", 'yaml:"account-key"'),
    ("sas-token", M, 1134, False, "newABSReplicaClientFromConfig", "Azure SAS token.", 'yaml:"sas-token"'),
    ("host", M, 1137, False, "newSFTPReplicaClientFromConfig", "SFTP host.", 'yaml:"host"'),
    ("user", M, 1138, False, "newSFTPReplicaClientFromConfig", "SFTP user.", 'yaml:"user"'),
    ("password", M, 1139, False, "newSFTPReplicaClientFromConfig / newNATSReplicaClientFromConfig", "SFTP or NATS password.", 'yaml:"password"'),
    ("key-path", M, 1140, False, "newSFTPReplicaClientFromConfig", "SFTP private key file.", 'yaml:"key-path"'),
    ("concurrent-writes", M, 1141, False, "newSFTPReplicaClientFromConfig", "SFTP concurrent writes (default true).", 'yaml:"concurrent-writes"'),
    ("host-key", M, 1142, False, "newSFTPReplicaClientFromConfig", "Expected SFTP host key; without it the host key is not verified.", 'yaml:"host-key"'),
    ("webdav-url", M, 1145, False, "newWebDAVReplicaClientFromConfig", "WebDAV server URL.", 'yaml:"webdav-url"'),
    ("webdav-username", M, 1146, False, "newWebDAVReplicaClientFromConfig", "WebDAV user.", 'yaml:"webdav-username"'),
    ("webdav-password", M, 1147, False, "newWebDAVReplicaClientFromConfig", "WebDAV password.", 'yaml:"webdav-password"'),
    ("jwt", M, 1150, False, "newNATSReplicaClientFromConfig", "NATS JWT.", 'yaml:"jwt"'),
    ("seed", M, 1151, False, "newNATSReplicaClientFromConfig", "NATS seed.", 'yaml:"seed"'),
    ("creds", M, 1152, False, "newNATSReplicaClientFromConfig", "NATS credentials file.", 'yaml:"creds"'),
    ("nkey", M, 1153, False, "newNATSReplicaClientFromConfig", "NATS NKey.", 'yaml:"nkey"'),
    ("username", M, 1154, False, "newNATSReplicaClientFromConfig", "NATS user.", 'yaml:"username"'),
    ("token", M, 1155, False, "newNATSReplicaClientFromConfig", "NATS token.", 'yaml:"token"'),
    ("root-cas", M, 1157, False, "newNATSReplicaClientFromConfig", "NATS root CA files.", 'yaml:"root-cas"'),
    ("client-cert", M, 1158, False, "newNATSReplicaClientFromConfig", "NATS mTLS client cert.", 'yaml:"client-cert"'),
    ("client-key", M, 1159, False, "newNATSReplicaClientFromConfig", "NATS mTLS client key.", 'yaml:"client-key"'),
    ("max-reconnects", M, 1160, False, "newNATSReplicaClientFromConfig", "NATS reconnect attempts.", 'yaml:"max-reconnects"'),
    ("reconnect-wait", M, 1161, False, "newNATSReplicaClientFromConfig", "NATS reconnect delay.", 'yaml:"reconnect-wait"'),
    ("timeout", M, 1162, False, "newNATSReplicaClientFromConfig", "NATS connect timeout.", 'yaml:"timeout"'),
]
for name, f, line, must, handler, why, check in setting_rows:
    path = f if f.startswith("cmd/") or f.startswith("internal/") else f
    I(CLI, "commands", "setting", name, f"{path}:{line}", handler, must, why, check)

# commands: environment variables
I(CLI, "commands", "env", "LITESTREAM_CONFIG", "cmd/litestream/main.go:2034", "DefaultConfigPath", True,
  "Default config path when -config is not given.", 'os.Getenv("LITESTREAM_CONFIG")')
I(CLI, "commands", "env", "LOG_LEVEL", "cmd/litestream/main.go:691", "ParseConfig (also internal.InitLog, internal/log.go:44)", True,
  "Overrides logging.level; replicate -log-level sets it.", 'os.Getenv("LOG_LEVEL")')
I(CLI, "commands", "env", "LITESTREAM_ACCESS_KEY_ID", "cmd/litestream/main.go:1990", "applyLitestreamEnv", True,
  "Copied to AWS_ACCESS_KEY_ID when that is unset.", 'os.LookupEnv("LITESTREAM_ACCESS_KEY_ID")')
I(CLI, "commands", "env", "LITESTREAM_SECRET_ACCESS_KEY", "cmd/litestream/main.go:1995", "applyLitestreamEnv", True,
  "Copied to AWS_SECRET_ACCESS_KEY when that is unset.", 'os.LookupEnv("LITESTREAM_SECRET_ACCESS_KEY")')
I(CLI, "commands", "env", "AWS_ACCESS_KEY_ID", "cmd/litestream/main.go:1991", "applyLitestreamEnv", False,
  "Checked/set here; consumed by the AWS SDK credential chain and the s3 package.", 'os.LookupEnv("AWS_ACCESS_KEY_ID")')
I(CLI, "commands", "env", "AWS_SECRET_ACCESS_KEY", "cmd/litestream/main.go:1996", "applyLitestreamEnv", False,
  "Checked/set here; consumed by the AWS SDK credential chain and the s3 package.", 'os.LookupEnv("AWS_SECRET_ACCESS_KEY")')
I(CLI, "commands", "env", "$VAR / ${VAR} expansion in the config file", "cmd/litestream/main.go:628", "ParseConfig", False,
  "Any environment variable can be substituted into the config unless -no-expand-env.", "return os.Getenv(key)")
I(CLI, "commands", "env", "$PID (config expansion)", "cmd/litestream/main.go:625", "ParseConfig", False,
  "Special variable expanded to the process id.", 'if key == "PID"')

# workers
I(CLI, "workers", "directory-watcher", "DirectoryMonitor (fsnotify)", "cmd/litestream/directory_watcher.go:98", "DirectoryMonitor.run", True,
  "For dbs[].watch, a goroutine consumes fsnotify events and registers/unregisters databases with the Store.",
  "go dm.run()")
I(CLI, "workers", "timer", "DirectoryMonitor debounce timer", "cmd/litestream/directory_watcher.go:114", "DirectoryMonitor.flushPendingEvents", False,
  "250ms debounce before new files are checked and registered.", "debounceTimer := time.NewTimer(debounceInterval)")
I(CLI, "workers", "goroutine", "metrics HTTP server", "cmd/litestream/replicate.go:361", "http.ListenAndServe", False,
  "Serves /metrics (and pprof) in the background when addr is set.", "go func() {")
I(CLI, "workers", "goroutine", "MCP server", "cmd/litestream/replicate.go:188", "MCPServer.Start", False,
  "Serves MCP in the background when mcp-addr is set.", "go c.MCP.Start(c.Config.MCPAddr)")
I(CLI, "workers", "goroutine", "exec child waiter", "cmd/litestream/replicate.go:383", "exec.Cmd.Wait", False,
  "Waits for the -exec/exec child and triggers shutdown when it exits.", "go func() { c.execCh <- c.cmd.Wait() }()")
I(CLI, "workers", "goroutine", "one-shot replication (-once)", "cmd/litestream/replicate.go:386", "ReplicateCommand.runOnce", False,
  "Runs one sync (and optional snapshot/retention) of every database, then signals exit.", "go c.runOnce(ctx)")
I(CLI, "workers", "wiring", "Store background monitors", "cmd/litestream/replicate.go:291", "litestream.Store.Open", False,
  "Wiring only: Store.Open starts DB/replica/compaction/snapshot/L0/heartbeat/validation loops listed under the library.",
  "if err := c.Store.Open(ctx); err != nil {")
I(CLI, "workers", "service-loop", "Windows service control loop", "cmd/litestream/main_windows.go:80", "windowsService.Execute", False,
  "Blocks on SCM change requests while running as a Windows service.", "for {")

# external
for typ, line, fn in [("file", 1359, "newFileReplicaClientFromConfig"), ("s3", 1363, "NewS3ReplicaClientFromConfig"),
                      ("gs", 1367, "newGSReplicaClientFromConfig"), ("abs", 1371, "newABSReplicaClientFromConfig"),
                      ("sftp", 1375, "newSFTPReplicaClientFromConfig"), ("webdav", 1379, "newWebDAVReplicaClientFromConfig"),
                      ("nats", 1383, "newNATSReplicaClientFromConfig"), ("oss", 1387, "newOSSReplicaClientFromConfig")]:
    I(CLI, "external", "replica-client", f"{typ} replica client", f"cmd/litestream/main.go:{line}", fn, True,
      f"Replica type `{typ}` selected from type/url builds the {typ} package client.", fn)
I(CLI, "external", "process", "exec child process", "cmd/litestream/replicate.go:376", "ReplicateCommand.Run", True,
  "Launches the configured exec command (shellwords-parsed) with the parent environment.",
  "c.cmd = exec.CommandContext(ctx, execArgs[0], execArgs[1:]...)")
I(CLI, "external", "process", "`litestream` CLI subprocess from MCP tools", "cmd/litestream/mcp.go:96", "DatabasesTool/InfoTool/RestoreTool/LTXTool/VersionTool/StatusTool/ResetTool closures", True,
  "Every MCP tool runs `litestream <subcommand>` found on PATH (also mcp.go:116,136,167,235,250,287,313,339).",
  'cmd := exec.CommandContext(ctx, "litestream", args...)')
for cmd, f, line, method in [("start", "start.go", 63, "POST /start"), ("stop", "stop.go", 63, "POST /stop"),
                             ("sync", "sync.go", 68, "POST /sync"), ("register", "register.go", 72, "POST /register"),
                             ("unregister", "unregister.go", 76, "POST /unregister"), ("info", "info.go", 48, "GET /info"),
                             ("list", "list.go", 48, "GET /list")]:
    I(CLI, "external", "unix-socket-http-client", f"{method} to control socket ({cmd})", f"cmd/litestream/{f}:{line}", handlers[cmd], True,
      "HTTP over the daemon's Unix control socket (-socket).", f'"http://localhost/{method.split("/")[1]}"')
I(CLI, "external", "wiring", "heartbeat HTTP client", "cmd/litestream/replicate.go:275", "litestream.NewHeartbeatClient", False,
  "Wiring only: heartbeat-url configures the library HeartbeatClient (GET ping).",
  "c.Store.Heartbeat = litestream.NewHeartbeatClient(c.Config.HeartbeatURL, interval)")
I(CLI, "external", "wiring", "replicated SQLite databases (via litestream.DB)", "cmd/litestream/main.go:739", "NewDBFromConfig -> litestream.NewDB", False,
  "Wiring only: each dbs[] entry becomes a library DB that opens the SQLite database (library item db.go:1039).",
  "db := litestream.NewDB(configPath)")
I(CLI, "external", "sqlite", "SQLite header probe", "cmd/litestream/main.go:1036", "IsSQLiteDatabase", False,
  "Directory mode opens candidate files to check the `SQLite format 3` header.", "file, err := os.Open(path)")
I(CLI, "external", "file", "SSE-C key file", "cmd/litestream/main.go:1638", "NewS3ReplicaClientFromConfig", False,
  "Reads sse-customer-key-path from disk.", "keyData, err := os.ReadFile(keyPath)")
I(CLI, "external", "os-service", "Windows event log", "cmd/litestream/main_windows.go:32", "runWindowsService", False,
  "Windows service mode logs to the Windows event log.", "elog, err := eventlog.Open(serviceName)")

# data
I(CLI, "data", "config-file", "/etc/litestream.yml", "cmd/litestream/main_notwindows.go:12", "ReadConfigFile / ParseConfig", True,
  "Default YAML config path on non-Windows (override with -config or LITESTREAM_CONFIG).",
  'const defaultConfigPath = "/etc/litestream.yml"')
I(CLI, "data", "config-file", "C:\\Litestream\\litestream.yml", "cmd/litestream/main_windows.go:17", "ReadConfigFile", False,
  "Default config path on Windows.", "const defaultConfigPath = `C:\\Litestream\\litestream.yml`")
I(CLI, "data", "file", "restore output sidecars (-wal, -shm, -journal)", "cmd/litestream/restore.go:291", "RestoreCommand.prepareOutputPath", False,
  "restore -force removes an existing output database and its SQLite sidecars before restoring.",
  'for _, removePath := range []string{path, path + "-wal", path + "-shm", path + "-journal"}')
I(CLI, "data", "in-memory", "DirectoryMonitor.dbs", "cmd/litestream/directory_watcher.go:36", "DirectoryMonitor", False,
  "Map of databases discovered in a watched directory.", "dbs         map[string]*litestream.DB")

# ---------------------------------------------------------------- library
for method, line, handler, why in [
    ("POST /start", 75, "Server.handleStart", "Opens/starts replication of a known database."),
    ("POST /stop", 76, "Server.handleStop", "Stops a database after a final sync (default 30s timeout)."),
    ("GET /txid", 77, "Server.handleTXID", "Returns the current local TXID of a database; no CLI caller."),
    ("POST /register", 78, "Server.handleRegister", "Creates DB + replica from replica_url and adds it to the Store."),
    ("POST /unregister", 79, "Server.handleUnregister", "Removes and closes a database."),
    ("POST /sync", 80, "Server.handleSync", "Forces a sync; wait=true also uploads to the replica."),
    ("GET /list", 81, "Server.handleList", "Lists managed databases with status and last sync time."),
    ("GET /info", 82, "Server.handleInfo", "Version, PID, uptime, database count."),
    ("GET /debug/sync-status", 83, "Server.handleSyncStatus", "Per-DB sync/checkpoint diagnostics; no CLI caller."),
]:
    I(LIB, "requests", "unix-socket-http", method, f"server.go:{line}", handler, True,
      why + " Served on the Unix control socket that `litestream replicate` starts when socket.enabled.",
      f'mux.HandleFunc("{method}"')
for route, line, fn in [("GET /debug/pprof/", 86, "pprof.Index"), ("GET /debug/pprof/cmdline", 87, "pprof.Cmdline"),
                        ("GET /debug/pprof/profile", 88, "pprof.Profile"), ("GET /debug/pprof/symbol", 89, "pprof.Symbol"),
                        ("GET /debug/pprof/trace", 90, "pprof.Trace")]:
    I(LIB, "requests", "unix-socket-http", route, f"server.go:{line}", fn, False,
      "Go profiling endpoint on the control socket.", f'mux.HandleFunc("{route}"')

I(LIB, "workers", "ticker", "DB monitor", "db.go:794", "DB.monitor", True,
  "Per database: every MonitorInterval (default 1s, db.go:3053) syncs new WAL frames into L0 LTX files and checkpoints; backoff on errors.",
  "go func() { defer db.wg.Done(); db.monitor() }()")
I(LIB, "workers", "ticker", "Replica monitor", "replica.go:118", "Replica.monitor", True,
  "Per database: every SyncInterval (default 1s, replica.go:383) or on WAL notify uploads pending L0 LTX files to the replica client.",
  "go func() { defer r.wg.Done(); r.monitor(ctx) }()")
I(LIB, "workers", "timer", "compaction monitor (per level L1..Ln)", "store.go:199", "Store.monitorCompactionLevel", True,
  "One goroutine per non-zero level compacts the previous level into this one at the level interval (timer store.go:543).",
  "go func() {")
I(LIB, "workers", "timer", "snapshot monitor (level 9) + retention", "store.go:207", "Store.monitorCompactionLevel(SnapshotLevel)", True,
  "Writes full snapshots every snapshot.interval and enforces snapshot/level retention (store.go:575).",
  "go func() {")
I(LIB, "workers", "ticker", "L0 retention monitor", "store.go:215", "Store.monitorL0Retention", True,
  "Every l0-retention-check-interval deletes L0 files older than l0-retention that are already compacted (ticker store.go:610).",
  "go func() {")
I(LIB, "workers", "ticker", "heartbeat monitor", "store.go:655", "Store.monitorHeartbeats", True,
  "Every 15s checks whether all databases synced within heartbeat-interval and pings heartbeat-url (ticker store.go:673).",
  "go func() {")
I(LIB, "workers", "ticker", "validation monitor", "store.go:227", "Store.monitorValidation", True,
  "When validation.interval is set, periodically checks LTX gaps/overlaps on the replica (ticker store.go:874).",
  "go func() {")
I(LIB, "workers", "ticker", "restore follow loop", "replica.go:817", "Replica.follow", True,
  "restore -f polls the replica every follow-interval and applies new LTX files to the restored database.",
  "ticker := time.NewTicker(interval)")
I(LIB, "workers", "goroutine", "control socket HTTP server", "server.go:134", "http.Server.Serve", False,
  "Serves the control routes on the Unix listener in the background.", "go func() {")
I(LIB, "workers", "ticker", "VFS compaction monitors (per level)", "vfs.go:2876", "VFSFile.monitorCompaction", False,
  "Library VFS only when VFS.CompactionEnabled; the litestream-vfs extension never enables it.",
  "go func(level *CompactionLevel) {")
I(LIB, "workers", "ticker", "VFS snapshot monitor", "vfs.go:2885", "VFSFile.monitorSnapshots", False,
  "Library VFS only when CompactionEnabled and SnapshotInterval > 0.", "go func() {")
I(LIB, "workers", "ticker", "VFS L0 retention monitor", "vfs.go:2894", "VFSFile.monitorL0Retention", False,
  "Library VFS only when CompactionEnabled and L0Retention > 0.", "go func() {")

I(LIB, "external", "sqlite", "replicated SQLite database (modernc.org/sqlite)", "db.go:1039", "DB.init", True,
  "Opens the user's database with busy_timeout and wal_autocheckpoint(0), sets journal_mode=wal, holds a read transaction, runs checkpoints.",
  'if db.db, err = sql.Open("sqlite", dsn); err != nil {')
I(LIB, "external", "file", "database file descriptor", "db.go:1049", "DB.init", False,
  "Long-running os.File on the database to avoid non-OFD lock issues.", "if db.f, err = os.Open(db.path); err != nil {")
I(LIB, "external", "sqlite", "restored database integrity check", "replica.go:1318", "checkIntegrity", True,
  "After restore with -integrity-check, opens the restored DB and runs PRAGMA quick_check/integrity_check.",
  'db, err := sql.Open("sqlite", dbPath)')
I(LIB, "external", "sqlite", "v0.3.x restore checkpoint", "replica.go:1302", "checkpointV3", False,
  "Legacy v0.3.x restore opens the restored DB to run wal_checkpoint(TRUNCATE).", 'db, err := sql.Open("sqlite", dbPath)')
I(LIB, "external", "http-client", "heartbeat ping (HTTP GET heartbeat-url)", "heartbeat.go:50", "HeartbeatClient.Ping", True,
  "GET to the configured URL with a 30s timeout; non-2xx is an error.",
  "req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)")
I(LIB, "external", "replica-client", "ReplicaClient interface", "replica_client.go:19", "Replica / Compactor / restore", True,
  "All remote I/O (list, open, write, delete LTX files) goes through this interface implemented by the backend packages.",
  "type ReplicaClient interface {")
I(LIB, "external", "registry", "replica URL scheme registry", "replica_url.go:31", "NewReplicaClientFromURL", False,
  "Maps URL schemes to backend factories registered by backend init() functions (used by /register and the VFS).",
  "func NewReplicaClientFromURL(rawURL string) (ReplicaClient, error) {")

I(LIB, "data", "sqlite-db", "replicated SQLite database", "db.go:1036", "DB.init", True,
  "The user's database; Litestream reads pages, owns checkpointing (autocheckpoint off) and may TRUNCATE/PASSIVE checkpoint it.",
  'dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)&_pragma=wal_autocheckpoint(0)"')
I(LIB, "data", "wal", "SQLite WAL (<db>-wal)", "db.go:494", "DB.WALPath / WALReader", True,
  "Source of replicated frames; read by the sync executor and kept alive with PERSIST_WAL.",
  'return db.path + "-wal"')
I(LIB, "data", "sqlite-table", "_litestream_seq", "db.go:1075", "DB.init / DB.bumpLitestreamSeq", True,
  "Table created inside the user's database to force WAL writes.", "CREATE TABLE IF NOT EXISTS _litestream_seq")
I(LIB, "data", "sqlite-table", "_litestream_lock", "db.go:1081", "DB.init / checkpoint", True,
  "Table created inside the user's database; written in rolled-back transactions to take write locks.",
  "CREATE TABLE IF NOT EXISTS _litestream_lock")
I(LIB, "data", "directory", "meta directory .<db>-litestream", "db.go:304", "NewDB", True,
  "Local metadata directory next to the database (MetaDirSuffix, litestream.go:20); overridable by meta-path.",
  'metaPath: filepath.Join(dir, "."+file+MetaDirSuffix)')
I(LIB, "data", "ltx-file", "local LTX files <meta>/ltx/<level>/<min>-<max>.ltx", "db.go:546", "DB.LTXPath", True,
  "L0 files written by every sync (db.go:1957) are the local 'shadow' log that the replica uploads and compaction reads.",
  "return filepath.Join(db.LTXLevelDir(level), ltx.FormatFilename(minTXID, maxTXID))")
I(LIB, "data", "temp-file", "*.tmp LTX files", "db.go:2055", "DB.sync", False,
  "LTX files are written to .tmp then renamed; leftovers removed on Open.", 'tmpFilename := filename + ".tmp"')
I(LIB, "data", "ltx-file", "remote LTX layout <path>/ltx/<level>/<min>-<max>.ltx", "litestream.go:195", "LTXFilePath", True,
  "Replica layout for file, gs, abs, sftp, webdav, nats (s3 and oss use <path>/<%04x level>/...); levels 0..3 plus snapshot level 9.",
  "func LTXFilePath(root string, level int, minTXID, maxTXID ltx.TXID) string {")
I(LIB, "data", "file", "follow-mode TXID sidecar <db>-txid", "replica.go:1696", "WriteTXIDFile / ReadTXIDFile", True,
  "restore -f records the last applied TXID for crash-safe resume.", 'return outputPath + "-txid"')
I(LIB, "data", "file", "restored database output (+ .tmp)", "replica.go:718", "Replica.Restore", False,
  "Restore decodes compacted LTX into <output>.tmp then renames it.", 'tmpOutputPath := opt.OutputPath + ".tmp"')
I(LIB, "data", "legacy-layout", "v0.3.x generations/<gen>/snapshots|wal", "v3.go:62", "Replica.RestoreV3", False,
  "Read-only legacy layout used when restoring backups made by Litestream v0.3.x; never written.",
  'GenerationsDirV3 = "generations"')
I(LIB, "data", "unix-socket", "control socket file", "server.go:116", "Server.Start", True,
  "Unix socket at socket.path (default /var/run/litestream.sock), chmod to socket.permissions.",
  'listener, err := net.Listen("unix", s.SocketPath)')
I(LIB, "data", "in-memory", "Store.dbs / levels", "store.go:89", "Store", True,
  "Main in-memory state: the set of managed databases and compaction levels.", "dbs    []*DB")
I(LIB, "data", "in-memory", "Replica.pos", "replica.go:39", "Replica", False,
  "Last replicated position.", "pos ltx.Pos // current replicated position")
I(LIB, "data", "in-memory", "DB.maxLTXFileInfos / DB.pos caches", "db.go:82", "DB", False,
  "Cached per-level max LTX file info and current position.", "maxLTXFileInfos struct {")
I(LIB, "data", "in-memory", "DB.syncDiag", "db.go:79", "DB.SyncDiagnostic", False,
  "Sync/checkpoint phase diagnostics exposed at /debug/sync-status.", "syncDiag  diagState")
I(LIB, "data", "metrics", "Prometheus metrics litestream_*", "db.go:3235", "promauto registry", False,
  "db/wal size, txid, sync/checkpoint counters, compaction verify errors; exposed at /metrics.",
  "dbSizeGaugeVec = promauto.NewGaugeVec(prometheus.GaugeOpts{")

# ---------------------------------------------------------------- cmd/litestream-vfs
I(VFS, "commands", "extension-entry", "sqlite3_litestreamvfs_init", "src/litestream-vfs.c:31", "LitestreamVFSRegister", True,
  "SQLite loadable-extension entry point (load_extension of litestream-vfs.so/.dylib).",
  "int sqlite3_litestreamvfs_init(")
I(VFS, "commands", "vfs", "VFS name \"litestream\"", "cmd/litestream-vfs/main.go:107", "LitestreamVFSRegister / litestream.VFS", True,
  "Registers the read-replica VFS that serves database pages from the replica.",
  'sqlite3vfs.RegisterVFS("litestream", vfs)')
for fn, line, impl, why in [
    ("litestream_set_time", 59, "litestream_set_time_impl -> GoLitestreamSetTime/GoLitestreamResetTime", "Time travel: view the database at a timestamp, or LATEST."),
    ("litestream_time", 62, "litestream_time_impl -> GoLitestreamTime", "Current view time."),
    ("litestream_txid", 63, "litestream_txid_impl -> GoLitestreamTxid", "Current TXID of the connection."),
    ("litestream_lag", 64, "litestream_lag_impl -> GoLitestreamLag", "Seconds since last successful replica poll."),
]:
    I(VFS, "commands", "sql-function", fn, f"src/litestream-vfs.c:{line}", impl, True, why,
      f'sqlite3_create_function_v2(db, "{fn}"')
for pragma, line, why, check in [
    ("litestream_txid", 2344, "Read-only current TXID.", 'case "litestream_txid":'),
    ("litestream_lag", 2352, "Read-only seconds since last poll (-1 if never).", 'case "litestream_lag":'),
    ("litestream_time", 2365, "Read or set the time-travel target (LATEST resets).", 'case "litestream_time":'),
    ("litestream_hydration_progress", 2387, "Read-only hydration percent.", 'case "litestream_hydration_progress":'),
    ("litestream_hydration_file", 2399, "Read-only hydration file path.", 'case "litestream_hydration_file":'),
    ("litestream_write_enabled", 2406, "Read or toggle write support at runtime.", 'case "litestream_write_enabled":'),
]:
    I(VFS, "commands", "pragma", f"PRAGMA {pragma}", f"vfs.go:{line}", "VFSFile.FileControl", True, why, check)
for env, line, why, check in [
    ("LITESTREAM_REPLICA_URL", 46, "Required replica URL the VFS reads from.", 'os.Getenv("LITESTREAM_REPLICA_URL")'),
    ("LITESTREAM_LOG_LEVEL", 62, "DEBUG enables debug logging.", 'os.Getenv("LITESTREAM_LOG_LEVEL")'),
    ("LITESTREAM_LOG_FILE", 70, "Append logs to a file instead of stdout.", 'os.Getenv("LITESTREAM_LOG_FILE")'),
    ("LITESTREAM_WRITE_ENABLED", 82, "true enables write support (dirty pages synced as LTX).", 'os.Getenv("LITESTREAM_WRITE_ENABLED")'),
    ("LITESTREAM_SYNC_INTERVAL", 85, "Write sync interval (Go duration).", 'os.Getenv("LITESTREAM_SYNC_INTERVAL")'),
    ("LITESTREAM_BUFFER_PATH", 93, "Write buffer file path.", 'os.Getenv("LITESTREAM_BUFFER_PATH")'),
    ("LITESTREAM_HYDRATION_ENABLED", 99, "true enables background hydration to a local file.", 'os.Getenv("LITESTREAM_HYDRATION_ENABLED")'),
    ("LITESTREAM_HYDRATION_PATH", 102, "Persistent hydration file path.", 'os.Getenv("LITESTREAM_HYDRATION_PATH")'),
]:
    I(VFS, "commands", "env", env, f"cmd/litestream-vfs/main.go:{line}", "LitestreamVFSRegister", True, why, check)

I(VFS, "workers", "ticker", "replica poller", "vfs.go:1088", "VFSFile.monitorReplicaClient", True,
  "Per opened database: every PollInterval (1s, ticker vfs.go:2472) polls the replica for new LTX files and updates the page index.",
  "go func() { defer f.wg.Done(); f.monitorReplicaClient(f.ctx) }()")
I(VFS, "workers", "ticker", "write sync loop", "vfs.go:1097", "VFSFile.syncLoop", True,
  "With LITESTREAM_WRITE_ENABLED, periodically uploads dirty pages as an L0 LTX file (also started at vfs.go:1154, 1859).",
  "go func() { defer f.wg.Done(); f.syncLoop(stopCh, tickerCh) }()")
I(VFS, "workers", "goroutine", "background hydration", "vfs.go:1296", "VFSFile.runHydration", True,
  "With LITESTREAM_HYDRATION_ENABLED, restores the database into a local file and then serves reads from it.",
  "go f.runHydration(infos)")
I(VFS, "workers", "poll-loop", "wait for first backup", "vfs.go:2756", "VFSFile.waitForRestorePlan", False,
  "Read-only open blocks, polling every PollInterval until backup files exist.", "case <-time.After(f.PollInterval):")

I(VFS, "external", "replica-client", "replica client from LITESTREAM_REPLICA_URL", "cmd/litestream-vfs/main.go:51", "litestream.NewReplicaClientFromURL", True,
  "Any registered backend (abs, file, gs, nats, oss, s3, sftp, webdav imported at main.go:29-36); reads LTX page indexes/pages, writes LTX when write-enabled.",
  "client, err = litestream.NewReplicaClientFromURL(replicaURL)")
I(VFS, "external", "replica-write", "LTX upload of dirty pages", "vfs.go:1917", "VFSFile.syncToRemoteWithLock", False,
  "Write mode uploads L0 LTX files with conflict detection against the remote TXID.",
  "info, err := f.client.WriteLTXFile(ctx, 0, f.pendingTXID, f.pendingTXID, ltxReader)")
I(VFS, "external", "host-sqlite", "host SQLite auto-extension", "src/litestream-vfs.c:44", "litestream_auto_extension", False,
  "Registers per-connection SQL functions in the host process's SQLite.",
  "rc = sqlite3_auto_extension((void (*)(void))litestream_auto_extension);")

I(VFS, "data", "in-memory", "page index (VFSFile.index)", "vfs.go:520", "VFSFile.buildIndex / pollReplicaClient", True,
  "Map page number -> LTX file/offset that lets SQLite read pages straight from the replica.",
  "index           map[uint32]ltx.PageIndexElem")
I(VFS, "data", "in-memory", "LRU page cache", "vfs.go:523", "VFSFile.ReadAt", False,
  "Page cache sized by CacheSize (default 10MB).", "cache           *lru.Cache[uint32, []byte]")
I(VFS, "data", "file", "write buffer file", "vfs.go:2104", "VFSFile.initWriteBufferWithLock", True,
  "Durable buffer of dirty pages (LITESTREAM_BUFFER_PATH or <tmp>/write-buffer-N, vfs.go:167).",
  "file, err := os.OpenFile(f.bufferPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)")
I(VFS, "data", "file", "hydration file", "vfs.go:616", "Hydrator.Init", True,
  "Local SQLite copy built in the background (LITESTREAM_HYDRATION_PATH or <tmp>/hydration.db, vfs.go:188).",
  "file, err := os.OpenFile(h.path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)")
I(VFS, "data", "file", "hydration meta file <hydration>.meta", "vfs.go:881", "Hydrator.saveMeta / loadMeta", True,
  "Stores the TXID of a persistent hydration file for reuse across restarts.", 'return h.path + ".meta"')
I(VFS, "data", "directory", "temp dir litestream-vfs-*", "vfs.go:272", "VFS.ensureTempDir", False,
  "Holds default write buffers, hydration file and SQLite temp files.", 'os.MkdirTemp("", "litestream-vfs-*")')
I(VFS, "data", "file", "SQLite temp files", "vfs.go:315", "VFS.openTempFile", False,
  "Temp/journal files SQLite asks the VFS to open are local files.", 'f, err = os.CreateTemp(dir, "temp-*")')
I(VFS, "data", "in-memory", "dirty page map", "vfs.go:533", "VFSFile.WriteAt", False,
  "Dirty pages -> buffer offsets awaiting sync.", "dirty         map[uint32]int64")
I(VFS, "data", "file", "log file (LITESTREAM_LOG_FILE)", "cmd/litestream-vfs/main.go:71", "LitestreamVFSRegister", False,
  "Optional append-only log file.", "f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)")
I(VFS, "data", "in-memory", "vfsConnectionMap", "vfs.go:49", "RegisterVFSConnection", False,
  "Maps sqlite3* connections to VFS files for the SQL functions.", "vfsConnectionMap sync.Map")

# ---------------------------------------------------------------- cmd/litestream-test
for name, line, handler, why in [
    ("populate", 54, "PopulateCommand.Run", "Fills a database to a target size."),
    ("load", 56, "LoadCommand.Run", "Generates continuous read/write load."),
    ("shrink", 58, "ShrinkCommand.Run", "Deletes a percentage of rows, optional checkpoint/VACUUM."),
    ("validate", 60, "ValidateCommand.Run", "Restores via `litestream restore` and compares/checks the result."),
    ("version", 62, "VersionCommand.Run", "Prints version/commit/go version."),
]:
    I(TEST, "commands", "subcommand", name, f"cmd/litestream-test/main.go:{line}", handler, True, why, f'case "{name}":')
I(TEST, "commands", "subcommand", "help", "cmd/litestream-test/main.go:48", "Main.Usage", False, "Prints usage.", 'fs.Arg(0) == "help"')
test_flags = [
    ("load", "load.go", [(44, "-db"), (45, "-write-rate"), (46, "-duration"), (47, "-pattern"), (48, "-payload-size"), (49, "-read-ratio"), (50, "-workers")], "LoadCommand.Run"),
    ("populate", "populate.go", [(31, "-db"), (32, "-target-size"), (33, "-row-size"), (34, "-batch-size"), (35, "-table-count"), (36, "-index-ratio"), (37, "-page-size")], "PopulateCommand.Run"),
    ("shrink", "shrink.go", [(27, "-db"), (28, "-delete-percentage"), (29, "-vacuum"), (30, "-checkpoint"), (31, "-checkpoint-mode")], "ShrinkCommand.Run"),
    ("validate", "validate.go", [(40, "-source-db"), (41, "-replica-url"), (42, "-restored-db"), (43, "-check-type"), (44, "-ltx-continuity"), (45, "-config")], "ValidateCommand.Run"),
]
for cmd, f, rows, handler in test_flags:
    for line, flag in rows:
        I(TEST, "commands", "flag", f"{cmd} {flag}", f"cmd/litestream-test/{f}:{line}", handler, False,
          f"Option of the test-harness {cmd} subcommand.", f'"{flag[1:]}"')
I(TEST, "workers", "ticker", "load workers", "cmd/litestream-test/load.go:110", "LoadCommand.worker", True,
  "-workers goroutines, each ticking at write-rate/workers (load.go:125) to write/read.", "go func(workerID int) {")
I(TEST, "workers", "ticker", "load stats reporter", "cmd/litestream-test/load.go:116", "LoadCommand.reportStats", True,
  "Logs throughput every 5s (ticker load.go:235).", "go c.reportStats(ctx, stats)")
I(TEST, "workers", "goroutine", "load signal handler", "cmd/litestream-test/load.go:96", "LoadCommand.generateLoad", False,
  "Cancels load on SIGINT/SIGTERM.", "go func() {")
for f, line, handler in [("load.go", 78, "LoadCommand.generateLoad"), ("populate.go", 74, "PopulateCommand.Run"),
                         ("shrink.go", 69, "ShrinkCommand.Run"), ("validate.go", 161, "ValidateCommand (quick/integrity/checksum checks)")]:
    I(TEST, "external", "sqlite", f"SQLite via mattn/go-sqlite3 ({f})", f"cmd/litestream-test/{f}:{line}", handler, True,
      "Opens the test/restored database with the cgo sqlite3 driver.", 'sql.Open("sqlite3"')
I(TEST, "external", "process", "`litestream restore` subprocess", "cmd/litestream-test/validate.go:112", "ValidateCommand.validateRestore", True,
  "Restores the source DB or replica URL into -restored-db (also validate.go:118).", 'exec.CommandContext(ctx, "litestream", "restore",')
I(TEST, "external", "process", "`litestream ltx` subprocess", "cmd/litestream-test/validate.go:372", "ValidateCommand.validateLTXContinuity", True,
  "Lists replica LTX files to check TXID continuity.", 'exec.CommandContext(ctx, "litestream", "ltx", c.ReplicaURL)')
I(TEST, "data", "sqlite-table", "test_table_N", "cmd/litestream-test/populate.go:93", "PopulateCommand", True,
  "Tables created in the -db database by populate.", 'tableName := fmt.Sprintf("test_table_%d", i)')
I(TEST, "data", "sqlite-table", "load_test", "cmd/litestream-test/load.go:196", "LoadCommand.ensureTestTable", True,
  "Table written by load.", "CREATE TABLE IF NOT EXISTS load_test (")
I(TEST, "data", "sqlite-db", "restored database (-restored-db)", "cmd/litestream-test/validate.go:106", "ValidateCommand.validateRestore", False,
  "Deleted and recreated by validate.", "if err := os.Remove(c.RestoredDB); err != nil && !os.IsNotExist(err) {")

# ---------------------------------------------------------------- backend packages
def backend(comp, ext, data, extra=()):
    for it in ext + data + list(extra):
        I(comp, *it)

backend("abs",
        [("external", "replica-client", "Azure Blob Storage", "abs/replica_client.go:100", "ReplicaClient.Init", True,
          "azblob client via SAS token, shared key or DefaultAzureCredential; default endpoint https://<account>.blob.core.windows.net (line 119).",
          "func (c *ReplicaClient) Init(ctx context.Context) (err error) {")],
        [("data", "blob", "blobs <path>/ltx/<level>/<min>-<max>.ltx", "abs/replica_client.go:222", "ReplicaClient.OpenLTXFile / WriteLTXFile", True,
          "LTX objects in the container.", "key := litestream.LTXFilePath(c.Path, level, minTXID, maxTXID)"),
         ("data", "metadata", "blob metadata litestreamtimestamp", "abs/replica_client.go:41", "ReplicaClient.WriteLTXFile", False,
          "Accurate LTX timestamp stored as blob metadata.", 'const MetadataKeyTimestamp = "litestreamtimestamp"')],
        [("commands", "env", "LITESTREAM_AZURE_SAS_TOKEN", "abs/replica_client.go:148", "ReplicaClient.Init", True,
          "SAS token fallback when sas-token is unset.", 'os.Getenv("LITESTREAM_AZURE_SAS_TOKEN")'),
         ("commands", "env", "LITESTREAM_AZURE_ACCOUNT_KEY", "abs/replica_client.go:154", "ReplicaClient.Init", True,
          "Account key fallback when account-key is unset.", 'os.Getenv("LITESTREAM_AZURE_ACCOUNT_KEY")'),
         ("commands", "url-scheme", "abs://[account@]container/path", "abs/replica_client.go:33", "NewReplicaClientFromURL", False,
          "Replica URL scheme registered for this backend.", 'RegisterReplicaClientFactory("abs"')])
backend("file",
        [("external", "filesystem", "local/NFS directory", "file/replica_client.go:157", "ReplicaClient.WriteLTXFile", True,
          "Writes LTX files to a directory with tmp+rename and sets mtime to the LTX timestamp (line 229).",
          "func (c *ReplicaClient) WriteLTXFile(")],
        [("data", "ltx-file", "<path>/ltx/<level>/<min>-<max>.ltx", "file/replica_client.go:91", "ReplicaClient.LTXFilePath", True,
          "File replica layout.", "return filepath.FromSlash(litestream.LTXFilePath(c.path, level, minTXID, maxTXID))"),
         ("data", "temp-file", "<file>.tmp", "file/replica_client.go:184", "ReplicaClient.WriteLTXFile", False,
          "Temporary file before rename.", 'tmpFilename := filename + ".tmp"')],
        [("commands", "url-scheme", "file:///path", "file/replica_client.go:22", "NewReplicaClientFromURL", False,
          "Replica URL scheme registered for this backend.", 'RegisterReplicaClientFactory("file"')])
backend("gs",
        [("external", "replica-client", "Google Cloud Storage", "gs/replica_client.go:86", "ReplicaClient.Init", True,
          "storage.NewClient with Application Default Credentials.", "if c.client, err = storage.NewClient(ctx); err != nil {")],
        [("data", "object", "objects <path>/ltx/<level>/<min>-<max>.ltx", "gs/replica_client.go:146", "ReplicaClient.WriteLTXFile", True,
          "LTX objects in the bucket.", "key := litestream.LTXFilePath(c.Path, level, minTXID, maxTXID)"),
         ("data", "metadata", "object metadata litestream-timestamp", "gs/replica_client.go:32", "ReplicaClient.WriteLTXFile", False,
          "Accurate LTX timestamp.", 'const MetadataKeyTimestamp = "litestream-timestamp"')],
        [("commands", "url-scheme", "gs://bucket/path", "gs/replica_client.go:25", "NewReplicaClientFromURL", False,
          "Replica URL scheme registered for this backend.", 'RegisterReplicaClientFactory("gs"')])
backend("nats",
        [("external", "replica-client", "NATS server connection", "nats/replica_client.go:195", "ReplicaClient.connect", True,
          "nats.Connect (JWT/creds/nkey/user/token, TLS) then JetStream (line 200).", "nc, err := nats.Connect(url, opts...)"),
         ("external", "replica-client", "JetStream object store bucket", "nats/replica_client.go:219", "ReplicaClient.initObjectStore", True,
          "Existing object store bucket; not auto-created.", "objectStore, err := c.js.ObjectStore(ctx, c.BucketName)")],
        [("data", "object", "objects <path>/ltx/<level>/<min>-<max>.ltx", "nats/replica_client.go:244", "ReplicaClient.ltxPath", True,
          "LTX objects in the object store.", "return litestream.LTXFilePath(c.Path, level, minTXID, maxTXID)")],
        [("commands", "url-scheme", "nats://host/bucket", "nats/replica_client.go:27", "NewReplicaClientFromURL", False,
          "Replica URL scheme registered for this backend.", 'RegisterReplicaClientFactory("nats"')])
backend("oss",
        [("external", "replica-client", "Alibaba Cloud OSS", "oss/replica_client.go:161", "ReplicaClient.Init", True,
          "OSS client with static keys or the SDK environment credentials provider (line 143).", "c.client = oss.NewClient(cfg)")],
        [("data", "object", "objects <path>/<%04x level>/<min>-<max>.ltx", "oss/replica_client.go:390", "ReplicaClient.ltxPath", True,
          "OSS key layout (no ltx/ segment, hex level).", 'return c.Path + "/" + fmt.Sprintf("%04x/%s", level, filename)'),
         ("data", "metadata", "object metadata litestream-timestamp", "oss/replica_client.go:37", "ReplicaClient.WriteLTXFile", False,
          "Accurate LTX timestamp.", 'const MetadataKeyTimestamp = "litestream-timestamp"')],
        [("commands", "url-scheme", "oss://bucket/path", "oss/replica_client.go:29", "NewReplicaClientFromURL", False,
          "Replica URL scheme registered for this backend.", 'RegisterReplicaClientFactory("oss"')])
s3_ext = [
    ("external", "replica-client", "AWS S3 / S3-compatible API", "s3/replica_client.go:455", "ReplicaClient.Init", True,
     "AWS SDK v2 client (custom transport, retryer, provider defaults for Tigris/R2/B2/MinIO/...).", "c.s3 = s3.NewFromConfig(cfg, s3Opts...)"),
    ("external", "replica-client", "bucket region lookup", "s3/replica_client.go:613", "ReplicaClient.findBucketRegion", False,
     "GetBucketLocation when neither region nor endpoint is set.", "out, err := client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{"),
]
s3_data = [
    ("data", "object", "objects <path>/<%04x level>/<min>-<max>.ltx", "s3/replica_client.go:703", "ReplicaClient.WriteLTXFile / OpenLTXFile", True,
     "S3 key layout (no ltx/ segment, hex level).", 'key := c.Path + "/" + fmt.Sprintf("%04x/%s", level, filename)'),
    ("data", "metadata", "object metadata litestream-timestamp", "s3/replica_client.go:55", "ReplicaClient.WriteLTXFile", False,
     "Accurate LTX timestamp.", 'const MetadataKeyTimestamp = "litestream-timestamp"'),
    ("data", "object", "lease object lock.json", "s3/leaser.go:23", "Leaser.AcquireLease", False,
     "Conditional-write lease in the bucket; library-only, no caller in this repo.", 'DefaultLeasePath = "lock.json"'),
]
s3_cmds = [
    ("commands", "env", "AWS_ACCESS_KEY_ID", "s3/replica_client.go:220", "NewReplicaClientFromURL", True,
     "Access key for URL-built clients (restore/ltx/register with s3:// URLs).", 'os.Getenv("AWS_ACCESS_KEY_ID")'),
    ("commands", "env", "LITESTREAM_ACCESS_KEY_ID", "s3/replica_client.go:222", "NewReplicaClientFromURL", True,
     "Fallback access key for URL-built clients.", 'os.Getenv("LITESTREAM_ACCESS_KEY_ID")'),
    ("commands", "env", "AWS_SECRET_ACCESS_KEY", "s3/replica_client.go:225", "NewReplicaClientFromURL", True,
     "Secret key for URL-built clients.", 'os.Getenv("AWS_SECRET_ACCESS_KEY")'),
    ("commands", "env", "LITESTREAM_SECRET_ACCESS_KEY", "s3/replica_client.go:227", "NewReplicaClientFromURL", True,
     "Fallback secret key for URL-built clients.", 'os.Getenv("LITESTREAM_SECRET_ACCESS_KEY")'),
    ("commands", "env", "LITESTREAM_S3_ENDPOINT", "s3/replica_client.go:232", "NewReplicaClientFromURL", True,
     "Default endpoint for URL-built clients (forces path style).", 'os.Getenv("LITESTREAM_S3_ENDPOINT")'),
    ("commands", "env", "LITESTREAM_S3_DEBUG", "s3/replica_client.go:1716", "parseS3DebugEnv", True,
     "Enables AWS SDK request/response/signing/retry logging.", 'os.Getenv("LITESTREAM_S3_DEBUG")'),
    ("commands", "url-scheme", "s3://bucket/path (incl. s3://arn:...)", "s3/replica_client.go:48", "NewReplicaClientFromURL", False,
     "Replica URL scheme registered for this backend.", 'RegisterReplicaClientFactory("s3"'),
]
for q, line, check in [("endpoint", 164, 'query.Get("endpoint")'), ("region", 173, 'query.Get("region")'),
                       ("forcePathStyle / force-path-style", 176, '"forcePathStyle", "force-path-style"'),
                       ("skipVerify / skip-verify", 179, '"skipVerify", "skip-verify"'),
                       ("signPayload / sign-payload", 182, '"signPayload", "sign-payload"'),
                       ("requireContentMD5 / require-content-md5", 186, '"requireContentMD5", "require-content-md5"'),
                       ("concurrency", 190, 'query.Get("concurrency")'), ("partSize / part-size", 196, 'query.Get("partSize")'),
                       ("storageClass / storage-class", 205, 'query.Get("storageClass")'),
                       ("sseCustomerAlgorithm", 300, 'query.Get("sseCustomerAlgorithm")'),
                       ("sseCustomerKey", 305, 'query.Get("sseCustomerKey")'),
                       ("sseCustomerKeyMD5", 310, 'query.Get("sseCustomerKeyMD5")'),
                       ("sseKmsKeyId", 317, 'query.Get("sseKmsKeyId")')]:
    s3_cmds.append(("commands", "url-query", f"?{q}", f"s3/replica_client.go:{line}", "NewReplicaClientFromURL", False,
                    "S3 replica URL query option.", check))
backend("s3", s3_ext, s3_data, s3_cmds)
backend("sftp",
        [("external", "replica-client", "SSH/SFTP server", "sftp/replica_client.go:163", "ReplicaClient.init", True,
          "ssh.Dial tcp (port 22 default) then sftp.NewClient; host key unchecked unless host-key is set (line 132).",
          'if c.sshClient, err = ssh.Dial("tcp", host, config); err != nil {'),
         ("external", "file", "SSH private key file (key-path)", "sftp/replica_client.go:144", "ReplicaClient.init", False,
          "Reads the private key from disk.", "buf, err := os.ReadFile(c.KeyPath)")],
        [("data", "file", "remote files <path>/ltx/<level>/<min>-<max>.ltx", "sftp/replica_client.go:275", "ReplicaClient.WriteLTXFile", True,
          "LTX files on the SFTP server.", "filename := litestream.LTXFilePath(c.Path, level, minTXID, maxTXID)")],
        [("commands", "url-scheme", "sftp://user[:pass]@host[:port]/path", "sftp/replica_client.go:26", "NewReplicaClientFromURL", False,
          "Replica URL scheme registered for this backend.", 'RegisterReplicaClientFactory("sftp"')])
backend("webdav",
        [("external", "replica-client", "WebDAV server (HTTP/HTTPS)", "webdav/replica_client.go:109", "ReplicaClient.Init", True,
          "gowebdav client; webdavs:// maps to https.", "c.client = gowebdav.NewClient(c.URL, c.Username, c.Password)")],
        [("data", "file", "remote files <path>/ltx/<level>/<min>-<max>.ltx", "webdav/replica_client.go:242", "ReplicaClient.WriteLTXFile", True,
          "LTX files on the WebDAV server.", "filename := litestream.LTXFilePath(c.Path, level, minTXID, maxTXID)")],
        [("commands", "url-scheme", "webdav:// and webdavs://", "webdav/replica_client.go:24", "NewReplicaClientFromURL", False,
          "Replica URL schemes registered for this backend (webdavs on line 25).", 'RegisterReplicaClientFactory("webdav"')])

# ---------------------------------------------------------------- traps
N(CLI, "validation-interval (replica setting)", "cmd/litestream/main.go:1098",
  "Parsed and propagated by SetDefaults but never applied; only top-level validation.interval enables validation.", 'yaml:"validation-interval"')
N(CLI, "tls (NATS setting)", "cmd/litestream/main.go:1156", "Parsed but never passed to the NATS client; TLS comes from root-cas/client-cert/client-key.", 'yaml:"tls"')
N(CLI, "name (replica setting)", "cmd/litestream/main.go:1323", "Deprecated field, never read.", 'yaml:"name"')
N(CLI, "age.identities / age.recipients", "cmd/litestream/main.go:1340", "Accepted by YAML but rejected with an error: encryption is not supported.", "len(c.Age.Identities) > 0")
N(CLI, "restore -integrity-check values none|quick|full", "cmd/litestream/restore.go:41", "Values of a flag, not flags.", 'fs.String("integrity-check"')
N(CLI, "ltx -level value \"all\"", "cmd/litestream/main.go:2130", "Value of the -level flag.", 'if s == "all" {')
N(CLI, "debounceInterval", "cmd/litestream/directory_watcher.go:18", "Internal constant of the directory monitor, not a setting.", "const debounceInterval = 250 * time.Millisecond")
N(CLI, "second-signal goroutine", "cmd/litestream/main.go:183", "Shutdown helper that closes `done` on a second signal, not a background loop.", "go func() {")
N(CLI, "restore follow signal goroutine", "cmd/litestream/restore.go:65", "Signal relay for restore -f, not a worker.", "go func() {")
N(LIB, "shadow WAL", "db.go:3247", "Only comments/metric help mention a shadow WAL; v0.5 writes L0 LTX files, no shadow WAL file exists.", "Total number of bytes written to shadow WAL")
N(LIB, "local generations/ and position files", "v3.go:62", "v0.5 keeps no local generation dirs or position files; generations/ is the v0.3.x remote layout read only by RestoreV3; position is derived from the newest L0 file (DB.Pos).", 'GenerationsDirV3 = "generations"')
for f, line, check, why in [
    ("compactor.go", 159, "go func() {", "io.Pipe compaction producer inside a single call."),
    ("replica.go", 730, "go func() {", "io.Pipe producer during restore."),
    ("db.go", 2716, "go func() {", "io.Pipe snapshot encoder."),
    ("vfs.go", 709, "go func() {", "io.Pipe producer during hydration restore."),
    ("vfs.go", 2028, "go func() {", "io.Pipe LTX encoder for write sync."),
    ("vfs.go", 2946, "go func() {", "io.Pipe snapshot encoder for VFS snapshots."),
]:
    N(LIB if f != "vfs.go" else VFS, f"pipe goroutine {f}:{line}", f"{f}:{line}", why + " Not a background loop.", check)
N(LIB, "Store.Open errgroup", "store.go:176", "Bounded parallel DB.Open at startup, not a worker.", "g.Go(func() error {")
N(LIB, "shutdown done-relay goroutine", "db.go:902", "Cancels the in-flight shutdown sync on second signal; not a background loop.", "go func() {")
N(LIB, "sync backoff waits", "db.go:3074", "Backoff inside DB.monitor, not a separate timer worker (same in replica.go:407).", "case <-time.After(backoff):")
N(LIB, "DefaultRetention / DefaultRetentionCheckInterval", "store.go:63", "Declared constants with no reader.", "DefaultRetention              = 24 * time.Hour")
N(LIB, "LogWriter / LogFlags", "litestream.go:103", "Legacy exported vars, unused.", "LogWriter = os.Stdout")
N(LIB, "verifyHeadersMatch", "db.go:1131", "Commented-out dead code.", "/*")
N("s3", "s3 metadata errgroup", "s3/replica_client.go:1436", "Bounded parallel HeadObject during listing, not a worker.", "g.Go(func() error {")
N("oss", "oss metadata errgroup", "oss/replica_client.go:453", "Bounded parallel metadata fetch, not a worker.", "g.Go(func() error {")
N("s3", "LITESTREAM_S3_DEBUG values", "s3/replica_client.go:1724", "Values of an env var, not separate env vars.", 'case "signing":')
N(VFS, "GoLitestream* cgo exports", "cmd/litestream-vfs/main.go:115", "Internal C<->Go bridge called by src/litestream-vfs.c, not user-facing commands.", "func GoLitestreamRegisterConnection(")
N(VFS, "page fetch retry timer", "vfs.go:1517", "Per-read retry delay, not a worker.", "timer := time.NewTimer(delay)")
N(VFS, "write-disable wake goroutine", "vfs.go:1739", "Wakes a cond wait on timeout/cancel, not a worker.", "go func() {")
N(VFS, "VFS compaction settings (CompactionEnabled, SnapshotInterval, L0Retention)", "vfs.go:88", "Library VFS fields never set by the extension; no env var exposes them.", "CompactionEnabled bool")
N(TEST, "validate -check-type values", "cmd/litestream-test/validate.go:43", "Values of a flag.", '"check-type"')
N(TEST, "load -pattern values", "cmd/litestream-test/load.go:47", "Values of a flag.", '"pattern"')
N("tests/integration", "integration test helpers (litestream/litestream-test/docker/aws subprocesses)", "tests/integration/helpers.go:241", "Test-only code.", 'exec.Command(getBinaryPath("litestream"), "replicate",')
N("tests/integration", "SOAK_* environment variables", "tests/integration/soak_helpers.go:62", "Test-only env vars.", 'os.Getenv("SOAK_AUTO_PURGE")')
N("internal/testingutil", "LITESTREAM_S3_* / LITESTREAM_*_ test env and -s3-* test flags", "internal/testingutil/testingutil.go:52", "Integration-test configuration only.", 'os.Getenv("LITESTREAM_S3_ACCESS_KEY_ID")')
N("internal/testingutil", "GOOGLE_APPLICATION_CREDENTIALS check", "internal/testingutil/testingutil.go:364", "Test gating only; product code relies on GCS ADC implicitly.", 'os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")')
N("internal/testingutil", "in-process SSH test server", "internal/testingutil/testingutil.go:477", "Test-only goroutine.", "go func() {")
N("mock", "mock ReplicaClient", "mock/replica_client.go:45", "Test double, not a backend.", "func (c *ReplicaClient) WriteLTXFile(")
N("_examples", "example programs (LITESTREAM_BUCKET, tickers)", "_examples/library/s3/main.go:46", "Examples under _examples are ignored by `go build ./...`.", 'os.Getenv("LITESTREAM_BUCKET")')
N("_examples", "example ticker", "_examples/library/basic/main.go:77", "Example program.", "ticker := time.NewTicker(2 * time.Second)")
N("etc", "moto S3 mock", "etc/s3_mock.py:5", "Test tooling.", "from moto.server import ThreadedMotoServer")
N("packages/python", "noop.c", "packages/python/litestream_vfs/noop.c:1", "Packaging placeholder, not the extension source.", "")
N("Makefile", "mcp-wrap / mcp-inspect targets", "Makefile:65", "Developer tooling (fly mcp), not product surface.", "fly mcp wrap")


# ---------------------------------------------------------------- review (2026-09-29)
# A read-only reviewer re-checked the writer's inventory against the source.
# Every change below is applied on top of the writer's rows and recorded in
# `review` (what, why, anchor) so the two layers stay distinguishable.
review = []


def rv(what, why, anchor):
    review.append(dict(what=what, why=why, anchor=anchor))


def find(comp, inv, name):
    hit = [it for it in items if it["component"] == comp and it["inventory"] == inv and it["name"] == name]
    assert len(hit) == 1, (comp, inv, name, len(hit))
    return hit[0]


def set_must(comp, inv, name, must, why):
    it = find(comp, inv, name)
    assert it["must"] != must, (comp, inv, name)
    it["must"] = must
    rv(f"{comp} / {inv} / {name}: must {not must} -> {must}", why, it["anchor"])


def set_field(comp, inv, name, field, value, why):
    it = find(comp, inv, name)
    old = it[field]
    it[field] = value
    rv(f"{comp} / {inv} / {name}: {field} changed (was: {old!r})", why, it["anchor"])


def add_also(pred, comps, why, label):
    touched = []
    for it in items:
        if pred(it):
            cur = it.setdefault("also_component", [])
            for c in comps:
                if c not in cur and c != it["component"]:
                    cur.append(c)
            touched.append(it["anchor"])
    assert touched, label
    rv(f"also_component {comps} added to {len(touched)} items: {label}", why, ", ".join(touched))


def find_trap(name):
    hit = [t for t in traps if t["name"] == name]
    assert len(hit) == 1, name
    return hit[0]


# 1. Attribution: library code that only runs inside a program may be credited to either.
LIB_NO_CALLER = {"VFS compaction monitors (per level)", "VFS snapshot monitor", "VFS L0 retention monitor"}
LIB_ALSO_VFS = {"ReplicaClient interface", "replica URL scheme registry", "remote LTX layout <path>/ltx/<level>/<min>-<max>.ltx"}
add_also(lambda it: it["component"] == LIB and it["name"] not in LIB_NO_CALLER and it["name"] not in LIB_ALSO_VFS,
         [CLI], "Owner-settled ambiguity: root-package items (control socket routes, Store/DB/Replica loops, heartbeat, local/remote "
         "LTX layout, restore/follow) run only inside `litestream replicate` / `restore`; a report may credit them to the library "
         "package or to cmd/litestream.", "root library items run by cmd/litestream")
add_also(lambda it: it["component"] == LIB and it["name"] in LIB_ALSO_VFS, [CLI, VFS],
         "Used by cmd/litestream and by the VFS extension (NewReplicaClientFromURL, ReplicaClient reads).",
         "root library items shared by cmd/litestream and cmd/litestream-vfs")
add_also(lambda it: it["component"] == VFS and it["anchor"].startswith("vfs.go:"), [LIB],
         "vfs.go is the root package compiled with build tag `vfs`; a report may credit these VFS items to the library package.",
         "cmd/litestream-vfs items anchored in root vfs.go")
BACKENDS = ["abs", "file", "gs", "nats", "oss", "s3", "sftp", "webdav"]
add_also(lambda it: it["component"] in BACKENDS and it["name"] != "lease object lock.json", [CLI, VFS],
         "Backend packages run only inside cmd/litestream (replicate/restore/ltx/register) and the VFS extension, which import all eight.",
         "replica backend package items")
add_also(lambda it: it["component"] == CLI and it["anchor"].startswith("server.go:"), [LIB],
         "socket.* keys are read by cmd/litestream but declared on litestream.SocketConfig in server.go.", "socket.* settings")

# 2. Tighten cmd/litestream commands to what changes behaviour a newcomer needs.
RULE_SETTING = ("Tightened must rule for settings: must = keys that choose what is replicated (dbs, path, dir), where it goes "
                "(replica, type, url), or switch on a server, worker, child process, outgoing call or startup action "
                "(addr, mcp-addr, socket.enabled, heartbeat-url, validation.interval, exec, watch, restore-if-db-not-exists) "
                "or switch off a default data behaviour (retention.enabled). Intervals, durations, local path overrides, "
                "companion keys and logging are tuning.")
for name in ["socket.path", "levels (levels[].interval)", "snapshot.interval", "snapshot.retention", "l0-retention",
             "heartbeat-interval", "logging.level", "dbs[].pattern", "dbs[].meta-path", "dbs[].monitor-interval",
             "replica.path", "sync-interval"]:
    set_must(CLI, "commands", name, False, RULE_SETTING)
RULE_FLAG = ("Tightened must rule for flags: must = the config source, the input/output a command cannot run without, and "
             "mode switches (exec, once, restore-if-db-not-exists, point-in-time selection, follow). Sub-options of a mode, "
             "guards, dry-runs, output formats, tuning and logging are must=false.")
for name in ["replicate -log-level", "replicate -force-snapshot", "replicate -enforce-retention", "ltx -level", "reset -dry-run",
             "restore -parallelism", "restore -if-db-not-exists", "restore -if-replica-exists", "restore -dry-run",
             "restore -force", "restore -follow-interval", "restore -integrity-check", "unregister -dry-run", "sync -wait"]:
    set_must(CLI, "commands", name, False, RULE_FLAG)
set_must(CLI, "commands", "LOG_LEVEL", False, "Logging knob; same rule as logging.level and replicate -log-level.")
set_field(CLI, "commands", "-config", "why",
          "Config path for replicate, databases, ltx, reset, restore and status; replicate/databases/ltx/restore/status default "
          "to LITESTREAM_CONFIG or /etc/litestream.yml, reset reads a config only when -config is given.",
          "reset.go never calls DefaultConfigPath(); only databases.go:29, ltx.go:49, restore.go:108, replicate.go:85, status.go:31 do.")

# Same rule applied to the other env-configured surfaces.
for name in ["LITESTREAM_LOG_LEVEL", "LITESTREAM_LOG_FILE", "LITESTREAM_SYNC_INTERVAL", "LITESTREAM_BUFFER_PATH", "LITESTREAM_HYDRATION_PATH"]:
    set_must(VFS, "commands", name, False,
             "Same tightened rule as cmd/litestream: logging, intervals and local path overrides are tuning; "
             "LITESTREAM_REPLICA_URL, LITESTREAM_WRITE_ENABLED and LITESTREAM_HYDRATION_ENABLED stay must.")
set_must(VFS, "data", "hydration meta file <hydration>.meta", False,
         "Exists only with LITESTREAM_HYDRATION_PATH (now must=false); the hydration file itself stays must.")
set_must("s3", "commands", "LITESTREAM_S3_DEBUG", False, "Debug logging knob; same rule as LOG_LEVEL.")

# 3. SQLite files are data by the owner's definition, not external.
RULE_SQLITE = ("Owner definition: data = files and DBs the program owns (the replicated SQLite DB and WAL ...); external = "
               "outgoing connections, launched programs, external services. An embedded SQLite open of a local file is data; "
               "the data inventory already carries it as must.")
set_must(LIB, "external", "replicated SQLite database (modernc.org/sqlite)", False, RULE_SQLITE + " (data item db.go:1036)")
set_must(LIB, "external", "restored database integrity check", False, RULE_SQLITE + " Also only with restore -integrity-check.")
for f in ["load.go", "populate.go", "shrink.go", "validate.go"]:
    set_must(TEST, "external", f"SQLite via mattn/go-sqlite3 ({f})", False,
             RULE_SQLITE + " (data items test_table_N, load_test)")

# 4. One outgoing connection, one must item.
RULE_SOCK = ("The seven CLI control commands share one outgoing connection (HTTP over the Unix control socket); one must item "
             "names them all, the per-command rows stay as must=false so a per-command claim still matches.")
set_field(CLI, "external", "POST /start to control socket (start)", "name",
          "control socket HTTP client (start, stop, sync, register, unregister, info, list)", RULE_SOCK)
set_field(CLI, "external", "control socket HTTP client (start, stop, sync, register, unregister, info, list)", "why",
          "start/stop/sync/register/unregister/info/list dial -socket (default /var/run/litestream.sock) and send HTTP requests to "
          "the daemon's control server (start.go:63, stop.go:63, sync.go:68, register.go:72, unregister.go:76, info.go:48, list.go:48).",
          RULE_SOCK)
set_field(CLI, "external", "control socket HTTP client (start, stop, sync, register, unregister, info, list)", "handler",
          "Start/Stop/Sync/Register/Unregister/Info/ListCommand.Run", RULE_SOCK)
for cmd, method in [("stop", "POST /stop"), ("sync", "POST /sync"), ("register", "POST /register"),
                    ("unregister", "POST /unregister"), ("info", "GET /info"), ("list", "GET /list")]:
    set_must(CLI, "external", f"{method} to control socket ({cmd})", False, RULE_SOCK)

# 5. Must promotions and why corrections found by spot checks.
set_must(LIB, "data", "restored database output (+ .tmp)", True,
         "The restored SQLite database is the primary artefact of `restore` (always written, unlike the must=true follow-mode "
         "-txid sidecar).")
S3_URL_WHY = ("Read only by s3.NewReplicaClientFromURL, i.e. for clients built from a URL through the scheme registry: control-socket "
              "POST /register (server.go:604) and the VFS extension (LITESTREAM_REPLICA_URL). restore/ltx with an s3:// URL go "
              "through cmd NewS3ReplicaClientFromConfig and the AWS SDK chain instead.")
for name, why in [("AWS_ACCESS_KEY_ID", "Access key for URL-built S3 clients (register, VFS)."),
                  ("LITESTREAM_ACCESS_KEY_ID", "Fallback access key for URL-built S3 clients (register, VFS)."),
                  ("AWS_SECRET_ACCESS_KEY", "Secret key for URL-built S3 clients (register, VFS)."),
                  ("LITESTREAM_SECRET_ACCESS_KEY", "Fallback secret key for URL-built S3 clients (register, VFS)."),
                  ("LITESTREAM_S3_ENDPOINT", "Default endpoint for URL-built S3 clients (register, VFS); forces path style.")]:
    set_field("s3", "commands", name, "why", why, S3_URL_WHY)
set_field(LIB, "workers", "DB monitor", "why",
          "Per database: every MonitorInterval (default 1s, db.go:32; ticker db.go:3053) syncs new WAL frames into L0 LTX files "
          "and checkpoints; exponential backoff on errors.",
          "The parenthetical pointed the default at the ticker line; the default is declared at db.go:32.")
set_field(LIB, "workers", "Replica monitor", "why",
          "Per database: every SyncInterval (default 1s, replica.go:26; ticker replica.go:383) or on WAL notify uploads pending "
          "L0 LTX files to the replica client.",
          "The parenthetical pointed the default at the ticker line; the default is declared at replica.go:26.")
set_field(VFS, "workers", "replica poller", "why",
          "Per opened database: every PollInterval (1s, ticker vfs.go:2472) polls the replica for new LTX files and updates the "
          "page index (also started for a new writable database at vfs.go:1145).",
          "Second start site vfs.go:1145 (openNewDatabase) was not mentioned.")

# 6. Gaps.
I("webdav", "data", "temp-file", "upload staging file litestream-webdav-*.ltx", "webdav/replica_client.go:257",
  "ReplicaClient.WriteLTXFile", False,
  "Each upload is staged in a temp file under os.TempDir to send a seekable body of known size; removed afterwards.",
  'os.CreateTemp("", "litestream-webdav-*.ltx")')
items[-1]["also_component"] = [CLI, VFS]
rv("added webdav / data / upload staging file litestream-webdav-*.ltx (must=false)",
   "File written by product code (os.CreateTemp) that the inventory did not list.", "webdav/replica_client.go:257")

# 7. Traps.
t = find_trap("GOOGLE_APPLICATION_CREDENTIALS check")
traps.remove(t)
rv("removed trap internal/testingutil / GOOGLE_APPLICATION_CREDENTIALS check",
   "Not a trap: the gs backend's storage.NewClient uses Application Default Credentials, which do read "
   "GOOGLE_APPLICATION_CREDENTIALS at runtime. Like other implicit SDK environment it is neither an item nor a trap.",
   t["anchor"])
t = find_trap("LITESTREAM_S3_* / LITESTREAM_*_ test env and -s3-* test flags")
t["reason"] = ("Integration-test configuration only (testingutil.go:52-132), EXCEPT LITESTREAM_S3_ENDPOINT, which product code "
               "also reads (s3/replica_client.go:232), and LITESTREAM_S3_DEBUG (s3/replica_client.go:1716). A claim naming those "
               "two as product env vars is correct.")
rv("trap reason narrowed: LITESTREAM_S3_* test env",
   "The wildcard also covered two env vars that product code reads (LITESTREAM_S3_ENDPOINT, LITESTREAM_S3_DEBUG).", t["anchor"])
t = find_trap("shadow WAL")
t["reason"] = ("A shadow-WAL file or directory (the v0.3 <meta>/generations/<gen>/wal/*.wal layout) does not exist in this "
               "version: v0.5 writes L0 LTX files. Code comments still call the L0 LTX files the 'shadow WAL'; describing L0 LTX "
               "files that way is acceptable, claiming a separate shadow WAL file is the trap.")
rv("trap reason clarified: shadow WAL", "Comments in db.go/replica.go use the term for L0 LTX files; only a separate file is wrong.",
   t["anchor"])
t = find_trap("debounceInterval")
t["reason"] = ("Internal constant of the directory monitor. The trap is listing it as a setting, flag or env var; the 250ms "
               "debounce itself is a real must=false worker item (directory_watcher.go:114).")
rv("trap reason clarified: debounceInterval", "The same debounce is a must=false worker item; only calling it configurable is wrong.",
   t["anchor"])
N(CLI, "`litestream mcp` subcommand", "Makefile:65",
  "The Makefile's mcp-wrap target runs `litestream mcp --debug`, but main.go has no `mcp` case (unknown command). The MCP server "
  "only runs inside `replicate` when mcp-addr is set.", 'fly mcp wrap --mcp="./dist/litestream"')
rv("added trap `litestream mcp` subcommand", "Developer Makefile invokes a subcommand the dispatcher (main.go:137-230) does not have.",
   "Makefile:65")
N(CLI, "`litestream snapshots` subcommand", "docs/PROVIDER_COMPATIBILITY.md:399",
  "Doc example of a v0.3 command; this version's dispatcher has no `snapshots` (unknown command). `ltx` lists backups.",
  "litestream snapshots s3://")
rv("added trap `litestream snapshots` subcommand", "Documented but not in main.go's switch.", "docs/PROVIDER_COMPATIBILITY.md:399")
N(CLI, "LITESTREAM_DEBUG env var", "docs/PROVIDER_COMPATIBILITY.md:383",
  "Documented debug switch that no product code reads; S3 SDK logging is LITESTREAM_S3_DEBUG and log level is LOG_LEVEL / logging.level.",
  "LITESTREAM_DEBUG=1 litestream replicate")
rv("added trap LITESTREAM_DEBUG", "No os.Getenv/LookupEnv of it anywhere outside docs.", "docs/PROVIDER_COMPATIBILITY.md:383")
N(CLI, "access-key-secret (OSS config key in docs)", "docs/PROVIDER_COMPATIBILITY.md:293",
  "Not a yaml tag; yaml.v2 Unmarshal (main.go:647) silently ignores it. The OSS secret key is secret-access-key.",
  "access-key-secret:")
rv("added trap access-key-secret", "Documented key with no yaml tag in ReplicaSettings (main.go:1110-1162).",
   "docs/PROVIDER_COMPATIBILITY.md:293")


# ---------------------------------------------------------------- errata (2026-09-29)
# Dated corrections after a skeptic pass. The inventory is corrected; the audit matcher is unchanged.
ERRATA_DATE = "2026-09-29"
errata = []


def erratum(it, change, reason, citation):
    errata.append(dict(date=ERRATA_DATE, item=f"{it['component']} / {it['inventory']} / {it['name']} ({it['anchor']})",
                       change=change, reason=reason, citation=citation))


def fix(comp, inv, name, reason, citation, must=None, flow=None):
    it = find(comp, inv, name)
    change = []
    if must is not None and it["must"] != must:
        change.append(f"must {it['must']} -> {must}")
        it["must"] = must
    if flow is not None:
        it["flow"] = flow
        change.append(f'"flow": {str(flow).lower()}')
    assert change, (comp, inv, name)
    erratum(it, "; ".join(change), reason, citation)


# 1. Foreground loops are the program's own flow, not workers.
fix(LIB, "workers", "restore follow loop",
    "The foreground loop of `litestream restore -f`: RestoreCommand.Run calls Replica.Restore synchronously, which "
    "returns r.follow(...) and blocks in the ticker loop until the signal context is cancelled. It belongs to the Main "
    "flow of cmd/litestream (also_component) rather than to workers.",
    "cmd/litestream/restore.go:145 r.Restore(ctx, opt); replica.go:651, :782 return r.follow(...); replica.go:817 "
    "ticker := time.NewTicker(interval)", must=False, flow=True)
fix(CLI, "workers", "Windows service control loop",
    "Consistency with the follow loop: when run as a Windows service, main -> runWindowsService blocks in svc.Run and "
    "Execute's SCM loop is the service's own foreground flow (it runs replicate, then waits for Stop).",
    "cmd/litestream/main.go:125 return runWindowsService(ctx); cmd/litestream/main_windows.go:44 svc.Run; :71 "
    "c.Run(s.ctx); :80 for {", flow=True)

# 2. Data: only a keyspace-like main store is must in-memory data; one remote layout, many variants.
RULE_MEM = ("The owner's must in-memory data is a keyspace-like main store that holds the program's data (Redis's "
            "keyspace qualifies); this is bookkeeping/metadata, and the data it points at (the replicated SQLite DB, "
            "local and remote LTX files) is already must.")
fix(LIB, "data", "Store.dbs / levels", RULE_MEM + " Store.dbs is the registry of managed *DB handles and compaction levels.",
    "store.go:89 dbs []*DB; store.go:90 levels CompactionLevels", must=False)
fix(VFS, "data", "page index (VFSFile.index)", RULE_MEM + " VFSFile.index maps page numbers to LTX file offsets on the replica.",
    "vfs.go:520 index map[uint32]ltx.PageIndexElem", must=False)
RULE_LAYOUT = ("One remote layout, counted once: the must item is the library's LTX layout (litestream.go:195 LTXFilePath, "
               "also_component cmd/litestream and cmd/litestream-vfs), whose why already names the s3/oss <%04x level> "
               "variant; the per-backend layouts are may variants of it (8 backend items; the skeptic's count said 7).")
for comp, name, cite in [
    ("abs", "blobs <path>/ltx/<level>/<min>-<max>.ltx", "abs/replica_client.go:222 key := litestream.LTXFilePath(...)"),
    ("file", "<path>/ltx/<level>/<min>-<max>.ltx", "file/replica_client.go:91 litestream.LTXFilePath(c.path, ...)"),
    ("gs", "objects <path>/ltx/<level>/<min>-<max>.ltx", "gs/replica_client.go:146 key := litestream.LTXFilePath(...)"),
    ("nats", "objects <path>/ltx/<level>/<min>-<max>.ltx", "nats/replica_client.go:244 return litestream.LTXFilePath(...)"),
    ("oss", "objects <path>/<%04x level>/<min>-<max>.ltx", "oss/replica_client.go:390 fmt.Sprintf(\"%04x/%s\", level, filename)"),
    ("s3", "objects <path>/<%04x level>/<min>-<max>.ltx", "s3/replica_client.go:703 fmt.Sprintf(\"%04x/%s\", level, filename)"),
    ("sftp", "remote files <path>/ltx/<level>/<min>-<max>.ltx", "sftp/replica_client.go:275 litestream.LTXFilePath(...)"),
    ("webdav", "remote files <path>/ltx/<level>/<min>-<max>.ltx", "webdav/replica_client.go:242 litestream.LTXFilePath(...)"),
]:
    fix(comp, "data", name, RULE_LAYOUT, cite + "; litestream.go:195 func LTXFilePath", must=False)

# 3. External: one system, one must item.
RULE_SWITCH = ("The replica-type switch only constructs the backend client; the system it reaches is already a must "
               "external item in the backend package ({dest}), carrying also_component cmd/litestream and "
               "cmd/litestream-vfs, so a cmd/litestream attribution is credited there.")
for typ, line, dest in [("file", 1359, "file / local/NFS directory, file/replica_client.go:157"),
                        ("s3", 1363, "s3 / AWS S3 / S3-compatible API, s3/replica_client.go:455"),
                        ("gs", 1367, "gs / Google Cloud Storage, gs/replica_client.go:86"),
                        ("abs", 1371, "abs / Azure Blob Storage, abs/replica_client.go:100"),
                        ("sftp", 1375, "sftp / SSH/SFTP server, sftp/replica_client.go:163"),
                        ("webdav", 1379, "webdav / WebDAV server (HTTP/HTTPS), webdav/replica_client.go:109"),
                        ("nats", 1383, "nats / NATS server connection, nats/replica_client.go:195"),
                        ("oss", 1387, "oss / Alibaba Cloud OSS, oss/replica_client.go:161")]:
    dest_comp, dest_name = dest.split(", ")[0].split(" / ", 1)
    d = find(dest_comp, "external", dest_name)
    assert d["must"] and CLI in d.get("also_component", []), dest
    fix(CLI, "external", f"{typ} replica client", RULE_SWITCH.format(dest=dest),
        f"cmd/litestream/main.go:1357 switch c.ReplicaType(); main.go:{line}; {dest.split(', ')[1]}", must=False)
d = find(VFS, "external", "replica client from LITESTREAM_REPLICA_URL")
for comp in BACKENDS:
    assert any(it["component"] == comp and it["inventory"] == "external" and it["must"]
               and VFS in it.get("also_component", []) for it in items), comp
fix(VFS, "external", "replica client from LITESTREAM_REPLICA_URL",
    "Consistency with the cmd/litestream switch items: the URL registry only constructs whichever backend client the "
    "scheme names; each backend system is already a must external item carrying also_component cmd/litestream-vfs.",
    "cmd/litestream-vfs/main.go:51 litestream.NewReplicaClientFromURL(replicaURL); main.go:29-36 backend imports; "
    "replica_url.go:31", must=False)


# ---------------------------------------------------------------- verify
def line_of(path, n):
    with open(os.path.join(REPO, path), encoding="utf-8", errors="replace") as fh:
        for i, l in enumerate(fh, 1):
            if i == n:
                return l
    return None


bad = []
for coll in (items, traps):
    for it in coll:
        path, n = it["anchor"].rsplit(":", 1)
        l = line_of(path, int(n))
        if l is None or it["_check"] not in l:
            bad.append((it["anchor"], it.get("name"), it["_check"], (l or "<missing>").rstrip()))
if bad:
    for b in bad:
        print("MISMATCH", *b, sep=" | ")
    sys.exit(1)

for coll in (items, traps):
    for it in coll:
        del it["_check"]

rev = subprocess.check_output(["git", "-C", REPO, "rev-parse", "HEAD"], text=True).strip()
assert rev == PINNED, f"{REPO} is at {rev}, the inventory is pinned to {PINNED}"
notes = [
    "Components: cmd/litestream (binary), cmd/litestream-vfs (Go c-archive + src/litestream-vfs.c built into the SQLite loadable extension litestream-vfs.so/.dylib; vfs.go in the root package is compiled in via build tag `vfs`), cmd/litestream-test (test harness binary), the root library package github.com/benbjohnson/litestream, and one component per replica backend package (abs, file, gs, nats, oss, s3, sftp, webdav). internal/ is a helper package with no items of its own; packages/python and packages/ruby only wrap the VFS extension; mock/, tests/, internal/testingutil and _examples are test/example code.",
    "Assignment rule: an item belongs to the package whose code holds the anchor. The control-socket routes (server.go), the Store/DB/Replica background loops, the heartbeat client and the local/remote LTX layout live in the library but only run inside `litestream replicate` (and `restore -f`). cmd/litestream carries must=false 'wiring' items pointing at them; a report attributing those library items to cmd/litestream is describing the same thing.",
    "VFS PRAGMAs (vfs.go FileControl) and SQL functions (src/litestream-vfs.c) are recorded as commands (kinds pragma / sql-function) of cmd/litestream-vfs: they are in-process calls from the host SQLite, not requests over a connection. VFS PRAGMA handlers live in vfs.go (library, build tag vfs) but are only reachable through the extension, so they are assigned to cmd/litestream-vfs.",
    "The control socket is disabled by default (server.go:27); start/stop/sync/register/unregister/info/list need socket.enabled. GET /txid and GET /debug/sync-status have no CLI caller.",
    "The metrics server uses http.ListenAndServe(addr, nil), so the blank net/http/pprof import also exposes /debug/pprof/* on `addr` (recorded as must=false).",
    "Remote key layouts differ: s3 and oss use <path>/<level as %04x>/<min>-<max>.ltx; file, gs, abs, sftp, webdav and nats use <path>/ltx/<level>/<min>-<max>.ltx (litestream.LTXFilePath). Level 9 holds snapshots; default compaction levels are L1 30s, L2 5m, L3 1h.",
    "No local shadow WAL, generation directories or position files exist in this version: the local state is <dir>/.<db>-litestream/ltx/<level>/*.ltx; the replicated position is derived from the newest L0 file; follow-mode restore keeps a <db>-txid sidecar. generations/ exists only as the v0.3.x remote layout read by RestoreV3.",
    "Parsed-but-ineffective config keys: validation-interval (replica level), tls (NATS), name (deprecated); age.* is rejected with an error. They are listed as traps, not settings.",
    "Settings must-flag judgment: keys that change what runs or where data goes are must; tuning knobs and backend credential/connection keys (access-key-id, bucket, host, jwt, ...) are must=false. Common repeated CLI flags (-json, -socket, -timeout, -no-expand-env) are must=false; all cmd/litestream-test flags are must=false.",
    "Implicit environment through SDK credential chains is not itemised because the repository never names it: AWS SDK default config (AWS_REGION, AWS_PROFILE, ...), GCS Application Default Credentials, Azure DefaultAzureCredential (AZURE_CLIENT_ID, ...), OSS NewEnvironmentVariableCredentialsProvider.",
    "The S3 Leaser (s3/leaser.go, lock.json) and VFS compaction monitors (VFS.CompactionEnabled) are library features with no caller in any program of this repository; recorded as must=false.",
    "restore-if-db-not-exists is implemented by ReplicateCommand.restoreIfNeeded (cmd), not by the library's DB.EnsureExists, which has no caller.",
    "replicate -once disables every background monitor (replicate.go:280-288) and runs runOnce instead.",
    "MCP tools shell out to a `litestream` binary found on PATH, not to the running executable.",
    "SFTP without host-key uses ssh.InsecureIgnoreHostKey (sftp/replica_client.go:132).",
]


def set_note(prefix, text, why, anchor):
    hit = [i for i, n in enumerate(notes) if n.startswith(prefix)]
    assert len(hit) == 1, prefix
    notes[hit[0]] = text
    rv(f"note rewritten: {prefix}...", why, anchor)


set_note("Components:",
         notes[0].replace("packages/python and packages/ruby only wrap", "packages/python, packages/ruby and packages/npm only wrap"),
         "packages/npm (index.js + per-platform packages) also exists and only wraps the extension.", "packages/npm/litestream-vfs/index.js:1")
set_note("Assignment rule:",
         "Assignment rule (settled): an item belongs to the package whose code holds the anchor. Root-package items that only run "
         "inside a program carry `also_component`, and the audit accepts either attribution: control-socket routes, Store/DB/Replica "
         "loops, heartbeat, restore/follow and local/remote LTX layout -> cmd/litestream; ReplicaClient, the URL registry and the "
         "remote layout -> also cmd/litestream-vfs; vfs.go-anchored VFS items -> also the library; backend package items -> "
         "cmd/litestream and cmd/litestream-vfs; socket.* settings (declared in server.go) -> also the library. cmd/litestream's "
         "must=false 'wiring' rows point at the same things.",
         "Ambiguity 1 settled per the owner's instruction; made machine-readable with also_component.", "server.go:75")
set_note("VFS PRAGMAs",
         notes[2] + " Settled: by the owner's definition requests are what arrives over a connection, so commands is correct; "
         "the vfs.go-anchored ones carry also_component = library.",
         "Ambiguity 2 settled against the requests definition.", "vfs.go:2344")
set_note("The metrics server uses",
         notes[4] + " Settled: must=false (debug surface), as are the pprof routes on the control socket. The Windows SCM "
         "stop/interrogate requests (main_windows.go:84) are OS service-control messages, not a connection; kept must=false in "
         "requests and a report omitting them is complete.",
         "Ambiguity 3 settled; also covers the windows-scm row.", "cmd/litestream/replicate.go:11")
set_note("Settings must-flag judgment:",
         "Must rule (tightened by review): subcommands are all must except help/wal. Flags: must = the config source (-config), "
         "the input/output a command cannot run without (replicate DB_PATH REPLICA_URL, register -replica, restore -o) and mode "
         "switches (replicate -exec/-once/-restore-if-db-not-exists, restore -txid/-timestamp/-f); sub-options of a mode, guards, "
         "dry-runs, output formats, tuning and logging are must=false. Settings: must = what is replicated (dbs, dbs[].path, "
         "dbs[].dir), where it goes (dbs[].replica, replica.type, replica.url), switches that start a server/worker/process/outgoing "
         "call/startup action (addr, mcp-addr, socket.enabled, heartbeat-url, validation.interval, exec, dbs[].watch, "
         "dbs[].restore-if-db-not-exists) and retention.enabled; intervals, durations, local path overrides, companion keys, logging "
         "and backend credential/connection keys are must=false. Env: product-named env vars are must except logging/debug knobs, "
         "intervals and local path overrides. cmd/litestream commands: 69 -> 42 must. All cmd/litestream-test flags are must=false.",
         "Ambiguity 5 / owner question: 27 settings and 23 flags were over-inclusive for orientation.", "cmd/litestream/main.go:270")
set_note("Implicit environment",
         notes[9] + " Settled: such variables (including GOOGLE_APPLICATION_CREDENTIALS, read by GCS ADC) are neither items nor "
         "traps; a report naming them is neither credited nor penalised.",
         "Ambiguity 6 settled; the GOOGLE_APPLICATION_CREDENTIALS trap was removed to match.", "gs/replica_client.go:86")
set_note("The S3 Leaser",
         notes[10] + " Settled: they stay must=false items (real library features), not traps; but a claim that `litestream "
         "replicate` takes an S3 lease or that the VFS extension compacts is false and should be judged as such.",
         "Ambiguity 7 settled.", "s3/leaser.go:23")
set_note("replicate -once disables",
         "replicate -once disables the compaction, snapshot, L0-retention, DB and replica monitors (replicate.go:280-288) and runs "
         "runOnce instead; the heartbeat and validation monitors still start in Store.Open if heartbeat-url / validation.interval "
         "are set (store.go:222-231).",
         "The original said 'every background monitor'; Store.Open starts heartbeat/validation monitors independently of "
         "CompactionMonitorEnabled.", "cmd/litestream/replicate.go:280")
notes.append("SQLite as data (settled): an embedded SQLite open of a local file (the replicated DB, a restored DB, the test "
             "harness's -db) is data by the owner's definition, not external. Those external rows are must=false; a report "
             "listing them under external is not wrong, only not required there.")
rv("note added: SQLite as data", "Ambiguity 4 (external vs data for SQLite opens) settled against the definitions.", "db.go:1036")
notes.append("Control socket clients (settled): the seven CLI control commands share one outgoing connection; one must row "
             "names them all, per-command rows are must=false.")
rv("note added: control socket clients", "Ambiguity (7 must rows for one connection) settled.", "cmd/litestream/start.go:63")

out = dict(repository="~/git/litestream-v24", revision=rev, items=items, **{"not": traps}, notes=notes, review=review,
           errata=errata)
with open(OUT, "w", encoding="utf-8") as fh:
    json.dump(out, fh, indent=2, ensure_ascii=False)
    fh.write("\n")

# summary
from collections import Counter
c = Counter()
for it in items:
    c[(it["component"], it["inventory"], it["must"])] += 1
comps = sorted({it["component"] for it in items})
invs = ["requests", "commands", "workers", "external", "data"]
for comp in comps:
    parts = [f"{inv} {c[(comp, inv, True)]}/{c[(comp, inv, False)]}" for inv in invs]
    tc = sum(1 for t in traps if t["component"] == comp)
    print(f"{comp}: " + ", ".join(parts) + f", traps {tc}")
print("other trap components:", Counter(t["component"] for t in traps if t["component"] not in comps))
print("total items", len(items), "traps", len(traps))
