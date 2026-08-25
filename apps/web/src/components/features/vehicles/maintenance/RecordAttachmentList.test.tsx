import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React from 'react';
import { RecordAttachmentList } from './RecordAttachmentList';
import { mediaService } from '../../../../services/api/MediaService';
import { downloadBlob } from '../../../../lib/utils/download';

vi.mock('../../../../services/api/MediaService', () => ({
  mediaService: { get: vi.fn(), getContentBlob: vi.fn() },
}));
vi.mock('../../../../lib/utils/download', () => ({ downloadBlob: vi.fn() }));

function wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

function mediaResource(id: string, contentType: string, status = 'ready') {
  return {
    id,
    type: 'media-objects',
    attributes: { contentType, status, originalFilename: `${id}.file` },
  };
}

describe('RecordAttachmentList', () => {
  beforeEach(() => vi.clearAllMocks());

  it('renders a document as a working download action', async () => {
    const user = userEvent.setup();
    vi.mocked(mediaService.get).mockResolvedValue(mediaResource('m1', 'application/pdf') as never);
    const blob = new Blob(['%PDF']);
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(blob);

    render(<RecordAttachmentList mediaIds={['m1']} />, { wrapper });

    const button = await screen.findByRole('button', { name: /m1\.file/i });
    await user.click(button);

    await waitFor(() => expect(downloadBlob).toHaveBeenCalledWith(blob, 'm1.file'));
  });

  it('renders an image attachment as a thumbnail, not a download button', async () => {
    vi.mocked(mediaService.get).mockResolvedValue(mediaResource('m2', 'image/jpeg') as never);

    render(<RecordAttachmentList mediaIds={['m2']} />, { wrapper });

    await waitFor(() =>
      expect(screen.queryByRole('button', { name: /m2\.file/i })).not.toBeInTheDocument(),
    );
    expect(await screen.findByText('m2.file')).toBeInTheDocument();
  });

  // A media object that is missing, soft-deleted, or in a fleet the caller
  // cannot read renders an explicit unavailable row — never a broken control or
  // a silently empty list (PRD FR-VIEW-4).
  it('renders an explicit unavailable row when the media object cannot be read', async () => {
    vi.mocked(mediaService.get).mockRejectedValue(new Error('404'));

    render(<RecordAttachmentList mediaIds={['gone']} />, { wrapper });

    expect(await screen.findByText(/unavailable/i)).toBeInTheDocument();
  });

  // A terminal processing failure is the same user-visible outcome as a deleted
  // attachment — no new state to handle (design D8/D13).
  it('treats a failed media object as unavailable', async () => {
    vi.mocked(mediaService.get).mockResolvedValue(
      mediaResource('m3', 'image/jpeg', 'failed') as never,
    );

    render(<RecordAttachmentList mediaIds={['m3']} />, { wrapper });

    expect(await screen.findByText(/unavailable/i)).toBeInTheDocument();
  });

  // Viewers keep read-only access: they see and download attachments and get
  // no remove control at all, rather than a control that fails late.
  it('omits the remove control when canRemove is false', async () => {
    vi.mocked(mediaService.get).mockResolvedValue(mediaResource('m1', 'application/pdf') as never);

    render(<RecordAttachmentList mediaIds={['m1']} onRemove={vi.fn()} canRemove={false} />, {
      wrapper,
    });

    await screen.findByRole('button', { name: /m1\.file/i });
    expect(screen.queryByRole('button', { name: /^remove/i })).not.toBeInTheDocument();
  });

  it('omits the remove control when no onRemove handler is supplied', async () => {
    vi.mocked(mediaService.get).mockResolvedValue(mediaResource('m1', 'application/pdf') as never);

    render(<RecordAttachmentList mediaIds={['m1']} canRemove />, { wrapper });

    await screen.findByRole('button', { name: /m1\.file/i });
    expect(screen.queryByRole('button', { name: /^remove/i })).not.toBeInTheDocument();
  });

  it('calls onRemove with the media id from a document row', async () => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    vi.mocked(mediaService.get).mockResolvedValue(mediaResource('m1', 'application/pdf') as never);

    render(<RecordAttachmentList mediaIds={['m1']} onRemove={onRemove} canRemove />, { wrapper });

    await user.click(await screen.findByRole('button', { name: 'Remove m1.file' }));

    expect(onRemove).toHaveBeenCalledWith('m1');
  });

  it('calls onRemove with the media id from an image row', async () => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    vi.mocked(mediaService.get).mockResolvedValue(mediaResource('m1', 'image/jpeg') as never);

    render(<RecordAttachmentList mediaIds={['m1']} onRemove={onRemove} canRemove />, { wrapper });

    await user.click(await screen.findByRole('button', { name: 'Remove m1.file' }));

    expect(onRemove).toHaveBeenCalledWith('m1');
  });

  // An unavailable attachment is exactly the one a user most wants to clear,
  // so the row that used to render nothing but a message must still offer it.
  it('offers removal on an unavailable attachment row', async () => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    vi.mocked(mediaService.get).mockRejectedValue(new Error('404'));

    render(<RecordAttachmentList mediaIds={['m1']} onRemove={onRemove} canRemove />, { wrapper });

    await screen.findByText(/attachment unavailable/i);
    await user.click(screen.getByRole('button', { name: 'Remove attachment' }));

    expect(onRemove).toHaveBeenCalledWith('m1');
  });
});
