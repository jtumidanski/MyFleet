import type { JsonApiDocument, JsonApiResource } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import type {
  MediaObjectAttributes,
  InitMediaUploadAttributes,
  MediaVariant,
} from '../../types/models/media';

/**
 * Media service — wraps the media-service endpoints (gateway prefix /api/media).
 * Backend routes (apps/media-service/internal/mediaobject/resource.go):
 *   POST   /api/media               — init upload: creates the row (uploaded)
 *   PUT    /api/media/{id}/content  — upload the raw bytes (proxied to MinIO)
 *   POST   /api/media/{id}/confirm  — mark uploaded→processing
 *   GET    /api/media/{id}          — get metadata
 *   GET    /api/media/{id}/content  — stream the bytes (proxied from MinIO);
 *                                     optional ?variant=thumbnail|display
 *   DELETE /api/media/{id}          — soft delete
 *
 * Bytes are proxied through media-service, not presigned: MinIO is a shared
 * cluster service and is never reachable from the browser.
 */
class MediaService {
  private readonly basePath = '/api/media';

  /** POST /api/media — init upload; creates the media row in the uploaded state. */
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
   * PUT /api/media/{id}/content — upload the raw bytes through the API. Goes
   * via apiClient so the bearer token and 401-refresh apply; the Content-Type
   * override replaces the default JSON:API media type.
   */
  async putContent(id: string, file: File): Promise<JsonApiResource<MediaObjectAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<MediaObjectAttributes>>>(
      `${this.basePath}/${id}/content`,
      {
        method: 'PUT',
        body: file,
        headers: { 'Content-Type': file.type || 'application/octet-stream' },
      },
    );
    return doc.data;
  }

  /**
   * GET /api/media/{id}/content — the raw bytes, authenticated.
   *
   * `original` sends no query parameter at all, so every pre-existing caller's
   * request stays byte-identical on the wire.
   */
  async getContentBlob(id: string, variant: MediaVariant = 'original'): Promise<Blob> {
    const suffix = variant === 'original' ? '' : `?variant=${variant}`;
    return apiClient.requestBlob(`${this.basePath}/${id}/content${suffix}`);
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

  /** DELETE /api/media/{id} — soft delete. */
  async remove(id: string): Promise<void> {
    await apiClient.request<null>(`${this.basePath}/${id}`, { method: 'DELETE' });
  }
}

export const mediaService = new MediaService();
