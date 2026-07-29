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
