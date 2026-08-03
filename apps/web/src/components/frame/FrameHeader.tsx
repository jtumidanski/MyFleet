import { ThemeToggle } from '../ThemeToggle';
import { SidebarTrigger } from '../ui/sidebar';
import { AppBreadcrumb } from './AppBreadcrumb';
import { ProfileMenu } from './ProfileMenu';

/**
 * The header row both shells render (FR-HEADER-1).
 *
 * Propless on purpose: every input it needs is ambient context both shells
 * already provide — the location (via AppBreadcrumb) and the user (via
 * ProfileMenu → useAuth). Threading them through would only give callers the
 * chance to pass something different.
 *
 * h-14 is fixed rather than shadcn's h-16 shrinking to h-12 on collapse: a
 * header that changes height when the sidebar collapses moves every page's
 * content up and down (FR-HEADER-2). px-6 keeps the header's content aligned
 * with <main>'s p-6 (FR-HEADER-4).
 */
export function FrameHeader() {
  return (
    <header className="flex h-14 shrink-0 items-center gap-2 border-b border-border px-6">
      <SidebarTrigger />
      <AppBreadcrumb />
      <div className="ml-auto flex items-center gap-2">
        <ThemeToggle />
        <ProfileMenu />
      </div>
    </header>
  );
}
