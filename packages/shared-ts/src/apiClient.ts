import { createErrorFromUnknown } from './errors';

export interface ApiClientOptions {
  baseUrl: string;
  getAccessToken: () => string | null;
  onRefresh: () => Promise<string | null>;
}

export class ApiClient {
  constructor(private opts: ApiClientOptions) {}

  async request<T>(path: string, init: RequestInit = {}, retried = false): Promise<T> {
    const token = this.opts.getAccessToken();
    const res = await fetch(this.opts.baseUrl + path, {
      ...init,
      headers: {
        'Content-Type': 'application/vnd.api+json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(init.headers ?? {}),
      },
    });
    if (res.status === 401 && !retried) {
      const refreshed = await this.opts.onRefresh();
      if (refreshed) return this.request<T>(path, init, true);
    }
    const body = res.status === 204 ? null : await res.json().catch(() => null);
    if (!res.ok) throw createErrorFromUnknown({ status: res.status, body });
    return body as T;
  }

  /**
   * Authenticated binary GET. Shares request's bearer-token and one-shot
   * 401-refresh behaviour, but returns the raw Blob instead of parsed JSON and
   * sets no Content-Type — used for media bytes proxied through the API.
   */
  async requestBlob(path: string, init: RequestInit = {}, retried = false): Promise<Blob> {
    const token = this.opts.getAccessToken();
    const res = await fetch(this.opts.baseUrl + path, {
      ...init,
      headers: {
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(init.headers ?? {}),
      },
    });
    if (res.status === 401 && !retried) {
      const refreshed = await this.opts.onRefresh();
      if (refreshed) return this.requestBlob(path, init, true);
    }
    if (!res.ok) {
      const body = await res.json().catch(() => null);
      throw createErrorFromUnknown({ status: res.status, body });
    }
    return res.blob();
  }
}
