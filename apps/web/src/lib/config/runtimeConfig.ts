import { z } from 'zod';

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
 */
export interface RuntimeConfig {
  carfaxUrlTemplate: string;
}

/**
 * Compiled into the bundle, so the app works with no ConfigMap at all: local
 * `vite dev`, a bare `docker run` of the image, and any overlay that has not
 * adopted the ConfigMap.
 */
export const DEFAULT_RUNTIME_CONFIG: RuntimeConfig = {
  carfaxUrlTemplate: 'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}',
};

const RUNTIME_CONFIG_URL = '/config/config.json';

/**
 * Without a bound, a wedged nginx worker turns a missing 60-byte file into a
 * permanent white screen, because nothing renders until this settles.
 */
const FETCH_TIMEOUT_MS = 2000;

// `.catch()` per field, then once more on the object, is what makes a partially
// broken document degrade rather than being discarded: one malformed key falls
// back on its own while the rest of the document is honoured. Unknown keys are
// stripped by zod's default object behaviour.
const runtimeConfigSchema = z
  .object({
    carfaxUrlTemplate: z.string().min(1).catch(DEFAULT_RUNTIME_CONFIG.carfaxUrlTemplate),
  })
  .catch(DEFAULT_RUNTIME_CONFIG);

/** Validates one parsed document, falling back per field. Never throws. */
export function parseRuntimeConfig(raw: unknown): RuntimeConfig {
  return runtimeConfigSchema.parse(raw);
}

let latched: RuntimeConfig = DEFAULT_RUNTIME_CONFIG;

/**
 * The latched config. Returns the compiled-in defaults until loadRuntimeConfig
 * has resolved, which is why the defaults must be a usable value rather than
 * placeholders.
 */
export function getRuntimeConfig(): RuntimeConfig {
  return latched;
}

/**
 * Fetches, parses, and latches the config. Never throws and never rejects —
 * every failure (404, network error, malformed body, hung request) resolves to
 * DEFAULT_RUNTIME_CONFIG and logs a single warning. Nothing about a config
 * failure may prevent the app from rendering.
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
    latched = parseRuntimeConfig(await res.json());
  } catch (err) {
    console.warn('[runtime-config] using built-in defaults:', err);
    latched = DEFAULT_RUNTIME_CONFIG;
  } finally {
    clearTimeout(timer);
  }
  return latched;
}
