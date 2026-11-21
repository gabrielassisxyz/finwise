// Shoots the reference screenshots from the design prototype.
//
// The prototype bundle is not versioned: place it in work/bundle (or point
// FINWISE_PROTOTYPE_DIR at it) before running. It loads its runtime and its
// fonts from the network.
//
// usage: npm run parity:reference
import { createReadStream, existsSync, mkdirSync } from 'node:fs';
import { createServer } from 'node:http';
import { extname, join, normalize, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { launchBrowser, newContext, settle } from './browser.mjs';
import { PROTOTYPE_STATE_ITEM, STATES, VIEWPORTS } from './states.mjs';

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

const server = await serveBundle();
const baseUrl = `http://127.0.0.1:${server.address().port}`;
const browser = await launchBrowser();
mkdirSync(referenceDir, { recursive: true });
let failed = false;

for (const [viewportName, viewport] of Object.entries(VIEWPORTS)) {
  for (const state of STATES) {
    if (!state.prototype) continue;
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
    for (const step of state.prototype.steps ?? []) await step(page);
    await settle(page);
    await page.screenshot({ path: join(referenceDir, `${viewportName}-${state.name}.png`), fullPage: true });
    console.log(`${viewportName}-${state.name}${errors.length ? `  page errors: ${errors.join('; ')}` : ''}`);
    if (errors.length) failed = true;
    await context.close();
  }
}

await browser.close();
server.close();
process.exit(failed ? 1 : 0);
