# finwise — Agent Briefing

> Read before every interaction. This file holds only the rules that are already in force; the stack,
> the commands and the gates are added as the project gains them.

> **What it is:** a personal finance app for one person, over the SQLite ledger that
> [pluggy-sync](https://github.com/gabrielassisxyz/pluggy-sync) collects. The numbers are someone's
> money: a wrong total or a silently dropped transaction is the failure that matters.

## Tracker
- Work is tracked with `br` in `.beads/` (`br ready`, `br show <id>`). A bead is self-contained:
  implement from it, not from an external plan.

## Merging
- An agent merges its own pull request once the repository's gate is green locally and every check
  on the pull request is green; nobody waits for a human to press merge. While the repository has
  no gate and no checks, the pull request body says how the change was verified. Work still goes
  through a branch, atomic commits and a pull request whose body carries the why.

## Stack & Commands
- **Frontend:** `web/` is Vue 3, Vite and TypeScript, with vue-router and Pinia. Styles are plain
  CSS in each component, built on CSS variables. No backend exists yet: the app runs on
  `web/src/api/fake`, an in-memory stand-in that answers with the design prototype's dataset.
- **Install:** `npm --prefix web ci`
- **Run:** `npm --prefix web run dev`. The PIN of the stand-in is `123456`.
- **Test:** `npm --prefix web test`
- **Gate:** `bin/ci` (secret scan, shellcheck, tokens current, no raw design value, types, tests,
  build). It is the exact thing CI runs. Run it before a pull request.
- **Parity:** `npm --prefix web run parity` measures every ported screen against the design. It is
  not part of `bin/ci` because the measurement depends on the browser build; run it for any change
  that touches a screen and put its output in the pull request body.
- **Backend:** Go, one module at the repository root (`github.com/gabrielassisxyz/finwise`, version in
  `go.mod`), driver `modernc.org/sqlite`. `internal/ledger` reads pluggy-sync's `pluggy-snapshot.db`
  read-only, reopening it on every call; `ledgertest` beside it builds invented snapshots for tests.
  `internal/money` holds amounts as integer cents and `internal/brday` turns Brazil days into UTC
  bounds. `go test ./...` is the test command and `bin/ci` runs it with `gofmt`, `go vet` and
  `go build`. A query that mirrors one in pluggy-sync's README cites its lines in a comment.

## Design
- The design arrives as an HTML prototype. It is a reference, never code to copy: `design/README.md`
  says how it becomes tokens, reference screenshots and screens.
- `design/tokens/finwise.tokens.json` is the only place a color, a type size, a radius, a shadow or
  a step of the spacing scale is written. `npm --prefix web run tokens` builds
  `web/src/styles/tokens.css` from it; that file is versioned and never edited by hand.
- A component references tokens and nothing else for those values. A one-off that is not a design
  decision carries a `token-exempt: <reason>` comment on its line.
- A screen is ported when its states in `web/scripts/parity/states.mjs` pass on both viewports.
  A screen being built is fixed in the component or in the tokens, never by raising the limit.
- Interface text is Brazilian Portuguese. Everything else in the repository is English.

## Tests
- Every feature is born with a test; every bugfix with a regression test.
- Tests run with one command, no manual setup, no credential.
- The backend is faked at the API boundary with `createFakeApi`, not with an inline stub.
- Fixtures are invented. Nothing derived from a real account is committed.
- A real ledger is a person's whole financial history and this repository is public. A real snapshot
  may be opened read-only to check a schema, the keys of a JSON column or a count; no value from it
  (amount, description, name, document, id) is ever printed, logged, copied, committed or pasted, and
  a row count, though it may be shown to the person running the test, goes into no file and no pull
  request. The one test that touches it runs only when `FINWISE_LEDGER_SNAPSHOT` names a file, and
  logs counts only.

## Common hurdles

| hurdle | class | gate |
|---|---|---|
| `vue-tsc` 3 loads the JavaScript compiler from the `typescript` package, which TypeScript 7 no longer ships; `typescript` stays on 6 until `vue-tsc` supports the native one | prose | none |
