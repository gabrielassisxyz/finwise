# finwise

A personal finance app for one person: transactions from every bank in one list, a budget to
follow, and a review of what an overnight classifier could not decide alone. It reads the SQLite
ledger that [pluggy-sync](https://github.com/gabrielassisxyz/pluggy-sync) collects through Open
Finance.

It is being built. The interface is being ported from its design, screen by screen, and runs on an
in-memory stand-in for the backend; the backend that reads the ledger comes after.

## Layout

| path | what it holds |
|---|---|
| `web/` | the interface: Vue 3, Vite, TypeScript |
| `design/` | the design tokens and the reference screenshots the interface is measured against |
| `bin/ci` | the gate: every check the pull requests run |

## Running it

```sh
npm --prefix web ci
npm --prefix web run dev
```

The stand-in backend unlocks with the PIN `123456`.

## Checking a change

```sh
bin/ci                        # secret scan, tokens, types, tests, build
npm --prefix web run parity   # each ported screen against the design
```

`design/README.md` explains the second one.
