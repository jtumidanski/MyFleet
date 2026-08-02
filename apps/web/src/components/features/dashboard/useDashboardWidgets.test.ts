/**
 * useDashboardWidgets — the widget-list state lifted out of DashboardGrid so
 * DashboardPage can own the Add Widget control (task-015, design §3.2).
 *
 * These tests exist because the extraction moved live logic across a file
 * boundary: an off-by-one in the reorder writers or a dropped save() call would
 * otherwise only surface as a user noticing their layout did not persist.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import { renderHook, act, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { dashboardService } from '../../../services/api/DashboardService';
import { useDashboardWidgets } from './useDashboardWidgets';
import type { Dashboard, WidgetResource } from '../../../types/models/dashboard';

vi.mock('../../../services/api/DashboardService', () => ({
  dashboardService: { getLayout: vi.fn(), saveLayout: vi.fn() },
}));

function widget(id: string, type: string, positionY: number): WidgetResource {
  return {
    type: 'dashboardWidgets',
    id,
    attributes: { type, positionX: 0, positionY, width: 1, height: 1 },
  };
}

function layout(widgets: WidgetResource[]): Dashboard {
  return {
    type: 'dashboards',
    id: 'd1',
    attributes: {
      fleetId: 'f1',
      userId: 'u1',
      widgets,
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    },
  };
}

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
  });
  return React.createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(dashboardService.saveLayout).mockResolvedValue(layout([]));
});

describe('useDashboardWidgets', () => {
  it('maps the server layout into the widget list', async () => {
    vi.mocked(dashboardService.getLayout).mockResolvedValue(
      layout([widget('w1', 'fleet-overview', 0), widget('w2', 'recent-activity', 1)]),
    );

    const { result } = renderHook(() => useDashboardWidgets('f1'), { wrapper });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.widgets.map((w) => w.type)).toEqual([
      'fleet-overview',
      'recent-activity',
    ]);
  });

  // A widget type the frontend does not know how to render would crash the
  // registry lookup. Dropping it is the existing behaviour and must survive
  // the extraction.
  it('drops widgets whose type is not in the catalog', async () => {
    vi.mocked(dashboardService.getLayout).mockResolvedValue(
      layout([widget('w1', 'fleet-overview', 0), widget('w2', 'not-a-real-widget', 1)]),
    );

    const { result } = renderHook(() => useDashboardWidgets('f1'), { wrapper });

    await waitFor(() => expect(result.current.widgets).toHaveLength(1));
    expect(result.current.widgets[0]?.type).toBe('fleet-overview');
  });

  it('appends a widget and persists the new layout', async () => {
    vi.mocked(dashboardService.getLayout).mockResolvedValue(
      layout([widget('w1', 'fleet-overview', 0)]),
    );

    const { result } = renderHook(() => useDashboardWidgets('f1'), { wrapper });
    await waitFor(() => expect(result.current.widgets).toHaveLength(1));

    act(() => result.current.addWidget('recent-activity'));

    expect(result.current.widgets.map((w) => w.type)).toEqual([
      'fleet-overview',
      'recent-activity',
    ]);
    await waitFor(() => expect(dashboardService.saveLayout).toHaveBeenCalledTimes(1));
    expect(vi.mocked(dashboardService.saveLayout).mock.calls[0]?.[1]).toEqual([
      expect.objectContaining({ type: 'fleet-overview', positionY: 0 }),
      expect.objectContaining({ type: 'recent-activity', positionY: 1 }),
    ]);
  });

  it('removes a widget by id and persists', async () => {
    vi.mocked(dashboardService.getLayout).mockResolvedValue(
      layout([widget('w1', 'fleet-overview', 0), widget('w2', 'recent-activity', 1)]),
    );

    const { result } = renderHook(() => useDashboardWidgets('f1'), { wrapper });
    await waitFor(() => expect(result.current.widgets).toHaveLength(2));

    act(() => result.current.removeWidget('w1'));

    expect(result.current.widgets.map((w) => w.id)).toEqual(['w2']);
    await waitFor(() => expect(dashboardService.saveLayout).toHaveBeenCalledTimes(1));
  });

  // positionY is rewritten from array order on every save, so a swap that only
  // reorders the array but never persists would look right until a reload.
  it('swaps adjacent widgets on moveUp and persists the new order', async () => {
    vi.mocked(dashboardService.getLayout).mockResolvedValue(
      layout([widget('w1', 'fleet-overview', 0), widget('w2', 'recent-activity', 1)]),
    );

    const { result } = renderHook(() => useDashboardWidgets('f1'), { wrapper });
    await waitFor(() => expect(result.current.widgets).toHaveLength(2));

    act(() => result.current.moveUp(1));

    expect(result.current.widgets.map((w) => w.id)).toEqual(['w2', 'w1']);
    await waitFor(() => expect(dashboardService.saveLayout).toHaveBeenCalledTimes(1));
    expect(vi.mocked(dashboardService.saveLayout).mock.calls[0]?.[1]).toEqual([
      expect.objectContaining({ type: 'recent-activity', positionY: 0 }),
      expect.objectContaining({ type: 'fleet-overview', positionY: 1 }),
    ]);
  });

  it('does nothing at the list boundaries', async () => {
    vi.mocked(dashboardService.getLayout).mockResolvedValue(
      layout([widget('w1', 'fleet-overview', 0), widget('w2', 'recent-activity', 1)]),
    );

    const { result } = renderHook(() => useDashboardWidgets('f1'), { wrapper });
    await waitFor(() => expect(result.current.widgets).toHaveLength(2));

    act(() => result.current.moveUp(0));
    act(() => result.current.moveDown(1));

    expect(result.current.widgets.map((w) => w.id)).toEqual(['w1', 'w2']);
    expect(dashboardService.saveLayout).not.toHaveBeenCalled();
  });
});
