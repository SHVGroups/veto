# Backup and restore

Everything Veto knows is in PostgreSQL, except one thing: the master key.
A backup is complete only when you have both, stored apart.

| What                                                  | Where                                    | How                                          |
| ----------------------------------------------------- | ---------------------------------------- | -------------------------------------------- |
| Actions, decisions, the ledger, configuration, people | PostgreSQL                               | Database backups                             |
| The master key                                        | `VETO_MASTER_KEY` in your secret manager | A copy kept separately from database backups |
| Previous master keys during a rotation                | `VETO_MASTER_KEY_PREVIOUS`               | Same as the master key                       |

vetod and the console keep nothing on disk; their containers can be
replaced at any time.

## Why the key is kept apart

The master key decrypts the workspace signing keys, connection credentials,
webhook secrets and Slack tokens stored in the database. Anyone holding a
database backup and the key holds all of those. Keep them in different
systems with different access, so one leak is not both.

Without the key, a restored database still works for everything except
those secrets: see [Losing the master key](#losing-the-master-key).

## Backing up PostgreSQL

Use what your platform offers; Veto has no special requirements.

- **Managed PostgreSQL** (RDS, Cloud SQL, Azure, Neon, Supabase): turn on
  automated backups with point-in-time recovery, and keep them for at least
  as long as you need to answer for past decisions.
- **Your own server:** continuous archiving with a tool such as pgBackRest
  or WAL-G gives point-in-time recovery. For small deployments a nightly
  logical dump is enough:

  ```sh
  pg_dump --format=custom --no-owner --file=veto-$(date -u +%Y%m%dT%H%M%SZ).dump "$VETO_DATABASE_URL"
  ```

- **Docker Compose:** the database lives in the `postgres` volume. Dump it
  from the running container:

  ```sh
  docker compose exec -T postgres pg_dump --format=custom --no-owner -U veto veto > veto.dump
  ```

Logical dumps are consistent snapshots, so they can run while Veto is
serving.

## Restoring

1. Stop vetod, or scale it to zero, so nothing writes during the restore.
2. Restore into an empty database:

   ```sh
   createdb veto
   pg_restore --no-owner --dbname="$VETO_DATABASE_URL" veto.dump
   ```

   For a point-in-time restore, follow your platform's procedure and point
   `VETO_DATABASE_URL` at the restored instance.

3. Start vetod with the **same master key** the backup was made with (plus
   `VETO_MASTER_KEY_PREVIOUS` if a rotation was under way). Pending
   migrations, if the binary is newer than the backup, are applied at
   start.
4. Verify the ledger:

   ```sh
   vetod ledger verify
   ```

   It checks every workspace's chain and its latest signed checkpoint, and exits
   non-zero if anything does not match. A restore from a consistent backup
   always verifies.

5. Open the console and check **Settings**: the server section shows the
   version, the schema version and whether the decision model is
   configured.

### What to expect after restoring to an earlier point

Actions after the backup point are gone from Veto, but the agents may have
run them. Before you restore to a point in the past, export what you can:

- the webhook deliveries your systems received after that point;
- the ledger entries after that point, from **Ledger** in the console or
  `GET /v1/ledger` on the damaged database, if it is still readable.

After the restore:

- Held actions whose deadlines have passed expire within seconds, and the
  agents waiting on them see `expired`.
- Webhook deliveries that were queued at the backup point are sent again.
  Receivers should ignore deliveries whose `Veto-Delivery` id they have
  already processed.
- Grants issued after the backup point are unknown to the restored
  database, so verifying one with `"consume": true` fails. Their
  signatures are still valid, though, so verification without consuming,
  and offline verification with the public keys, accept them until they
  expire five minutes after issue. Keep agents stopped for five minutes
  after a restore if that matters to you.
- Agent keys and sessions created after the backup point no longer work.
  Recreate the keys and sign in again.

## Losing the master key

If the key is lost and no copy exists, the restored database is still
usable, but its encrypted values cannot be read. Veto reports each one as
it is needed. To recover:

1. Generate a new key with `vetod keygen` and start vetod with it.
2. **Settings:** rotate the signing key (owners only). Veto cannot sign
   grants or checkpoints until you do. Grants and checkpoints signed with
   the old key keep verifying, because public keys are stored in clear;
   tools pick up the new key from `/v1/grants/keys`.
3. **Connections:** enter each connection's credential again.
4. **Webhooks:** rotate each endpoint's secret and give the new secret to
   the receiver.
5. **Slack:** connect the integration again.

Actions, decisions, the ledger, policies, packs and people are unaffected.

## Testing your backups

A backup you have not restored is a hope, not a backup. At least once a
quarter:

1. Restore the latest backup into a scratch database.
2. Run `vetod ledger verify` against it with the production master key.
3. Start a vetod against it with `VETO_WORKERS=false` and no agent traffic,
   sign in, and open a recent action.
4. Delete the scratch database.
