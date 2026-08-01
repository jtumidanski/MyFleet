/**
 * Saves a Blob to disk under `filename`.
 *
 * A plain `<a href>` cannot be used for media downloads: GET
 * /api/media/{id}/content requires an Authorization header, which the browser
 * does not send for a navigation. So the bytes are fetched through the
 * authenticated API client and handed to a detached anchor via an object URL
 * (PRD FR-VIEW-3).
 *
 * The revoke is deferred to a macrotask so the click-driven download has
 * started before the URL is invalidated; revoking synchronously cancels it in
 * some browsers.
 */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  anchor.rel = 'noopener';
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
