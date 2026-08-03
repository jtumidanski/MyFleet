import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Sidebar, SidebarInset, SidebarProvider, SidebarTrigger, useSidebar } from './sidebar';

const COOKIE = 'sidebar_state';

function setCookie(value: string): void {
  document.cookie = `${COOKIE}=${value}; path=/`;
}

/** document.cookie persists across tests in a file (design §8.3). */
function clearCookie(): void {
  document.cookie = `${COOKIE}=; path=/; max-age=0`;
}

function StateProbe() {
  const { state } = useSidebar();
  return <span data-testid="state">{state}</span>;
}

function renderShell() {
  return render(
    <SidebarProvider>
      <Sidebar collapsible="icon">
        <StateProbe />
      </Sidebar>
      <SidebarInset>
        <SidebarTrigger />
      </SidebarInset>
    </SidebarProvider>,
  );
}

describe('SidebarProvider', () => {
  beforeEach(() => {
    clearCookie();
  });

  // FR-SIDEBAR-5: a fresh browser with no cookie starts expanded.
  it('starts expanded when no cookie is set', () => {
    renderShell();
    expect(screen.getByTestId('state')).toHaveTextContent('expanded');
  });

  // The deviation from upstream that makes FR-SIDEBAR-5 real: upstream writes
  // this cookie and never reads it, so without this the sidebar would reopen
  // expanded on every reload.
  it('starts collapsed when the cookie says so', () => {
    setCookie('false');
    renderShell();
    expect(screen.getByTestId('state')).toHaveTextContent('collapsed');
  });

  it('starts expanded when the cookie says so', () => {
    setCookie('true');
    renderShell();
    expect(screen.getByTestId('state')).toHaveTextContent('expanded');
  });

  it('ignores a cookie holding something that is not a boolean', () => {
    setCookie('banana');
    renderShell();
    expect(screen.getByTestId('state')).toHaveTextContent('expanded');
  });

  // An explicit prop still wins, so the component stays controllable.
  it('prefers an explicit defaultOpen over the cookie', () => {
    setCookie('true');
    render(
      <SidebarProvider defaultOpen={false}>
        <Sidebar collapsible="icon">
          <StateProbe />
        </Sidebar>
      </SidebarProvider>,
    );
    expect(screen.getByTestId('state')).toHaveTextContent('collapsed');
  });
});

describe('SidebarTrigger', () => {
  beforeEach(() => {
    clearCookie();
  });

  it('toggles the sidebar and persists the choice', async () => {
    const user = userEvent.setup();
    renderShell();

    await user.click(screen.getByRole('button', { name: 'Toggle Sidebar' }));

    expect(screen.getByTestId('state')).toHaveTextContent('collapsed');
    expect(document.cookie).toContain(`${COOKIE}=false`);

    await user.click(screen.getByRole('button', { name: 'Toggle Sidebar' }));

    expect(screen.getByTestId('state')).toHaveTextContent('expanded');
    expect(document.cookie).toContain(`${COOKIE}=true`);
  });

  // The rail duplicates the trigger; if it were announced too, this query would
  // find two buttons and throw.
  it('is the only control announced as "Toggle Sidebar"', () => {
    renderShell();
    expect(screen.getAllByRole('button', { name: 'Toggle Sidebar' })).toHaveLength(1);
  });
});
