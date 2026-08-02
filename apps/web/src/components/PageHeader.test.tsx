import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { PageHeader } from './PageHeader';

describe('PageHeader', () => {
  // FR-3 + the NFR that every authenticated route exposes exactly one <h1>
  // naming the page. The class assertion is not cosmetic pedantry: the whole
  // task exists because the Dashboard drifted to text-lg.
  it('renders the title as the page h1 at the standard size', () => {
    render(<PageHeader title="Vehicles" />);

    const heading = screen.getByRole('heading', { level: 1, name: 'Vehicles' });
    expect(heading).toHaveClass('text-2xl', 'font-semibold');
  });

  // FR-4: title left, actions right, on ONE row. Two stacked rows is the
  // failure mode this component exists to prevent.
  it('renders actions alongside the title in a flex row', () => {
    const { container } = render(
      <PageHeader title="Vehicles" actions={<button type="button">Add Vehicle</button>} />,
    );

    expect(screen.getByRole('heading', { level: 1 })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add Vehicle' })).toBeInTheDocument();
    expect(container.firstElementChild).toHaveClass('flex', 'justify-between');
  });

  // FR-4: "no empty flex child is rendered" when actions is absent. An empty
  // <div> in a justify-between row is invisible but changes how a long title
  // wraps, so it is worth pinning.
  it('renders no actions container when actions is absent', () => {
    const { container } = render(<PageHeader title="Activity" />);

    expect(container.firstElementChild?.children).toHaveLength(1);
  });

  // Same for a falsy actions expression — pages pass `isOwner && <Menu/>`,
  // which is `false`, not `undefined`, for a non-owner.
  it('renders no actions container when actions is false', () => {
    const { container } = render(<PageHeader title="Dashboard" actions={false} />);

    expect(container.firstElementChild?.children).toHaveLength(1);
  });

  // FR-6: className merges via cn() rather than replacing the base classes.
  it('merges className with the base classes', () => {
    const { container } = render(<PageHeader title="Activity" className="max-w-md" />);

    expect(container.firstElementChild).toHaveClass('flex', 'max-w-md');
  });

  // Design §2.2: the badge sits ON the title line but OUTSIDE the heading, so
  // the h1's accessible name stays the page name. Widening `title` to a
  // ReactNode would have put "Sold" inside it.
  it('renders titleAdornment outside the h1', () => {
    render(<PageHeader title="Daily Driver" titleAdornment={<span>Sold</span>} />);

    const heading = screen.getByRole('heading', { level: 1 });
    expect(heading).toHaveAccessibleName('Daily Driver');
    expect(heading.textContent).not.toContain('Sold');
    expect(screen.getByText('Sold')).toBeInTheDocument();
  });

  // Design §2.2: description is optional and produces no empty <p> when unset.
  it('renders description only when provided', () => {
    const { container, rerender } = render(<PageHeader title="Daily Driver" />);
    expect(container.querySelector('p')).toBeNull();

    rerender(<PageHeader title="Daily Driver" description="2019 Subaru Outback" />);
    expect(screen.getByText('2019 Subaru Outback')).toBeInTheDocument();
  });

  // Design §2.2: a single-line title against a 36px Button needs items-center
  // or the button rides high; a two-line title block needs items-start. The
  // component derives it so no page has to remember.
  it('derives vertical alignment from whether a description is present', () => {
    const { container, rerender } = render(<PageHeader title="Vehicles" />);
    expect(container.firstElementChild).toHaveClass('items-center');

    rerender(<PageHeader title="Vehicles" description="Sub" />);
    expect(container.firstElementChild).toHaveClass('items-start');
  });
});
