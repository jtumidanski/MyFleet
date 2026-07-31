import { createErrorFromUnknown } from '@myfleet/shared-ts';
import {
  DEFAULT_CARFAX_URL_TEMPLATE,
  runtimeConfigSchema,
  type RuntimeConfig,
} from '../schemas/runtimeConfig';

export type { RuntimeConfig };

/**
 * Configuration the SPA reads at runtime rather than at build time.
 *
 * Vite inlines `import.meta.env` into the bundle, so anything configured that
 * way can only change by rebuilding and republishing the web image. This module
 * fetches a small JSON document instead, served by the SPA's own nginx and
 * backed by a ConfigMap in Kubernetes, so a value can be edited with
 * `kubectl apply` alone.
 *
 * It is deliberately built as a general facility with one key today rather than
 * as a Carfax special case, so the second key does not require redesigning it.
 *
 * The module is a plain observable store, deliberately free of React: the
 * Carfax URL builder that consumes it is a pure function, and `useRuntimeConfig`
 * (lib/hooks/useRuntimeConfig.ts) is the only thing that knows about React.
 */

/**
 * Compiled into the bundle, so the app works with no ConfigMap at all: local
 * `vite dev`, a bare `docker run` of the image, and any overlay that has not
 * adopted the ConfigMap.
 */
export const DEFAULT_RUNTIME_CONFIG: RuntimeConfig = {
  carfaxUrlTemplate: DEFAULT_CARFAX_URL_TEMPLATE,
};

const RUNTIME_CONFIG_URL = '/config/config.json';

/**
 * Without a bound, a wedged nginx worker leaves the config pending forever. The
 * app renders regardless (main.tsx no longer waits on it), so this only bounds
 * how long the compiled-in defaults stay in force — but an unbounded fetch also
 * never releases its connection, so the ceiling stays.
 */
const FETCH_TIMEOUT_MS = 2000;

/** Validates one parsed document, falling back per field. Never throws. */
export function parseRuntimeConfig(raw: unknown): RuntimeConfig {
  return runtimeConfigSchema.parse(raw);
}

let latched: RuntimeConfig = DEFAULT_RUNTIME_CONFIG;
const listeners = new Set<() => void>();

/**
 * The latched config. Returns the compiled-in defaults until loadRuntimeConfig
 * has resolved, which is why the defaults must be a usable value rather than
 * placeholders.
 *
 * The returned object is referentially stable until the config actually changes,
 * which is what makes it a valid `useSyncExternalStore` snapshot.
 */
export function getRuntimeConfig(): RuntimeConfig {
  return latched;
}

/**
 * Subscribes to config changes; returns the unsubscribe function.
 *
 * This exists because the tree now mounts BEFORE the config arrives. Without a
 * notification, every component that rendered during that window would hold the
 * compiled-in default forever and a ConfigMap override would silently never take
 * effect — a quieter and worse bug than the blank page this replaced.
 */
export function subscribeRuntimeConfig(onChange: () => void): () => void {
  listeners.add(onChange);
  return () => {
    listeners.delete(onChange);
  };
}

function latch(next: RuntimeConfig): void {
  latched = next;
  for (const listener of listeners) {
    listener();
  }
}

/**
 * Fetches, parses, and latches the config. Never throws and never rejects —
 * every failure (404, network error, malformed body, hung request) resolves to
 * DEFAULT_RUNTIME_CONFIG and logs a single warning. Nothing about a config
 * failure may prevent the app from rendering, and callers rely on being able to
 * `void` the returned promise without an unhandled rejection.
 */
export async function loadRuntimeConfig(): Promise<RuntimeConfig> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);
  try {
    const res = await fetch(RUNTIME_CONFIG_URL, {
      signal: controller.signal,
      cache: 'no-store',
    });
    if (!res.ok) {
      throw new Error(`config request returned ${res.status}`);
    }
    latch(parseRuntimeConfig(await res.json()));
  } catch (err) {
    // createErrorFromUnknown is the project's one normalisation point for an
    // unknown throwable, so a network TypeError, an ApiError-shaped object, and
    // a plain string all surface the same readable message.
    const apiError = createErrorFromUnknown(err);
    console.warn('[runtime-config] using built-in defaults:', apiError.message);
    latch(DEFAULT_RUNTIME_CONFIG);
  } finally {
    clearTimeout(timer);
  }
  return latched;
}
