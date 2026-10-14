# Contributing to Veto

Thank you for helping. Veto decides what AI agents are allowed to do, so we
hold changes to a high bar for correctness and clarity. This page explains
how to make a change that can be merged quickly.

## Before you start

- **Bugs:** open an issue with the version, what you did, what you
  expected and what happened. Include the `request_id` from the error and
  the relevant log lines.
- **Security problems:** do not open an issue. Follow [SECURITY.md](SECURITY.md).
- **Features and larger changes:** open an issue describing the problem
  first. Agreeing on the approach before code is written saves everyone
  time, particularly for anything touching decisions, grants, the ledger
  or the API.
- **Small fixes** (typos, docs, clear bugs with an obvious fix): send a
  pull request directly.

## Setting up

Follow [Local development](docs/getting-started/local-development.md). In
short:

```sh
make setup
make db
cp server/.env.example server/.env    # then set VETO_MASTER_KEY with `make keygen`
make dev                              # https://veto.localhost and https://api.veto.localhost
```

`make help` lists every task. `make check` runs what CI runs, apart from the
container builds; database tests need `VETO_TEST_DATABASE_URL`.

## Making a change

### Code

- **Go** is formatted with `gofmt` and must pass `go vet`. Business rules
  live in `internal/app` and the pure packages below it; `internal/httpapi`
  only translates HTTP. New SQL goes in `internal/store`. Wrap errors with
  context. No global state. See [Architecture](docs/architecture.md).
- **TypeScript** is strict, formatted with Prettier, and must pass ESLint,
  including the React compiler rules. Server state goes through TanStack
  Query.
- Keep dependencies few. A new dependency needs a reason in the pull
  request.
- Comments explain why, not what. Exported Go identifiers have doc
  comments.

### Tests

Every behaviour change comes with a test that fails without it.

- Decision rules, packs, policies, grants and the ledger: table-driven
  unit tests in the package.
- Anything that touches the database: a test against a real PostgreSQL via
  `storetest` or `apptest`; no mocks of the store.
- API changes: update `api/openapi.yaml`; a test checks it against the
  route table.
- Console: Vitest for logic; check pages by hand in both themes and at
  phone width.
- SDKs: both have mock-transport tests; keep the two SDKs' behaviour the
  same.

### Docs

Docs are part of the change. If behaviour, configuration, an API or a
message changes, update the page that describes it in the same pull
request. Configuration variables are documented in
`docs/reference/configuration.md`, metrics in `docs/reference/metrics.md`.

### Writing style

For the console and the docs:

- Sentence case, plain words, short sentences.
- Say what a control does and what an error means and what to do about it.
- No exclamation marks, no emoji, no marketing language.

### Database migrations

- Add a new file in `server/internal/store/migrations`, numbered after the
  last one. Never edit a migration that has been released.
- Within a major version, migrations only add: new tables, new nullable
  columns or columns with defaults, new indexes (created so they do not
  block writes on large tables). The previous minor version must keep
  working against the new schema during a rolling deploy.

### Question packs

Changes to built-in packs move decisions for everyone. Include
`vetod eval` results before and after for the affected pack, using the
cases in `evals/`. New labelled cases are very welcome on their own; see
[evals/README.md](evals/README.md).

## Pull requests

- One logical change per pull request, with a description of what changes,
  why, and how it was tested (the template asks).
- Keep commits focused. We squash on merge, so the title should read well
  in the changelog: imperative mood, such as "Hold calls when a forbid
  policy errors".
- CI must pass. A maintainer reviews every change; for changes to
  decisions, grants, the ledger, the vault or authentication, two do.

## Licence

By contributing, you agree that your contributions are licensed under the
[Apache License 2.0](LICENSE), the same licence as the project.
