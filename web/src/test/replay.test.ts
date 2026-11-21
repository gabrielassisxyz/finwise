import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import {
  GOLDEN_DIRECTORY,
  MANIFEST_METHODS,
  RECORDED_LEDGERS,
  goldenFileName,
  goldenFilesOf,
  readMethods,
  replayApi,
} from './replay';

type Method = (...args: unknown[]) => Promise<unknown>;
const call = (api: object, domain: string, name: string): Method => (api as Record<string, Record<string, Method>>)[domain]![name]!;

describe('the name of a golden file', () => {
  it.each([
    ['shell.overview', [], 'shell.overview.json'],
    ['transactions.detail', ['tx-instalment'], 'transactions.detail.tx-instalment.json'],
    ['transactions.list', [{ period: 'month', month: '2026-02' }], 'transactions.list.month-2026-02-period-month.json'],
    ['transactions.list', [{ month: '2026-03', period: 'last-3-months' }], 'transactions.list.month-2026-03-period-last-3-months.json'],
    ['savingsPots.detail', ['reserve:acc-pots/pot-1'], 'savingsPots.detail.reserve-acc-pots-pot-1.json'],
    ['budget.view', ['2026-03', undefined], 'budget.view.2026-03-null.json'],
    ['budget.view', ['2026-03'], 'budget.view.2026-03.json'],
  ])('%s %j is %s', (method, args, file) => {
    expect(goldenFileName(method, args)).toBe(file);
  });
});

describe.each(RECORDED_LEDGERS)('the replay over the %s ledger', (ledger) => {
  it('holds a recorded answer for every read method of the manifest', () => {
    const files = goldenFilesOf(ledger);
    const missing = readMethods().filter((method) => !files.some((file) => file.startsWith(`${method}.`)));
    expect(missing).toEqual([]);
  });

  it('holds no file for a method that is not a read of the manifest', () => {
    const reads = new Set(readMethods());
    const strays = goldenFilesOf(ledger).filter((file) => ![...reads].some((method) => file.startsWith(`${method}.`)));
    expect(strays).toEqual([]);
  });

  it('answers every recorded file when asked for the arguments it was recorded with', async () => {
    const { api, unrecorded } = replayApi(ledger);
    expect(await call(api, 'accounts', 'list')()).toBeInstanceOf(Array);
    expect(await call(api, 'shell', 'overview')()).toHaveProperty('today');
    expect(await call(api, 'transactions', 'detail')('no-such-transaction')).toBeNull();
    expect(unrecorded).toEqual([]);
  });

  it('refuses a request for arguments that were not recorded, naming the method and the arguments', async () => {
    const { api, unrecorded } = replayApi(ledger);
    const request = call(api, 'transactions', 'list')({ month: '1999-01', period: 'month' });
    await expect(request).rejects.toThrow(/transactions\.list\(\{"month":"1999-01","period":"month"\}\)/);
    expect(unrecorded).toHaveLength(1);
  });

  it('rejects every write and evaluate method with read_only', async () => {
    const { api } = replayApi(ledger);
    const refusing = MANIFEST_METHODS.filter((entry) => entry.class === 'write' || entry.class === 'evaluate');
    expect(refusing.length).toBeGreaterThan(0);
    for (const { domain, name } of refusing) {
      await expect(call(api, domain, name)(), `${domain}.${name}`).rejects.toMatchObject({ kind: 'read_only' });
    }
  });

  it('answers the session as unlocked', async () => {
    const { api } = replayApi(ledger);
    expect(await call(api, 'session', 'status')()).toEqual({ locked: false, retryAfterSeconds: 0 });
    expect(await call(api, 'session', 'unlock')('000000')).toEqual({ ok: true });
  });

  it('hands out a copy, so a screen that mutates an answer changes no later answer', async () => {
    const { api } = replayApi(ledger);
    const first = (await call(api, 'shell', 'overview')()) as { today: string };
    first.today = 'changed';
    expect(await call(api, 'shell', 'overview')()).not.toHaveProperty('today', 'changed');
  });
});

describe('a golden file that is missing', () => {
  it('fails the request by the file name expected', async () => {
    const directory = mkdtempSync(join(tmpdir(), 'finwise-replay-'));
    try {
      mkdirSync(join(directory, 'full'));
      writeFileSync(join(directory, 'full', 'accounts.list.json'), '[]\n');
      const { api, unrecorded } = replayApi('full', { directory });
      expect(await call(api, 'accounts', 'list')()).toEqual([]);
      await expect(call(api, 'shell', 'overview')()).rejects.toThrow('full/shell.overview.json');
      expect(unrecorded).toEqual(['shell.overview()']);
    } finally {
      rmSync(directory, { recursive: true });
    }
  });

  it('is read from the Go side of the repository', () => {
    expect(GOLDEN_DIRECTORY.endsWith('internal/api/testdata/golden')).toBe(true);
  });
});
