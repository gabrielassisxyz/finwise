// @vitest-environment node
/// <reference types="node" />
import { spawnSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import Ajv from 'ajv';
import type { ValidateFunction } from 'ajv';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { createFakeApi } from '../fake';
import type { FakeState } from '../fake/model';
import { emptyLedgerState, freshInstallState } from '../fake/first-run-data';
import { prototypeState } from '../fake/prototype-data';
import type { FinwiseApi } from '../types';
import { API_ERROR_KINDS } from './errors';
import { METHOD_CLASSES } from './classification';
import type { MethodClass } from './classification';

interface ManifestMethod {
  domain: string;
  name: string;
  route: string;
  class: MethodClass;
  arguments: Record<string, unknown>;
  result: Record<string, unknown>;
}

interface Manifest {
  formatVersion: number;
  methods: ManifestMethod[];
  error: Record<string, unknown>;
  definitions: Record<string, unknown>;
}

const scriptPath = resolve(import.meta.dirname, '../../../scripts/contract-manifest.mjs');
const manifestPath = resolve(import.meta.dirname, '../../../../internal/contract/contract.json');
// The script is plain JavaScript run by node; a computed path keeps the compiler from asking for its types.
const script = (await import(/* @vite-ignore */ scriptPath)) as {
  findEmptySchemas: (schema: unknown) => string[];
  acceptsAnything: (schema: unknown) => boolean;
};

const manifest = JSON.parse(readFileSync(manifestPath, 'utf8')) as Manifest;
const key = (method: { domain: string; name: string }) => `${method.domain}.${method.name}`;
const byKey = new Map(manifest.methods.map((method) => [key(method), method]));

describe('the manifest covers the classification one to one', () => {
  const classified = Object.entries(METHOD_CLASSES).flatMap(([domain, methods]) =>
    Object.entries(methods).map(([name, cls]) => ({ domain, name, cls: cls as MethodClass })),
  );

  it('lists every classified method exactly once, with its class and a /api/<domain>/<method> route', () => {
    expect(manifest.methods.map(key).sort()).toEqual(classified.map(key).sort());
    expect(new Set(manifest.methods.map(key)).size).toBe(manifest.methods.length);
    for (const { domain, name, cls } of classified) {
      const entry = byKey.get(`${domain}.${name}`);
      expect(entry?.class).toBe(cls);
      expect(entry?.route).toBe(`/api/${domain}/${name}`);
    }
  });

  it('keeps the classes the contract names', () => {
    const classOf = (name: string) => byKey.get(name)?.class;
    expect(['session.status', 'session.unlock', 'session.lock'].map(classOf)).toEqual(['session', 'session', 'session']);
    expect(['session.changePin', 'changes.undo'].map(classOf)).toEqual(['write', 'write']);
    expect(classOf('budget.evaluateScenario')).toBe('evaluate');
  });

  it('matches the methods the stand-in really has', () => {
    const api = createFakeApi({ startLocked: false }) as unknown as Record<string, Record<string, unknown>>;
    const real = Object.entries(api).flatMap(([domain, methods]) => Object.keys(methods).map((name) => `${domain}.${name}`));
    expect(real.sort()).toEqual([...byKey.keys()].sort());
  });
});

describe('no schema accepts anything', () => {
  it('finds no empty schema in the manifest', () => {
    expect(script.findEmptySchemas(manifest)).toEqual([]);
  });

  it('does recognise an empty schema, however it is spelled', () => {
    expect(script.acceptsAnything({})).toBe(true);
    expect(script.acceptsAnything(true)).toBe(true);
    expect(script.acceptsAnything({ description: 'anything' })).toBe(true);
    expect(script.acceptsAnything({ type: 'null' })).toBe(false);
    expect(script.findEmptySchemas({ methods: [{ result: {} }], definitions: { X: { properties: { y: {} } } } })).toEqual([
      '$.methods[0].result',
      '$.definitions.X.properties.y',
    ]);
  });

  it('gives every method an array of arguments and a result that is not unconstrained', () => {
    for (const method of manifest.methods) {
      expect(method.arguments.type, key(method)).toBe('array');
      expect(Object.keys(method.result).length, key(method)).toBeGreaterThan(0);
    }
  });
});

describe('the error envelope', () => {
  it('is in the manifest and lists the kinds a server answers with, not the client-only one', () => {
    const ajv = new Ajv({ strict: true, strictTuples: false });
    ajv.addSchema({ definitions: manifest.definitions }, 'contract');
    const valid = ajv.compile(withContractRefs(manifest.error));
    for (const kind of API_ERROR_KINDS.filter((candidate) => candidate !== 'network')) {
      expect(valid({ error: { kind } }), kind).toBe(true);
    }
    expect(valid({ error: { kind: 'network' } })).toBe(false);
    expect(valid({ error: { kind: 'read_only', detail: 'x' } })).toBe(false);
    expect(valid({ kind: 'read_only' })).toBe(false);
  });
});

/** A $ref in the manifest resolves against the manifest; here the definitions are registered as `contract`. */
function withContractRefs<T>(schema: T): T {
  return JSON.parse(JSON.stringify(schema).replaceAll('"#/definitions/', '"contract#/definitions/')) as T;
}

// An optional trailing argument makes minItems differ from the tuple's length, which strictTuples dislikes.
const ajv = new Ajv({ strict: true, strictTuples: false });
ajv.addSchema({ definitions: manifest.definitions }, 'contract');
const validators = new Map<string, { args: ValidateFunction; result: ValidateFunction }>();
for (const method of manifest.methods) {
  validators.set(key(method), {
    args: ajv.compile(withContractRefs(method.arguments)),
    result: ajv.compile(withContractRefs(method.result)),
  });
}

function unknownClosingState(): FakeState {
  const state = prototypeState();
  state.card!.bankClosingDay = null;
  return state;
}

function unknownYieldDayState(): FakeState {
  const state = prototypeState();
  state.savingsPots.find((pot) => pot.id === 'rfs')!.unknownYieldDays = [25];
  return state;
}

const STATES: { name: string; build: () => FakeState; minimumAnswers: number }[] = [
  { name: 'the prototype dataset', build: prototypeState, minimumAnswers: 100 },
  { name: 'a fresh install', build: freshInstallState, minimumAnswers: 30 },
  { name: 'an empty ledger', build: emptyLedgerState, minimumAnswers: 20 },
  { name: 'a card with no known closing', build: unknownClosingState, minimumAnswers: 100 },
  { name: 'a pot with a day of unknown yield', build: unknownYieldDayState, minimumAnswers: 100 },
];

describe.each(STATES)('the stand-in over $name against the schemas', ({ name, build, minimumAnswers }) => {
  const NOV = '2026-11';
  const CLOSED = '2026-10';
  type Calls = Record<string, unknown[][]>;
  let api: FinwiseApi;
  let calls: Calls;
  const answers: { method: string; args: unknown[]; answer: unknown }[] = [];

  beforeAll(async () => {
    api = createFakeApi({ startLocked: false, state: build() });
    const transactions = await api.transactions.list({ month: NOV, period: 'year' });
    const pots = (await api.savingsPots.list({ includeClosed: true })).flatMap((group) => group.pots);
    const subscriptions = (await api.subscriptions.list({ grouped: true })).groups.flatMap((group) => group.subscriptions);
    const accounts = await api.accounts.list();
    const lines = (await api.transactions.filterOptions(NOV)).lines.map((line) => line.name);
    calls = {
      'shell.overview': [[]],
      'home.summary': [[NOV], [CLOSED]],
      'transactions.list': [
        [{ month: NOV, period: 'month' }],
        [{ month: NOV, period: 'last-3-months' }],
        [{ month: NOV, period: 'year' }],
        [{ month: '2026-12', period: 'month' }],
      ],
      'transactions.filterOptions': [[NOV], [CLOSED]],
      'transactions.detail': [...transactions.map((transaction) => [transaction.id]), ['nobody']],
      'savingsPots.list': [[{ includeClosed: true }], [{ includeClosed: false }]],
      'savingsPots.detail': [...pots.map((pot) => [pot.id]), ['nobody']],
      'netWorth.overview': [[]],
      'budget.document': [[NOV], [CLOSED]],
      'budget.view': [[NOV], [CLOSED], [NOV, await api.budget.document(NOV)]],
      'budget.establishments': lines.map((line) => [NOV, line]),
      'budget.scenarios': [[]],
      'subscriptions.list': [[{ grouped: true }], [{ grouped: false }]],
      'subscriptions.detail': [...subscriptions.map((row) => [row.id]), ['nobody']],
      'subscriptions.candidates': [[]],
      'subscriptions.recurring': [[]],
      'review.weekly': [[NOV], [CLOSED]],
      'review.closing': [[NOV], [CLOSED]],
      'review.payday': [[NOV], [CLOSED]],
      'accounts.list': [[]],
      'accounts.detail': [...accounts.map((account) => [account.id]), ['nobody']],
      'card.overview': [[]],
      'settings.overview': [[]],
    };
    for (const [method, argumentLists] of Object.entries(calls)) {
      const [domain, name] = method.split('.') as [string, string];
      const target = (api as unknown as Record<string, Record<string, (...args: unknown[]) => Promise<unknown>>>)[domain]![name]!;
      for (const args of argumentLists) answers.push({ method, args, answer: await target(...args) });
    }
  });

  afterAll(() => answers.splice(0));

  it('has a call for every read method and for nothing else', () => {
    const reads = manifest.methods.filter((method) => method.class === 'read').map(key);
    expect(Object.keys(calls).sort()).toEqual(reads.sort());
  });

  it('only ever sends arguments its own schema accepts', () => {
    for (const { method, args } of answers) {
      const valid = validators.get(method)!.args;
      expect(valid(args), `${method} ${JSON.stringify(args)} ${JSON.stringify(valid.errors)}`).toBe(true);
    }
  });

  it.runIf(name === 'a pot with a day of unknown yield')('answers the unknown day with a null amount', async () => {
    const detail = await api.savingsPots.detail('rfs');
    expect(detail!.yield.days.filter((day) => day.amount === null)).toHaveLength(1);
  });

  it('validates every answer of every read method', () => {
    expect(answers.length).toBeGreaterThan(minimumAnswers);
    for (const { method, args, answer } of answers) {
      const valid = validators.get(method)!.result;
      const where = `${method} ${JSON.stringify(args)}`;
      expect(valid(answer), `${where} ${JSON.stringify(valid.errors?.slice(0, 3))}`).toBe(true);
    }
  });

  it('rejects arguments of the wrong shape', () => {
    const month = validators.get('home.summary')!.args;
    expect(month([])).toBe(false);
    expect(month([NOV, NOV])).toBe(false);
    expect(month([7])).toBe(false);
    const view = validators.get('budget.view')!.args;
    expect(view([NOV, null])).toBe(true);
    expect(view([NOV, 'draft'])).toBe(false);
    const line = (id: unknown) => [{ key: 'var', name: 'Variáveis', lines: [{ id, name: 'Mercado', amount: 1 }] }];
    expect(view([NOV, line('7')])).toBe(true);
    expect(view([NOV, line(7)])).toBe(false);
    const menu = validators.get('transactions.filterOptions')!.args;
    expect(menu([])).toBe(false);
    expect(menu([NOV])).toBe(true);
    expect(menu([NOV, NOV])).toBe(false);
    const list = validators.get('savingsPots.list')!.args;
    expect(list([{ includeClosed: 'yes' }])).toBe(false);
    expect(list([{ includeClosed: true, extra: 1 }])).toBe(false);
  });

  describe.runIf(name === 'the prototype dataset')('a broken answer is rejected', () => {
    const answerOf = (method: string, index = 0) => structuredClone(answers.filter((entry) => entry.method === method)[index]!.answer);
    const rejects = (method: string, answer: unknown) => validators.get(method)!.result(answer);

    it('takes the unmutated answer as valid, so the rejections below are the mutation and nothing else', () => {
      expect(rejects('home.summary', answerOf('home.summary'))).toBe(true);
    });

    it('rejects a number turned into a string', () => {
      const summary = answerOf('home.summary') as { limit: { cap: unknown } };
      summary.limit.cap = String(summary.limit.cap);
      expect(rejects('home.summary', summary)).toBe(false);
    });

    it('rejects a required field removed', () => {
      const summary = answerOf('home.summary') as { limit?: unknown };
      delete summary.limit;
      expect(rejects('home.summary', summary)).toBe(false);
    });

    it('rejects an unknown field added', () => {
      const summary = answerOf('home.summary') as Record<string, unknown>;
      summary.surprise = 1;
      expect(rejects('home.summary', summary)).toBe(false);
    });

    it('rejects the same three mutations deep inside a list', () => {
      const list = answerOf('transactions.list') as Record<string, unknown>[];
      expect(rejects('transactions.list', list)).toBe(true);
      const first = list[0]!;
      const amount = Object.keys(first).find((field) => typeof first[field] === 'number')!;
      expect(rejects('transactions.list', [{ ...first, [amount]: String(first[amount]) }, ...list.slice(1)])).toBe(false);
      expect(rejects('transactions.list', [{ ...first, surprise: 1 }, ...list.slice(1)])).toBe(false);
      const { id: _removed, ...withoutId } = first;
      expect(rejects('transactions.list', [withoutId, ...list.slice(1)])).toBe(false);
    });

    it('rejects a union member that is neither shape', () => {
      expect(validators.get('session.unlock')!.result({ ok: false, reason: 'locked-out' })).toBe(false);
      expect(validators.get('session.unlock')!.result({ ok: true, extra: 1 })).toBe(false);
      expect(validators.get('session.unlock')!.result({ ok: true })).toBe(true);
    });
  });
});

describe('the freshness check', { timeout: 60_000 }, () => {
  let dir: string;
  const check = (file: string) =>
    spawnSync(process.execPath, [scriptPath, '--check', '--out', file], { encoding: 'utf8' });

  beforeAll(() => {
    dir = mkdtempSync(join(tmpdir(), 'finwise-manifest-test-'));
  });
  afterAll(() => rmSync(dir, { recursive: true, force: true }));

  it('passes on the file in the repository', () => {
    const result = check(manifestPath);
    expect(result.stderr).toBe('');
    expect(result.status).toBe(0);
  });

  it('fails, naming the manifest, on a stale file and on a missing one', () => {
    const stale = join(dir, 'contract.json');
    writeFileSync(stale, readFileSync(manifestPath, 'utf8').replace('"formatVersion": 1', '"formatVersion": 0'));
    const staleResult = check(stale);
    expect(staleResult.status).toBe(1);
    expect(staleResult.stderr).toContain('contract.json');
    expect(staleResult.stderr).toContain('npm --prefix web run contract');

    const missing = check(join(dir, 'absent.json'));
    expect(missing.status).toBe(1);
    expect(missing.stderr).toContain('absent.json');
  });

  it('writes a file that then passes, byte for byte the same on a second run', () => {
    const target = join(dir, 'written.json');
    const run = () => spawnSync(process.execPath, [scriptPath, '--out', target], { encoding: 'utf8' });
    expect(run().status).toBe(0);
    const first = readFileSync(target, 'utf8');
    expect(run().status).toBe(0);
    expect(readFileSync(target, 'utf8')).toBe(first);
    expect(first).toBe(readFileSync(manifestPath, 'utf8'));
    expect(check(target).status).toBe(0);
  });
});
