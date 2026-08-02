import { describe, it, expect } from 'vitest';
import { buildLoginUrl } from './authRoutes';

describe('buildLoginUrl', () => {
  it('starts the OAuth dance with no return path by default', () => {
    expect(buildLoginUrl()).toBe('/api/auth/login/google');
  });

  it('passes the return path through as an encoded return_to param', () => {
    expect(buildLoginUrl('/invites/abc123/accept')).toBe(
      '/api/auth/login/google?return_to=%2Finvites%2Fabc123%2Faccept',
    );
  });

  // auth-service sanitizes return_to authoritatively; this is a second gate so
  // an off-site value never leaves the browser in the first place.
  it('drops anything that is not a site-relative path', () => {
    for (const bad of ['https://evil.example', '//evil.example', '/\\evil.example', 'vehicles']) {
      expect(buildLoginUrl(bad)).toBe('/api/auth/login/google');
    }
  });
});
