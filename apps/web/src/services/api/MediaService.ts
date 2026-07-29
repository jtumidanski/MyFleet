import type { JsonApiDocument, JsonApiResource } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import type { MediaObjectAttributes, InitMediaUploadAttributes } from '../../types/models/media';

/**
 * Media service — wraps the media-service endpoints (gateway prefix /api/media).
 * Backend routes (apps/media-service/internal/mediaobject/resource.go):
 *   POST   /api/media              — init upload: returns media row + presigned PUT URL
 *   POST   /api/media/{id}/confirm — mark uploaded→processing
 *   GET    /api/media/{id}         — get metadata
 *   GET    /api/media/{id}/download — get metadata + presigned GET URL (downloadUrl)
 *   DELETE /api/media/{id}         — soft delete
 *
 * NOTE: The presigned PUT to MinIO is NOT routed through apiClient — it is a
 * raw fetch() with no auth header and no /api prefix.
 */
class MediaService {
  private readonly basePath = '/api/media';

  /** POST /api/media — init upload; returns a resource with uploadUrl in attributes. */
  async initUpload(
    attrs: InitMediaUploadAttributes,
  ): Promise<JsonApiResource<MediaObjectAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<MediaObjectAttributes>>>(
      this.basePath,
      {
        method: 'POST',
        body: JSON.stringify({
          data: { type: 'media-objects', attributes: attrs },
        }),
      },
    );
    return doc.data;
  }

  /**
   * PUT the raw file bytes directly to MinIO via the presigned URL.
   * This intentionally bypasses apiClient (no auth header, no /api prefix).
   */
  async putToPresignedUrl(presignedUrl: string, file: File): Promise<void> {
    const res = await fetch(presignedUrl, {
      method: 'PUT',
      body: file,
      headers: { 'Content-Type': file.type },
    });
    if (!res.ok) {
      throw new Error(`Presigned PUT failed: ${res.status} ${res.statusText}`);
    }
  }

  /** POST /api/media/{id}/confirm — move from uploaded → processing. */
  async confirm(id: string): Promise<JsonApiResource<MediaObjectAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<MediaObjectAttributes>>>(
      `${this.basePath}/${id}/confirm`,
      {
        method: 'POST',
        body: JSON.stringify({ data: { type: 'media-objects', id, attributes: {} } }),
      },
    );
    return doc.data;
  }

  /** GET /api/media/{id} — poll for status (uploaded → processing → ready). */
  async get(id: string): Promise<JsonApiResource<MediaObjectAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<MediaObjectAttributes>>>(
      `${this.basePath}/${id}`,
    );
    return doc.data;
  }

  /**
   * GET /api/media/{id}/download — returns metadata + presigned GET URL in
   * attributes.downloadUrl. Use downloadUrl as the <img src>.
   */
  async getDownloadUrl(id: string): Promise<JsonApiResource<MediaObjectAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<MediaObjectAttributes>>>(
      `${this.basePath}/${id}/download`,
    );
    return doc.data;
  }

  /** DELETE /api/media/{id} — soft delete. */
  async remove(id: string): Promise<void> {
    await apiClient.request<null>(`${this.basePath}/${id}`, { method: 'DELETE' });
  }
}

export const mediaService = new MediaService();
