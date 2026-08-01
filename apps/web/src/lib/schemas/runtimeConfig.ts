import { z } from 'zod';

/**
 * Compiled into the bundle, so the app works with no ConfigMap at all: local
 * `vite dev`, a bare `docker run` of the image, and any overlay that has not
 * adopted the ConfigMap.
 *
 * This string is duplicated in `apps/web/public/config/config.json` and
 * `deploy/k8s/base/web/configmap.yaml`; `tools/check-carfax-template.sh` fails
 * CI if the three ever drift.
 */
export const DEFAULT_CARFAX_URL_TEMPLATE =
  'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}';

/**
 * Configuration the SPA reads at runtime rather than at build time.
 *
 * `.catch()` per field, then once more on the object, is what makes a partially
 * broken document degrade rather than being discarded: one malformed key falls
 * back on its own while the rest of the document is honoured. Unknown keys are
 * stripped by zod's default object behaviour.
 */
export const runtimeConfigSchema = z
  .object({
    carfaxUrlTemplate: z.string().min(1).catch(DEFAULT_CARFAX_URL_TEMPLATE),
  })
  .catch({ carfaxUrlTemplate: DEFAULT_CARFAX_URL_TEMPLATE });

/**
 * Derived from the schema rather than hand-written, so a key added above is a
 * type error at every consumer instead of silently invisible to them.
 */
export type RuntimeConfig = z.infer<typeof runtimeConfigSchema>;
