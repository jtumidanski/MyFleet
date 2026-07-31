/** The only placeholder a Carfax URL template may use. */
export const VIN_PLACEHOLDER = '{vin}';

/**
 * Builds a Carfax report URL from a template, or returns null when no button
 * should be rendered at all.
 *
 * null is returned when:
 *  - the VIN is missing or empty after trimming — there is nothing to look up;
 *  - the template does not contain {vin} — it would send every user to the same
 *    generic page, and failing closed is the correct reading of "the button
 *    opens THIS vehicle's report";
 *  - the constructed URL does not parse, or its scheme is not https:. The
 *    template is runtime configuration, so whoever can edit the ConfigMap
 *    chooses what an anchor's href points at; a javascript: template would
 *    otherwise be stored XSS, and permitting http: would let a config change
 *    silently downgrade a link that carries a VIN.
 *
 * Pure: no React, no config import. The template arrives as an argument so this
 * stays directly unit-testable.
 */
export function buildCarfaxUrl(template: string, vin: string | null | undefined): string | null {
  const trimmed = vin?.trim() ?? '';
  if (!trimmed) return null;
  if (!template.includes(VIN_PLACEHOLDER)) return null;

  // split/join replaces every occurrence without needing a regex, so a template
  // that uses the placeholder twice (path and query) works.
  const url = template.split(VIN_PLACEHOLDER).join(encodeURIComponent(trimmed));

  try {
    if (new URL(url).protocol !== 'https:') return null;
  } catch {
    return null;
  }
  return url;
}
