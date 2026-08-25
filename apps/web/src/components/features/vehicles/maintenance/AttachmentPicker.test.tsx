import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AttachmentPicker } from './AttachmentPicker';
import { MAX_ATTACHMENTS, type PendingAttachment } from '../../../../lib/hooks/usePendingAttachments';

function pending(name: string): PendingAttachment {
  return {
    localId: name,
    file: new File(['x'], name, { type: 'application/pdf' }),
    status: 'ready',
    mediaId: `media-${name}`,
  };
}

const noop = vi.fn();

describe('AttachmentPicker', () => {
  it('keeps the create-flow helper text when the record has no attachments yet', () => {
    render(<AttachmentPicker items={[]} onAdd={noop} onRemove={noop} />);

    expect(screen.getByText(/PDF, image, Word, Excel or CSV/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /add files/i })).toBeEnabled();
  });

  // The whole point: the user must learn the remaining capacity BEFORE picking,
  // not from a 422 afterwards.
  it('reports remaining capacity against the existing attachments', () => {
    render(<AttachmentPicker items={[]} onAdd={noop} onRemove={noop} existingCount={3} />);

    expect(screen.getByText(`3 of ${MAX_ATTACHMENTS} attached. You can add 7 more.`)).toBeInTheDocument();
  });

  it('counts pending files against the same capacity', () => {
    render(
      <AttachmentPicker
        items={[pending('a.pdf'), pending('b.pdf')]}
        onAdd={noop}
        onRemove={noop}
        existingCount={3}
      />,
    );

    expect(screen.getByText(`3 of ${MAX_ATTACHMENTS} attached. You can add 5 more.`)).toBeInTheDocument();
  });

  it('disables adding and says the record is at the limit when existing plus pending fills it', () => {
    render(
      <AttachmentPicker
        items={[pending('a.pdf')]}
        onAdd={noop}
        onRemove={noop}
        existingCount={MAX_ATTACHMENTS - 1}
      />,
    );

    expect(screen.getByRole('button', { name: /add files/i })).toBeDisabled();
    expect(
      screen.getByText(`This record is at the ${MAX_ATTACHMENTS}-attachment limit.`),
    ).toBeInTheDocument();
  });

  it('keeps the create-flow at-limit copy when there are no existing attachments', () => {
    const items = Array.from({ length: MAX_ATTACHMENTS }, (_, i) => pending(`f${i}.pdf`));
    render(<AttachmentPicker items={items} onAdd={noop} onRemove={noop} />);

    expect(screen.getByRole('button', { name: /add files/i })).toBeDisabled();
    expect(
      screen.getByText(`Maximum ${MAX_ATTACHMENTS} attachments per record.`),
    ).toBeInTheDocument();
  });
});
