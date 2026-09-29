#!/usr/bin/env python3
"""Generate and verify the freqtrade ground-truth inventory (read-only on the repo).

Run from anywhere: python3 testdata/audit/freqtrade/generate.py
It reads ~/git/freqtrade at the pinned revision only, refuses any other HEAD,
and rewrites inventory.json beside this script (internal/audit).
"""
import ast
import json
import pathlib
import subprocess
import sys

REPO = pathlib.Path.home() / "git" / "freqtrade"
PINNED = "9f10e357a93c1dcf10c2a2b367659214d89c073e"
OUT = pathlib.Path(__file__).resolve().parent / "inventory.json"

items = []
nots = []
errors = []


def line_of(path, lineno):
    lines = (REPO / path).read_text(encoding="utf-8").splitlines()
    return lines[lineno - 1]


def add(component, inventory, kind, name, path, lineno, chk, handler, must, why):
    text = line_of(path, lineno)
    if chk not in text:
        errors.append(f"{component}/{inventory}/{name}: {path}:{lineno} lacks {chk!r}: {text.strip()!r}")
    items.append(
        {
            "component": component,
            "inventory": inventory,
            "kind": kind,
            "name": name,
            "anchor": f"{path}:{lineno}",
            "handler": handler,
            "must": must,
            "why": why,
        }
    )


def trap(name, path, lineno, chk, reason, component="freqtrade"):
    text = line_of(path, lineno)
    if chk not in text:
        errors.append(f"not/{name}: {path}:{lineno} lacks {chk!r}: {text.strip()!r}")
    nots.append({"component": component, "name": name, "anchor": f"{path}:{lineno}", "reason": reason})


FT = "freqtrade"

# ---------------------------------------------------------------- freqtrade: commands
add(FT, "commands", "entrypoint", "freqtrade", "pyproject.toml", 143, 'freqtrade = "freqtrade.main:main"',
    "freqtrade.main.main", True, "Console script: parses argv with Arguments and calls the chosen subcommand's func.")
add(FT, "commands", "entrypoint", "python -m freqtrade", "freqtrade/__main__.py", 13, "main.main()",
    "freqtrade.main.main", False, "Module entry that forwards to freqtrade.main.main.")

A = "freqtrade/commands/arguments.py"
subcommands = [
    ("trade", 429, "freqtrade.commands.trade_commands.start_trading", True,
     "Runs the live or dry-run trading bot through Worker (the default Docker/systemd command)."),
    ("create-userdir", 436, "freqtrade.commands.deploy_commands.start_create_userdir", True,
     "Creates the user_data directory tree and copies sample files."),
    ("new-config", 444, "freqtrade.commands.build_config_commands.start_new_config", True,
     "Interactive questionnaire that writes a new config.json."),
    ("show-config", 452, "freqtrade.commands.build_config_commands.start_show_config", True,
     "Prints the resolved (merged) configuration, secrets hidden unless --show-sensitive."),
    ("new-strategy", 460, "freqtrade.commands.deploy_commands.start_new_strategy", True,
     "Renders a strategy file from templates into user_data/strategies."),
    ("download-data", 468, "freqtrade.commands.data_commands.start_download_data", True,
     "Downloads OHLCV (and optionally trades) history from the exchange into the data directory."),
    ("convert-data", 477, "partial(freqtrade.commands.data_commands.start_convert_data, ohlcv=True)", True,
     "Converts stored candle data between json/jsongz/feather/parquet formats."),
    ("convert-trade-data", 486, "partial(freqtrade.commands.data_commands.start_convert_data, ohlcv=False)", True,
     "Converts stored trades data between formats."),
    ("trades-to-ohlcv", 495, "freqtrade.commands.data_commands.start_convert_trades", True,
     "Builds OHLCV candles from downloaded trades data."),
    ("list-data", 504, "freqtrade.commands.data_commands.start_list_data", True,
     "Lists downloaded data (pairs/timeframes, or trades with --trades)."),
    ("backtesting", 513, "freqtrade.commands.optimize_commands.start_backtesting", True,
     "Backtests a strategy on stored data and exports results to user_data/backtest_results."),
    ("backtesting-show", 520, "freqtrade.commands.optimize_commands.start_backtesting_show", True,
     "Prints a previously stored backtest result."),
    ("backtesting-analysis", 529, "freqtrade.commands.analyze_commands.start_analysis_entries_exits", True,
     "Analyzes entry/exit tags and reasons of a stored backtest (signals export)."),
    ("edge", 536, "freqtrade.commands.optimize_commands.start_edge", False,
     "Still registered (help hidden) but its handler only raises ConfigurationError: Edge was removed."),
    ("hyperopt", 545, "freqtrade.commands.optimize_commands.start_hyperopt", True,
     "Optimizes strategy parameters over many backtest epochs in parallel workers."),
    ("hyperopt-list", 554, "freqtrade.commands.hyperopt_commands.start_hyperopt_list", True,
     "Lists epochs from a stored .fthypt hyperopt result file with filters."),
    ("hyperopt-show", 563, "freqtrade.commands.hyperopt_commands.start_hyperopt_show", True,
     "Shows details of one hyperopt epoch."),
    ("list-exchanges", 572, "freqtrade.commands.list_commands.start_list_exchanges", True,
     "Lists exchanges known to ccxt and their freqtrade support status."),
    ("list-markets", 581, "partial(freqtrade.commands.list_commands.start_list_markets, pairs_only=False)", True,
     "Lists markets on an exchange."),
    ("list-pairs", 590, "partial(freqtrade.commands.list_commands.start_list_markets, pairs_only=True)", True,
     "Lists tradable pairs on an exchange (same handler, pairs_only=True)."),
    ("list-strategies", 599, "freqtrade.commands.list_commands.start_list_strategies", True,
     "Lists strategy classes found in the strategies directory."),
    ("list-hyperoptloss", 608, "freqtrade.commands.list_commands.start_list_hyperopt_loss_functions", True,
     "Lists available hyperopt loss functions."),
    ("list-freqaimodels", 617, "freqtrade.commands.list_commands.start_list_freqAI_models", True,
     "Lists available FreqAI model classes."),
    ("list-timeframes", 626, "freqtrade.commands.list_commands.start_list_timeframes", True,
     "Lists timeframes supported by an exchange."),
    ("show-trades", 635, "freqtrade.commands.list_commands.start_show_trades", True,
     "Prints trades stored in the trades database."),
    ("test-pairlist", 644, "freqtrade.commands.pairlist_commands.start_test_pairlist", True,
     "Evaluates the configured pairlist handlers against the exchange and prints the result."),
    ("convert-db", 652, "freqtrade.commands.db_commands.start_convert_db", True,
     "Copies the trades database from --db-url-from to --db-url (e.g. SQLite to PostgreSQL)."),
    ("install-ui", 660, "freqtrade.commands.deploy_commands.start_install_ui", True,
     "Downloads the FreqUI web frontend from GitHub into rpc/api_server/ui/installed."),
    ("plot-dataframe", 668, "freqtrade.commands.plot_commands.start_plot_dataframe", True,
     "Writes plotly HTML charts of candles, indicators and trades to user_data/plot."),
    ("plot-profit", 677, "freqtrade.commands.plot_commands.start_plot_profit", True,
     "Writes a plotly HTML profit chart to user_data/plot."),
    ("webserver", 686, "freqtrade.commands.webserver_commands.start_webserver", True,
     "Runs the REST API/FreqUI server standalone (webserver mode: backtests, data download) without trading."),
    ("strategy-updater", 693, "freqtrade.commands.strategy_utils_commands.start_strategy_update", True,
     "Rewrites outdated strategy files to the current interface version."),
    ("lookahead-analysis", 702, "freqtrade.commands.optimize_commands.start_lookahead_analysis", True,
     "Checks a strategy for lookahead bias by comparing backtests."),
    ("recursive-analysis", 712, "freqtrade.commands.optimize_commands.start_recursive_analysis", True,
     "Checks a strategy for indicators that depend on startup candle count."),
]
for name, ln, handler, must, why in subcommands:
    add(FT, "commands", "subcommand", name, A, ln, f'"{name}"', handler, must, why)

C = "freqtrade/commands/cli_options.py"
options = [
    ("-c/--config", 83, '"--config"', True, "Config file(s), repeatable, '-' reads stdin; default user_data/config.json or ./config.json."),
    ("--userdir/--user-data-dir", 100, '"--userdir"', True, "Selects the user_data directory that holds data, strategies and results."),
    ("-s/--strategy", 118, '"--strategy"', True, "Strategy class name to load."),
    ("--strategy-path", 123, '"--strategy-path"', False, "Additional strategy lookup directory."),
    ("--db-url", 128, '"--db-url"', True, "Overrides the trades database URL (default sqlite tradesv3[.dryrun].sqlite)."),
    ("--dry-run", 145, '"--dry-run"', True, "Forces dry-run: removes exchange secrets and simulates orders."),
    ("--dry-run-wallet/--starting-balance", 150, '"--dry-run-wallet"', False, "Starting balance for dry-run/backtesting."),
    ("-v/--verbose", 57, '"--verbose"', False, "Increases log verbosity."),
    ("--logfile/--log-file", 62, '"--logfile"', False, "Log to a file (rotating), or deprecated syslog:/journald targets."),
    ("-d/--datadir/--data-dir", 93, '"--datadir"', False, "Directory with historical candle data."),
    ("--timerange", 162, '"--timerange"', True, "Time range for backtesting/hyperopt/download."),
    ("-i/--timeframe", 158, '"--timeframe"', False, "Timeframe override for optimize commands."),
    ("-p/--pairs", 464, '"--pairs"', False, "Limits a command to given pairs."),
    ("--export", 228, '"--export"', False, "Backtest export mode (none/trades/signals)."),
    ("--backtest-directory/--export-directory", 233, '"--backtest-directory"', False, "Directory for backtest results."),
    ("--strategy-list", 216, '"--strategy-list"', False, "Backtests several strategies in one run."),
    ("--breakdown", 267, '"--breakdown"', False, "Backtest result breakdown per day/week/month/year/weekday."),
    ("--cache", 273, '"--cache"', False, "Reuse a cached backtest result not older than the given age."),
    ("-e/--epochs", 286, '"--epochs"', False, "Number of hyperopt epochs."),
    ("--spaces", 300, '"--spaces"', False, "Hyperopt parameter spaces to optimize."),
    ("--hyperopt-loss", 366, '"--hyperopt-loss"', False, "Hyperopt loss function class."),
    ("-j/--job-workers", 342, '"--job-workers"', False, "Parallel hyperopt worker count."),
    ("--exchange", 549, '"--exchange"', False, "Exchange name for data/list commands without a config."),
    ("-t/--timeframes", 554, '"--timeframes"', False, "Timeframes to download/convert."),
    ("--days", 476, '"--days"', False, "Days of history to download."),
    ("--trading-mode", 442, '"--trading-mode"', False, "spot/margin/futures for data and list commands."),
    ("--sd-notify", 140, '"--sd-notify"', False, "Enables systemd notify/watchdog messages from Worker."),
    ("--freqaimodel", 804, '"--freqaimodel"', False, "FreqAI model class to use."),
    ("-V/--version", 70, '"--version"', False, "Prints version info and exits."),
]
for name, ln, chk, must, why in options:
    add(FT, "commands", "option", name, C, ln, chk, None, must, why)

S = "freqtrade/config_schema/config_schema.py"
settings = [
    ("exchange", 520, True, "Exchange section: name, API credentials, pair_whitelist/blacklist, ccxt options."),
    ("exchange.name", 938, False, "Exchange id passed to ccxt."),
    ("exchange.key", 945, False, "Exchange API key (recommended via FREQTRADE__EXCHANGE__KEY)."),
    ("exchange.secret", 952, False, "Exchange API secret (recommended via env)."),
    ("exchange.pair_whitelist", 996, False, "Static list of pairs to trade (used by StaticPairList)."),
    ("exchange.pair_blacklist", 1002, False, "Pairs never traded."),
    ("exchange.ccxt_config", 1035, False, "Extra options passed to both ccxt clients."),
    ("dry_run", 91, True, "Simulated trading when true; selects the dry-run database default."),
    ("stake_currency", 52, True, "Quote currency used for stakes."),
    ("stake_amount", 56, True, "Amount per trade or 'unlimited'."),
    ("max_open_trades", 37, True, "Maximum concurrent open trades."),
    ("timeframe", 42, True, "Candle timeframe the strategy runs on."),
    ("minimal_roi", 111, True, "ROI table that triggers exits (usually in the strategy)."),
    ("stoploss", 122, True, "Stoploss ratio (usually in the strategy)."),
    ("pairlists", 552, True, "Ordered pairlist handlers that build the active whitelist."),
    ("strategy", 167, True, "Strategy class name to load."),
    ("db_url", 805, True, "SQLAlchemy URL of the trades database."),
    ("telegram", 569, True, "Telegram bot: enabled, token, chat_id, notification settings."),
    ("api_server", 750, True, "REST API/FreqUI server: enabled, listen address/port, credentials, jwt/ws tokens."),
    ("webhook", 686, True, "Generic webhook notifications (enabled, url, format, retries); turns on the webhook POST sender."),
    ("discord", 701, True, "Discord webhook notifications (enabled, webhook_url); turns on the Discord sender."),
    ("dry_run_wallet", 95, False, "Simulated starting balance."),
    ("tradable_balance_ratio", 62, False, "Share of balance the bot may use."),
    ("fiat_display_currency", 86, False, "Fiat currency for RPC value display (CoinGecko conversion)."),
    ("entry_pricing", 351, False, "How entry prices are picked from orderbook/ticker."),
    ("exit_pricing", 396, False, "How exit prices are picked."),
    ("order_types", 433, False, "Order types for entry/exit/stoploss, stoploss_on_exchange."),
    ("unfilledtimeout", 323, False, "Timeouts after which unfilled orders are cancelled."),
    ("trading_mode", 198, False, "spot, margin or futures."),
    ("margin_mode", 203, False, "cross or isolated margin."),
    ("initial_state", 819, False, "Bot state at startup (running/paused/stopped)."),
    ("internals", 832, False, "process_throttle_secs, heartbeat and sd_notify for the Worker loop."),
    ("user_data_dir", 178, False, "user_data directory path."),
    ("datadir", 181, False, "Historical data directory."),
    ("external_message_consumer", 543, True, "Consumer mode: producers to subscribe to over websocket; turns on the consumer thread and producer websocket client."),
    ("freqai", 539, True, "FreqAI machine-learning section; enabled live FreqAI starts the background retraining thread."),
    ("freqaimodel", 528, False, "FreqAI model class name."),
    ("coingecko", 507, False, "CoinGecko API key/demo flag for fiat conversion and MarketCapPairList."),
    ("log_config", 524, False, "Python logging dictConfig override."),
    ("add_config_files", 882, False, "Additional config files merged into this one."),
    ("dataformat_ohlcv", 851, False, "Storage format of candle data."),
    ("position_adjustment_enable", 863, False, "Enables DCA/position adjustment."),
    ("bot_name", 319, False, "Bot name shown in RPC messages."),
    ("cancel_open_orders_on_exit", 102, False, "Cancel open orders when the bot stops."),
    ("force_entry_enable", 824, False, "Enables /forceenter via Telegram/API."),
    ("api_server.listen_port", 759, False, "API server port."),
    ("api_server.username", 765, False, "API basic-auth username."),
    ("api_server.password", 769, False, "API basic-auth password."),
    ("api_server.ws_token", 773, False, "Token(s) accepted on the message websocket."),
    ("api_server.jwt_secret_key", 778, False, "Secret used to sign JWT tokens."),
]
for name, ln, must, why in settings:
    leaf = name.split(".")[-1]
    add(FT, "commands", "setting", name, S, ln, f'"{leaf}":', None, must, why)

add(FT, "commands", "env", "FREQTRADE__{section}__{key}", "freqtrade/configuration/environment_vars.py", 82,
    "ENV_VAR_PREFIX", "freqtrade.configuration.environment_vars.environment_vars_to_dict", True,
    "Any FREQTRADE__-prefixed variable overrides the matching nested config key (merged over config files).")
add(FT, "commands", "env", "FREQTRADE__EXCHANGE__KEY", S, 947, "FREQTRADE__EXCHANGE__KEY", None, False,
    "Documented env for the exchange API key.")
add(FT, "commands", "env", "FREQTRADE__EXCHANGE__SECRET", S, 954, "FREQTRADE__EXCHANGE__SECRET", None, False,
    "Documented env for the exchange API secret.")
add(FT, "commands", "env", "FREQTRADE__TELEGRAM__CHAT_ID", S, 580, "FREQTRADE__TELEGRAM__CHAT_ID", None, False,
    "Documented env for the Telegram chat id (kept as string).")
add(FT, "commands", "env", "FREQTRADE__TELEGRAM__TOKEN", "docs/configuration.md", 44, "FREQTRADE__TELEGRAM__TOKEN",
    None, False, "Documented env for the Telegram bot token.")
add(FT, "commands", "env", "FREQTRADE__WEBHOOK__URL", S, 692, "FREQTRADE__WEBHOOK__URL", None, False,
    "Documented env for the webhook URL.")
add(FT, "commands", "env", "FREQTRADE__DISCORD__WEBHOOK_URL", S, 708, "FREQTRADE__DISCORD__WEBHOOK_URL", None,
    False, "Documented env for the Discord webhook URL.")
add(FT, "commands", "env", "FT_APP_ENV", "freqtrade/configuration/detect_environment.py", 8, "FT_APP_ENV",
    "freqtrade.configuration.detect_environment.running_in_docker", False,
    "'docker' (set by the Dockerfile) enables sudo chown of user_data and silences the non-loopback API warning.")

# ---------------------------------------------------------------- freqtrade: requests (REST/WS)
MOD = {
    "api_v1": "freqtrade.rpc.api_server.api_v1",
}
TRADE = "trade mode"
WEB = "webserver mode"
route_why = {
    ("POST", "/recursive_analysis"): "Starts a recursive-formula analysis as a background job (webserver mode).",
    ("GET", "/recursive_analysis/{jobid}"): "Returns status/result of a recursive-analysis job (webserver mode).",
    ("GET", "/lookahead_analysis/{jobid}"): "Returns status/result of a lookahead-analysis job (webserver mode).",
    ("POST", "/lookahead_analysis"): "Starts a lookahead-bias analysis as a background job (webserver mode).",
    ("POST", "/token/login"): "Exchanges HTTP basic credentials for JWT access/refresh tokens.",
    ("POST", "/token/refresh"): "Issues a new access token from a refresh token.",
    ("GET", "/background"): "Lists background jobs (webserver mode).",
    ("GET", "/background/{jobid}"): "Returns one background job's status (webserver mode).",
    ("DELETE", "/background/clear"): "Deletes all finished background jobs (webserver mode).",
    ("DELETE", "/background/{jobid}"): "Deletes one finished background job (webserver mode).",
    ("POST", "/backtest"): "Starts a backtest as a FastAPI background task (webserver mode).",
    ("GET", "/backtest"): "Returns running/finished backtest status and result (webserver mode).",
    ("DELETE", "/backtest"): "Resets backtest state and cached data (webserver mode).",
    ("GET", "/backtest/abort"): "Aborts the running backtest (webserver mode).",
    ("GET", "/backtest/history"): "Lists stored results in user_data/backtest_results (webserver mode).",
    ("GET", "/backtest/history/result"): "Loads one stored backtest result (webserver mode).",
    ("DELETE", "/backtest/history/{file}"): "Deletes a stored backtest result (webserver mode).",
    ("PATCH", "/backtest/history/{file}"): "Updates the notes of a stored backtest result (webserver mode).",
    ("GET", "/backtest/history/{file}/market_change"): "Returns market-change data stored with a backtest (webserver mode).",
    ("GET", "/backtest/history/{file}/{strategy}/wallet"): "Returns wallet history stored with a backtest (webserver mode).",
    ("POST", "/download_data"): "Starts a historical data download as a background job (webserver mode).",
    ("GET", "/pair_history"): "Loads stored candles with strategy indicators (webserver mode).",
    ("POST", "/pair_history"): "Same as GET /pair_history with a JSON body and column filter (webserver mode).",
    ("GET", "/pairlists/available"): "Lists available pairlist handlers (webserver mode).",
    ("POST", "/pairlists/evaluate"): "Evaluates a pairlist configuration as a background job (webserver mode).",
    ("GET", "/pairlists/evaluate/{jobid}"): "Returns a pairlist evaluation result (webserver mode).",
    ("GET", "/balance"): "Returns the account balance (trade mode).",
    ("GET", "/count"): "Returns open trade count and limits (trade mode).",
    ("GET", "/entries"): "Returns performance per entry tag (trade mode).",
    ("GET", "/exits"): "Returns performance per exit reason (trade mode).",
    ("GET", "/mix_tags"): "Returns performance per entry-tag/exit-reason mix (trade mode).",
    ("GET", "/performance"): "Returns per-pair performance (trade mode).",
    ("GET", "/profit"): "Returns the profit summary (trade mode).",
    ("GET", "/profit_all"): "Returns profit summaries for all, long and short trades (trade mode).",
    ("GET", "/stats"): "Returns exit-reason and duration statistics (trade mode).",
    ("GET", "/historic_balance"): "Returns recorded wallet history from wallet_history (trade mode).",
    ("GET", "/daily"): "Returns profit per day (trade mode).",
    ("GET", "/weekly"): "Returns profit per week (trade mode).",
    ("GET", "/monthly"): "Returns profit per month (trade mode).",
    ("GET", "/status"): "Returns open trades with current profit (trade mode).",
    ("GET", "/trades"): "Returns trade history, paginated (trade mode).",
    ("GET", "/trade/{tradeid}"): "Returns one trade (trade mode).",
    ("DELETE", "/trades/{tradeid}"): "Deletes a trade from the database, cancelling its open orders (trade mode).",
    ("DELETE", "/trades/{tradeid}/open-order"): "Cancels the trade's open order (trade mode).",
    ("POST", "/trades/{tradeid}/reload"): "Reloads a trade's orders from the exchange (trade mode).",
    ("GET", "/trades/open/custom-data"): "Lists custom data of open trades (trade mode).",
    ("GET", "/trades/{trade_id}/custom-data"): "Lists custom data of one trade (trade mode).",
    ("POST", "/forceenter"): "Forces a long/short entry for a pair (trade mode).",
    ("POST", "/forcebuy"): "Deprecated alias of /forceenter on the same handler.",
    ("POST", "/forceexit"): "Forces exit of a trade or all trades (trade mode).",
    ("POST", "/forcesell"): "Deprecated alias of /forceexit on the same handler.",
    ("GET", "/blacklist"): "Returns the pair blacklist (trade mode).",
    ("POST", "/blacklist"): "Adds pairs to the blacklist (trade mode).",
    ("DELETE", "/blacklist"): "Removes pairs from the blacklist (trade mode).",
    ("GET", "/whitelist"): "Returns the active pair whitelist (trade mode).",
    ("GET", "/locks"): "Returns pair locks (trade mode).",
    ("DELETE", "/locks/{lockid}"): "Deletes a pair lock by id (trade mode).",
    ("POST", "/locks/delete"): "Deletes pair locks by id or pair (trade mode).",
    ("POST", "/locks"): "Adds pair locks (trade mode).",
    ("POST", "/start"): "Sets the bot state to RUNNING (trade mode).",
    ("POST", "/stop"): "Sets the bot state to STOPPED (trade mode).",
    ("POST", "/pause"): "Sets the bot state to PAUSED: no new entries (trade mode).",
    ("POST", "/stopentry"): "Alias of /pause on the same handler.",
    ("POST", "/stopbuy"): "Deprecated alias of /pause on the same handler.",
    ("POST", "/reload_config"): "Requests a config reload; Worker rebuilds FreqtradeBot (trade mode).",
    ("GET", "/pair_candles"): "Returns live analyzed candles for a pair (trade mode).",
    ("POST", "/pair_candles"): "Same as GET /pair_candles with a JSON body and column filter (trade mode).",
    ("GET", "/ping"): "Unauthenticated liveness check.",
    ("HEAD", "/ping"): "HEAD variant of the unauthenticated ping.",
    ("GET", "/version"): "Returns the bot version.",
    ("GET", "/show_config"): "Returns the sanitized running configuration and bot state.",
    ("GET", "/logs"): "Returns recent log lines from the in-memory log buffer.",
    ("GET", "/plot_config"): "Returns the strategy's plot configuration.",
    ("GET", "/markets"): "Returns exchange markets (bot exchange or a webserver-mode exchange).",
    ("GET", "/strategy/{strategy}"): "Returns a strategy's source code and parameters.",
    ("GET", "/sysinfo"): "Returns CPU and RAM usage.",
    ("GET", "/health"): "Returns bot start and last loop timestamps.",
    ("GET", "/strategies"): "Lists strategies in the strategies directory (webserver mode).",
    ("GET", "/exchanges"): "Lists supported exchanges (webserver mode).",
    ("GET", "/hyperoptloss"): "Lists hyperopt loss functions (webserver mode).",
    ("GET", "/freqaimodels"): "Lists FreqAI models (webserver mode).",
    ("GET", "/available_pairs"): "Lists pairs/timeframes that have downloaded data (webserver mode).",
    ("WEBSOCKET", "/message/ws"): "Message websocket for FreqUI and consumer bots; token checked against ws_token or JWT.",
    ("GET", "/favicon.ico"): "Serves the UI favicon.",
    ("GET", "/fallback_file.html"): "Serves the page shown when FreqUI is not installed.",
    ("GET", "/ui_version"): "Returns the installed FreqUI version.",
    ("GET", "/{rest_of_path:path}"): "Serves installed FreqUI files, falling back to index.html (must stay last).",
}
not_must_routes = {
    ("HEAD", "/ping"), ("POST", "/forcebuy"), ("POST", "/forcesell"), ("POST", "/stopentry"),
    ("POST", "/stopbuy"), ("GET", "/favicon.ico"), ("GET", "/fallback_file.html"),
}
seen = set()
for f in sorted((REPO / "freqtrade/rpc/api_server").glob("*.py")):
    rel = str(f.relative_to(REPO))
    mod = rel[:-3].replace("/", ".")
    tree = ast.parse(f.read_text(encoding="utf-8"))
    for node in ast.walk(tree):
        if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            continue
        for d in node.decorator_list:
            if not (isinstance(d, ast.Call) and isinstance(d.func, ast.Attribute)
                    and isinstance(d.func.value, ast.Name) and d.func.value.id.startswith("router")):
                continue
            method = d.func.attr.upper()
            path = d.args[0].value
            key = (method, path)
            if key in seen:
                errors.append(f"duplicate route {key}")
            seen.add(key)
            prefix = "" if d.func.value.id == "router_ui" else "/api/v1"
            kind = "websocket" if method == "WEBSOCKET" else "http"
            name = f"{'WS' if method == 'WEBSOCKET' else method} {prefix}{path}"
            if key not in route_why:
                errors.append(f"no why for {key}")
                continue
            add(FT, "requests", kind, name, rel, d.args[0].lineno, f'"{path}"', f"{mod}.{node.name}",
                key not in not_must_routes, route_why[key])
missing = set(route_why) - seen
if missing:
    errors.append(f"route_why keys not found in code: {missing}")

add(FT, "requests", "http", "GET /docs", "freqtrade/rpc/api_server/webserver.py", 150, 'docs_url="/docs"', None,
    False, "FastAPI Swagger UI, only when api_server.enable_openapi is true.")
add(FT, "requests", "http", "GET /openapi.json", "freqtrade/rpc/api_server/webserver.py", 148, "FastAPI(", None,
    False, "FastAPI's default OpenAPI schema route (openapi_url is not disabled).")

W = "freqtrade/rpc/api_server/api_ws.py"
add(FT, "requests", "ws-message", "subscribe", W, 84, "RPCRequestType.SUBSCRIBE",
    "freqtrade.rpc.api_server.api_ws._process_consumer_request", True,
    "Sets which RPCMessageType topics the websocket channel receives; no reply.")
add(FT, "requests", "ws-message", "whitelist", W, 96, "RPCRequestType.WHITELIST",
    "freqtrade.rpc.api_server.api_ws._process_consumer_request", True,
    "Replies with the current whitelist (used by consumer bots).")
add(FT, "requests", "ws-message", "analyzed_df", W, 104, "RPCRequestType.ANALYZED_DF",
    "freqtrade.rpc.api_server.api_ws._process_consumer_request", True,
    "Replies with analyzed dataframes per pair (limit capped at 1500 candles).")

# ---------------------------------------------------------------- freqtrade: requests (Telegram)
T = "freqtrade/rpc/telegram.py"
tg = [
    ("status", 268, "_status", "Open trades (or a table with 'table')."),
    ("profit", 269, "_profit", "Profit summary."),
    ("balance", 270, "_balance", "Account balance."),
    ("start", 271, "_start", "Sets bot state to RUNNING."),
    ("stop", 272, "_stop", "Sets bot state to STOPPED."),
    ("forcesell, forceexit, fx", 273, "_force_exit", "Forces exit of a trade or all trades (inline keyboard if no id)."),
    ("forcebuy, forcelong", 275, "_force_enter (order_side=LONG)", "Forces a long entry for a pair."),
    ("forceshort", 279, "_force_enter (order_side=SHORT)", "Forces a short entry for a pair."),
    ("reload_trade", 281, "_reload_trade_from_exchange", "Reloads a trade's orders from the exchange."),
    ("trades", 282, "_trades", "Recent closed trades."),
    ("delete", 283, "_delete_trade", "Deletes a trade from the database."),
    ("coo, cancel_open_order", 284, "_cancel_open_order", "Cancels a trade's open order."),
    ("performance", 285, "_performance", "Per-pair performance."),
    ("buys, entries", 286, "_enter_tag_performance", "Performance per entry tag."),
    ("sells, exits", 287, "_exit_reason_performance", "Performance per exit reason."),
    ("mix_tags", 288, "_mix_tag_performance", "Performance per tag mix."),
    ("stats", 289, "_stats", "Exit reason and duration statistics."),
    ("daily", 290, "_daily", "Profit per day."),
    ("weekly", 291, "_weekly", "Profit per week."),
    ("monthly", 292, "_monthly", "Profit per month."),
    ("count", 293, "_count", "Open trade count."),
    ("locks", 294, "_locks", "Active pair locks."),
    ("unlock, delete_locks", 295, "_delete_locks", "Deletes pair locks."),
    ("reload_config, reload_conf", 296, "_reload_config", "Reloads the configuration."),
    ("show_config, show_conf", 297, "_show_config", "Shows the running configuration."),
    ("stopbuy, stopentry, pause", 298, "_pause", "Pauses new entries."),
    ("whitelist", 299, "_whitelist", "Shows the active whitelist."),
    ("blacklist", 300, "_blacklist", "Shows or extends the blacklist."),
    ("blacklist_delete, bl_delete", 301, "_blacklist_delete", "Removes pairs from the blacklist."),
    ("logs", 302, "_logs", "Recent log lines from the in-memory buffer."),
    ("health", 303, "_health", "Last loop timestamp."),
    ("help", 304, "_help", "Command help."),
    ("version", 305, "_version", "Bot and strategy version."),
    ("marketdir", 306, "_changemarketdir", "Sets the strategy market direction (long/short/even/none)."),
    ("order", 307, "_order", "Orders of a trade."),
    ("list_custom_data", 308, "_list_custom_data", "Custom data of a trade."),
    ("tg_info", 309, "_tg_info", "Intentionally unauthenticated: reports chat/topic ids for configuration."),
    ("profit_long", 310, "_profit_long", "Profit summary of long trades."),
    ("profit_short", 311, "_profit_short", "Profit summary of short trades."),
]
for name, ln, handler, why in tg:
    first = name.split(",")[0].strip()
    add(FT, "requests", "telegram-command", name, T, ln, f'"{first}"',
        f"freqtrade.rpc.telegram.Telegram.{handler}", True, why)

tgcb = [
    ("update_status_table", 314, "_status_table"),
    ("update_daily", 315, "_daily"),
    ("update_weekly", 316, "_weekly"),
    ("update_monthly", 317, "_monthly"),
    ("update_profit_long", 318, "_profit_long"),
    ("update_profit_short", 319, "_profit_short"),
    ("update_profit$", 320, "_profit"),
    ("update_balance", 321, "_balance"),
    ("update_performance", 322, "_performance"),
    ("update_enter_tag_performance", 324, "_enter_tag_performance"),
    ("update_exit_reason_performance", 327, "_exit_reason_performance"),
    ("update_mix_tag_performance", 329, "_mix_tag_performance"),
    ("update_count", 330, "_count"),
    ("force_exit__\\S+", 331, "_force_exit_inline"),
    ("force_enter__\\S+", 332, "_force_enter_inline"),
]
for name, ln, handler in tgcb:
    why = ("Inline-keyboard choice that completes /forceexit." if "force_exit" in name else
           "Inline-keyboard choice that completes /forcelong or /forceshort." if "force_enter" in name else
           "Refresh button callback that re-renders the same report.")
    chk = name.replace("\\S+", "").replace("$", "")
    add(FT, "requests", "telegram-callback", name, T, ln, chk, f"freqtrade.rpc.telegram.Telegram.{handler}",
        False, why)

# ---------------------------------------------------------------- freqtrade: workers
add(FT, "workers", "main-loop", "Worker.run", "freqtrade/worker.py", 78, "while True:",
    "freqtrade.worker.Worker._worker", True,
    "The `trade` process loop: handles state changes (RUNNING/PAUSED/STOPPED/RELOAD_CONFIG) and throttles each iteration.")
add(FT, "workers", "main-loop", "Worker._throttle", "freqtrade/worker.py", 186, "self._sleep(sleep_duration)",
    "freqtrade.worker.Worker._throttle", False,
    "Sleeps so an iteration lasts process_throttle_secs, aligned to the next candle.")
add(FT, "workers", "main-loop", "FreqtradeBot.process", "freqtrade/freqtradebot.py", 257, "def process(self)",
    "freqtrade.freqtradebot.FreqtradeBot.process", True,
    "One trading iteration: reload markets, refresh whitelist and candles, analyze, manage orders, exits, entries.")
add(FT, "workers", "scheduler", "FreqtradeBot._schedule (schedule.Scheduler)", "freqtrade/freqtradebot.py", 157,
    "Scheduler()", "schedule.Scheduler.run_pending", True,
    "Time-of-day jobs, executed from process() via run_pending (not a separate thread).")
add(FT, "workers", "scheduled-job", "futures funding/liquidation update", "freqtrade/freqtradebot.py", 172,
    "self._schedule.every().day.at(t).do(update)", "freqtrade.freqtradebot.FreqtradeBot.__init__.update", False,
    "In futures mode, at hh:01:02 and hh:31:02 updates funding fees, liquidation prices and wallets.")
add(FT, "workers", "scheduled-job", "ws_connection_reset", "freqtrade/freqtradebot.py", 174,
    "ws_connection_reset", "freqtrade.exchange.exchange.Exchange.ws_connection_reset", False,
    "Daily 00:02 reset of exchange websocket subscriptions.")
add(FT, "workers", "scheduled-job", "record_wallet_state", "freqtrade/freqtradebot.py", 175,
    "record_wallet_state", "freqtrade.wallets.Wallets.record_wallet_state", False,
    "Daily 00:07 snapshot of wallet value into the wallet_history table.")
add(FT, "workers", "thread", "FTTelegram", T, 164, 'name="FTTelegram"', "freqtrade.rpc.telegram.Telegram._init", True,
    "Telegram thread with its own asyncio loop: registers handlers and long-polls for updates.")
add(FT, "workers", "async-task", "Telegram updater polling", T, 364, "start_polling",
    "freqtrade.rpc.telegram.Telegram._startup_telegram", False,
    "python-telegram-bot long polling (drop_pending_updates) inside the FTTelegram loop.")
add(FT, "workers", "thread", "FTUvicorn", "freqtrade/rpc/api_server/uvicorn_threaded.py", 57, 'name="FTUvicorn"',
    "freqtrade.rpc.api_server.uvicorn_threaded.UvicornServer.run", True,
    "API server thread when api_server.enabled during trade; serves REST, websocket and FreqUI.")
add(FT, "workers", "server", "standalone uvicorn (webserver mode)", "freqtrade/rpc/api_server/webserver.py", 339,
    "self._server.run()", "freqtrade.rpc.api_server.uvicorn_threaded.UvicornServer.run", True,
    "`freqtrade webserver` runs uvicorn in the main thread instead of FTUvicorn.")
add(FT, "workers", "thread", "ExternalMessageConsumer thread", "freqtrade/rpc/external_message_consumer.py", 114,
    "Thread(target=self._loop.run_forever)", "freqtrade.rpc.external_message_consumer.ExternalMessageConsumer._main",
    True, "Consumer mode: event-loop thread holding one websocket task per configured producer.")
add(FT, "workers", "thread", "ccxt_ws", "freqtrade/exchange/exchange_ws.py", 34, 'name="ccxt_ws"',
    "freqtrade.exchange.exchange_ws.ExchangeWS._start_forever", True,
    "Exchange websocket thread (trade modes, enable_ws): continuously watches OHLCV candles via ccxt.pro.")
add(FT, "workers", "thread", "FreqAI training scan", "freqtrade/freqai/freqai_interface.py", 216,
    "threading.Thread(target=self._start_scanning", "freqtrade.freqai.freqai_interface.IFreqaiModel._start_scanning",
    True, "Live FreqAI: background thread that keeps retraining models per pair.")
add(FT, "workers", "background-job", "backtest job", "freqtrade/rpc/api_server/api_backtest.py", 196,
    "background_tasks.add_task(__run_backtest_bg", "freqtrade.rpc.api_server.api_backtest.__run_backtest_bg", True,
    "Webserver mode: backtest runs as a FastAPI background task tracked in ApiBG.jobs.")
add(FT, "workers", "background-job", "pairlist evaluation job", "freqtrade/rpc/api_server/api_pairlists.py", 98,
    "background_tasks.add_task(__run_pairlist", "freqtrade.rpc.api_server.api_pairlists.__run_pairlist", False,
    "Webserver mode: evaluates a pairlist config in the background.")
add(FT, "workers", "background-job", "download data job", "freqtrade/rpc/api_server/api_download_data.py", 83,
    "background_tasks.add_task(__run_download", "freqtrade.rpc.api_server.api_download_data.__run_download", False,
    "Webserver mode: downloads history in the background.")
add(FT, "workers", "background-job", "lookahead analysis job", "freqtrade/rpc/api_server/api_analysis.py", 228,
    "background_tasks.add_task(__run_lookahead_analysis_bg",
    "freqtrade.rpc.api_server.api_analysis.__run_lookahead_analysis_bg", False,
    "Webserver mode: lookahead analysis in the background.")
add(FT, "workers", "background-job", "recursive analysis job", "freqtrade/rpc/api_server/api_analysis.py", 141,
    "background_tasks.add_task(__run_recursive_analysis_bg",
    "freqtrade.rpc.api_server.api_analysis.__run_recursive_analysis_bg", False,
    "Webserver mode: recursive analysis in the background.")
add(FT, "workers", "async-task", "websocket channel reader/broadcaster", W, 125, "await channel.run_channel_tasks(",
    "freqtrade.rpc.api_server.api_ws.channel_reader / channel_broadcaster", False,
    "Per websocket connection: one task handles requests, one pushes MessageStream messages.")
add(FT, "workers", "process-pool", "hyperopt joblib Parallel", "freqtrade/optimize/hyperopt/hyperopt.py", 231,
    "with Parallel(n_jobs=config_jobs) as parallel:", "freqtrade.optimize.hyperopt.hyperopt_optimizer.HyperOptimizer.generate_optimizer_wrapped",
    True, "Hyperopt evaluates epochs in joblib worker processes (-j/--job-workers).")

# ---------------------------------------------------------------- freqtrade: external
add(FT, "external", "exchange", "ccxt (sync client)", "freqtrade/exchange/exchange.py", 274,
    "self._api = self._init_ccxt(exchange_conf, True, ccxt_config)", "freqtrade.exchange.exchange.Exchange._init_ccxt",
    True, "Synchronous ccxt client for orders, balances, tickers, markets.")
add(FT, "external", "exchange", "ccxt.pro / ccxt.async_support (async client)", "freqtrade/exchange/exchange.py", 283,
    "self._api_async = self._init_ccxt(exchange_conf, False, ccxt_async_config)",
    "freqtrade.exchange.exchange.Exchange._init_ccxt", False,
    "Async ccxt client used for candle and trades downloads.")
add(FT, "external", "exchange", "ccxt.pro watch_ohlcv websocket", "freqtrade/exchange/exchange_ws.py", 248,
    "watch_ohlcv(pair, timeframe)", "freqtrade.exchange.exchange_ws.ExchangeWS._continuously_async_watch_ohlcv",
    False, "Streams candles from the exchange websocket.")
add(FT, "external", "http", "data.binance.vision archive download", "freqtrade/exchange/binance_public_data.py", 121,
    "aiohttp.ClientSession", "freqtrade.exchange.binance_public_data._download_archive_ohlcv", False,
    "Binance history downloads fetch daily OHLCV/trades zip archives from data.binance.vision over aiohttp.")
add(FT, "external", "chat", "Telegram Bot API send_message", T, 2185, "await self._app.bot.send_message(",
    "freqtrade.rpc.telegram.Telegram._send_msg", True, "Sends notifications and command replies to the configured chat.")
add(FT, "external", "chat", "Telegram Bot API polling", T, 364, "start_polling",
    "freqtrade.rpc.telegram.Telegram._startup_telegram", False, "Long-polls Telegram for incoming commands.")
add(FT, "external", "http", "webhook POST", "freqtrade/rpc/webhook.py", 129, "post(self._url",
    "freqtrade.rpc.webhook.Webhook._send_msg", True, "POSTs RPC messages (form/json/raw) to webhook.url with retries.")
add(FT, "external", "http", "Discord webhook POST", "freqtrade/rpc/discord.py", 60, "self._send_msg(payload)",
    "freqtrade.rpc.webhook.Webhook._send_msg", True, "Posts embeds to discord.webhook_url through the Webhook sender.")
add(FT, "external", "database", "SQLAlchemy engine", "freqtrade/persistence/models.py", 77,
    "create_engine(db_url, future=True, **kwargs)", "freqtrade.persistence.models.init_db", True,
    "Connects to db_url (SQLite by default, any SQLAlchemy URL), creates and migrates tables.")
add(FT, "external", "http", "CoinGecko (fiat conversion)", "freqtrade/rpc/fiat_convert.py", 49,
    "FtCoinGeckoApi(", "freqtrade.rpc.fiat_convert.CryptoToFiatConverter", False,
    "pycoingecko client for fiat_display_currency conversion in RPC messages.")
add(FT, "external", "http", "CoinGecko (MarketCapPairList)", "freqtrade/plugins/pairlist/MarketCapPairList.py", 45,
    "FtCoinGeckoApi(", "freqtrade.plugins.pairlist.MarketCapPairList.MarketCapPairList", False,
    "Fetches market-cap rankings to build the whitelist.")
add(FT, "external", "http", "RemotePairList GET", "freqtrade/plugins/pairlist/RemotePairList.py", 169,
    "requests.get(self._pairlist_url", "freqtrade.plugins.pairlist.RemotePairList.RemotePairList.fetch_pairlist",
    False, "Fetches a pairlist from a configured URL (optional bearer token).")
add(FT, "external", "http", "GitHub API frequi releases", "freqtrade/commands/deploy_ui.py", 63,
    'requests.get(f"{base_url}releases"', "freqtrade.commands.deploy_ui.get_ui_download_url", False,
    "install-ui looks up FreqUI releases on api.github.com.")
add(FT, "external", "http", "FreqUI zip download", "freqtrade/commands/deploy_ui.py", 42, "requests.get(dl_url",
    "freqtrade.commands.deploy_ui.download_and_install_ui", False, "install-ui downloads and unpacks the release asset.")
add(FT, "external", "websocket", "producer websocket client", "freqtrade/rpc/external_message_consumer.py", 200,
    "websockets.connect(", "freqtrade.rpc.external_message_consumer.ExternalMessageConsumer._create_connection",
    True, "Consumer mode connects to other freqtrade bots' /api/v1/message/ws and subscribes to whitelist/analyzed_df.")
add(FT, "external", "systemd", "sd_notify", "freqtrade/worker.py", 62, "sdnotify.SystemdNotifier()",
    "freqtrade.worker.Worker._notify", False, "READY/WATCHDOG/RELOADING/STOPPING notifications to systemd.")
add(FT, "external", "process", "sudo chown", "freqtrade/configuration/directory_operations.py", 42,
    'subprocess.check_output(["sudo", "chown"', "freqtrade.configuration.directory_operations.chown_user_directory",
    False, "Only in Docker (FT_APP_ENV=docker): fixes user_data ownership.")
add(FT, "external", "logging", "syslog handler", "freqtrade/loggers/__init__.py", 139,
    "logging.handlers.SysLogHandler", "freqtrade.loggers._create_log_config", False,
    "--logfile syslog:... sends logs to syslog (deprecated CLI form).")
add(FT, "external", "logging", "journald handler", "freqtrade/loggers/__init__.py", 163,
    "cysystemd.journal.JournaldLogHandler", "freqtrade.loggers._create_log_config", False,
    "--logfile journald sends logs to journald (optional cysystemd).")
add(FT, "external", "process", "plotly auto_open browser", "freqtrade/plot/plotting.py", 624,
    "plot(fig, filename=str(_filename), auto_open=auto_open)", "freqtrade.plot.plotting.store_plot_file", False,
    "plot-profit --auto-open opens the generated HTML in a browser.")

# ---------------------------------------------------------------- freqtrade: data
add(FT, "data", "db-table", "trades (Trade)", "freqtrade/persistence/trade_model.py", 1712, '__tablename__ = "trades"',
    "freqtrade.persistence.trade_model.Trade", True, "One row per position with entry/exit, profit and stoploss state.")
add(FT, "data", "db-table", "orders (Order)", "freqtrade/persistence/trade_model.py", 77, '__tablename__ = "orders"',
    "freqtrade.persistence.trade_model.Order", True, "Exchange orders belonging to trades (mirrors ccxt orders).")
add(FT, "data", "db-table", "pairlocks (PairLock)", "freqtrade/persistence/pairlock.py", 16,
    '__tablename__ = "pairlocks"', "freqtrade.persistence.pairlock.PairLock", True,
    "Locks that block trading a pair until a time (protections, manual locks).")
add(FT, "data", "db-table", "KeyValueStore", "freqtrade/persistence/key_value_store.py", 36,
    '__tablename__ = "KeyValueStore"', "freqtrade.persistence.key_value_store._KeyValueStoreModel", True,
    "Bot-level key/value facts: bot_start_time, startup_time and migration markers (accessed through the KeyValueStore facade).")
add(FT, "data", "db-table", "trade_custom_data", "freqtrade/persistence/custom_data.py", 28,
    '__tablename__ = "trade_custom_data"', "freqtrade.persistence.custom_data._CustomData", True,
    "Strategy-set custom key/value data per trade (accessed through CustomDataWrapper).")
add(FT, "data", "db-table", "wallet_history", "freqtrade/persistence/wallet_history.py", 15,
    '__tablename__ = "wallet_history"', "freqtrade.persistence.wallet_history.WalletHistory", True,
    "Daily wallet snapshots behind /historic_balance.")
add(FT, "data", "db-file", "tradesv3.sqlite", "freqtrade/constants.py", 22, "tradesv3.sqlite", None, True,
    "Default live database (relative to the working directory).")
add(FT, "data", "db-file", "tradesv3.dryrun.sqlite", "freqtrade/constants.py", 23, "tradesv3.dryrun.sqlite", None,
    True, "Default dry-run database, chosen when dry_run is true and db_url is default.")
add(FT, "data", "db-migration", "check_migrate", "freqtrade/persistence/models.py", 99, "check_migrate(",
    "freqtrade.persistence.migrations.check_migrate", False, "Creates tables and migrates older schemas on startup.")
add(FT, "data", "directory", "user_data layout", "freqtrade/configuration/directory_operations.py", 57,
    "sub_dirs = [", "freqtrade.configuration.directory_operations.create_userdata_dir", True,
    "backtest_results, data, hyperopts, hyperopt_results, logs, notebooks, plot, strategies, freqaimodels.")
add(FT, "data", "file", "OHLCV data files", "freqtrade/data/history/datahandlers/idatahandler.py", 370,
    'f"{pair_s}-{timeframe}{candle}.{cls._get_file_extension()}"',
    "freqtrade.data.history.datahandlers.idatahandler.IDataHandler._pair_data_filename", True,
    "Candles stored as <datadir>/<exchange>/[futures/]<PAIR>-<tf>[-<candle>].<feather|json|jsongz|parquet>.")
add(FT, "data", "file", "trades data files", "freqtrade/data/history/datahandlers/idatahandler.py", 380,
    '-trades.', "freqtrade.data.history.datahandlers.idatahandler.IDataHandler._pair_trades_filename", False,
    "Downloaded trades stored as <PAIR>-trades.<ext>.")
add(FT, "data", "file", "pairs.json", "freqtrade/configuration/configuration.py", 546, 'pairs.json',
    "freqtrade.configuration.configuration.Configuration._resolve_pairs_list", False,
    "Fallback pair list file in the data directory for download-data.")
add(FT, "data", "file", "backtest result zip", "freqtrade/optimize/optimize_reports/bt_storage.py", 69,
    'zip_filename = _generate_filename(recordfilename, dtappendix, ".zip")',
    "freqtrade.optimize.optimize_reports.bt_storage.store_backtest_results", True,
    "backtest-result-<date>.zip in backtest_results with stats, config, strategy source and signals.")
add(FT, "data", "file", "backtest metadata json", "freqtrade/optimize/optimize_reports/bt_storage.py", 74,
    "get_backtest_metadata_filename(json_filename)", "freqtrade.optimize.optimize_reports.bt_storage.store_backtest_results",
    False, "Metadata stored next to the zip for listing results.")
add(FT, "data", "file", ".last_result.json", "freqtrade/optimize/optimize_reports/bt_storage.py", 77, "LAST_BT_RESULT_FN",
    "freqtrade.optimize.optimize_reports.bt_storage.store_backtest_results", False,
    "Pointer to the latest backtest (also used for hyperopt results).")
add(FT, "data", "file", "hyperopt results (.fthypt)", "freqtrade/optimize/hyperopt/hyperopt.py", 56, ".fthypt",
    "freqtrade.optimize.hyperopt.hyperopt.Hyperopt", True,
    "hyperopt_results/strategy_<name>_<date>.fthypt holds every epoch; read by hyperopt-list/show.")
add(FT, "data", "file", "hyperopt_tickerdata.pkl", "freqtrade/optimize/hyperopt/hyperopt.py", 59,
    "hyperopt_tickerdata.pkl", "freqtrade.optimize.hyperopt.hyperopt.Hyperopt", False,
    "Pickled preprocessed data shared with hyperopt workers.")
add(FT, "data", "file", "strategy parameter file (<strategy>.json)", "freqtrade/strategy/hyper.py", 106,
    'with_suffix(".json")', "freqtrade.strategy.hyper.HyperStrategyMixin.load_params_from_file", False,
    "Hyperopt exports best parameters next to the strategy file; the strategy loads them on start.")
add(FT, "data", "directory", "strategies", "freqtrade/resolvers/strategy_resolver.py", 33, "user_subdir = USERPATH_STRATEGIES",
    "freqtrade.resolvers.strategy_resolver.StrategyResolver", True,
    "User strategy classes are loaded from user_data/strategies (plus --strategy-path).")
add(FT, "data", "directory", "hyperopts", "freqtrade/resolvers/hyperopt_resolver.py", 26, "USERPATH_HYPEROPTS",
    "freqtrade.resolvers.hyperopt_resolver.HyperOptLossResolver", False, "Custom hyperopt loss classes.")
add(FT, "data", "directory", "freqaimodels", "freqtrade/resolvers/freqaimodel_resolver.py", 26, "USERPATH_FREQAIMODELS",
    "freqtrade.resolvers.freqaimodel_resolver.FreqaiModelResolver", False, "Custom FreqAI model classes.")
add(FT, "data", "directory", "FreqAI models dir", "freqtrade/freqai/data_kitchen.py", 962, '"models"',
    "freqtrade.freqai.data_kitchen.FreqaiDataKitchen", False, "Trained FreqAI models under user_data/models/<identifier>.")
add(FT, "data", "file", "plot HTML files", "freqtrade/plot/plotting.py", 673, '/ "plot"',
    "freqtrade.plot.plotting.load_and_plot_trades", False, "plot-dataframe writes one HTML per pair to user_data/plot.")
add(FT, "data", "file", "freqtrade-profit-plot.html", "freqtrade/plot/plotting.py", 716, "freqtrade-profit-plot.html",
    "freqtrade.plot.plotting.plot_profit", False, "plot-profit output file.")
add(FT, "data", "file", "log file (RotatingFileHandler)", "freqtrade/loggers/__init__.py", 177,
    "logging.handlers.RotatingFileHandler", "freqtrade.loggers._create_log_config", False,
    "--logfile path, 10 MB x 10 rotation (Docker compose uses user_data/logs/freqtrade.log).")
add(FT, "data", "file", "config.json", "freqtrade/constants.py", 16, 'DEFAULT_CONFIG = "config.json"', None, True,
    "Default config file name, looked up in user_data/ then the working directory.")
add(FT, "data", "file", "add_config_files", "freqtrade/configuration/load_config.py", 106, '"add_config_files"',
    "freqtrade.configuration.load_config.load_from_files", False,
    "Config files may include further files (relative, up to 5 levels).")
add(FT, "data", "file", "config from stdin", "freqtrade/configuration/load_config.py", 62, 'sys.stdin',
    "freqtrade.configuration.load_config.load_config_file", False, "-c - reads the config from stdin.")
add(FT, "data", "file", "new-config output", "freqtrade/configuration/deploy_config.py", 250,
    "config_path.write_text(config_text)", "freqtrade.configuration.deploy_config.deploy_new_config", False,
    "new-config renders base_config.json.j2 into the chosen config path.")
add(FT, "data", "file", "new-strategy output", "freqtrade/commands/deploy_commands.py", 80,
    "strategy_path.write_text(strategy_text)", "freqtrade.commands.deploy_commands.deploy_new_strategy", False,
    "new-strategy writes <Strategy>.py rendered from templates.")
add(FT, "data", "file", "sample user_data files", "freqtrade/constants.py", 147, "USER_DATA_FILES = {", None, False,
    "Templates copied by create-userdir: sample strategy, hyperopt loss, analysis notebook.")
add(FT, "data", "directory", "FreqUI installed files", "freqtrade/commands/deploy_commands.py", 117,
    "rpc/api_server/ui/installed/", "freqtrade.commands.deploy_commands.start_install_ui", False,
    "install-ui unpacks FreqUI inside the package; web_ui routes serve it.")
add(FT, "data", "file", "binance_leverage_tiers.json", "freqtrade/exchange/binance.py", 388,
    "binance_leverage_tiers.json", "freqtrade.exchange.binance.Binance.load_leverage_tiers", False,
    "Shipped Binance leverage-tier snapshot loaded instead of the API in dry-run futures.")
add(FT, "data", "file", "leverage_tiers_<stake>.json cache", "freqtrade/exchange/exchange.py", 3650,
    'f"leverage_tiers_{stake_currency}.json"', "freqtrade.exchange.exchange.Exchange.cache_leverage_tiers", False,
    "Futures mode caches fetched leverage tiers under <datadir>/futures/ and reloads them on start.")
add(FT, "data", "memory", "FreqtradeBot.state", "freqtrade/freqtradebot.py", 149, "self.state = State[",
    "freqtrade.freqtradebot.FreqtradeBot", True,
    "RUNNING/PAUSED/STOPPED/RELOAD_CONFIG, read by Worker and changed by Telegram/API.")
add(FT, "data", "memory", "active_pair_whitelist", "freqtrade/freqtradebot.py", 85, "self.active_pair_whitelist",
    "freqtrade.freqtradebot.FreqtradeBot._refresh_active_whitelist", False, "Current tradable pairs from pairlists.")
add(FT, "data", "memory", "DataProvider analyzed-dataframe cache", "freqtrade/data/dataprovider.py", 51,
    "self.__cached_pairs", "freqtrade.data.dataprovider.DataProvider", False,
    "Analyzed dataframes per pair/timeframe served to RPC, websocket and producers.")
add(FT, "data", "memory", "DataProvider _msg_queue", "freqtrade/data/dataprovider.py", 60, "self._msg_queue",
    "freqtrade.rpc.rpc_manager.RPCManager.process_msg_queue", False,
    "Queued analyzed-df messages flushed to RPC each loop.")
add(FT, "data", "memory", "Exchange._klines", "freqtrade/exchange/exchange.py", 246, "self._klines",
    "freqtrade.exchange.exchange.Exchange.refresh_latest_ohlcv", False, "In-memory candle cache per pair/timeframe.")
add(FT, "data", "memory", "Wallets._wallets", "freqtrade/wallets.py", 43, "self._wallets",
    "freqtrade.wallets.Wallets.update", False, "Cached balances (real or simulated in dry-run).")
add(FT, "data", "memory", "LocalTrade.bt_trades", "freqtrade/persistence/trade_model.py", 393, "bt_trades:",
    "freqtrade.persistence.trade_model.LocalTrade", False, "Backtesting keeps trades in memory instead of the database.")
add(FT, "data", "memory", "PairLocks.locks", "freqtrade/persistence/pairlock_middleware.py", 22, "locks:",
    "freqtrade.persistence.pairlock_middleware.PairLocks", False, "In-memory locks when use_db is off (backtesting).")
add(FT, "data", "memory", "ApiBG.jobs", "freqtrade/rpc/api_server/webserver_bgwork.py", 66, "jobs:",
    "freqtrade.rpc.api_server.webserver_bgwork.ApiBG", False, "Webserver-mode background job registry.")
add(FT, "data", "memory", "ApiBG.bt", "freqtrade/rpc/api_server/webserver_bgwork.py", 52, "bt: BtContainer",
    "freqtrade.rpc.api_server.webserver_bgwork.ApiBG", False, "Webserver-mode backtest instance and cached data.")
add(FT, "data", "memory", "ApiServer._message_stream", "freqtrade/rpc/api_server/webserver.py", 125,
    "_message_stream: MessageStream | None = None", "freqtrade.rpc.api_server.ws.message_stream.MessageStream", False,
    "In-memory pub/sub: ApiServer.send_msg publishes RPC messages that each websocket channel's broadcaster sends.")
add(FT, "data", "memory", "bufferHandler", "freqtrade/loggers/__init__.py", 23, "FTBufferingHandler(1000)",
    "freqtrade.rpc.rpc.RPC._rpc_get_logs", False, "Last 1000 log records, served by /logs (API and Telegram).")

# ---------------------------------------------------------------- freqtrade-client
CL = "freqtrade-client"
FC = "ft_client/freqtrade_client/ft_client.py"
RC = "ft_client/freqtrade_client/ft_rest_client.py"
add(CL, "commands", "entrypoint", "freqtrade-client", "ft_client/pyproject.toml", 44,
    'freqtrade-client = "freqtrade_client.ft_client:main"', "freqtrade_client.ft_client.main", True,
    "Console script of the separate freqtrade-client package (depends only on requests and python-rapidjson).")
add(CL, "commands", "option", "command (positional)", FC, 31, '"command"', "freqtrade_client.ft_client.main_exec",
    True, "The command slot: name of one of the 43 FtRestClient public methods, called via getattr with the remaining "
    "args (key=value become kwargs); each method calls one /api/v1 route of the bot. The individual method names "
    "are listed below as must=false client-command items.")
add(CL, "commands", "option", "command_arguments", FC, 53, '"command_arguments"', "freqtrade_client.ft_client.main_exec",
    False, "Positional args; 'k=v' items become keyword arguments.")
add(CL, "commands", "option", "--show", FC, 35, '"--show"', "freqtrade_client.ft_client.print_commands", True,
    "Lists available commands from FtRestClient docstrings (also command 'show'/'help').")
add(CL, "commands", "option", "-c/--config", FC, 44, '"--config"', "freqtrade_client.ft_client.load_config", True,
    "Bot config file (default config.json) from which api_server address and credentials are read.")
add(CL, "commands", "option", "-V/--version", FC, 33, '"--version"', None, False, "Prints the client version.")
for key, ln in [("api_server.listen_ip_address", 92), ("api_server.listen_port", 93),
                ("api_server.username", 94), ("api_server.password", 95)]:
    leaf = key.split(".")[1]
    add(CL, "commands", "setting", key, FC, ln, f'"{leaf}"', "freqtrade_client.ft_client.main_exec",
        key in ("api_server.listen_ip_address", "api_server.listen_port"),
        "Read from the bot config to build http://<ip>:<port> and basic auth.")
client_methods = [
    ("start", 78, "POST start"), ("stop", 85, "POST stop"), ("stopbuy", 92, "POST stopbuy"),
    ("reload_config", 99, "POST reload_config"), ("balance", 106, "GET balance"), ("count", 113, "GET count"),
    ("entries", 120, "GET entries"), ("exits", 128, "GET exits"), ("mix_tags", 136, "GET mix_tags"),
    ("locks", 144, "GET locks"), ("delete_lock", 151, "DELETE locks/{lock_id}"), ("lock_add", 159, "POST locks"),
    ("daily", 171, "GET daily"), ("weekly", 178, "GET weekly"), ("monthly", 185, "GET monthly"),
    ("profit", 192, "GET profit"), ("stats", 199, "GET stats"), ("performance", 206, "GET performance"),
    ("status", 213, "GET status"), ("version", 220, "GET version"), ("show_config", 227, "GET show_config"),
    ("ping", 233, "GET show_config (not /ping)"), ("logs", 243, "GET logs"), ("trades", 251, "GET trades"),
    ("list_open_trades_custom_data", 268, "GET trades/open/custom-data"),
    ("list_custom_data", 284, "GET trades/{trade_id}/custom-data"), ("trade", 298, "GET trade/{trade_id}"),
    ("delete_trade", 306, "DELETE trades/{trade_id}"), ("cancel_open_order", 315, "DELETE trades/{trade_id}/open-order"),
    ("whitelist", 323, "GET whitelist"), ("blacklist", 330, "GET blacklist or POST blacklist with args"),
    ("forcebuy", 341, "POST forcebuy (deprecated route)"), ("forceenter", 351, "POST forceenter"),
    ("forceexit", 395, "POST forceexit"), ("strategies", 413, "GET strategies"), ("strategy", 420, "GET strategy/{strategy}"),
    ("pairlists_available", 428, "GET pairlists/available"), ("plot_config", 435, "GET plot_config"),
    ("available_pairs", 442, "GET available_pairs"), ("pair_candles", 457, "GET pair_candles, or POST with columns"),
    ("pair_history", 479, "GET pair_history"), ("sysinfo", 500, "GET sysinfo"), ("health", 507, "GET health"),
]
for name, ln, route in client_methods:
    add(CL, "commands", "client-command", name, RC, ln, f"def {name}(", f"freqtrade_client.ft_rest_client.FtRestClient.{name}",
        False, f"CLI command mapped to {route} under /api/v1.")
add(CL, "external", "http", "freqtrade REST API", RC, 61, "self._session.request(", "freqtrade_client.ft_rest_client.FtRestClient._call",
    True, "requests.Session calls {serverurl}/api/v1/{apipath} with JSON body and optional basic auth.")
add(CL, "external", "http", "HTTP basic auth", RC, 44, "self._session.auth = (username, password)",
    "freqtrade_client.ft_rest_client.FtRestClient.__init__", False, "Uses api_server username/password, not JWT tokens.")
add(CL, "data", "file", "config.json (read)", FC, 67, "rapidjson.load(", "freqtrade_client.ft_client.load_config", True,
    "Reads the bot config file only for api_server settings; owns no state.")

# ---------------------------------------------------------------- scripts/rest_client.py
RS = "scripts/rest_client.py"
add(RS, "commands", "entrypoint", "scripts/rest_client.py", RS, 14, "main()", "freqtrade_client.ft_client.main", True,
    "Legacy script path that only calls freqtrade-client's main; same commands and HTTP calls.")

# ---------------------------------------------------------------- scripts/ws_client.py
WS = "scripts/ws_client.py"
add(WS, "commands", "entrypoint", "scripts/ws_client.py", WS, 313, "def main():", "scripts.ws_client.main", True,
    "Debug client for a bot's message websocket.")
add(WS, "commands", "option", "-c/--config", WS, 42, '"--config"', "scripts.ws_client.load_config", True,
    "Config file whose external_message_consumer.producers[0] is used.")
add(WS, "commands", "option", "-l/--logfile", WS, 51, '"--logfile"', "scripts.ws_client.setup_logging", False,
    "Log file (default ws_client.log).")
add(WS, "commands", "setting", "external_message_consumer.producers", WS, 291, '"producers"', "scripts.ws_client._main",
    True, "First producer's host, port, ws_token and secure flag select the websocket to connect to.")
add(WS, "commands", "setting", "external_message_consumer.wait_timeout/ping_timeout/sleep_time/message_size_limit", WS,
    294, '"wait_timeout"', "scripts.ws_client._main", False, "Connection timing and message size limits.")
add(WS, "workers", "async-loop", "create_client reconnect loop", WS, 221, "while 1:", "scripts.ws_client.create_client",
    True, "Reconnects forever, receiving messages and pinging on timeout.")
add(WS, "external", "websocket", "bot message websocket", WS, 226, "websockets.connect(websocket_url",
    "scripts.ws_client.create_client", True,
    "Connects to {ws|wss}://host:port/api/v1/message/ws?token=... and sends subscribe/whitelist/analyzed_df requests.")
add(WS, "data", "file", "ws_client.log", WS, 55, '"ws_client.log"', "scripts.ws_client.setup_logging", False,
    "Default debug log file.")

# ---------------------------------------------------------------- build_helpers
B1 = "build_helpers/binance_update_lev_tiers.py"
add(B1, "commands", "env", "FREQTRADE__EXCHANGE__KEY", B1, 8, "FREQTRADE__EXCHANGE__KEY", None, True,
    "Binance API key read directly from the environment.")
add(B1, "commands", "env", "FREQTRADE__EXCHANGE__SECRET", B1, 9, "FREQTRADE__EXCHANGE__SECRET", None, True,
    "Binance API secret.")
add(B1, "commands", "env", "CI_WEB_PROXY", B1, 11, "CI_WEB_PROXY", None, False, "Optional HTTPS proxy for ccxt.")
add(B1, "external", "exchange", "ccxt.binance fetch_leverage_tiers", B1, 23, "fetch_leverage_tiers()", None, True,
    "Fetches Binance futures leverage tiers.")
add(B1, "data", "file", "freqtrade/exchange/binance_leverage_tiers.json", B1, 26, "binance_leverage_tiers.json", None,
    True, "Overwrites the shipped leverage-tier snapshot.")

B2 = "build_helpers/create_command_partials.py"
add(B2, "commands", "entrypoint", "create_command_partials.py", B2, 109, "extract_command_partials()",
    "extract_command_partials", True, "Regenerates docs/commands/*.md from argparse help (Python 3.13+).")
add(B2, "external", "process", "freqtrade-client --show", B2, 97, '["freqtrade-client", "--show"]',
    "extract_command_partials", True, "Runs the installed client to capture its command list.")
add(B2, "data", "file", "docs/commands/*.md", B2, 90, 'docs/commands/{command}.md', "extract_command_partials", True,
    "Writes one help partial per subcommand (plus main.md and freqtrade-client.md).")

B3 = "build_helpers/extract_config_json_schema.py"
add(B3, "commands", "entrypoint", "extract_config_json_schema.py", B3, 30, "extract_config_json_schema()",
    "extract_config_json_schema", True, "Dumps CONF_SCHEMA to JSON.")
add(B3, "data", "file", "build_helpers/schema.json", B3, 24, '"schema.json"', "extract_config_json_schema", True,
    "Generated config JSON schema.")

B4 = "build_helpers/freqtrade_client_version_align.py"
add(B4, "commands", "entrypoint", "freqtrade_client_version_align.py", B4, 17, "main()", "main", True,
    "Exits 1 when freqtrade and freqtrade_client __version__ differ.")

B5 = "build_helpers/pre_commit_update.py"
add(B5, "commands", "option", "--update", B5, 17, '"--update"', None, True,
    "Rewrites .pre-commit-config.yaml type-stub pins to match requirements files.")
add(B5, "data", "file", ".pre-commit-config.yaml", B5, 11, ".pre-commit-config.yaml", None, True,
    "Checked (and with --update rewritten) against requirements-dev.txt and requirements.txt.")

# ---------------------------------------------------------------- setup.sh / setup.ps1
SH = "setup.sh"
for opt, ln, why, must in [
    ("--install|-i", 288, "Installs system packages, creates .venv, pip-installs freqtrade and FreqUI.", True),
    ("--config|-c", 291, "Only prints a hint to use `freqtrade new-config`.", False),
    ("--update|-u", 294, "git pull and reinstall dependencies.", True),
    ("--reset|-r", 297, "Hard-resets develop/stable to origin and recreates the venv.", True),
    ("--plot|-p", 300, "Installs plotly.", False),
]:
    add(SH, "commands", "option", opt, SH, ln, opt, None, must, why)
add(SH, "external", "process", "pip install", SH, 107, "${PIP} install --upgrade -r ${REQUIREMENTS}", "updateenv",
    True, "Installs requirement sets chosen interactively, then `pip install -e .`.")
add(SH, "external", "process", "freqtrade install-ui", SH, 119, "freqtrade install-ui", "updateenv", False,
    "Downloads FreqUI after installation.")
add(SH, "external", "process", "git pull", SH, 160, "git pull", "update", False, "Updates the checkout.")
add(SH, "external", "process", "git reset --hard origin/develop", SH, 222, "git reset --hard origin/develop", "reset",
    False, "Discards local changes on develop (stable at line 226).")
add(SH, "external", "process", "apt-get install", SH, 149, "sudo apt-get install", "install_debian", False,
    "Installs build dependencies on Debian/Ubuntu (yum on RedHat, brew on macOS).")
add(SH, "external", "http", "get-pip.py download", SH, 18, "curl https://bootstrap.pypa.io/get-pip.py",
    "check_installed_pip", False, "Downloads and runs get-pip.py when pip is missing (macOS also fetches the Homebrew installer, line 137).")
add(SH, "data", "directory", ".venv", SH, 57, ".venv/bin/activate", "updateenv", False, "Virtual environment the script creates and uses.")

PS = "setup.ps1"
add(PS, "commands", "entrypoint", "Main", PS, 290, "Main", "Main", True,
    "Windows installer: venv, git pull when clean, interactive requirement selection, pip install, FreqUI.")
add(PS, "external", "process", "git pull", PS, 242, '& "git" pull', "Main", False, "Updates a clean checkout.")
add(PS, "external", "process", "pip install", PS, 267, "& pip install @PipInstallArguments", "Main", True,
    "Installs selected requirement files, then `pip install -e .`.")
add(PS, "external", "process", "python freqtrade install-ui", PS, 279, "python freqtrade install-ui", "Main", False,
    "Downloads FreqUI.")
add(PS, "data", "file", "script_log_<timestamp>.txt", PS, 5, "script_log_$Timestamp.txt", None, False,
    "Installer log in the temp directory.")

# ---------------------------------------------------------------- traps
trap("backtest-filter", A, 282, '"backtest-filter"',
     "Listed in NO_CONF_REQURIED but no such subcommand is registered.")
trap("start_list_trades_data", "freqtrade/commands/data_commands.py", 182, "def start_list_trades_data",
     "Exported from freqtrade.commands but not a subcommand: list-data --trades calls it (data_commands.py:124).")
trap("start_conversion", "freqtrade/commands/strategy_utils_commands.py", 46, "def start_conversion",
     "Helper of strategy-updater, not a subcommand.")
trap("HYPEROPT_LOSS_BUILTIN", "freqtrade/constants.py", 33, "HYPEROPT_LOSS_BUILTIN",
     "Loss class names are option values of --hyperopt-loss, not commands.")
trap("HYPEROPT_BUILTIN_SPACES", "freqtrade/constants.py", 47, "HYPEROPT_BUILTIN_SPACES",
     "Values of --spaces, not commands or settings.")
trap("EXPORT_OPTIONS", "freqtrade/constants.py", 21, "EXPORT_OPTIONS", "Values of --export.")
trap("AVAILABLE_PAIRLISTS", "freqtrade/constants.py", 60, "AVAILABLE_PAIRLISTS",
     "Pairlist handler names are values of pairlists[].method, not top-level settings.")
trap("TELEGRAM_SETTING_OPTIONS", "freqtrade/constants.py", 125, "TELEGRAM_SETTING_OPTIONS",
     "on/off/silent notification loudness values, not Telegram commands.")
trap("Telegram keyboard valid_keys", T, 181, "valid_keys",
     "Regexes validating custom keyboard buttons (e.g. '/status table'); not command registrations.")
trap("RPCMessageType", "freqtrade/enums/rpcmessagetype.py", 4, "class RPCMessageType",
     "Outgoing notification/websocket topics; only subscribe/whitelist/analyzed_df (RPCRequestType) are received.")
trap("_OPENAPI_TAGS", "freqtrade/rpc/api_server/webserver.py", 28, "_OPENAPI_TAGS",
     "OpenAPI tag descriptions, not routes.")
trap("API_VERSION changelog", "freqtrade/rpc/api_server/api_v1.py", 72, "/backtest/history/wallets",
     "Comment names '/backtest/history/wallets'; the real route is /backtest/history/{file}/{strategy}/wallet.")
trap("download_data handler named pairlists_evaluate", "freqtrade/rpc/api_server/api_download_data.py", 52,
     "def pairlists_evaluate(", "POST /download_data's handler reuses the name pairlists_evaluate; it does not evaluate pairlists.")
trap("Exchange.loop", "freqtrade/exchange/exchange.py", 205, "self.loop = self._init_async_loop()",
     "An asyncio loop driven synchronously with run_until_complete by the caller; not a separate worker.")
trap("PYTEST_VERSION", "freqtrade/loggers/__init__.py", 211, "PYTEST_VERSION",
     "Test-only guard in setup_logging, not a user environment variable.")
trap("git log subprocess (freqtrade)", "freqtrade/__init__.py", 16, "subprocess.check_output(",
     "Runs only for 'dev' versions; __version__ is \"2026.8\" at this revision.")
trap("git log subprocess (freqtrade_client)", "ft_client/freqtrade_client/__init__.py", 17, "subprocess.check_output(",
     "Same dev-only branch; inactive for version 2026.8.", component="freqtrade-client")
trap("COLUMNS / NO_COLOR", "build_helpers/create_command_partials.py", 22, 'os.environ["COLUMNS"]',
     "Set (not read) to format argparse help; not configuration.", component="build_helpers/create_command_partials.py")
trap("tests/ (e.g. get_patched_freqtradebot)", "tests/conftest.py", 303, "def get_patched_freqtradebot",
     "Test-only fixtures and mocks; not part of any component.")
trap("ft_client/test_client", "ft_client/test_client/test_rest_client.py", 24, "def test_FtRestClient_init",
     "Test-only code of the client package.", component="freqtrade-client")
trap("freqtrade/templates/sample_strategy.py", "freqtrade/templates/sample_strategy.py", 1, "",
     "Template copied into user_data/strategies by create-userdir; not the bot's own strategy.")
trap("FtRestClient.ping", RC, 235, "configstatus = self.show_config()",
     "Client 'ping' calls /show_config, not the /ping route.", component="freqtrade-client")
trap("edge subcommand help", A, 537, '# help="Edge module. No longer part of Freqtrade"',
     "Edge is registered only to raise ConfigurationError; not a working mode.")
trap("docker-compose command", "docker-compose.yml", 32, "trade",
     "Deployment invocation of `freqtrade trade`, not a separate program.")
trap("freqtrade.service", "freqtrade.service", 9, "ExecStart=/usr/bin/freqtrade trade",
     "systemd unit launching `freqtrade trade`, not a separate program.")
trap("freqtrade/vendor/qtpylib", "freqtrade/vendor/qtpylib/indicators.py", 1, "",
     "Vendored indicator library, not a component.")

# ---------------------------------------------------------------- errata (2026-09-29)
# Dated corrections after a skeptic pass. The inventory is corrected; the audit matcher is unchanged.
ERRATA_DATE = "2026-09-29"
errata = []


def erratum(item, change, reason, citation):
    errata.append({"date": ERRATA_DATE, "item": item, "change": change, "reason": reason, "citation": citation})


def find_item(component, inventory, name):
    hit = [it for it in items if it["component"] == component and it["inventory"] == inventory and it["name"] == name]
    assert len(hit) == 1, (component, inventory, name, len(hit))
    return hit[0]


def label(it):
    return f"{it['component']} / {it['inventory']} / {it['name']} ({it['anchor']})"


def set_must(component, inventory, name, must, reason, citation, flow=None):
    it = find_item(component, inventory, name)
    change = []
    if it["must"] != must:
        change.append(f"must {it['must']} -> {must}")
        it["must"] = must
    if flow is not None:
        it["flow"] = flow
        change.append(f'"flow": {str(flow).lower()}')
    assert change, (component, inventory, name)
    erratum(label(it), "; ".join(change), reason, citation)


# 1. Background work that is how a request/command does its job, not a way in.
set_must(FT, "workers", "backtest job", False,
         "The job is the body of POST /api/v1/backtest (a must request): the route handler registers it as a FastAPI "
         "BackgroundTask after answering; it is not a separate way into the program.",
         "freqtrade/rpc/api_server/api_backtest.py:155 @router.post(\"/backtest\") -> :196 "
         "background_tasks.add_task(__run_backtest_bg ...)")
set_must(FT, "workers", "hyperopt joblib Parallel", False,
         "Not a worker: Hyperopt.start blocks in `with Parallel(...)` and waits for every batch of epochs "
         "(run_optimizer_parallel) before continuing, so the pool runs in place as how the `hyperopt` subcommand (a must "
         "command) does its work. Kept as a may item instead of dropped because joblib does spawn worker processes; "
         "reverses the earlier review promotion.",
         "freqtrade/optimize/hyperopt/hyperopt.py:231 with Parallel(n_jobs=config_jobs); :136 run_optimizer_parallel; "
         ":264 f_val = self.run_optimizer_parallel(...)")

# 2. The program's own foreground flow belongs to the Main flow, not to workers.
FLOW_TRADE = ("The `trade` subcommand's own foreground loop (start_trading -> Worker(args).run()); it is the program's "
              "Main flow, not something that runs on its own beside it.")
set_must(FT, "workers", "Worker.run", False, FLOW_TRADE,
         "freqtrade/commands/trade_commands.py:24-25 worker = Worker(args); worker.run(); freqtrade/worker.py:78 while True:",
         flow=True)
set_must(FT, "workers", "FreqtradeBot.process", False,
         "One iteration of the same foreground loop: Worker._process_running calls it on every throttled pass; part of "
         "the Main flow of `trade`, not a worker.",
         "freqtrade/worker.py:197-199 _process_running -> self.freqtrade.process(); freqtrade/freqtradebot.py:257",
         flow=True)
it = find_item(FT, "workers", "Worker._throttle")
it["flow"] = True
erratum(label(it), '"flow": true (must stays false)',
        "Consistency with Worker.run: the throttle sleep is inside the same foreground loop.",
        "freqtrade/worker.py:116, :124 self._throttle(...) inside Worker._worker; :186 self._sleep(sleep_duration)")
set_must(FT, "workers", "standalone uvicorn (webserver mode)", False,
         "Not a worker: in `freqtrade webserver` ApiServer runs uvicorn in the foreground main thread (standalone=True), "
         "so it is that subcommand's Main flow; the requests it serves are inventoried separately and the "
         "background-thread variant (FTUvicorn) stays a must worker. Reverses the earlier review promotion.",
         "freqtrade/commands/webserver_commands.py:16 ApiServer(config, standalone=True); "
         "freqtrade/rpc/api_server/webserver.py:338-339 if self._standalone: self._server.run()",
         flow=True)
set_must("scripts/ws_client.py", "workers", "create_client reconnect loop", False,
         "Consistency with the flow marks: the reconnect loop is the script's own foreground (main -> asyncio.run(_main) "
         "-> await create_client), the same shape as litestream `restore -f`; not a worker beside the flow.",
         "scripts/ws_client.py:316 asyncio.run(_main(args)); :299 await create_client(; :221 while 1:",
         flow=True)

# 3. Anchor on the use a newcomer lands on, not on a construction.
set_must(FT, "workers", "FreqtradeBot._schedule (schedule.Scheduler)", False,
         "Scheduler() only constructs the empty holder; what runs are the jobs registered on it (now must) and run "
         "inline from process() by run_pending. The holder stays a may item.",
         "freqtrade/freqtradebot.py:157 self._schedule = Scheduler(); :308 self._schedule.run_pending()")
set_must(FT, "workers", "futures funding/liquidation update", True,
         "A scheduled job is the time-triggered way in a newcomer lands on (futures mode registers it for hh:01:02 and "
         "hh:31:02); config-gated like the other must workers (ccxt_ws, FreqAI).",
         "freqtrade/freqtradebot.py:159 if self.trading_mode == TradingMode.FUTURES; :172 "
         "self._schedule.every().day.at(t).do(update)")
set_must(FT, "workers", "ws_connection_reset", True,
         "Scheduled job registered unconditionally (daily 00:02).",
         "freqtrade/freqtradebot.py:174 self._schedule.every().day.at(\"00:02\").do(self.exchange.ws_connection_reset)")
set_must(FT, "workers", "record_wallet_state", True,
         "Scheduled job registered unconditionally (daily 00:07); it writes the wallet_history table.",
         "freqtrade/freqtradebot.py:175 self._schedule.every().day.at(\"00:07\").do(self.wallets.record_wallet_state)")

it = find_item(FT, "external", "ccxt (sync client)")
it["use_site"] = "freqtrade/exchange/exchange.py:1481"
it["why"] += (" Use site a newcomer lands on: Exchange.create_order -> self._api.create_order "
              "(freqtrade/exchange/exchange.py:1481), recorded as a may item.")
add(FT, "external", "exchange", "ccxt order placement (Exchange.create_order)", "freqtrade/exchange/exchange.py", 1481,
    "order = self._api.create_order(", "freqtrade.exchange.exchange.Exchange.create_order", False,
    "Live (non-dry-run) orders go to the exchange through the sync ccxt client built by _init_ccxt.")
erratum(label(it), 'kept must at the construction; added "use_site" and a may item at the use site',
        "_init_ccxt is where the exchange connection is configured (exchange name, keys, ccxt_config), so it stays; "
        "the call a newcomer following an entry lands on is the order placement, added as a may item so a report "
        "anchored there is credited without widening the matcher.",
        "freqtrade/exchange/exchange.py:274 self._api = self._init_ccxt(...); :1464 if self._config[\"dry_run\"]; "
        ":1481 order = self._api.create_order(")

it = find_item(FT, "external", "Telegram Bot API send_message")
it["why"] = ("Send path for every notification and command reply: RPCManager.send_msg -> Telegram.send_msg -> "
             "run_coroutine_threadsafe(self._send_msg(...)) -> self._app.bot.send_message to the configured chat.")
erratum(label(it), "anchor kept at telegram.py:2185; why rewritten to name the send path",
        "Of the Telegram call sites (bot.send_message at :2185 and its NetworkError retry :2199, /tg_info's "
        "context.bot.send_message :2281, callback edit_message_text at :1458-1503 and :2136), :2185 is the one on the "
        "send path that notifications (send_msg) and all command replies (self._send_msg) go through.",
        "freqtrade/rpc/telegram.py:617-629 send_msg -> run_coroutine_threadsafe(self._send_msg(...)); :2147 "
        "async def _send_msg; :2185 await self._app.bot.send_message(")

# 4. Data: only a keyspace-like main store qualifies as must in-memory data.
set_must(FT, "data", "FreqtradeBot.state", False,
         "A run-state flag (RUNNING/PAUSED/STOPPED/RELOAD_CONFIG), not the owner's keyspace-like main store; the bot's "
         "data lives in the trades DB, which stays must.",
         "freqtrade/freqtradebot.py:149 self.state = State[initial_state.upper()] if initial_state else State.STOPPED")

# 5. A flow item says "the Main flow runs through X": a claim about function X, and a Main flow step is anchored
# at its function's declaration (internal/audit flowSteps). At the loop or the call inside X no step could match it.
FLOW_ANCHOR = ("A flow item's claim is that the Main flow runs through a function, and the audit anchors a Main flow "
               "step at the declaration of the function it names (internal/audit flowSteps: the step subject's "
               "declaration). Anchored at a loop or a call site no step can match it (0/8 flow items found across "
               "the three inventories), so the item is anchored at its function's declaration; the line it was "
               "anchored at stays cited.")


def reanchor(component, inventory, name, path, lineno, chk, citation):
    it = find_item(component, inventory, name)
    assert it.get("flow") is True, label(it)
    text = line_of(path, lineno)
    assert chk in text, (path, lineno, chk, text)
    erratum(label(it), f"anchor {it['anchor']} -> {path}:{lineno}", FLOW_ANCHOR, citation)
    it["anchor"] = f"{path}:{lineno}"


reanchor(FT, "workers", "Worker.run", "freqtrade/worker.py", 76, "def run(self) -> None:",
         "freqtrade/worker.py:76 def run(self) -> None:, the declaration; :78 while True:, the loop it was anchored at")
reanchor(FT, "workers", "Worker._throttle", "freqtrade/worker.py", 145, "def _throttle(",
         "freqtrade/worker.py:145 def _throttle(, the declaration; :186 self._sleep(sleep_duration), the call it was "
         "anchored at")
reanchor("scripts/ws_client.py", "workers", "create_client reconnect loop", "scripts/ws_client.py", 197,
         "async def create_client(",
         "scripts/ws_client.py:197 async def create_client(, the declaration; :221 while 1:, the loop it was anchored at")
reanchor(FT, "workers", "standalone uvicorn (webserver mode)", "freqtrade/rpc/api_server/uvicorn_threaded.py", 36,
         "def run(self, sockets=None):",
         "freqtrade/rpc/api_server/uvicorn_threaded.py:36 def run(self, sockets=None):, the declaration of "
         "UvicornServer.run, the item's handler, whose :54 loop.run_until_complete(self.serve(...)) serves in the "
         "foreground; freqtrade/rpc/api_server/webserver.py:339 self._server.run() in ApiServer.start_api, the call "
         "it was anchored at")
# FreqtradeBot.process is already anchored at its declaration: no erratum.
it = find_item(FT, "workers", "FreqtradeBot.process")
assert it["anchor"] == "freqtrade/freqtradebot.py:257" and line_of("freqtrade/freqtradebot.py", 257).strip() == \
    "def process(self) -> None:", it

revision =subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=REPO, text=True).strip()
assert revision == PINNED, f"{REPO} is at {revision}, the inventory is pinned to {PINNED}"

notes = [
    "Components: freqtrade (bot program, console script freqtrade = freqtrade.main:main, all subcommands); "
    "freqtrade-client (separate package in ft_client/, console script freqtrade-client); scripts/rest_client.py "
    "(thin wrapper around freqtrade_client.ft_client.main, same inventory as freqtrade-client); scripts/ws_client.py; "
    "five build_helpers scripts; setup.sh and setup.ps1 installers.",
    "REST routes were extracted with an AST pass over freqtrade/rpc/api_server/*.py (87 decorators incl. the websocket "
    "and 4 UI routes) and anchored on the line holding the path literal; names carry the /api/v1 prefix added in "
    "webserver.py configure_app (UI router has no prefix).",
    "Route availability: api_trading routes need trade mode (is_trading_mode); api_webserver, backtest, background, "
    "pair_history, pairlists, download_data, lookahead/recursive routes need webserver mode (`freqtrade webserver`); "
    "api_v1 routes work in both; /ping and /token/* use no JWT/basic dependency; the websocket checks ws_token or JWT.",
    "Telegram commands anchor on their CommandHandler line (telegram.py:268-311); a registration with aliases is one "
    "item named by its comma-separated strings (39 registrations, 51 command strings). All command handlers except "
    "/tg_info are wrapped by authorized_only (chat_id, topic_id, authorized_users); the force_exit__/force_enter__ "
    "inline callbacks (_force_exit_inline/_force_enter_inline) are not decorated.",
    "Workers: the trade process is single-threaded Worker.run + FreqtradeBot.process with schedule jobs run inline; "
    "other threads are FTTelegram, FTUvicorn, ccxt_ws, the ExternalMessageConsumer loop and FreqAI retraining, each "
    "enabled by config. Discord and webhook POST synchronously in whichever thread calls RPCManager.send_msg (no queue/thread).",
    "Settings list only top-level or clearly documented schema keys from config_schema.py. Must rule: the core "
    "trading parameters a trade config cannot run without (exchange, dry_run, stake_currency, stake_amount, "
    "max_open_trades, timeframe, minimal_roi, stoploss, pairlists, strategy) plus the sections that switch on a "
    "must worker, external connection or data location (db_url, telegram, api_server, webhook, discord, "
    "external_message_consumer, freqai). Tuning sections stay must=false even when SCHEMA_TRADE_REQUIRED "
    "(config_schema.py:1494) requires them: entry_pricing and exit_pricing (required, no schema default), "
    "tradable_balance_ratio, dry_run_wallet, internals and dataformat_* (required but schema-defaulted). "
    "internals.heartbeat_interval is read (worker.py:59) but absent from the schema.",
    "The repository's own user_data/ contains only backtest_results, data, freqaimodels, hyperopts, logs, notebooks "
    "(.gitkeep); strategies, plot and hyperopt_results are created by create_userdata_dir at runtime.",
    "Ambiguities: 'edge' is a registered subcommand that only errors (must=false); /stopentry is a non-deprecated alias "
    "of /pause but marked must=false with the other aliases; FreqUI catch-all route is must=true because it is how the "
    "web UI is served; freqtrade-client commands are its FtRestClient public methods (dynamic getattr dispatch): "
    "the single must item is the positional `command` slot, the 43 method names are must=false client-command "
    "items (same convention as redis-cli's cmdTable words). All six SQLAlchemy tables of the trades DB are must.",
]

if errors:
    print("VERIFICATION ERRORS:")
    for e in errors:
        print(" -", e)
    sys.exit(1)

review = [
    {"what": "freqtrade-client: 43 client-command items (FtRestClient methods) must=true -> must=false; the "
             "`command (positional)` option stays the one must item and its why now names the slot",
     "why": "Owner convention from redis-cli: individual client command words are may, one must item covers the "
            "command slot; each method only forwards to a /api/v1 route already inventoried as a must request",
     "anchor": "ft_client/freqtrade_client/ft_client.py:31 (slot); ft_client/freqtrade_client/ft_rest_client.py:78-507 (methods)"},
    {"what": "settings webhook, discord, external_message_consumer, freqai must=false -> must=true",
     "why": "Each switches on a must worker/external (webhook and Discord POST senders, consumer thread + producer "
            "websocket, FreqAI retraining thread), the same reason telegram and api_server were already must",
     "anchor": "freqtrade/config_schema/config_schema.py:686, :701, :543, :539"},
    {"what": "note on settings: replaced 'SCHEMA_TRADE_REQUIRED drove the must marks' with the actual rule",
     "why": "The old note was false: entry_pricing, exit_pricing, tradable_balance_ratio, dry_run_wallet, internals "
            "and dataformat_* are in SCHEMA_TRADE_REQUIRED but were must=false; strategy/db_url/telegram/api_server "
            "are must but not in it",
     "anchor": "freqtrade/config_schema/config_schema.py:1494"},
    {"what": "workers 'standalone uvicorn (webserver mode)' must=false -> must=true",
     "why": "It is the main loop of `freqtrade webserver` (runs in the main thread), the counterpart of Worker.run for trade",
     "anchor": "freqtrade/rpc/api_server/webserver.py:339"},
    {"what": "workers 'FreqAI training scan' thread must=false -> must=true",
     "why": "A real threading.Thread; every other config-gated thread (FTTelegram, FTUvicorn, ccxt_ws, "
            "ExternalMessageConsumer) is must",
     "anchor": "freqtrade/freqai/freqai_interface.py:216"},
    {"what": "workers 'hyperopt joblib Parallel' must=false -> must=true",
     "why": "The only process pool; it is how the hyperopt subcommand (a must command) executes epochs",
     "anchor": "freqtrade/optimize/hyperopt/hyperopt.py:231"},
    {"what": "data tables KeyValueStore, trade_custom_data, wallet_history must=false -> must=true",
     "why": "Owner definition names 'the trades DB and its models'; these complete the six ModelBase tables "
            "created by init_db (wallet_history backs /historic_balance and the daily record_wallet_state job)",
     "anchor": "freqtrade/persistence/key_value_store.py:36; freqtrade/persistence/custom_data.py:28; "
               "freqtrade/persistence/wallet_history.py:15"},
    {"what": "handler of KeyValueStore table KeyValueStore -> _KeyValueStoreModel; of trade_custom_data "
             "CustomDataWrapper -> _CustomData",
     "why": "The handler of a db-table item is the ORM model class holding __tablename__; the named classes are facades",
     "anchor": "freqtrade/persistence/key_value_store.py:31; freqtrade/persistence/custom_data.py:18"},
    {"what": "data 'strategies' directory anchor strategy_resolver.py:269 -> :33",
     "why": "Line 269 is inside the optional recursive_strategy_search branch; line 33 is the resolver's "
            "user_subdir = USERPATH_STRATEGIES, the same anchor form as the hyperopts/freqaimodels siblings",
     "anchor": "freqtrade/resolvers/strategy_resolver.py:33"},
    {"what": "added data may item 'leverage_tiers_<stake>.json cache'",
     "why": "File the bot writes under <datadir>/futures/ and reloads; was missing from the data inventory",
     "anchor": "freqtrade/exchange/exchange.py:3650"},
    {"what": "added data may item 'ApiServer._message_stream'",
     "why": "In-memory pub/sub between RPC send_msg and websocket channel broadcasters; was missing",
     "anchor": "freqtrade/rpc/api_server/webserver.py:125"},
    {"what": "added setup.sh external may item 'get-pip.py download'",
     "why": "Outgoing curl download in check_installed_pip; was missing (Homebrew installer fetch noted in why)",
     "anchor": "setup.sh:18"},
]

doc = {"repository": "~/git/freqtrade", "revision": revision, "notes": notes, "items": items, "not": nots,
       "review": review, "errata": errata}
OUT.write_text(json.dumps(doc, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

from collections import Counter
cnt = Counter()
for it in items:
    cnt[(it["component"], it["inventory"], "must" if it["must"] else "may")] += 1
tc = Counter(n["component"] for n in nots)
comps = []
for it in items:
    if it["component"] not in comps:
        comps.append(it["component"])
for c in comps:
    parts = []
    for inv in ["requests", "commands", "workers", "external", "data"]:
        m, y = cnt[(c, inv, "must")], cnt[(c, inv, "may")]
        if m or y:
            parts.append(f"{inv} {m}/{y}")
    print(f"{c}: {', '.join(parts)}; traps {tc.get(c, 0)}")
print("total items", len(items), "traps", len(nots))
