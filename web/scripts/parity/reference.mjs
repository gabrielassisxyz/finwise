// Shoots the reference screenshots from the design prototype.
//
// The prototype bundle is not versioned: place it in work/bundle (or point
// FINWISE_PROTOTYPE_DIR at it) before running. It loads its runtime and its
// fonts from the network.
//
// A reference that already exists is never replaced unless asked: a shot that
// comes out different from it is reported and discarded. Shots have differed
// by a handful of anti-aliasing pixels on a loaded machine, and a baseline that
// drifts on every run measures nothing.
//
// usage: npm run parity:reference [-- [--update] <state-name>...]
import { createReadStream, existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { extname, join, normalize, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { launchBrowser, newContext, settle } from './browser.mjs';
import { decodePng, diffImages, readPng } from './diff.mjs';
import { PROTOTYPE_STATE_ITEM, STATES, VIEWPORTS, viewportsOf } from './states.mjs';

const args = process.argv.slice(2);
const update = args.includes('--update');
const only = args.filter((arg) => !arg.startsWith('--'));

const repoRoot = resolve(fileURLToPath(new URL('../../..', import.meta.url)));
const bundleDir = resolve(process.env.FINWISE_PROTOTYPE_DIR ?? join(repoRoot, 'work/bundle'));
const referenceDir = join(repoRoot, 'design/reference');
const PROTOTYPE_FILE = 'Finwise.dc.html';
const CONTENT_TYPES = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8' };

if (!existsSync(join(bundleDir, PROTOTYPE_FILE))) {
  console.error(`No prototype at ${bundleDir}. Place the design bundle there first.`);
  process.exit(2);
}

function serveBundle() {
  const server = createServer((request, response) => {
    const requested = normalize(decodeURIComponent(new URL(request.url, 'http://x').pathname));
    const file = join(bundleDir, requested);
    if (!file.startsWith(bundleDir) || !existsSync(file)) {
      response.writeHead(404).end();
      return;
    }
    response.writeHead(200, { 'content-type': CONTENT_TYPES[extname(file)] ?? 'application/octet-stream' });
    createReadStream(file).pipe(response);
  });
  return new Promise((done) => server.listen(0, '127.0.0.1', () => done(server)));
}

/** Writes a new reference, keeps an existing one, and says which happened. */
function record(target, shot) {
  if (!existsSync(target)) {
    writeFileSync(target, shot);
    return { verdict: 'new', refused: false };
  }
  const existing = readPng(target);
  const fresh = decodePng(shot);
  const same = existing.width === fresh.width && existing.height === fresh.height && existing.data.equals(fresh.data);
  if (same) return { verdict: 'unchanged', refused: false };
  const percent = diffImages(existing, fresh).percent.toFixed(3);
  if (update) {
    writeFileSync(target, shot);
    return { verdict: `updated, was ${percent}% away`, refused: false };
  }
  return { verdict: `DIFFERS by ${percent}% from the existing reference, which was kept`, refused: true };
}

const server = await serveBundle();
const baseUrl = `http://127.0.0.1:${server.address().port}`;
const browser = await launchBrowser();
mkdirSync(referenceDir, { recursive: true });
let failed = false;

for (const [viewportName, viewport] of Object.entries(VIEWPORTS)) {
  for (const state of STATES) {
    if (!state.prototype) continue;
    if (only.length && !only.includes(state.name)) continue;
    if (!viewportsOf(state).includes(viewportName)) continue;
    const context = await newContext(browser, viewport);
    await context.addInitScript(
      ([key, seed]) => localStorage.setItem(key, JSON.stringify(seed)),
      [PROTOTYPE_STATE_ITEM, state.prototype.seed],
    );
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await page.goto(`${baseUrl}/${PROTOTYPE_FILE}`, { waitUntil: 'networkidle' });
    await page.waitForSelector('[data-screen-label]');
    for (const step of state.steps ?? []) await step(page, viewportName);
    await settle(page);
    const shot = await page.screenshot({ fullPage: true });
    const { verdict, refused } = record(join(referenceDir, `${viewportName}-${state.name}.png`), shot);
    console.log(`${viewportName}-${state.name}  ${verdict}${errors.length ? `  page errors: ${errors.join('; ')}` : ''}`);
    if (errors.length || refused) failed = true;
    await context.close();
  }
}

await browser.close();
server.close();
process.exit(failed ? 1 : 0);
