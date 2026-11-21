# Golden files

Each file is the exact answer of one read method, as the HTTP layer sends it
(pretty-printed, keys in the order the server writes them), over one of the two
invented fixture ledgers built in `internal/api/fixtures_test.go`:

- `full/` holds every shape the transaction rules know.
- `empty/` holds a ledger with no account and no transaction.

Nothing here comes from a real account, and `-update` refuses to run while
`FINWISE_LEDGER_SNAPSHOT` is set so that it cannot.

## Naming

```
<domain>.<method>.json                    a method that takes no argument
<domain>.<method>.<slug>.json             a method that takes arguments
```

The slug is derived from the argument array alone, so a reader in any language
can find the file for a call:

1. Write the array of arguments as compact JSON with every object's keys sorted
   alphabetically, without HTML escaping.
2. Lowercase it.
3. Replace every run of characters outside `a-z0-9` with one `-`.
4. Trim `-` from both ends.

In Python: `re.sub('[^a-z0-9]+', '-', json.dumps(args, sort_keys=True, separators=(',', ':'), ensure_ascii=False).lower()).strip('-')`.

| call | file |
| --- | --- |
| `shell.overview()` | `shell.overview.json` |
| `transactions.detail("tx-instalment")` | `transactions.detail.tx-instalment.json` |
| `transactions.list({month: "2026-02", period: "month"})` | `transactions.list.month-2026-02-period-month.json` |

An answer of `null` is a file holding `null`.

## Updating

```
go test ./internal/api -run TestEveryAnswerMatchesItsGoldenFile -update
```

Read the diff before committing it: a changed golden file is a changed answer.
