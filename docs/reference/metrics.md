# Metrics

vetod serves Prometheus metrics at `/metrics`. When `VETO_METRICS_TOKEN` is
set, scrape with `Authorization: Bearer <token>`. Every replica serves its
own counters; aggregate across replicas in your queries.

| Metric                             | Type      | Labels                                                                                                                                           |                                                          |
| ---------------------------------- | --------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------- |
| `veto_actions_total`               | counter   | `source` (`api`, `mcp`, `connection`, `expiry`), `route` (`auto`, `human`), `resolution` (`proceed`, `proceed_safer`, `stop`, `held`, `expired`) | Actions handled.                                         |
| `veto_decision_seconds`            | histogram | `route`                                                                                                                                          | Time from receiving an action to deciding or holding it. |
| `veto_model_requests_total`        | counter   | `stage` (`model`, `model_recheck`), `result` (`ok`, `error`)                                                                                     | Decision model requests.                                 |
| `veto_model_request_seconds`       | histogram | `stage`                                                                                                                                          | Model latency, including retries.                        |
| `veto_reviews_total`               | counter   | `option`, `channel` (`console`, `slack`, `api`)                                                                                                  | Held actions decided by a person.                        |
| `veto_outcomes_total`              | counter   | `status`                                                                                                                                         | Outcome reports.                                         |
| `veto_grant_verifications_total`   | counter   | `result` (`valid`, `already_used`, `expired`, `mismatch`, `malformed`, ...)                                                                      | Grant checks.                                            |
| `veto_connection_executions_total` | counter   | `result`                                                                                                                                         | Calls made through connections.                          |
| `veto_webhook_deliveries_total`    | counter   | `result` (`delivered`, `retry`, `failed`)                                                                                                        | Webhook attempts.                                        |
| `veto_worker_runs_total`           | counter   | `job`, `result`                                                                                                                                  | Background job runs.                                     |
| `veto_http_requests_total`         | counter   | `route`, `code` (`2xx`, `4xx`, ...)                                                                                                              | HTTP requests, by route pattern.                         |
| `veto_http_request_seconds`        | histogram | `route`                                                                                                                                          | HTTP latency. The event stream is excluded.              |

Histograms use buckets from 5 ms to 10 s.

See [Monitoring](../operations/monitoring.md) for alerts built on these.
