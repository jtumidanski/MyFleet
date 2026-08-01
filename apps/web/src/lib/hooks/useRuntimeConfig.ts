import { useSyncExternalStore } from 'react';
import {
  getRuntimeConfig,
  subscribeRuntimeConfig,
  type RuntimeConfig,
} from '../config/runtimeConfig';

/**
 * The runtime config, re-rendering the caller when it lands.
 *
 * `main.tsx` mounts the tree immediately and lets the config fetch resolve
 * afterwards, so a component that renders in that window would otherwise be
 * stuck with the compiled-in defaults and a ConfigMap override would never take
 * effect. `useSyncExternalStore` over the module's latch is the whole mechanism:
 * no context provider, no prop drilling, and the store itself stays a plain
 * module so the pure Carfax URL builder can keep reading it directly.
 *
 * `getRuntimeConfig` doubles as the server snapshot — the value is a compiled-in
 * constant until a fetch that only ever runs in a browser resolves, so the two
 * cannot disagree.
 */
export function useRuntimeConfig(): RuntimeConfig {
  return useSyncExternalStore(subscribeRuntimeConfig, getRuntimeConfig, getRuntimeConfig);
}
