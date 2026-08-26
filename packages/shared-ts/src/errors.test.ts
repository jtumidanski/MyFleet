import { describe, it, expect } from 'vitest';
import { createErrorFromUnknown, ApiError } from './errors';

describe('createErrorFromUnknown', () => {
  it('maps a JSON:API error envelope to ApiError', () => {
    const err = createErrorFromUnknown({
      status: 422,
      body: { errors: [{ status: '422', code: 'validation_error', title: 'bad', detail: 'x' }] },
    });
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(422);
    expect(err.code).toBe('validation_error');
  });

  it('falls back for non-envelope errors', () => {
    const err = createErrorFromUnknown(new Error('boom'));
    expect(err.status).toBe(0);
    expect(err.message).toContain('boom');
  });

  it('passes an ApiError through unchanged', () => {
    // ApiClient.request already converts a failed response into an ApiError
    // before throwing, so every hook that calls createErrorFromUnknown on a
    // caught error hands it one of these. Rebuilding it would drop `detail`
    // and reset `status` to 0 — which is exactly what the console needs to
    // decide what to tell the operator.
    const original = new ApiError(409, 'conflict', 'conflict', 'vehicle is pending purge');
    const err = createErrorFromUnknown(original);
    expect(err).toBe(original);
    expect(err.status).toBe(409);
    expect(err.detail).toBe('vehicle is pending purge');
  });
});
