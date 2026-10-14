# Deployment

Veto has two processes and one dependency:

- **vetod**, the Go server: the API, the MCP proxy and background jobs. A
  single static binary, also published as the `veto-server` image.
- **the console**, a Next.js server that serves the pages and forwards
  `/v1` and `/mcp` to vetod. Published as the `veto-console` image.
- **PostgreSQL** 14 or newer.

People and agents reach both through one address: the console's. Put your
TLS-terminating load balancer in front of the console and set
`VETO_PUBLIC_URL` to that https address.

```
agents, browsers, Slack ──https──> load balancer ──> console :3000 ──> vetod :8080 ──> PostgreSQL
                                                                         └──> TypeSafe API, connections, webhooks
```

Agents and services inside your network can also call vetod directly on
port 8080.

## Docker Compose

For a single machine, `deploy/compose` runs PostgreSQL, vetod and the
console. Follow the [quickstart](../getting-started/quickstart.md), then:

- put a reverse proxy with TLS in front of port 3000 (Caddy, nginx,
  Traefik or a cloud load balancer);
- set `VETO_PUBLIC_URL` to its https address and leave
  `VETO_COOKIE_SECURE=true`;
- set `VETO_SETUP_TOKEN` before the first start if the address is public;
- back up the `postgres` volume and the master key (see
  [Backup and restore](backup-and-restore.md)).

## Kubernetes

The Helm chart in `deploy/helm/veto` runs vetod and the console with two
replicas each. Bring your own PostgreSQL.

```sh
helm install veto deploy/helm/veto \
  --set publicUrl=https://veto.example.com \
  --set database.existingSecret=veto-db --set database.existingSecretKey=url \
  --set secrets.masterKey="$(openssl rand -base64 32)" \
  --set secrets.jevApiKey=... \
  --set ingress.enabled=true --set ingress.className=nginx \
  --set 'ingress.tls[0].hosts[0]=veto.example.com' --set 'ingress.tls[0].secretName=veto-tls'
```

For production, keep secrets out of values: create a Secret with the keys
`master-key`, `jev-api-key` and optionally `master-key-previous`,
`setup-token` and `metrics-token`, and set `secrets.existingSecret` to its
name. See `values.yaml` for every option.

The chart's pods run as non-root with read-only root filesystems, a
PodDisruptionBudget keeps one server up during node drains, and probes use
`/readyz` (the database is reachable) and `/healthz`.

Two options are off by default:

- `worker.enabled=true` runs background jobs in their own pods, so the
  server pods only serve requests (see [Scaling](scaling.md#background-jobs)).
- `serviceMonitor.enabled=true` adds a ServiceMonitor for the Prometheus
  Operator that scrapes every server and worker pod, with the metrics token
  when one is set (see [Monitoring](monitoring.md)).

## The binary

```sh
VETO_DATABASE_URL=postgres://... VETO_MASTER_KEY=... VETO_PUBLIC_URL=https://veto.example.com ./vetod serve
```

Run it under systemd or similar. Build the console with
`pnpm --filter @veto/console build` and run
`node apps/console/.next/standalone/apps/console/server.js` with
`VETO_API_URL` pointing at vetod, or serve both behind a proxy that sends
`/v1` and `/mcp` to vetod and everything else to the console.

## Reverse proxy settings

- Pass `Host` and `X-Forwarded-*` headers, and set `VETO_TRUSTED_PROXIES`
  to the proxy's addresses so rate limits and logs see real client
  addresses.
- Do not buffer `/v1/events` (Server-Sent Events) or `/mcp/*` (MCP
  streams), and allow long-lived responses: set read timeouts above
  `VETO_MAX_WAIT` and `VETO_MCP_HOLD_TIMEOUT`, for example 90 seconds.
- Allow request bodies of at least 1 MiB.

## PostgreSQL

- Version 14 or newer. Managed services (RDS, Cloud SQL, Azure, Neon,
  Supabase) work; Veto needs `LISTEN`/`NOTIFY`, so connect directly or
  through a pooler in session mode, not transaction mode.
- vetod keeps a pool of 10 connections per replica, one of them held by the
  event listener; add `pool_max_conns=N` to `VETO_DATABASE_URL` to allow
  more. Size `max_connections` for your replica count.
- Use `sslmode=require` or stricter across networks.

## Checklist

- [ ] HTTPS in front of the console, `VETO_PUBLIC_URL` set to it
- [ ] `VETO_MASTER_KEY` generated, stored in a secret manager, and backed up
- [ ] `VETO_SETUP_TOKEN` set until the first owner exists
- [ ] `VETO_JEV_API_KEY` set
- [ ] `VETO_TRUSTED_PROXIES` set to your proxy and console networks
- [ ] `VETO_METRICS_TOKEN` set, or `/metrics` not exposed publicly
- [ ] Database backups scheduled and a restore tested
- [ ] Alerts from [Monitoring](monitoring.md) in place
