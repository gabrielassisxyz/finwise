// @vitest-environment node
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import config from '../vite.config.ts';
import { filesContaining } from './check-bundle.mjs';

const resolved = (command, mode) => config({ command, mode });
const standIn = (command, mode) => JSON.parse(resolved(command, mode).define.__FINWISE_STAND_IN__);

describe('which API a build provides', () => {
  it('builds for real use with the HTTP client', () => {
    expect(standIn('build', 'production')).toBe(false);
  });

  it('builds the prototype mode with the stand-in', () => {
    expect(standIn('build', 'prototype')).toBe(true);
  });

  it('serves the dev server with the stand-in, and the backend mode with the HTTP client', () => {
    expect(standIn('serve', 'development')).toBe(true);
    expect(standIn('serve', 'backend')).toBe(false);
  });
});

describe('the dev proxy', () => {
  it('sends /api to the local backend in the backend mode only', () => {
    expect(resolved('serve', 'backend').server.proxy).toEqual({ '/api': 'http://127.0.0.1:8080' });
    expect(resolved('serve', 'development').server.proxy).toBeUndefined();
  });
});

describe('looking for text in built files', () => {
  it('finds the files at any depth that hold the text', () => {
    const dir = mkdtempSync(join(tmpdir(), 'finwise-scan-'));
    try {
      mkdirSync(join(dir, 'assets'));
      writeFileSync(join(dir, 'assets', 'a.js'), 'x = "needle"');
      writeFileSync(join(dir, 'b.js'), 'x = "hay"');
      expect(filesContaining(dir, 'needle')).toEqual([join(dir, 'assets', 'a.js')]);
      expect(filesContaining(dir, 'absent')).toEqual([]);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
