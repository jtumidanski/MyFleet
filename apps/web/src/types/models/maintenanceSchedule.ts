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
  /** True when the schedule is due once and never repeats. Always present. */
  oneTime: boolean;
  /** RFC3339. The permanent due point of a one-time schedule, or a recurring schedule's first-due anchor. */
  dueDate?: string;
  dueMileage?: number;
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
  oneTime?: boolean;
  intervalMonths?: number;
  intervalMiles?: number;
  /** RFC3339 */
  dueDate?: string;
  dueMileage?: number;
}

/** PATCH /api/fleet/maintenance-schedules/{id} body attributes */
export interface UpdateMaintenanceScheduleAttributes {
  recurrenceType?: string;
  oneTime?: boolean;
  intervalMonths?: number;
  intervalMiles?: number;
  /**
   * RFC3339, or an explicit `null` to clear the stored anchor. The server
   * distinguishes an absent key from a null one (server.Nullable), so the two
   * are NOT interchangeable — omitting the key leaves the anchor in place.
   */
  dueDate?: string | null;
  /** 0 clears the stored odometer anchor. */
  dueMileage?: number;
  active?: boolean;
}

/** POST /api/fleet/maintenance-schedules/{id}/complete body attributes */
export interface CompleteMaintenanceScheduleAttributes {
  /** RFC3339 */
  date?: string;
  latestMileage?: number;
}
