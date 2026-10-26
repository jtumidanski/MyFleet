import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { MockInstance } from 'vitest';
import { renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React from 'react';
import {
  useCreateMaintenanceRecord,
  useUpdateMaintenanceRecord,
  useDeleteMaintenanceRecord,
  useCompleteMaintenanceSchedule,
} from './maintenance';
import { useCreateFuelLog, useUpdateFuelLog, useDeleteFuelLog } from './fuel';
import { useCreateMileageRecord, mileageKeys, invalidateOdometerQueries } from './mileage';
import { vehicleKeys } from './vehicles';
import { dashboardKeys } from './dashboard';
import { maintenanceRecordService } from '../../../services/api/MaintenanceRecordService';
import { maintenanceScheduleService } from '../../../services/api/MaintenanceScheduleService';
import { fuelService } from '../../../services/api/FuelService';
import { mileageService } from '../../../services/api/MileageService';

vi.mock('../../../services/api/MaintenanceRecordService', () => ({
  maintenanceRecordService: { createForVehicle: vi.fn(), patch: vi.fn(), remove: vi.fn() },
}));
vi.mock('../../../services/api/MaintenanceScheduleService', () => ({
  maintenanceScheduleService: { complete: vi.fn() },
}));
vi.mock('../../../services/api/FuelService', () => ({
  fuelService: { createForVehicle: vi.fn(), patch: vi.fn(), remove: vi.fn() },
}));
vi.mock('../../../services/api/MileageService', () => ({
  mileageService: { create: vi.fn() },
}));

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const invalidate = vi.spyOn(qc, 'invalidateQueries');
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { qc, wrapper, invalidate };
}

function invalidatedKeys(spy: MockInstance<QueryClient['invalidateQueries']>): string[] {
  return spy.mock.calls.map((call) => JSON.stringify((call[0] as { queryKey: unknown }).queryKey));
}

const j = (k: unknown) => JSON.stringify(k);

function expectScopedToVehicle(keys: string[]) {
  expect(keys).toContain(j(vehicleKeys.lists()));
  expect(keys).toContain(j(vehicleKeys.detail('v1')));
  expect(keys).toContain(j(mileageKeys.lists()));
  expect(keys).toContain(j(dashboardKeys.all));
}

function expectUnscoped(keys: string[]) {
  expect(keys).toContain(j(vehicleKeys.lists()));
  expect(keys).toContain(j(vehicleKeys.all));
  expect(keys).toContain(j(mileageKeys.lists()));
  expect(keys).toContain(j(dashboardKeys.all));
}

describe('odometer invalidation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(maintenanceRecordService.createForVehicle).mockResolvedValue({} as never);
    vi.mocked(maintenanceRecordService.patch).mockResolvedValue({} as never);
    vi.mocked(maintenanceRecordService.remove).mockResolvedValue(undefined as never);
    vi.mocked(maintenanceScheduleService.complete).mockResolvedValue({} as never);
    vi.mocked(fuelService.createForVehicle).mockResolvedValue({} as never);
    vi.mocked(fuelService.patch).mockResolvedValue({} as never);
    vi.mocked(fuelService.remove).mockResolvedValue(undefined as never);
    vi.mocked(mileageService.create).mockResolvedValue({} as never);
  });

  it('useCreateMaintenanceRecord', async () => {
    const { wrapper, invalidate } = makeWrapper();
    const { result } = renderHook(() => useCreateMaintenanceRecord('v1'), { wrapper });
    await result.current.mutateAsync({} as never);
    expectScopedToVehicle(invalidatedKeys(invalidate));
  });

  it('useUpdateMaintenanceRecord', async () => {
    const { wrapper, invalidate } = makeWrapper();
    const { result } = renderHook(() => useUpdateMaintenanceRecord('v1'), { wrapper });
    await result.current.mutateAsync({ id: 'r1', attributes: {} });
    expectScopedToVehicle(invalidatedKeys(invalidate));
  });

  it('useDeleteMaintenanceRecord', async () => {
    const { wrapper, invalidate } = makeWrapper();
    const { result } = renderHook(() => useDeleteMaintenanceRecord('v1'), { wrapper });
    await result.current.mutateAsync('r1');
    expectScopedToVehicle(invalidatedKeys(invalidate));
  });

  it('useCompleteMaintenanceSchedule', async () => {
    const { wrapper, invalidate } = makeWrapper();
    const { result } = renderHook(() => useCompleteMaintenanceSchedule('v1'), { wrapper });
    await result.current.mutateAsync({ id: 's1', attributes: {} as never });
    expectScopedToVehicle(invalidatedKeys(invalidate));
  });

  it('useCreateFuelLog', async () => {
    const { wrapper, invalidate } = makeWrapper();
    const { result } = renderHook(() => useCreateFuelLog('v1'), { wrapper });
    await result.current.mutateAsync({} as never);
    expectScopedToVehicle(invalidatedKeys(invalidate));
  });

  it('useCreateMileageRecord', async () => {
    const { wrapper, invalidate } = makeWrapper();
    const { result } = renderHook(() => useCreateMileageRecord('v1'), { wrapper });
    await result.current.mutateAsync({} as never);
    expectScopedToVehicle(invalidatedKeys(invalidate));
  });

  it('useUpdateFuelLog', async () => {
    const { wrapper, invalidate } = makeWrapper();
    const { result } = renderHook(() => useUpdateFuelLog(), { wrapper });
    await result.current.mutateAsync({ id: 'f1', attributes: {} });
    expectUnscoped(invalidatedKeys(invalidate));
  });

  it('useDeleteFuelLog', async () => {
    const { wrapper, invalidate } = makeWrapper();
    const { result } = renderHook(() => useDeleteFuelLog(), { wrapper });
    await result.current.mutateAsync('f1');
    expectUnscoped(invalidatedKeys(invalidate));
  });

  it('invalidateOdometerQueries without a vehicleId falls back to vehicleKeys.all', () => {
    const { qc, invalidate } = makeWrapper();
    invalidateOdometerQueries(qc);
    expectUnscoped(invalidatedKeys(invalidate));
  });
});
