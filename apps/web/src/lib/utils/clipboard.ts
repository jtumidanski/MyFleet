/**
 * Clipboard helper (task-009, FR-UI-1).
 *
 * navigator.clipboard is undefined outside a secure context, and local dev runs
 * over plain HTTP on myfleet.home — the exact environment where the copy-link
 * button gets tested first. The execCommand fallback is deprecated but is the
 * only thing that works there.
 *
 * Returns false rather than throwing: the caller decides what to tell the user.
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  if (navigator?.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Permission denied or a non-secure context that still exposes the API —
      // fall through to the legacy path rather than giving up.
    }
  }

  const textarea = document.createElement('textarea');
  textarea.value = text;
  // Keep it out of the viewport so focusing it does not scroll the page.
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  textarea.setAttribute('readonly', '');
  document.body.appendChild(textarea);
  try {
    textarea.select();
    return document.execCommand('copy');
  } catch {
    return false;
  } finally {
    document.body.removeChild(textarea);
  }
}
