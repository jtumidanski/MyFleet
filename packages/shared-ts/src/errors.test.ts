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
});
