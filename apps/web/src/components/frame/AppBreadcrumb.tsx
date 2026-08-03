import { Fragment } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { cn } from '../../lib/utils';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '../ui/breadcrumb';
import { resolveTrail, type Crumb } from './breadcrumbTrails';
import { FleetNameCrumb } from './crumbs/FleetNameCrumb';
import { VehicleNameCrumb } from './crumbs/VehicleNameCrumb';

function CrumbLabel({ crumb, id }: { crumb: Crumb; id: string | undefined }) {
  if (crumb.kind === 'static') return <>{crumb.label}</>;
  // Unreachable: resolveTrail only returns a dynamic crumb when the matched
  // pattern captured `:id`. The guard is what keeps the types honest.
  if (!id) return null;
  return crumb.kind === 'vehicle' ? <VehicleNameCrumb id={id} /> : <FleetNameCrumb id={id} />;
}

/**
 * The trail across the top of both shells (FR-CRUMB-1).
 *
 * Overflow is handled by truncating each crumb rather than collapsing the
 * middle to an ellipsis (open question 3): the longest real trail is three
 * crumbs, so BreadcrumbEllipsis would be machinery for a case that does not
 * occur — the vendored primitive keeps it available if a deeper route lands.
 * Below `sm` the intermediate crumbs and every separator hide, so a phone shows
 * only the current page beside the trigger. That hiding is CSS-only, so the
 * full trail stays in the DOM at every width and FR-CRUMB-4's "renders its
 * exact trail" stays honest under jsdom.
 */
export function AppBreadcrumb() {
  const { pathname } = useLocation();
  const resolved = resolveTrail(pathname);
  if (!resolved) return null;

  const { crumbs, id } = resolved;
  const lastIndex = crumbs.length - 1;

  return (
    <Breadcrumb className="min-w-0">
      <BreadcrumbList className="flex-nowrap">
        {crumbs.map((crumb, index) => {
          const isLast = index === lastIndex;
          return (
            <Fragment key={crumb.kind === 'static' ? crumb.to : crumb.kind}>
              {index > 0 && <BreadcrumbSeparator className="hidden sm:flex" />}
              <BreadcrumbItem className={cn(!isLast && 'hidden sm:inline-flex')}>
                {isLast || crumb.kind !== 'static' ? (
                  <BreadcrumbPage className="max-w-[12rem] truncate">
                    <CrumbLabel crumb={crumb} id={id} />
                  </BreadcrumbPage>
                ) : (
                  <BreadcrumbLink asChild className="max-w-[12rem] truncate">
                    <Link to={crumb.to}>{crumb.label}</Link>
                  </BreadcrumbLink>
                )}
              </BreadcrumbItem>
            </Fragment>
          );
        })}
      </BreadcrumbList>
    </Breadcrumb>
  );
}
