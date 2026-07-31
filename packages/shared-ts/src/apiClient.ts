import { createErrorFromUnknown } from './errors';

export interface ApiClientOptions {
  baseUrl: string;
  getAccessToken: () => string | null;
  onRefresh: () => Promise<string | null>;
}

export class ApiClient {
  constructor(private opts: ApiClientOptions) {}

  /**
   * Shared fetch / bearer-token / one-shot 401-refresh-and-retry logic used by
   * both `request` and `requestBlob`. Returns the raw Response so each caller
   * can decide how to read the body (json() vs blob()). `defaultHeaders` lets
   * `request` set the JSON:API Content-Type while `requestBlob` sets none;
   * `init.headers` is still spread last so a caller's own header override
   * always wins, matching the previous per-method behaviour exactly.
   */
  private async fetchAuthenticated(
    path: string,
    init: RequestInit,
    defaultHeaders: Record<string, string>,
    retried: boolean,
  ): Promise<Response> {
    const token = this.opts.getAccessToken();
    const res = await fetch(this.opts.baseUrl + path, {
      ...init,
      headers: {
        ...defaultHeaders,
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(init.headers ?? {}),
      },
    });
    if (res.status === 401 && !retried) {
      const refreshed = await this.opts.onRefresh();
      if (refreshed) return this.fetchAuthenticated(path, init, defaultHeaders, true);
    }
    return res;
  }

  async request<T>(path: string, init: RequestInit = {}, retried = false): Promise<T> {
    const res = await this.fetchAuthenticated(
      path,
      init,
      { 'Content-Type': 'application/vnd.api+json' },
      retried,
    );
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
    const res = await this.fetchAuthenticated(path, init, {}, retried);
    if (!res.ok) {
      const body = await res.json().catch(() => null);
      throw createErrorFromUnknown({ status: res.status, body });
    }
    return res.blob();
  }
}
