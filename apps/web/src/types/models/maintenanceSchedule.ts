import type { JsonApiResource } from '@myfleet/shared-ts';

/**
 * Mirrors apps/fleet-service/internal/maintenanceschedule/rest.go Attributes.
 */
export interface MaintenanceScheduleAttributes {
  vehicleId: string;
  categoryId: string;
  /** 'time' | 'mileage' | 'hybrid' */
  recurrenceType: string;
  intervalMonths?: number;
  intervalMiles?: number;
  /** RFC3339 */
  lastCompletedDate?: string;
  lastCompletedMileage?: number;
  /** RFC3339 */
  nextDueDate?: string;
  nextDueMileage?: number;
  /** e.g. 'ok' | 'upcoming' | 'overdue' */
  status: string;
  /** 'urgent' | 'recommended' | 'informational' */
  severity: string;
  active: boolean;
}

export type MaintenanceSchedule = JsonApiResource<MaintenanceScheduleAttributes>;

/** POST /api/fleet/vehicles/{id}/maintenance-schedules body attributes */
export interface CreateMaintenanceScheduleAttributes {
  categoryId: string;
  recurrenceType: string;
  intervalMonths?: number;
  intervalMiles?: number;
}

/** PATCH /api/fleet/maintenance-schedules/{id} body attributes */
export interface UpdateMaintenanceScheduleAttributes {
  recurrenceType?: string;
  intervalMonths?: number;
  intervalMiles?: number;
  active?: boolean;
}

/** POST /api/fleet/maintenance-schedules/{id}/complete body attributes */
export interface CompleteMaintenanceScheduleAttributes {
  /** RFC3339 */
  date?: string;
  latestMileage?: number;
}
