import type { JsonApiDocument, JsonApiResource } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import type { VehicleMediaAttributes, AddVehicleMediaAttributes } from '../../types/models/media';

/**
 * Vehicle-media service — wraps the fleet-service vehicle-media endpoints.
 * Backend routes (apps/fleet-service/internal/vehiclemedia/resource.go, gateway-prefixed):
 *   GET  /api/fleet/vehicles/{id}/media  — list media refs for a vehicle
 *   POST /api/fleet/vehicles/{id}/media  — attach a media ref to a vehicle
 *
 * Primary-image route is on the vehicle resource:
 *   PUT  /api/fleet/vehicles/{id}/primary-image  { mediaId }
 *   (handled in VehicleService or here for colocation; using apiClient directly)
 */
class VehicleMediaService {
  private readonly basePath = '/api/fleet/vehicles';

  /** GET /api/fleet/vehicles/{vehicleId}/media — list media refs. */
  async listByVehicle(vehicleId: string): Promise<Array<JsonApiResource<VehicleMediaAttributes>>> {
    const doc = await apiClient.request<
      JsonApiDocument<Array<JsonApiResource<VehicleMediaAttributes>>>
    >(`${this.basePath}/${vehicleId}/media`);
    return doc.data;
  }

  /** POST /api/fleet/vehicles/{vehicleId}/media — attach a media object to a vehicle. */
  async addMedia(
    vehicleId: string,
    attrs: AddVehicleMediaAttributes,
  ): Promise<JsonApiResource<VehicleMediaAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<VehicleMediaAttributes>>>(
      `${this.basePath}/${vehicleId}/media`,
      {
        method: 'POST',
        body: JSON.stringify({ data: { type: 'vehicleMedia', attributes: attrs } }),
      },
    );
    return doc.data;
  }

  /**
   * DELETE /api/fleet/vehicles/{vehicleId}/media/{mediaId} — detach a media
   * object from a vehicle.
   *
   * Distinct from `DELETE /api/media/{id}`, which removes the media object
   * itself in media-service. Removing a photo from a gallery needs BOTH: this
   * call drops the reference the gallery lists, the other releases the bytes.
   */
  async removeMedia(vehicleId: string, mediaId: string): Promise<void> {
    await apiClient.request<void>(`${this.basePath}/${vehicleId}/media/${mediaId}`, {
      method: 'DELETE',
    });
  }

  /**
   * PUT /api/fleet/vehicles/{vehicleId}/primary-image — set the primary image.
   * Clears all is_primary rows and mirrors to vehicles.primary_image_media_id.
   * Returns the updated Vehicle resource (re-fetched by the server).
   */
  async setPrimaryImage(vehicleId: string, mediaId: string): Promise<void> {
    await apiClient.request<JsonApiDocument<unknown>>(
      `${this.basePath}/${vehicleId}/primary-image`,
      {
        method: 'PUT',
        body: JSON.stringify({
          data: { type: 'vehicles', attributes: { mediaId } },
        }),
      },
    );
  }
}

export const vehicleMediaService = new VehicleMediaService();
