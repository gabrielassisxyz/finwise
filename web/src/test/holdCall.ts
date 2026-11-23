import { vi } from 'vitest';

/**
 * Intercepts an async method on a fake API domain so its next call runs right away against the current state,
 * holds the answer until `release()` is called, and lets every later call answer immediately.
 */
export function holdFirstCall<T extends object, K extends keyof T>(
  domain: T,
  method: K & (T[K] extends (...args: never[]) => Promise<unknown> ? K : never),
): { release: () => void } {
  const original = (domain[method] as (...args: unknown[]) => Promise<unknown>).bind(domain);
  let calls = 0;
  let release = (): void => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });

  vi.spyOn(domain, method as never).mockImplementation(((...args: unknown[]) => {
    calls += 1;
    const answer = original(...args);
    if (calls !== 1) return answer;
    return answer.then(async (value) => {
      await gate;
      return value;
    });
  }) as never);

  return { release: () => release() };
}
