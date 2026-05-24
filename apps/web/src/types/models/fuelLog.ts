import type { JsonApiResource } from '@myfleet/shared-ts';

/**
 * Mirrors apps/fleet-service/internal/fuel/rest.go Attributes.
 */
export interface FuelLogAttributes {
  vehicleId: string;
  /** RFC3339 */
  date: string;
  mileage: number;
  gallons: number;
  totalCost: number;
  pricePerGallon: number;
  createdByUserId?: string;
  /** RFC3339 */
  createdAt: string;
  /** RFC3339 */
  updatedAt: string;
}

export type FuelLog = JsonApiResource<FuelLogAttributes>;

/** POST /api/fleet/vehicles/{id}/fuel-logs body attributes */
export interface CreateFuelLogAttributes {
  /** RFC3339 */
  date?: string;
  mileage: number;
  gallons: number;
  /** Provide either totalCost or pricePerGallon (or both); server derives the missing one. */
  totalCost?: number;
  pricePerGallon?: number;
}

/** PATCH /api/fleet/fuel-logs/{id} body attributes */
export interface UpdateFuelLogAttributes {
  /** RFC3339 */
  date?: string;
  mileage?: number;
  gallons?: number;
  totalCost?: number;
  pricePerGallon?: number;
}
