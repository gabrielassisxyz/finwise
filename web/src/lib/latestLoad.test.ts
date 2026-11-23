import { describe, expect, it } from 'vitest';
import { createLatestLoad } from './latestLoad';

describe('createLatestLoad', () => {
  it('keeps a load as the latest until another starts', () => {
    const loads = createLatestLoad();
    const first = loads.start();
    expect(first()).toBe(true);
    expect(first()).toBe(true);
  });

  it('tells an older load it is no longer the latest when two loads answer last first', async () => {
    const loads = createLatestLoad();
    const landed: string[] = [];

    let answerFirst!: (value: string) => void;
    let answerSecond!: (value: string) => void;

    async function load(pending: Promise<string>): Promise<void> {
      const isLatest = loads.start();
      const value = await pending;
      if (isLatest()) landed.push(value);
    }

    const first = load(new Promise<string>((resolve) => (answerFirst = resolve)));
    const second = load(new Promise<string>((resolve) => (answerSecond = resolve)));

    answerSecond('newer');
    await second;
    answerFirst('older');
    await first;

    expect(landed).toEqual(['newer']);
  });

  it('treats a new key as a new load, and drops an in-flight load when the key is cleared', async () => {
    const loads = createLatestLoad();
    const landed: string[] = [];

    let answerFirst!: (value: string) => void;
    let answerSecond!: (value: string) => void;
    let answerThird!: (value: string) => void;

    async function loadFor(key: string | null, pending?: Promise<string>): Promise<void> {
      if (key === null) {
        loads.cancel();
        return;
      }
      const isLatest = loads.start();
      const value = await pending!;
      if (isLatest()) landed.push(`${key}:${value}`);
    }

    const forA = loadFor('2026-10', new Promise<string>((resolve) => (answerFirst = resolve)));
    const forB = loadFor('2026-11', new Promise<string>((resolve) => (answerSecond = resolve)));

    answerSecond('nov');
    await forB;
    answerFirst('oct');
    await forA;
    expect(landed).toEqual(['2026-11:nov']);

    const forC = loadFor('2026-12', new Promise<string>((resolve) => (answerThird = resolve)));
    await loadFor(null);
    answerThird('dec');
    await forC;
    expect(landed).toEqual(['2026-11:nov']);
  });

  it('keeps checks valid across chained steps until a newer load starts', () => {
    const loads = createLatestLoad();
    const first = loads.start();
    expect(first()).toBe(true);

    const second = loads.start();
    expect(first()).toBe(false);
    expect(second()).toBe(true);
  });

  it('keeps separate guards independent', () => {
    const detail = createLatestLoad();
    const lines = createLatestLoad();

    const firstDetail = detail.start();
    const firstLines = lines.start();
    const secondLines = lines.start();

    expect(firstDetail()).toBe(true);
    expect(firstLines()).toBe(false);
    expect(secondLines()).toBe(true);
  });
});
