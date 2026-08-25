import type { VehicleAttributes } from '../../types/models/vehicle';

/**
 * A vehicle's display title: the nickname the user set, otherwise
 * "<year> <make> <model>".
 *
 * `||` not `??`: fleet-service marshals an unset nickname as "" (Attributes are
 * plain Go strings), so `??` would let the empty string through and render a
 * blank title — the same trap displayName.ts documents. `.trim()` extends the
 * check to a whitespace-only nickname, which is user-enterable.
 *
 * The body is VehicleCard's previous expression verbatim, trailing `.trim()`
 * included, so its rendered output is unchanged for every input.
 */
export function vehicleTitle(attributes: VehicleAttributes): string {
  return (
    attributes.nickname?.trim() ||
    `${attributes.year} ${attributes.make} ${attributes.model}`.trim()
  );
}
