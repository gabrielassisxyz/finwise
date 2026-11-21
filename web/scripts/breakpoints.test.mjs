// @vitest-environment node
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// jsdom evaluates no media query, so the breakpoints are checked in the stylesheet itself.
const css = readFileSync(new URL('../src/styles/base.css', import.meta.url), 'utf8');

describe('the breakpoints in base.css', () => {
  it('hides the secondary columns below 1200px and the subtitle from 1200px', () => {
    expect(css).toMatch(/@media \(max-width: 1199px\)\s*{\s*\.wide-only\s*{\s*display: none !important/);
    expect(css).toMatch(/@media \(min-width: 1200px\)\s*{\s*\.narrow-only\s*{\s*display: none !important/);
  });

  it('turns a table row into a two-column card at 820px', () => {
    const phone = css.slice(css.indexOf('@media (max-width: 820px) {\n  .split-columns'));
    expect(phone).toMatch(/\.stack-row\s*{[^}]*display: grid !important/);
    expect(phone).toMatch(/\.stack-row\s*{[^}]*grid-template-columns: 1fr 1fr !important/);
    expect(phone).toMatch(/\.stack-row > \[data-label\]::before\s*{\s*content: attr\(data-label\)/);
    expect(phone).toMatch(/\.split-columns\s*{\s*grid-template-columns: minmax\(0, 1fr\);/);
  });
});
