# Upgrading

Veto follows [semantic versioning](https://semver.org). Within a major
version:

- the `/v1` HTTP API, webhook payloads, the grant format and the SDKs stay
  compatible: fields are added, never removed or changed in meaning;
- configuration variables keep their names and defaults;
- database migrations only add, so the previous minor version runs
  against the upgraded schema during a rolling deploy.

Breaking changes wait for a major version and are listed in
[CHANGELOG.md](../../CHANGELOG.md) with what to do. Read the changelog for
every version between yours and the target before you upgrade.

## Before you start

1. Check the version you run: **Settings** in the console, `vetod version`,
   or `GET /healthz`.
2. Take a database backup, or confirm the latest automatic one finished
   (see [Backup and restore](backup-and-restore.md)).
3. Upgrade vetod and the console to the same version. The console is built
   against its release's API; running it against an older vetod is not
   supported.

## Rolling upgrade

The default, and what the Helm chart does.

1. Deploy the new images. Each new vetod replica applies pending migrations
   at start; an advisory lock lets exactly one of them run each migration
   while the others wait.
2. Old and new replicas serve side by side until the rollout completes.
   Agents with requests on a stopping replica get their responses for up to
   30 seconds, then retry on another replica with the same idempotency
   key, which returns the same action.
3. Confirm afterwards: the console's **Settings** shows the new version and
   schema version, and `veto_http_requests_total{code="5xx"}` has not
   risen.

## Running migrations as a separate step

If your process requires schema changes to run before any new code serves
traffic, or you use a database role without DDL rights at runtime:

1. Set `VETO_AUTO_MIGRATE=false` on the server.
2. Run `vetod migrate` with the new image and a role that may change the
   schema, for example as a Kubernetes Job or a pre-deploy hook. It prints
   the migrations it applied and exits.
3. Deploy the new images.

## Docker Compose

```sh
cd deploy/compose
docker compose pull
docker compose up -d
```

Compose recreates vetod and the console with the new images; vetod migrates
as it starts. Requests in flight during the few seconds of restart fail and
are retried by the SDKs.

## Rolling back

Migrations are not reversed. Within a major version this rarely matters:
the previous minor version runs against the newer schema, so redeploying
the previous images is a valid rollback.

Across a major version, or if the changelog says a migration is not
backwards compatible, roll back by restoring the backup taken before the
upgrade. Anything recorded after it is lost; see
[What to expect after restoring to an earlier point](backup-and-restore.md#what-to-expect-after-restoring-to-an-earlier-point).

## SDKs

Upgrade the SDKs independently of the server. Any 1.x SDK works with any
1.x server; newer SDKs ignore fields an older server does not send, and
older SDKs ignore fields they do not know.

## Question packs

New releases can improve the built-in packs. A workspace that has saved its
own version of a built-in pack keeps it: compare yours with the new
built-in on the pack's page, and use **Restore built-in version** if you
want the update. Run your [evaluations](../guides/evaluations.md) before
and after, since a pack change moves decisions.

## The decision model

`VETO_JEV_MODEL` is pinned (`jev-1.13.0` by default) so an upgrade on
TypeSafe's side never changes decisions without you knowing. When you move
to a newer model:

1. Run `vetod eval` with your cases twice, with `VETO_JEV_MODEL` set to
   the current and to the new model, and compare accuracy and wrong
   automatic approvals.
2. Change `VETO_JEV_MODEL` and redeploy.
3. Watch **Calibration** and the audit labels over the following weeks.
   Until most of the chosen range falls after the switch, the chart mixes
   both models. Thresholds tuned for the old model may need adjusting.
