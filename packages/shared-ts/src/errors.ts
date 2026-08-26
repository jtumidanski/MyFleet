import type { JsonApiError } from './jsonapi';

export class ApiError extends Error {
  status: number;
  code: string;
  detail?: string;
  pointer?: string;
  constructor(status: number, code: string, message: string, detail?: string, pointer?: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.detail = detail;
    this.pointer = pointer;
  }
}

interface EnvelopeShape {
  status?: number;
  body?: { errors?: JsonApiError[] };
}

export function createErrorFromUnknown(e: unknown): ApiError {
  // Already converted — by ApiClient.request, which throws an ApiError rather
  // than a raw envelope. Rebuilding it would fall through to the generic
  // `instanceof Error` branch below and silently reset status to 0 and drop
  // detail, which is precisely what callers switch on.
  if (e instanceof ApiError) return e;
  const env = e as EnvelopeShape;
  const first = env?.body?.errors?.[0];
  if (first) {
    return new ApiError(
      env.status ?? Number(first.status) ?? 0,
      first.code,
      first.title,
      first.detail,
      first.source?.pointer,
    );
  }
  if (e instanceof Error) return new ApiError(0, 'unknown', e.message);
  return new ApiError(0, 'unknown', 'Unknown error');
}
