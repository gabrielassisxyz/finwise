/// <reference types="node" />
import { readFileSync, readdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { createFakeApi } from '@/api/fake';
import { emptyLedgerState } from '@/api/fake/first-run-data';
import { GOLDEN_DIRECTORY, goldenFileName, goldenFilesOf, readMethods } from './replay';

// The stand-in and the backend each say what an empty ledger looks like, and nothing else keeps them saying the
// same thing. Over a ledger with no bank data there is nothing to differ, so every answer must be equal.

const API_SOURCES = resolve(GOLDEN_DIRECTORY, '../..');

interface Call {
  method: string;
  args: unknown[];
}

/**
 * The calls the Go side records are its read cases, and its test files are the only place the arguments of a file are
 * kept whole: the file name is a lossy slug. A case is `readCase{"m.n", `[...]`}`, or a bare entry of the base list.
 */
function recordedCalls(): Call[] {
  return goTestSources().flatMap(callsIn);
}

function goTestSources(): string[] {
  return readdirSync(API_SOURCES)
    .filter((name) => name.endsWith('_test.go'))
    .map((file) => readFileSync(join(API_SOURCES, file), 'utf8'));
}

function callsIn(source: string): Call[] {
  const entry = '"([A-Za-z]+\\.[A-Za-z]+)",\\s*`(\\[[^`]*\\])`';
  const appended = new RegExp(`readCase\\{${entry}\\}`, 'g');
  const listed = new RegExp(`^\\t\\{${entry}\\},$`, 'gm');
  const baseList = source.match(/var readCases = \[\]readCase\{\n([\s\S]*?)\n\}/)?.[1] ?? '';
  return [...source.matchAll(appended), ...baseList.matchAll(listed)].map(([, method, args]) => ({
    method: method!,
    args: JSON.parse(args!) as unknown[],
  }));
}

/** The list of the third read state: the full ledger with an invented authored state, whose files are not the empty ledger's. */
const AUTHORED_LIST = /var authoredReadCases = \[\]authoredReadCase\{\n[\s\S]*?\n\}\n/;

interface Difference {
  method: string;
  path: string;
  backend: unknown;
  standIn: unknown;
}

/** Fields that legitimately differ over an empty ledger, with the reason; matched on the field's path within an answer. */
const CLOCK = 'The backend\'s invented ledgers run on a clock fixed at 2026-03-20 and the stand-in\'s on 2026-11-28.';
const SYNC_RECORD =
  'The backend\'s empty ledger holds a sync record, and answers it; the stand-in\'s empty state is the moment before the first sync.';
const EXCEPTED_FIELDS: { method: string; path: string; reason: string }[] = [
  { method: 'shell.overview', path: 'today', reason: CLOCK },
  { method: 'shell.overview', path: 'firstMonth', reason: `The first month is the clock's. ${CLOCK}` },
  { method: 'shell.overview', path: 'lastMonth', reason: `The last month is the clock's (the stand-in offers the next one too). ${CLOCK}` },
  { method: 'shell.overview', path: 'sync.state', reason: SYNC_RECORD },
  { method: 'shell.overview', path: 'sync.lastSyncDay', reason: SYNC_RECORD },
  { method: 'shell.overview', path: 'sync.lastSyncTime', reason: SYNC_RECORD },
  { method: 'shell.overview', path: 'sync.message', reason: 'The wording of a sync state is each side\'s own.' },
  { method: 'netWorth.overview', path: 'historyStart', reason: `With no reading the history starts on the clock's month. ${CLOCK}` },
  { method: 'netWorth.overview', path: 'investmentRate.month', reason: `The rate is of the clock's month. ${CLOCK}` },
  { method: 'settings.overview', path: 'data.lastSync', reason: SYNC_RECORD },
];

function differences(method: string, backend: unknown, standIn: unknown, path = ''): Difference[] {
  if (Object.is(backend, standIn)) return [];
  const both = typeof backend === 'object' && backend !== null && typeof standIn === 'object' && standIn !== null;
  if (!both || Array.isArray(backend) !== Array.isArray(standIn)) return [{ method, path: path || '(whole answer)', backend, standIn }];
  const left = backend as Record<string, unknown>;
  const right = standIn as Record<string, unknown>;
  const keys = [...new Set([...Object.keys(left), ...Object.keys(right)])].sort();
  return keys.flatMap((key) => differences(method, left[key], right[key], Array.isArray(backend) ? `${path}[${key}]` : path ? `${path}.${key}` : key));
}

function isExcepted(difference: Difference): boolean {
  return EXCEPTED_FIELDS.some((entry) => entry.method === difference.method && entry.path === difference.path);
}

const describeDifference = ({ method, path, backend, standIn }: Difference): string =>
  `${method} ${path}: the backend answers ${JSON.stringify(backend)}, the stand-in ${JSON.stringify(standIn)}`;

describe('the backend and the stand-in agree on an empty ledger', () => {
  const calls = recordedCalls();
  const files = goldenFilesOf('empty');

  it('finds the arguments of every golden file of the empty ledger in the Go cases', () => {
    const named = new Set(calls.map((call) => goldenFileName(call.method, call.args)));
    expect(files.filter((file) => !named.has(file))).toEqual([]);
    expect([...named].filter((file) => !files.includes(file))).toEqual([]);
  });

  it('leaves the calls of the authored read cases out', () => {
    const lists = goTestSources().flatMap((source) => source.match(AUTHORED_LIST) ?? []);
    expect(lists.length).toBe(1);
    expect(lists[0]!.match(/authoredRead\("/g)?.length ?? 0).toBeGreaterThan(0);
    for (const source of goTestSources()) {
      const without = source.replace(AUTHORED_LIST, '');
      expect(callsIn(source)).toEqual(callsIn(without));
    }
  });

  it('compares every read method of the manifest', () => {
    const compared = new Set(calls.map((call) => call.method));
    expect(readMethods().filter((method) => !compared.has(method))).toEqual([]);
  });

  async function allDifferences(): Promise<(Difference & { file: string })[]> {
    const api = createFakeApi({ state: emptyLedgerState(), startLocked: false });
    const found: (Difference & { file: string })[] = [];
    for (const { method, args } of calls) {
      const [domain, name] = method.split('.') as [string, string];
      const call = (api as unknown as Record<string, Record<string, (...given: unknown[]) => Promise<unknown>>>)[domain]![name]!;
      const file = goldenFileName(method, args);
      const backend = JSON.parse(readFileSync(join(GOLDEN_DIRECTORY, 'empty', file), 'utf8')) as unknown;
      const standIn = JSON.parse(JSON.stringify((await call(...args)) ?? null)) as unknown;
      found.push(...differences(method, backend, standIn).map((difference) => ({ ...difference, file })));
    }
    return found;
  }

  it('has no difference outside the list of excepted fields', async () => {
    const unexplained = (await allDifferences()).filter((difference) => !isExcepted(difference));
    expect(unexplained.map((difference) => `${difference.file}: ${describeDifference(difference)}`)).toEqual([]);
  });

  it('lists no excepted field that no longer differs, and none without a reason', async () => {
    const found = await allDifferences();
    const stale = EXCEPTED_FIELDS.filter((entry) => !found.some((difference) => difference.method === entry.method && difference.path === entry.path));
    expect(stale).toEqual([]);
    expect(EXCEPTED_FIELDS.filter((entry) => !entry.reason.trim())).toEqual([]);
  });
});
