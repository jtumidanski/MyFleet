/**
 * The absolute URL an invitee opens to accept an invite.
 *
 * Nothing delivers invites yet — `task-009-smtp-invite-delivery` is specced but
 * unimplemented — so the fleet owner sends this link themselves. It is the only
 * thing that turns a pending invite row into an invite the recipient can act on.
 *
 * Absolute, not site-relative: this is meant to be pasted into a chat client or
 * an email, where a bare `/invites/...` resolves to nothing.
 */
export function inviteAcceptUrl(token: string): string {
  return `${window.location.origin}/invites/${encodeURIComponent(token)}/accept`;
}
