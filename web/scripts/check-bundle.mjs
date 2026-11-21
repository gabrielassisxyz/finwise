// Proves the build for real use leaves the in-memory stand-in out, and that
// the proof can fail: the same sentinel must show up in the prototype build.
//
// The sentinel is the bank's raw text of one transaction, a string that exists
// only in the prototype's dataset (src/api/fake/prototype-data.ts).
//
// usage: node scripts/check-bundle.mjs
import { mkdtempSync, readdirSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from 'vite';

export const SENTINEL = 'POUSADA SERRA AZUL ME';

const webRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));

export function filesContaining(dir, text) {
  return readdirSync(dir, { recursive: true, withFileTypes: true })
    .filter((entry) => entry.isFile() && readFileSync(join(entry.parentPath, entry.name), 'utf8').includes(text))
    .map((entry) => join(entry.parentPath, entry.name));
}

async function builtIn(mode) {
  const outDir = mkdtempSync(join(tmpdir(), `finwise-bundle-${mode}-`));
  await build({ root: webRoot, mode, logLevel: 'error', build: { outDir, emptyOutDir: true } });
  return outDir;
}

export async function sentinelFilesOf(mode) {
  const dir = await builtIn(mode);
  try {
    return filesContaining(dir, SENTINEL);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

async function main() {
  const prototype = await sentinelFilesOf('prototype');
  if (prototype.length === 0) {
    console.error(`the prototype build does not contain "${SENTINEL}": the check cannot tell anything`);
    return 1;
  }
  const real = await sentinelFilesOf('production');
  if (real.length > 0) {
    console.error(`the build for real use contains "${SENTINEL}", so it carries the stand-in's dataset:`);
    for (const file of real) console.error(`  ${file}`);
    return 1;
  }
  console.log('the build for real use leaves the stand-in out; the prototype build keeps it');
  return 0;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) process.exit(await main());
