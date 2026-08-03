// `act` MUST come from @testing-library/react, not from react. RTL sets
// IS_REACT_ACT_ENVIRONMENT = true at import time (not at first render), which
// is what lets this helper run inside pure-logic test files with no React root
// without emitting an act-scope warning. React's own `act` does not set the
// flag and would warn. See task-019 design F-1.
import { act } from '@testing-library/react';
import { expect } from 'vitest';
import type { MockInstance } from 'vitest';

/**
 * Drains the microtask queue and one macrotask tick.
 *
 * React Query dispatches both queries and mutations from a promise
 * continuation, so an assertion placed before this flush reads a spy that
 * could not yet have been called — it passes whether or not the guard it
 * exists to prove actually works. See issue #22.
 *
 * NOT compatible with vi.useFakeTimers(): the setTimeout below never fires
 * under fake timers and the await would hang until the test times out.
 */
export async function flushPending(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

/**
 * Flushes pending work, then asserts the spy was never called.
 *
 * `label` names the spy in the failure message — Vitest reports an unnamed
 * vi.fn() as the literal string "spy", which tells a reader nothing about
 * which of four toast variants fired.
 */
export async function expectNoCall(spy: MockInstance, label?: string): Promise<void> {
  await flushPending();
  if (label) spy.mockName(label);
  expect(spy).not.toHaveBeenCalled();
}

/**
 * Flushes pending work, then asserts the spy was never called with `args`.
 *
 * Note this assertion is also satisfied when the spy was never called at all,
 * which a flush cannot fix. Where the intent is "called, but not with X", pair
 * this with a positive assertion at the call site.
 */
export async function expectNoCallWith(
  spy: MockInstance,
  args: unknown[],
  label?: string,
): Promise<void> {
  await flushPending();
  if (label) spy.mockName(label);
  expect(spy).not.toHaveBeenCalledWith(...args);
}
