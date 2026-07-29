import type { JsonApiResource } from '@myfleet/shared-ts';

// Mirrors fleet-service vehicle resource (apps/fleet-service/internal/vehicle/rest.go).
// `status` is derived read-only on the server and never written by the client.
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
