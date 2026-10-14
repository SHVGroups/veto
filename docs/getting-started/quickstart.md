# Quickstart

This guide runs Veto with Docker Compose, creates a workspace, and sends a
first action from an agent. It takes about ten minutes.

You need Docker with Compose v2 and, for model decisions, a TypeSafe API key.
Without a key Veto still works: policies decide what they can and every
other call waits for a person.

## 1. Configure

```sh
git clone https://github.com/shvgroups/veto.git
cd veto/deploy/compose
cp .env.example .env
```

Fill in `.env`:

| Setting              | Value                                                                                                                                   |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| `POSTGRES_PASSWORD`  | A strong password of letters and digits, such as the output of `openssl rand -hex 24`. It goes into a connection URL, so avoid symbols. |
| `VETO_MASTER_KEY`    | Run `openssl rand -base64 32` (or `vetod keygen`) and paste the output. Keep a copy somewhere safe: it encrypts stored secrets.         |
| `VETO_PUBLIC_URL`    | Where people will open the console. For a first try on your machine, `http://localhost:3000`.                                           |
| `VETO_COOKIE_SECURE` | `false` when `VETO_PUBLIC_URL` is plain `http`. Leave `true` in any real deployment.                                                    |
| `VETO_JEV_API_KEY`   | Your TypeSafe key, if you have one.                                                                                                     |

## 2. Start

```sh
docker compose up -d
docker compose logs -f vetod
```

vetod applies its database migrations on start and logs `vetod listening`
when it is ready.

## 3. Create the workspace

Open `VETO_PUBLIC_URL`. On a new deployment Veto asks for a workspace name
and the first owner's account. If you set `VETO_SETUP_TOKEN`, it asks for
that too, so nobody else can claim the deployment before you do.

The workspace starts with three policies: read-only tools run without the
model, critical tools always wait for a person, and tools Veto has not seen
before wait for a person until you classify them.

## 4. Add an agent

In the console, open **Agents** and choose **Add agent**. Copy the key it
shows: Veto stores only its hash.

## 5. Send an action

```sh
export VETO_URL=http://localhost:3000
export VETO_API_KEY=veto_sk_...

curl -s "$VETO_URL/v1/actions?wait=10" \
  -H "Authorization: Bearer $VETO_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "tool": "gmail.send",
    "args": {"to": ["jane@globex.com"], "subject": "Your refund", "body": "Done."},
    "request": "Tell Jane her refund went through"
  }'
```

`gmail.send` is new to Veto, so the default policy holds it and the answer
is `202` with `"status": "pending_review"`. Open **Review** in the console:
the action is there with the three options. Choose one.

Ask for the result again:

```sh
curl -s "$VETO_URL/v1/actions/act_...?wait=10" -H "Authorization: Bearer $VETO_API_KEY"
```

The `decision` says what to run (`decision.tool`, `decision.args`) and
`grant.token` proves it was approved. Run exactly that call, then report how
it went:

```sh
curl -s "$VETO_URL/v1/actions/act_.../outcome" \
  -H "Authorization: Bearer $VETO_API_KEY" -H "Content-Type: application/json" \
  -d '{"status": "succeeded"}'
```

## 6. Classify the tool

Open **Tools**, select `gmail.send`, give it the **Medium** risk class, pick
the **communication** question pack, and add a safer-version rule: **Run a
different tool**, `gmail.create_draft`. Save.

Send the action again. Now the decision model chooses: if it is confident
the email matches the request, Veto approves it alone; if not, the call is
held, and reviewers see the draft as option B.

## Next

- Use an SDK instead of curl: [Connecting agents](../guides/agents.md).
- Put Veto in front of an MCP server: [The MCP proxy](../guides/mcp.md).
- Understand what the model decides and when: [Decisions](../concepts/decisions.md).
- Deploy for real, with TLS: [Deployment](../operations/deployment.md).
