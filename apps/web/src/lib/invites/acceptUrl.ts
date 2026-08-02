/**
 * The absolute URL an invitee opens to accept an invite.
 *
 * Invite email delivers this same link as of `task-009-smtp-invite-delivery`,
 * built server-side from `PUBLIC_WEB_URL`. This copy stays because delivery is
 * best-effort: a spam filter, or a relay outage outlasting the sender's bounded
 * retry budget, drops the email with no second attempt — and then the owner
 * sending this link by hand is the only way the invite reaches anyone.
 *
 * Keep the path shape in step with the sender (`mailconsumer`'s `render`) and
 * the SPA route; a divergence here mails a link that 404s.
 *
 * Absolute, not site-relative: this is meant to be pasted into a chat client or
 * an email, where a bare `/invites/...` resolves to nothing.
 */
export function inviteAcceptUrl(token: string): string {
  return `${window.location.origin}/invites/${encodeURIComponent(token)}/accept`;
}
