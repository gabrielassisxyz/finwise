// Builds the app, renders every ported state and measures it against the
// design's reference screenshot. Exits non-zero when a state differs by more
// than the threshold. Candidates and heatmaps land in work/parity.
//
// usage: npm run parity [-- <state-name>...]
import { mkdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { build, preview } from 'vite';
import { launchBrowser, newContext, settle } from './browser.mjs';
import { crop, diffImages, readPng, writePng } from './diff.mjs';
import { PROTOTYPE_PIN, STATES, VIEWPORTS } from './states.mjs';

// Share of pixels allowed to differ, in percent. A faithful screen measures
// 0.00 to 0.01; a wrong logo color measured 0.11 and a missing webfont 0.09 on
// the PIN screen, where most of the page is empty. The limit sits between the
// two, so a defect that small still fails.
const THRESHOLD = 0.05;

const webRoot = resolve(fileURLToPath(new URL('../..', import.meta.url)));
const repoRoot = resolve(webRoot, '..');
const referenceDir = join(repoRoot, 'design/reference');
const outDir = join(repoRoot, 'work/parity');
const only = process.argv.slice(2);

async function reach(page, baseUrl, state, viewportName) {
  const { app } = state;
  await page.goto(baseUrl + app.path, { waitUntil: 'networkidle' });
  if (app.locked) {
    await page.waitForSelector('[data-screen-label="PIN"]');
  } else {
    await page.keyboard.type(PROTOTYPE_PIN);
    await page.waitForSelector('main');
  }
  for (const step of state.steps ?? []) await step(page, viewportName);
  await settle(page);
}

function compare(state, viewportName, candidatePath) {
  const rect = state.clip?.[viewportName];
  let reference = readPng(join(referenceDir, `${viewportName}-${state.reference ?? state.name}.png`));
  let candidate = readPng(candidatePath);
  if (rect) {
    reference = crop(reference, rect);
    candidate = crop(candidate, rect);
  }
  const result = diffImages(reference, candidate);
  writePng(join(outDir, `${viewportName}-${state.name}-diff.png`), result.heatmap);
  const sizes =
    reference.height === candidate.height && reference.width === candidate.width
      ? ''
      : `  size ${candidate.width}x${candidate.height}, reference ${reference.width}x${reference.height}`;
  return { percent: result.percent, sizes };
}

await build({ root: webRoot, logLevel: 'warn' });
const server = await preview({ root: webRoot, logLevel: 'warn', preview: { host: '127.0.0.1', port: 0 } });
const address = server.httpServer.address();
const baseUrl = `http://127.0.0.1:${address.port}`;
const browser = await launchBrowser();
mkdirSync(outDir, { recursive: true });

let failures = 0;
let checked = 0;
const notPorted = new Set();

for (const [viewportName, viewport] of Object.entries(VIEWPORTS)) {
  for (const state of STATES) {
    if (only.length && !only.includes(state.name)) continue;
    if (state.viewports && !state.viewports.includes(viewportName)) continue;
    if (!state.app) {
      notPorted.add(state.name);
      continue;
    }
    const context = await newContext(browser, viewport);
    const page = await context.newPage();
    await reach(page, baseUrl, state, viewportName);
    const candidatePath = join(outDir, `${viewportName}-${state.name}.png`);
    await page.screenshot({ path: candidatePath, fullPage: true });
    await context.close();

    const { percent, sizes } = compare(state, viewportName, candidatePath);
    const passed = percent <= THRESHOLD;
    checked += 1;
    if (!passed) failures += 1;
    console.log(`${passed ? 'PASS' : 'FAIL'}  ${percent.toFixed(2).padStart(6)}%  ${viewportName}-${state.name}${sizes}`);
  }
}

await browser.close();
await new Promise((done) => server.httpServer.close(done));

console.log(`\n${checked} checked, ${failures} over ${THRESHOLD}%, ${notPorted.size} states not ported yet`);
if (notPorted.size) console.log(`not ported: ${[...notPorted].join(', ')}`);
process.exit(failures ? 1 : 0);
