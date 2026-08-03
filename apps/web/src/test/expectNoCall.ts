// `act` MUST come from @testing-library/react, not from react. Importing
// @testing-library/react registers a `beforeAll` that sets
// IS_REACT_ACT_ENVIRONMENT = true (active here because vitest.config.ts sets
// `globals: true`, so `beforeAll`/`afterAll` are ambient), and RTL's `act`
// wrapper additionally sets the flag for the duration of each call it makes.
// Either mechanism alone would be enough to let this helper run inside
// pure-logic test files with no React root without emitting an act-scope
// warning; together they make it robust to environments where one of the two
// doesn't apply. React's own `act` does not set the flag and would warn. See
// task-019 design F-1.
import { act } from '@testing-library/react';
import { expect, vi } from 'vitest';
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
 * under fake timers and the await would hang until the test times out. Rather
 * than let that surface as a bare test-runner timeout with no indication of
 * the cause, this throws immediately when fake timers are active — assert
 * synchronously with an inline eslint-disable carrying probe evidence
 * instead, as download.test.ts does.
 */
export async function flushPending(): Promise<void> {
  if (vi.isFakeTimers()) {
    throw new Error(
      'expectNoCall/flushPending cannot run under vi.useFakeTimers(): its ' +
        'setTimeout(0) never fires. Assert synchronously with an inline ' +
        'eslint-disable carrying probe evidence, as download.test.ts does.',
    );
  }
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
 *
 * `mockName(label)` renames the spy itself, not just this assertion's output:
 * the name persists on the spy across `vi.clearAllMocks()`/`mockReset()` and
 * shows up in any later failure for the same instance. Give a single spy only
 * one label per test.
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
