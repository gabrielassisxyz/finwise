import { describe, expect, it } from 'vitest';
import { createFakeApi } from '@/api/fake';
import { ApiFailure } from '@/api/http/failure';
import { failingOne, type ApiMethodName } from './readOnly';

async function rejection(call: Promise<unknown>): Promise<unknown> {
  return call.then(
    () => 'resolved',
    (error: unknown) => error,
  );
}

describe('failingOne', () => {
  it('fails the one method named with the kind, the reason and the class it has in the contract', async () => {
    const api = failingOne(createFakeApi({ startLocked: false }), 'budget.save', 'refused', 'invalid_budget');

    const failure = await rejection(api.budget.save('2026-11', []));

    expect(failure).toBeInstanceOf(ApiFailure);
    const { kind, reason, methodClass } = failure as ApiFailure;
    expect({ kind, reason, methodClass }).toEqual({ kind: 'refused', reason: 'invalid_budget', methodClass: 'write' });
  });

  it('takes the class from the contract, whatever the method', async () => {
    const cases: [ApiMethodName, (api: ReturnType<typeof createFakeApi>) => Promise<unknown>, string][] = [
      ['home.summary', (api) => api.home.summary('2026-11'), 'read'],
      ['session.status', (api) => api.session.status(), 'session'],
      ['budget.evaluateScenario', (api) => api.budget.evaluateScenario('2026-11', []), 'evaluate'],
    ];
    for (const [method, call, methodClass] of cases) {
      const failure = (await rejection(call(failingOne(createFakeApi(), method, 'network')))) as ApiFailure;
      expect([failure.kind, failure.methodClass, failure.reason], method).toEqual(['network', methodClass, null]);
    }
  });

  it('leaves every other method answering as the wrapped API does', async () => {
    const real = createFakeApi({ startLocked: false });
    const api = failingOne(createFakeApi({ startLocked: false }), 'budget.save', 'internal');

    expect(await api.budget.view('2026-11')).toEqual(await real.budget.view('2026-11'));
    expect(await api.budget.document('2026-11')).toEqual(await real.budget.document('2026-11'));
    expect(await api.session.status()).toEqual(await real.session.status());
    await expect(api.transactions.setNote('t2', 'uma nota')).resolves.not.toBeInstanceOf(ApiFailure);
  });

  it('refuses a name the contract does not have, instead of failing nothing', () => {
    expect(() => failingOne(createFakeApi(), 'budget.delete' as ApiMethodName, 'internal')).toThrow(/budget\.delete/);
    expect(() => failingOne(createFakeApi(), 'nowhere.save' as ApiMethodName, 'internal')).toThrow(/nowhere\.save/);
    // @ts-expect-error a method the contract does not have does not compile
    expect(() => failingOne(createFakeApi(), 'budget.delete', 'internal')).toThrow();
  });
});
