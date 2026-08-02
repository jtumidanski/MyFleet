import { describe, it, expect } from 'vitest';
import { inviteAcceptUrl } from './acceptUrl';

describe('inviteAcceptUrl', () => {
  // The whole point of the helper: the owner has to be able to send this to
  // someone. Creating an invite used to toast "Invite sent" while nothing was
  // sent anywhere and the token stayed buried in an API response.
  it('builds an absolute URL on the current origin', () => {
    expect(inviteAcceptUrl('tok-123')).toBe(`${window.location.origin}/invites/tok-123/accept`);
  });

  // The path must survive being pasted into a chat client or an address bar. A
  // token is server-generated, but encoding it is the difference between a link
  // that works and one that silently truncates at the first reserved character.
  it('percent-encodes a token containing URL-reserved characters', () => {
    expect(inviteAcceptUrl('a/b?c#d')).toBe(
      `${window.location.origin}/invites/a%2Fb%3Fc%23d/accept`,
    );
  });
});
