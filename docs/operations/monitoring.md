# Monitoring

Veto sits in the path of every agent action, so when it is unhealthy your
agents notice at once: calls wait for people, or stop. Watch it the way you
watch anything on that path.

There are four sources:

| Source             | What it tells you                                        |
| ------------------ | -------------------------------------------------------- |
| Health endpoints   | Whether a replica is up and can reach the database.      |
| Prometheus metrics | Rates, latencies and errors, aggregated across replicas. |
| Logs               | What went wrong and for which action, workspace or job.  |
| The console        | What decisions are being made and why.                   |

## Health endpoints

| Endpoint       | Returns                                                                  | Use       |
| -------------- | ------------------------------------------------------------------------ | --------- |
| `GET /healthz` | `200 {"status":"ok","version":"1.0.0"}` while the process serves         | Liveness  |
| `GET /readyz`  | `200` when PostgreSQL answers within 2 seconds, else `503` with a reason | Readiness |

Neither requires authentication, and neither is logged above debug level.
`vetod healthcheck` calls `/healthz` for container health checks.

Readiness does not include the decision model. When the model is
unavailable Veto keeps serving and applies each tool's fail mode, which is
what you want; alert on model errors separately, below.

## Metrics

Scrape `/metrics` on every vetod replica (port 8080), with
`Authorization: Bearer <VETO_METRICS_TOKEN>` when the token is set. In
Kubernetes, `serviceMonitor.enabled=true` in the Helm chart sets this up
for the Prometheus Operator. A plain Prometheus job looks like:

```yaml
scrape_configs:
  - job_name: veto
    authorization:
      credentials_file: /etc/prometheus/veto-metrics-token
    static_configs:
      - targets: ["vetod-1:8080", "vetod-2:8080"]
```

Every metric is listed in [Metrics](../reference/metrics.md).

### Dashboard queries

| Panel                             | Query                                                                                                   |
| --------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Actions per second, by resolution | `sum by (resolution) (rate(veto_actions_total[5m]))`                                                    |
| Share decided without a person    | `sum(rate(veto_actions_total{route="auto"}[1h])) / sum(rate(veto_actions_total{source!="expiry"}[1h]))` |
| Decision latency, p50 and p95     | `histogram_quantile(0.95, sum by (le) (rate(veto_decision_seconds_bucket{route="auto"}[5m])))`          |
| Model latency, p95                | `histogram_quantile(0.95, sum by (le) (rate(veto_model_request_seconds_bucket{stage="model"}[5m])))`    |
| Model error rate                  | `sum(rate(veto_model_requests_total{result="error"}[5m])) / sum(rate(veto_model_requests_total[5m]))`   |
| Reviews per hour, by channel      | `sum by (channel) (increase(veto_reviews_total[1h]))`                                                   |
| Held actions that expired         | `sum(increase(veto_actions_total{resolution="expired"}[1h]))`                                           |
| Bad outcomes                      | `sum by (status) (increase(veto_outcomes_total{status!="succeeded"}[1d]))`                              |
| Grant checks, by result           | `sum by (result) (rate(veto_grant_verifications_total[5m]))`                                            |
| Webhook deliveries, by result     | `sum by (result) (rate(veto_webhook_deliveries_total[5m]))`                                             |
| HTTP errors                       | `sum by (route) (rate(veto_http_requests_total{code="5xx"}[5m]))`                                       |
| HTTP latency, p95 by route        | `histogram_quantile(0.95, sum by (le, route) (rate(veto_http_request_seconds_bucket[5m])))`             |

`veto_decision_seconds{route="human"}` measures the time to hold an
action, not the time a person took; the console's **Overview** shows the
median time to review.

## Alerts

[`deploy/monitoring/prometheus-rules.yaml`](../../deploy/monitoring/prometheus-rules.yaml)
holds ready-made rules. Load it with `rule_files`, or wrap its `groups` in
a `PrometheusRule` for the Prometheus Operator. The thresholds are
starting points; tune them once you know your traffic.

| Alert                      | Severity | Means                                                   | First step                                                                       |
| -------------------------- | -------- | ------------------------------------------------------- | -------------------------------------------------------------------------------- |
| `VetoDown`                 | page     | A replica stopped answering scrapes.                    | Check the pod or process and `/readyz`.                                          |
| `VetoServerErrors`         | page     | Over 2% of requests fail with 5xx.                      | Logs with `level=ERROR` and `msg="request failed"`; usually the database.        |
| `VetoModelErrors`          | page     | Over 5% of model requests fail.                         | [Troubleshooting: the model](../troubleshooting.md#the-decision-model).          |
| `VetoModelSlow`            | ticket   | Model p95 above 2 s, close to the 2.5 s timeout.        | Check TypeSafe's status; consider fallbacks or a longer `VETO_DECISION_TIMEOUT`. |
| `VetoHoldRateHigh`         | ticket   | Over 30% of actions wait for a person.                  | **Overview > Why actions were held**.                                            |
| `VetoReviewsExpiring`      | ticket   | Held actions expire undecided.                          | Check that reviewers are notified (Slack, webhooks).                             |
| `VetoWebhookFailures`      | ticket   | Deliveries failed after every retry.                    | **Webhooks** in the console shows each endpoint's last error.                    |
| `VetoJobFailing`           | ticket   | A background job keeps failing.                         | Logs with `msg="background job failed"` and that job.                            |
| `VetoJobsNotRunning`       | page     | No replica runs background jobs.                        | At least one replica needs `VETO_WORKERS=true`.                                  |
| `VetoGrantForgeryAttempts` | page     | A tool was shown a grant Veto did not issue.            | Find the caller in the tool's own logs.                                          |
| `VetoGrantReuse`           | ticket   | Grants are being replayed or used with other arguments. | Check the agent's retry logic.                                                   |

### Ledger verification

Every 24 hours a worker verifies each workspace's ledger. If a chain or a
checkpoint no longer matches, vetod logs `ledger verification failed` at
error level with the workspace and sequence number, and sends the
`ledger.verification_failed` webhook event. Subscribe a webhook that pages
someone to that event, or alert on the log line; there is no metric for it
because it should never happen. See
[Troubleshooting: the ledger](../troubleshooting.md#the-ledger).

## Logs

vetod writes structured logs to standard error, JSON by default
(`VETO_LOG_FORMAT=text` for people). Every line has `time`, `level` and
`msg`; the rest depends on the message.

Each HTTP request is logged once when it finishes:

```json
{
  "time": "2026-10-14T09:26:05.412Z",
  "level": "INFO",
  "msg": "request",
  "request_id": "req_9f2c4e1ab07d3368",
  "method": "POST",
  "path": "/v1/actions",
  "status": 200,
  "duration_ms": 412,
  "ip": "203.0.113.7"
}
```

Responses with status 500 or above are logged at `ERROR`, and health and
metrics requests at `DEBUG`. The `request_id` is returned to clients in
the `X-Request-Id` header and in every error body, so a user's error report
leads straight to the log line. A request that arrives with its own
`X-Request-Id` (up to 64 characters) keeps it, so ids from your load
balancer or the console carry through.

Messages worth an alert or a saved search:

| Message                                            | Level | Meaning                                                                                                                                                                                            |
| -------------------------------------------------- | ----- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `decision model unavailable`                       | WARN  | A model request failed after retries and fallbacks; the tool's fail mode applied.                                                                                                                  |
| `policy evaluation unavailable; holding action`    | ERROR | Policies could not be evaluated; the action was held.                                                                                                                                              |
| `ledger verification failed`                       | ERROR | See above.                                                                                                                                                                                         |
| `background job failed`, `background job panicked` | ERROR | A job run failed; it runs again on its next tick.                                                                                                                                                  |
| `could not checkpoint ledger`                      | ERROR | Checkpoints are not being signed for a workspace.                                                                                                                                                  |
| `event listener disconnected; reconnecting`        | WARN  | The `LISTEN` connection dropped. Live updates and long polls in this replica pause until it reconnects.                                                                                            |
| `could not post held action to Slack`              | WARN  | Slack rejected a message; see [Troubleshooting: Slack](../troubleshooting.md#slack).                                                                                                               |
| `mcp upstream request failed`                      | WARN  | An MCP server Veto proxies to could not be reached.                                                                                                                                                |
| `ignoring invalid stored pack`                     | ERROR | A saved question pack no longer validates, usually after an upgrade tightened validation; the built-in pack of that name is used, if there is one. Open the pack in the console and save it again. |

vetod does not log API keys, session tokens, passwords, grant tokens,
decrypted secrets or action arguments. Logs do contain action ids, tool
names, workspace ids and client addresses.

## In the console

The **Overview** page is the operational view of decisions rather than of
the service: how many actions arrived, how they were decided, how fast the
model answered and how often it errored, why actions were held, and what
the audits found. **Calibration** shows whether confidence can be trusted.
**Settings** shows the version, the schema version and whether the model
is configured on the replica that served the page.
