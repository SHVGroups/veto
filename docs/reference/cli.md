# Command line

`vetod` is one binary: the server and its operator commands.

| Command                        | Description                                                                                                                                |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `vetod serve`                  | Run the server: agent API, console API, MCP proxy and, unless `VETO_WORKERS=false`, background jobs. The default when no command is given. |
| `vetod migrate`                | Apply database migrations and exit. Safe to run concurrently; a database lock serialises it.                                               |
| `vetod keygen`                 | Print a new master key.                                                                                                                    |
| `vetod vault rekey`            | Reseal every stored secret with the current master key. See [rotating the master key](../operations/security.md#rotating-the-master-key).  |
| `vetod ledger verify`          | Verify every workspace's ledger. Exits non-zero if any chain is broken.                                                                    |
| `vetod eval -file cases.jsonl` | Score question packs against labelled cases. See [Evaluations](../guides/evaluations.md).                                                  |
| `vetod healthcheck`            | Exit 0 if the server on `VETO_LISTEN_ADDR` answers `/healthz`. Used by the container health check.                                         |
| `vetod version`                | Print the version.                                                                                                                         |

`vetod eval` options:

| Option         | Default  |                                                                        |
| -------------- | -------- | ---------------------------------------------------------------------- |
| `-file`        | required | JSON Lines file of cases.                                              |
| `-packs`       | none     | Directory of pack JSON files that replace or add to the built-in ones. |
| `-order-check` | `true`   | Ask each disposition twice with the options reversed.                  |
| `-timeout`     | `10s`    | Time limit per case.                                                   |
| `-json`        | `false`  | Print the full report as JSON.                                         |

Commands other than `keygen`, `version`, `help` and `eval` read the same
environment as `serve`. `eval` needs only the decision model settings.

## Signals and shutdown

On `SIGINT` or `SIGTERM`, `serve` stops accepting connections, lets
in-flight requests, including long-poll waits, finish for up to 30
seconds, then stops background jobs and closes the database pool. Run more
than one replica so agents waiting on a stopping replica can retry on
another.
