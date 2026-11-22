# Golden files

There are four directories. Three hold the exact answer of one read method, as the HTTP layer
sends it (pretty-printed, keys in the order the server writes them), over the invented fixture
ledgers built in `internal/api/fixtures_test.go`; the fourth holds what a recorded write changed.

- `full/` holds every shape the transaction rules know, and what the card, the pots and the net worth
  read: two banks' pots (one reporting yield gross and one net, one of them closed and one without an
  identity), investments on a connection with pots and on one without, and a net worth history of three months.
- `empty/` holds a ledger with no account and no transaction.
- `full-authored/` holds the answers of `authoredReadCases` over the full ledger together with an
  invented authored state (`inventedAuthoredState`, in `fixtures_test.go`), served by a router that
  has the app's database too. A read that does not consult the app's database answers here what it
  answers in `full/`; the reads that merge what the person authored are cased here.
- `writes/` holds one file per recorded-change case of `writeCases`
  (`internal/api/write_conformance_test.go`): the answer and the change's effects in the order of the
  journal, each with its table, key, column, before and after as the recorder wrote them. The
  harness's own proof cases hold their golden content in the test.

Nothing here comes from a real account, and `-update` refuses to run while
`FINWISE_LEDGER_SNAPSHOT` is set so that it cannot.

## Naming

In all four directories the file is named after the call (for `writes/`, the case's method and
arguments) by one rule:

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

Because the name comes from the call alone, a file never moves when another case is added, removed or
reordered, and two recorded-change cases of one method must differ in their arguments: a test fails
when two cases name the same file.

## Updating

```
go test ./internal/api -run TestEveryAnswerMatchesItsGoldenFile -update                      # full/ and empty/
go test ./internal/api -run TestEveryAuthoredReadCaseAnswersAndMatchesItsGoldenFile -update  # full-authored/
go test ./internal/api -run TestEveryWriteCaseConformsAndMatchesItsGoldenFile -update        # writes/
```

Read the diff before committing it: a changed golden file is a changed answer. Every update is
refused while `FINWISE_LEDGER_SNAPSHOT` is set, in all four directories. A file that belongs to no case,
or that sits in a directory of another name, fails `TestEveryGoldenFileHasACase`.

Each case of `readCases` is also answered over a router that has a freshly created app's database
holding nothing authored, and must equal its file in `full/` or `empty/`: turning the database on
moves no figure.
