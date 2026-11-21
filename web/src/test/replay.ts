/// <reference types="node" />
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { methods } from '../../../internal/contract/contract.json';
import { ApiFailure } from '@/api/http/failure';
import type { FinwiseApi } from '@/api/types';

// The golden files belong to the Go side and are read where they live: a copy under web/ would go stale
// without anything failing.
export const GOLDEN_DIRECTORY = resolve(import.meta.dirname, '../../../internal/api/testdata/golden');

export type RecordedLedger = 'full' | 'empty';
export const RECORDED_LEDGERS: readonly RecordedLedger[] = ['full', 'empty'];

interface ManifestMethod {
  domain: string;
  name: string;
  class: 'session' | 'read' | 'evaluate' | 'write';
}

type Method = (...args: unknown[]) => Promise<unknown>;

/** Every method of the contract, as the manifest the Go side validates against lists it. */
export const MANIFEST_METHODS = methods as ManifestMethod[];

export function readMethods(): string[] {
  return MANIFEST_METHODS.filter((entry) => entry.class === 'read').map((entry) => `${entry.domain}.${entry.name}`);
}

function sortedKeys(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortedKeys);
  if (value === null || typeof value !== 'object') return value;
  const record = value as Record<string, unknown>;
  return Object.fromEntries(Object.keys(record).sort().map((key) => [key, sortedKeys(record[key])]));
}

/**
 * The file name of a call, by the rule of the golden README. The arguments go through JSON first, as the HTTP client
 * sends them, so an argument left undefined is the `null` the backend would have received.
 */
export function goldenFileName(method: string, args: unknown[]): string {
  const compact = JSON.stringify(sortedKeys(JSON.parse(JSON.stringify(args))));
  const slug = compact.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
  return slug === '' ? `${method}.json` : `${method}.${slug}.json`;
}

export interface ReplayOptions {
  directory?: string;
}

export interface Replay {
  api: FinwiseApi;
  /** Every request that had no recorded answer, so a test can assert none was made even if a screen swallowed the rejection. */
  unrecorded: string[];
}

/**
 * A `FinwiseApi` whose reads answer from the golden files of one recorded ledger and whose writes are refused as
 * the read-only backend refuses them. It is built from the manifest, so a method added to the contract is served
 * with no change here, and the file of a call is found by name at the moment of the call.
 */
export function replayApi(ledger: RecordedLedger, options: ReplayOptions = {}): Replay {
  const directory = join(options.directory ?? GOLDEN_DIRECTORY, ledger);
  const unrecorded: string[] = [];
  const cache = new Map<string, unknown>();

  const answerFromFile = (domain: string, name: string): Method => async (...args) => {
    const method = `${domain}.${name}`;
    const file = goldenFileName(method, args);
    const path = join(directory, file);
    if (!existsSync(path)) {
      const request = `${method}(${args.map((arg) => JSON.stringify(arg)).join(', ')})`;
      unrecorded.push(request);
      throw new Error(`no recorded answer over the ${ledger} ledger for ${request}: expected ${ledger}/${file}`);
    }
    if (!cache.has(path)) cache.set(path, JSON.parse(readFileSync(path, 'utf8')));
    return structuredClone(cache.get(path));
  };

  const refused: Method = async () => {
    throw new ApiFailure('read_only');
  };

  const sessionAnswers: Record<string, Method> = {
    status: async () => ({ locked: false, retryAfterSeconds: 0 }),
    unlock: async () => ({ ok: true }),
    lock: async () => undefined,
  };

  const api: Record<string, Record<string, Method>> = {};
  for (const { domain, name, class: kind } of MANIFEST_METHODS) {
    const methodsOfDomain = (api[domain] ??= {});
    if (kind === 'read') methodsOfDomain[name] = answerFromFile(domain, name);
    else if (kind === 'session') methodsOfDomain[name] = sessionAnswers[name] ?? refused;
    else methodsOfDomain[name] = refused;
  }
  return { api: api as unknown as FinwiseApi, unrecorded };
}

/** The golden files of a ledger as `<domain>.<method>` plus the file name, for the tests that check coverage. */
export function goldenFilesOf(ledger: RecordedLedger, directory: string = GOLDEN_DIRECTORY): string[] {
  return readdirSync(join(directory, ledger)).filter((file) => file.endsWith('.json'));
}
