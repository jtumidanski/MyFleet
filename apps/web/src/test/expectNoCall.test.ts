import { describe, it, expect, vi } from 'vitest';
import { expectNoCall, expectNoCallWith, flushPending } from './expectNoCall';

// This file is exempted from the no-restricted-syntax rule in eslint.config.js
// precisely so it can contain the bare form below: the contrast between the
// bare assertion and the helper IS the thing under test.

describe('flushPending', () => {
  it('drains microtasks and one macrotask tick, in that order', async () => {
    const order: string[] = [];
    void Promise.resolve().then(() => order.push('microtask'));
    setTimeout(() => order.push('macrotask'), 0);

    await flushPending();

    expect(order).toEqual(['microtask', 'macrotask']);
  });
});

describe('expectNoCall', () => {
  // The whole bug class in four lines: React Query dispatches both queries and
  // mutations from a promise continuation, so a synchronous read sees zero.
  it('catches a promise-continuation call that a bare synchronous assertion misses', async () => {
    const spy = vi.fn();
    void Promise.resolve().then(() => spy());

    // Vacuously green — this is the form the lint rule exists to ban.
    expect(spy).not.toHaveBeenCalled();

    await expect(expectNoCall(spy, 'deferredSpy')).rejects.toThrow(/deferredSpy/);
  });

  it('passes when the spy is genuinely never called', async () => {
    const spy = vi.fn();
    await expectNoCall(spy, 'unusedSpy');
  });

  it('names the spy and reports the call count in the failure', async () => {
    const spy = vi.fn();
    spy();
    spy();
    spy();

    await expect(expectNoCall(spy, 'mediaService.getContentBlob')).rejects.toThrow(
      /mediaService\.getContentBlob.*3 times/s,
    );
  });

  it('works with no label', async () => {
    const spy = vi.fn();
    await expectNoCall(spy);
  });
});

describe('expectNoCallWith', () => {
  it('catches a deferred call carrying the banned arguments', async () => {
    const spy = vi.fn();
    void Promise.resolve().then(() => spy('banned'));

    // Vacuously green for the same reason.
    expect(spy).not.toHaveBeenCalledWith('banned');

    await expect(expectNoCallWith(spy, ['banned'], 'onChange')).rejects.toThrow(/onChange/);
  });

  it('passes when the spy was called with different arguments', async () => {
    const spy = vi.fn();
    void Promise.resolve().then(() => spy('allowed'));

    await expectNoCallWith(spy, ['banned'], 'onChange');
  });
});
