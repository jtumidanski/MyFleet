/**
 * PageHeader — the single owner of page-title markup for every authenticated
 * page (task-015).
 *
 * Renders ONLY the header row. It does not render the page container and does
 * not own the gap to the content below it (FR-5): the page's own
 * `space-y-6` container supplies that, so one class governs the whole vertical
 * rhythm instead of the header and the container each having an opinion.
 *
 * App-shell furniture, not a design-system primitive — it encodes this app's
 * h1 level, its type scale and the 24px rhythm it participates in, so it
 * deliberately stays out of packages/ui-components (PRD §7).
 */
import type { ReactNode } from 'react';
import { cn } from '../lib/utils';

export interface PageHeaderProps {
  /** Page title. Rendered as the page's sole <h1>. */
  title: string;
  /**
   * Inline element sitting immediately right of the title (e.g. a StatusBadge).
   * Deliberately a separate slot rather than widening `title` to ReactNode: a
   * badge inside the <h1> would become part of the heading's accessible name.
   */
  titleAdornment?: ReactNode;
  /** Secondary line beneath the title, muted. */
  description?: ReactNode;
  /**
   * Right-aligned controls on the title row. Taller than the title line box
   * by design (a Button is 36-40px against a 32px line); the slot absorbs the
   * difference so the title lands in the same place on every page.
   */
  actions?: ReactNode;
  /** Per-page escape hatch; merged via cn() so a caller can override. */
  className?: string;
}

export function PageHeader({
  title,
  titleAdornment,
  description,
  actions,
  className,
}: PageHeaderProps) {
  return (
    <div
      className={cn(
        'flex justify-between gap-2',
        // A single-line title against a 36px Button needs items-center or the
        // button rides ~2px high. A title-plus-description block needs
        // items-start. Derived, so no caller has to know.
        description ? 'items-start' : 'items-center',
        className,
      )}
    >
      {/* min-w-0 + shrink-0 below are what actually stop a long title from
          colliding with the actions — gap-2 alone only guarantees the gap once
          flex has decided which child shrinks. */}
      <div className="min-w-0">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">{title}</h1>
          {titleAdornment}
        </div>
        {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
      </div>
      {actions && (
        // -my-1 is what keeps a page with actions vertically identical to one
        // without. The h1's line box is 32px; a default Button is 40px and a
        // size="sm" one 36px, so as a plain flex item the tallest action sets
        // the row height — pushing the title down 4px (Vehicles) or 2px
        // (Dashboard) and widening the gap to the content below by the same
        // amount, while Activity/Notifications/Settings stayed at 32px.
        // Shrinking the actions' margin box by 4px top and bottom hands the
        // row height back to the title on both sides; the buttons simply
        // overhang the 32px line box symmetrically, which is where an
        // optically-centred control belongs anyway.
        <div className="-my-1 flex shrink-0 items-center gap-2">{actions}</div>
      )}
    </div>
  );
}
