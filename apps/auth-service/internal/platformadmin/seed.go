package platformadmin

import (
	"strings"

	"gorm.io/gorm"
)

// ParseBootstrapEmails splits PLATFORM_ADMIN_BOOTSTRAP_EMAILS into a lookup set,
// trimming and lower-casing each entry and dropping empties.
//
// Same shape as KAFKA_BROKERS parsing in the other composition roots, with the
// case fold added because Google returns whatever casing the user typed while
// the list is hand-written in a ConfigMap.
func ParseBootstrapEmails(raw string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		if e := strings.ToLower(strings.TrimSpace(part)); e != "" {
			out[e] = true
		}
	}
	return out
}

// SeedFromEmails grants the privilege to every EXISTING, VERIFIED user whose
// email is in the set and who has not been explicitly revoked, and returns how
// many grants it made.
//
// A bootstrap email with no user row is silently skipped — that is the normal
// case on a fresh database, and user.Processor's provision-time hook is what
// covers it at first login (FR-ADMIN-AUTH-2). A bootstrap email that HAS a
// revoked_at tombstone is also skipped — that is what makes revocation durable
// across restarts: without this check, every boot would re-grant the very
// admin an operator just revoked.
//
// The email_verified filter mirrors the gate user.Processor.maybeGrantAdmin
// applies at login time. This function runs at boot with no id_token in hand,
// so it can only honor verification by reading the persisted column — without
// it, a corporate address Google marked email_verified: false at login would
// still be silently granted admin on the next restart, reopening the exact
// escalation the login-time gate exists to close.
//
// The users read is raw SQL rather than a user.Provider call so this package
// does not import user; both live in the same service and schema, so this is
// not the cross-service DB read that design D6 forbids.
func SeedFromEmails(db *gorm.DB, emails map[string]bool) (int, error) {
	if len(emails) == 0 {
		return 0, nil
	}
	list := make([]string, 0, len(emails))
	for e := range emails {
		list = append(list, e)
	}

	var ids []string
	if err := db.Raw(`SELECT id FROM auth.users WHERE lower(email) IN ? AND email_verified = ?`, list, true).Scan(&ids).Error; err != nil {
		return 0, err
	}

	prov := NewProvider(db)
	adm := NewAdministrator(db)
	granted := 0
	for _, id := range ids {
		if revoked, err := prov.IsRevoked(id); err != nil {
			return granted, err
		} else if revoked {
			continue
		}
		if err := adm.Grant(id, BootstrapGrantedBy); err != nil {
			return granted, err
		}
		granted++
	}
	return granted, nil
}
