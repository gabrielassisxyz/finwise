import { describe, expect, it } from 'vitest';
import { findViolations } from './check-token-usage.mjs';

const rulesFound = (css) => findViolations('sample.css', css).map((violation) => violation.rule);

describe('findViolations', () => {
  it('accepts declarations that only reference tokens', () => {
    const css = `
.card {
  color: var(--color-text-ink);
  padding: var(--space-16) var(--space-20);
  border-radius: var(--radius-card);
  font-size: var(--font-size-body);
  box-shadow: var(--shadow-popover);
}`;
    expect(rulesFound(css)).toEqual([]);
  });

  it('flags a color literal in any notation', () => {
    expect(rulesFound('  color: oklch(0.5 0.1 155);')).toEqual(['color']);
    expect(rulesFound('  background: #fff;')).toEqual(['color']);
    expect(rulesFound('  border: 1px solid white;')).toEqual(['color']);
  });

  it('flags raw type, radius and shadow values', () => {
    expect(rulesFound('  font-size: 14px;')).toEqual(['font-size']);
    expect(rulesFound('  font-weight: 600;')).toEqual(['font-weight']);
    expect(rulesFound('  border-radius: 10px;')).toEqual(['border-radius']);
    expect(rulesFound('  box-shadow: 0 1px 2px var(--color-text-ink);')).toEqual(['box-shadow']);
  });

  it('flags spacing only when the value is on the scale', () => {
    expect(rulesFound('  padding: 8px 22px;')).toEqual(['spacing']);
    expect(rulesFound('  gap: 7px;')).toEqual([]);
    expect(rulesFound('  margin-right: -8px;')).toEqual(['spacing']);
  });

  it('leaves layout dimensions alone', () => {
    expect(rulesFound('  width: 8px;\n  height: 36px;\n  border-radius: 50%;')).toEqual([]);
  });

  it('honours an explicit exemption', () => {
    expect(rulesFound('  font-size: 21px; /* token-exempt: logo glyph */')).toEqual([]);
  });

  it('only reads the style block of a component', () => {
    const component = `<template>
  <div style="color: red">font-size: 14px;</div>
</template>

<style scoped>
.a {
  font-size: 14px;
}
</style>`;
    const violations = findViolations('A.vue', component);
    expect(violations.map((violation) => [violation.rule, violation.number])).toEqual([['font-size', 7]]);
  });
});
