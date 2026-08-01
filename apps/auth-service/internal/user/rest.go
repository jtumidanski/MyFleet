package user

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Mirrored by apps/web/src/types/models/user.ts — keep the JSON tags and that
// file's UserAttributes in step.
type Attributes struct {
	Email           string `json:"email"`
	DisplayName     string `json:"displayName"`
	AvatarURL       string `json:"avatarUrl"`
	ThemePreference string `json:"themePreference"`
}

func Transform(m Model) server.Resource {
	return server.Resource{Type: "users", ID: m.ID(), Attributes: Attributes{Email: m.Email(), DisplayName: m.DisplayName(), AvatarURL: m.AvatarURL(), ThemePreference: m.ThemePreference()}}
}

func TransformSlice(ms []Model) []server.Resource {
	out := make([]server.Resource, 0, len(ms))
	for _, m := range ms {
		out = append(out, Transform(m))
	}
	return out
}
