package session

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Attributes is the JSON:API attribute payload for a minted token pair.
type Attributes struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

// Transform renders an issued token pair as a JSON:API resource.
func Transform(i Issued) server.Resource {
	return server.Resource{
		Type:       "sessions",
		ID:         "current",
		Attributes: Attributes{AccessToken: i.Access, RefreshToken: i.Refresh},
	}
}
