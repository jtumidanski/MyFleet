import { describe, it, expect, vi, afterEach } from 'vitest';
import {
  DEFAULT_RUNTIME_CONFIG,
  getRuntimeConfig,
  loadRuntimeConfig,
  parseRuntimeConfig,
  subscribeRuntimeConfig,
} from './runtimeConfig';

const CUSTOM = 'https://example.test/report?vin={vin}';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// parseRuntimeConfig is pure, so it is tested without touching fetch or the
// module's latched state.
describe('parseRuntimeConfig', () => {
  it('accepts a valid document', () => {
    expect(parseRuntimeConfig({ carfaxUrlTemplate: CUSTOM })).toEqual({
      carfaxUrlTemplate: CUSTOM,
    });
  });

  it('falls back per field rather than discarding the document', () => {
    expect(parseRuntimeConfig({ carfaxUrlTemplate: 42 })).toEqual(DEFAULT_RUNTIME_CONFIG);
    expect(parseRuntimeConfig({ carfaxUrlTemplate: '' })).toEqual(DEFAULT_RUNTIME_CONFIG);
    expect(parseRuntimeConfig({})).toEqual(DEFAULT_RUNTIME_CONFIG);
  });

  it('ignores unknown keys', () => {
    expect(parseRuntimeConfig({ carfaxUrlTemplate: CUSTOM, somethingElse: true })).toEqual({
      carfaxUrlTemplate: CUSTOM,
    });
  });

  it('never throws on a non-object', () => {
    expect(parseRuntimeConfig(null)).toEqual(DEFAULT_RUNTIME_CONFIG);
    expect(parseRuntimeConfig('nope')).toEqual(DEFAULT_RUNTIME_CONFIG);
    expect(parseRuntimeConfig(undefined)).toEqual(DEFAULT_RUNTIME_CONFIG);
  });
});

describe('loadRuntimeConfig', () => {
  it('latches a served document so getRuntimeConfig returns it', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () => new Response(JSON.stringify({ carfaxUrlTemplate: CUSTOM }), { status: 200 }),
      ),
    );

    await expect(loadRuntimeConfig()).resolves.toEqual({ carfaxUrlTemplate: CUSTOM });
    expect(getRuntimeConfig()).toEqual({ carfaxUrlTemplate: CUSTOM });
  });

  // Each of these must resolve to the defaults, never reject: a config failure
  // must not be able to stop the app from rendering.
  it('falls back to defaults on a 404 (no ConfigMap, older image)', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('not found', { status: 404 })),
    );
    vi.spyOn(console, 'warn').mockImplementation(() => {});

    await expect(loadRuntimeConfig()).resolves.toEqual(DEFAULT_RUNTIME_CONFIG);
    expect(getRuntimeConfig()).toEqual(DEFAULT_RUNTIME_CONFIG);
  });

  it('falls back to defaults on a network error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new TypeError('Failed to fetch');
      }),
    );
    vi.spyOn(console, 'warn').mockImplementation(() => {});

    await expect(loadRuntimeConfig()).resolves.toEqual(DEFAULT_RUNTIME_CONFIG);
  });

  it('falls back to defaults on malformed JSON', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('{ not json', { status: 200 })),
    );
    vi.spyOn(console, 'warn').mockImplementation(() => {});

    await expect(loadRuntimeConfig()).resolves.toEqual(DEFAULT_RUNTIME_CONFIG);
  });

  it('notifies subscribers so a tree mounted before the fetch resolves updates', async () => {
    // main.tsx renders first and loads the config after, so a latch that nobody
    // is told about is a config that silently never takes effect.
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () => new Response(JSON.stringify({ carfaxUrlTemplate: CUSTOM }), { status: 200 }),
      ),
    );
    const seen: string[] = [];
    const unsubscribe = subscribeRuntimeConfig(() => {
      seen.push(getRuntimeConfig().carfaxUrlTemplate);
    });

    await loadRuntimeConfig();
    expect(seen).toEqual([CUSTOM]);

    unsubscribe();
    await loadRuntimeConfig();
    // Still one entry: unsubscribe must actually detach, or a long-lived page
    // accumulates listeners for every unmounted card.
    expect(seen).toEqual([CUSTOM]);
  });

  it('notifies subscribers on the failure path too', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('not found', { status: 404 })),
    );
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    const onChange = vi.fn();
    const unsubscribe = subscribeRuntimeConfig(onChange);

    await loadRuntimeConfig();

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(getRuntimeConfig()).toEqual(DEFAULT_RUNTIME_CONFIG);
    unsubscribe();
  });

  it('logs the failure through the project error helper, and still never rejects', async () => {
    // console.warn on a raw unknown is what this replaced; createErrorFromUnknown
    // is the one place that normalises a network TypeError, a JSON:API envelope,
    // and a bare string into the same readable message.
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new TypeError('Failed to fetch');
      }),
    );
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});

    await expect(loadRuntimeConfig()).resolves.toEqual(DEFAULT_RUNTIME_CONFIG);

    expect(warn).toHaveBeenCalledTimes(1);
    expect(warn.mock.calls[0]?.[1]).toBe('Failed to fetch');
  });

  it('gives up rather than hanging when the request never settles', async () => {
    // A wedged server would otherwise leave the app unmounted forever. The
    // abort signal is what breaks the deadlock.
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_url: string, init?: RequestInit) =>
          new Promise<Response>((_resolve, reject) => {
            init?.signal?.addEventListener('abort', () => reject(new Error('aborted')));
          }),
      ),
    );
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    vi.useFakeTimers();

    const pending = loadRuntimeConfig();
    await vi.advanceTimersByTimeAsync(2000);
    await expect(pending).resolves.toEqual(DEFAULT_RUNTIME_CONFIG);

    vi.useRealTimers();
  });
});
