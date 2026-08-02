import type { JsonApiResource } from '@myfleet/shared-ts';

// The single governing due detail behind a vehicle's status. Nested rather than
// four flat fields because axis determines which magnitude is present: flattening
// would make illegal combinations (axis 'time' with a miles value) representable
// and turn "is there any due detail?" into a multi-field presence test.
export interface VehicleNextDue {
  state: 'upcoming' | 'overdue';
  axis: 'time' | 'mileage';
  miles?: number; // present iff axis === 'mileage'
  days?: number; // present iff axis === 'time'
}

// Mirrors fleet-service vehicle resource (apps/fleet-service/internal/vehicle/rest.go).
// `status`, `lastActivityAt`, and `nextDue` are derived read-only on the server and never written by the client.
export interface VehicleAttributes {
  fleetId: string;
  nickname?: string;
  make: string;
  model: string;
  trim?: string;
  year: number;
  vin?: string;
  currentMileage?: number;
  primaryImageMediaId?: string;
  notes?: string;
  status?: string;
  /** RFC 3339. Derived read-only on the server; omitted when unavailable. */
  lastActivityAt?: string;
  /** Derived read-only on the server; omitted when no schedule is non-ok. */
  nextDue?: VehicleNextDue;
}

export type Vehicle = JsonApiResource<VehicleAttributes>;

// Create payload — fields accepted by POST /fleets/{id}/vehicles.
export interface CreateVehicleAttributes {
  nickname?: string;
  make: string;
  model: string;
  trim?: string;
  year: number;
  vin?: string;
  currentMileage?: number;
  notes?: string;
}

// Patch payload — only these are mutable via PATCH /vehicles/{id}.
export interface UpdateVehicleAttributes {
  nickname?: string;
  currentMileage?: number;
  notes?: string;
}
