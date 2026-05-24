package user

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

type Attributes struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}

func Transform(m Model) server.Resource {
	return server.Resource{Type: "users", ID: m.ID(), Attributes: Attributes{Email: m.Email(), DisplayName: m.DisplayName(), AvatarURL: m.AvatarURL()}}
}
