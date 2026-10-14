# Scaling

vetod keeps no state between requests that another replica would need:
actions, decisions, grants and sessions live in PostgreSQL, and events
reach every replica through `LISTEN`/`NOTIFY`. Add replicas behind a load
balancer and they share the work without any further setup.

## What one replica does

Most of a decision's time is the call to the decision model. Everything
else (policy evaluation, building the safer version, writing the action,
its decision and its ledger entries) takes a few milliseconds and a handful
of queries. A replica therefore spends most of its time waiting, not
computing, and its limits are:

- **Database connections.** Each replica keeps a pool of 10, one of them
  held by the event listener. Requests that write an action hold a
  connection for the length of a transaction, not for the model call.
- **Waiting requests.** Agents that pass `?wait=` hold an HTTP request open
  until their action is decided. Waiting costs a goroutine and a
  subscription, not a database connection, so a replica can hold many
  waiting requests at once; make sure your load balancer allows that many
  open connections and idle timeouts above `VETO_MAX_WAIT`.
- **The model's latency and rate limits** at TypeSafe. Check
  `veto_model_request_seconds` and your TypeSafe quota before adding
  replicas for throughput.

## Adding replicas

1. Raise the replica count (`server.replicas` in the Helm chart). The
   chart's PodDisruptionBudget keeps at least one server up during drains.
2. Check that PostgreSQL's `max_connections` covers every replica's pool
   and your other clients. With the default pool, each vetod replica needs
   10; the console needs none.
3. Raise per-replica limits if you need to. `VETO_RATE_LIMIT_RPS` and
   `VETO_RATE_LIMIT_BURST` apply per API key **per replica**, so an agent
   spread by your load balancer across N replicas gets up to N times the
   configured rate. Set the values with your replica count in mind.

No sticky sessions are needed. A console session, an action's long poll,
the console's live event stream and an MCP session can each land on any
replica: MCP session state lives in the upstream MCP server, which Veto
proxies to as-is.

## Background jobs

Every replica runs the background jobs unless `VETO_WORKERS=false`. They
coordinate through the database:

| Job                                                             | Every  | Coordination                                                     |
| --------------------------------------------------------------- | ------ | ---------------------------------------------------------------- |
| Expire held actions past their deadline                         | 5 s    | one replica at a time (advisory lock)                            |
| Send the `action.pending_review` reminder halfway to a deadline | 30 s   | one replica at a time                                            |
| Deliver webhooks                                                | 2 s    | all replicas, each leasing up to 50 due deliveries for 2 minutes |
| Sign ledger checkpoints                                         | 15 min | one replica at a time                                            |
| Apply retention and remove expired sessions                     | 1 h    | one replica at a time                                            |
| Verify every ledger                                             | 24 h   | one replica at a time                                            |

A replica that cannot take a job's lock skips that run and counts it as
`skipped` in `veto_worker_runs_total`; this is normal.

To keep request latency independent of job load, you can split roles:
run API replicas with `VETO_WORKERS=false` and one or two worker replicas
with `VETO_WORKERS=true` that receive no traffic. Both use the same image
and configuration otherwise. Keep at least two worker replicas so jobs keep
running during a deploy. The Helm chart does this with
`worker.enabled=true` and `worker.replicas`.

## PostgreSQL

PostgreSQL is the part to size with care.

- **Writes.** Each action writes roughly six to ten rows across `actions`,
  `evaluations`, `decisions` and `ledger_entries`, in one or two
  transactions. Commit latency, not CPU, is usually the limit; put the
  database on fast storage close to vetod.
- **Ledger serialisation.** A ledger append locks its workspace's row in
  `ledger_heads` until the transaction commits, so the chain stays linear.
  Ledger throughput is therefore bounded by commit latency. Measure with
  your own hardware before planning for very high rates.
- **Storage.** `evaluations` dominates: the model's request and response,
  a few kilobytes per action. Retention clears these fields after the
  workspace's retention period, so storage settles at roughly the
  retention period's worth of actions plus the slim rows kept forever.
  Run `VACUUM` as usual; autovacuum defaults are adequate.
- **Poolers.** Veto needs `LISTEN`/`NOTIFY`. Connect directly, or through
  a pooler in session mode. Transaction-mode pooling (the PgBouncer
  default on some platforms) breaks the event listener and the job locks.
- **Read replicas** are not used; every query goes to the primary.

## The console

The console renders pages and forwards `/v1` and `/mcp` to vetod. It holds
no state and no database connections. Two replicas cover almost any load;
add more only if your load balancer shows it is the bottleneck. Do not
buffer responses at the console's load balancer: the live event stream,
long polls and MCP streams depend on it passing bytes through as they
arrive.
