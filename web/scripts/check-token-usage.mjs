// Fails when a stylesheet or a component carries a raw design value that has
// a token. One token file is the only place a color, a type size, a radius or
// a step of the spacing scale is written down; a literal elsewhere is a value
// the next design change will miss.
//
// A line may opt out with a `token-exempt: <reason>` comment, for a value that
// is a one-off by design.
//
// usage: node scripts/check-token-usage.mjs
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const SPACING_SCALE = new Set([2, 4, 6, 8, 10, 12, 14, 16, 20, 24, 36]);
const SPACING_PROPERTIES = /^(padding|margin|gap|row-gap|column-gap)(-(top|right|bottom|left|inline|block))?$/;
const COLOR_LITERAL = /(#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|oklch|oklab)\(|\b(?:white|black)\b)/;
const RAW_PX = /(?<![\w.-])-?\d*\.?\d+px\b/g;
const DECLARATION = /^\s*([a-z-]+)\s*:\s*([^;{}]+);?\s*(\/\*.*\*\/)?\s*$/;

const RULES = [
  {
    name: 'color',
    applies: (property) => !property.startsWith('--'),
    violates: (_property, value) => COLOR_LITERAL.test(value),
    hint: 'use a color token',
  },
  {
    name: 'font-size',
    applies: (property) => property === 'font-size',
    violates: (_property, value) => RAW_PX.test(value),
    hint: 'use a --font-size-* token',
  },
  {
    name: 'font-weight',
    applies: (property) => property === 'font-weight',
    violates: (_property, value) => /^\d+$/.test(value.trim()),
    hint: 'use a --font-weight-* token',
  },
  {
    name: 'font-family',
    applies: (property) => property === 'font-family',
    violates: (_property, value) => !/^(var\(|inherit$)/.test(value.trim()),
    hint: 'use a --font-family-* token',
  },
  {
    name: 'border-radius',
    applies: (property) => property === 'border-radius',
    violates: (_property, value) => RAW_PX.test(value),
    hint: 'use a --radius-* token',
  },
  {
    name: 'box-shadow',
    applies: (property) => property === 'box-shadow',
    violates: (_property, value) => !/^(var\(|none$)/.test(value.trim()),
    hint: 'use a --shadow-* token',
  },
  {
    name: 'spacing',
    applies: (property) => SPACING_PROPERTIES.test(property),
    violates: (_property, value) =>
      (value.match(RAW_PX) ?? []).some((literal) => SPACING_SCALE.has(Math.abs(parseFloat(literal)))),
    hint: 'use a --space-* token for a value on the spacing scale',
  },
];

function styleLines(path, text) {
  const lines = text.split('\n');
  if (!path.endsWith('.vue')) return lines.map((line, index) => ({ line, number: index + 1 }));
  const kept = [];
  let inStyle = false;
  lines.forEach((line, index) => {
    if (/^<style[\s>]/.test(line)) inStyle = true;
    else if (/^<\/style>/.test(line)) inStyle = false;
    else if (inStyle) kept.push({ line, number: index + 1 });
  });
  return kept;
}

/** Returns one entry per raw design value found in the given file text. */
export function findViolations(path, text) {
  const violations = [];
  for (const { line, number } of styleLines(path, text)) {
    if (line.includes('token-exempt:')) continue;
    const declaration = DECLARATION.exec(line);
    if (!declaration) continue;
    const [, property, value] = declaration;
    for (const rule of RULES) {
      RAW_PX.lastIndex = 0;
      if (rule.applies(property) && rule.violates(property, value)) {
        violations.push({ path, number, rule: rule.name, hint: rule.hint, line: line.trim() });
      }
    }
  }
  return violations;
}

function sourceFiles(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sourceFiles(path);
    return /\.(vue|css)$/.test(entry.name) ? [path] : [];
  });
}

function main() {
  const webRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));
  const generated = join(webRoot, 'src/styles/tokens.css');
  const violations = sourceFiles(join(webRoot, 'src'))
    .filter((path) => path !== generated)
    .flatMap((path) => findViolations(relative(webRoot, path), readFileSync(path, 'utf8')));

  for (const found of violations) {
    console.error(`${found.path}:${found.number}  [${found.rule}] ${found.line}\n    ${found.hint}`);
  }
  if (violations.length) {
    console.error(`\n${violations.length} raw design value(s). Use a token, or mark a one-off with token-exempt.`);
    process.exit(1);
  }
  console.log('Every design value in src/ comes from a token.');
}

if (process.argv[1] === fileURLToPath(import.meta.url)) main();
