import { BRAND_MARK_PATH } from './brandMarkPath';

/**
 * The MyFleet mark, inline.
 *
 * `fill="currentColor"` means it inherits the surrounding text colour and needs
 * no dark variant (FR-ICON-9). Sizing comes from `className`; there are no
 * hardcoded pixel dimensions.
 *
 * aria-hidden because its only call site sits beside the visible "MyFleet"
 * wordmark, which already supplies the accessible name — a duplicate would make
 * screen readers announce the brand twice (FR-ICON-10). A future non-decorative
 * placement needs its own labelled wrapper.
 */
export function BrandMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
      <path d={BRAND_MARK_PATH} />
    </svg>
  );
}
