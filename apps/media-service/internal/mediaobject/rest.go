package mediaobject

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Attributes is the JSON:API attributes payload for a media object.
type Attributes struct {
	FleetID          string `json:"fleetId"`
	UploadedByUserID string `json:"uploadedByUserId"`
	Bucket           string `json:"bucket"`
	ObjectKey        string `json:"objectKey"`
	ContentType      string `json:"contentType,omitempty"`
	Size             int64  `json:"size,omitempty"`
	OriginalFilename string `json:"originalFilename,omitempty"`
	Status           string `json:"status"`
}

// Transform converts a Model to a JSON:API Resource.
func Transform(m Model) server.Resource {
	return server.Resource{
		Type: "media-objects",
		ID:   m.ID(),
		Attributes: Attributes{
			FleetID:          m.FleetID(),
			UploadedByUserID: m.UploadedByUserID(),
			Bucket:           m.Bucket(),
			ObjectKey:        m.ObjectKey(),
			ContentType:      m.ContentType(),
			Size:             m.Size(),
			OriginalFilename: m.OriginalFilename(),
			Status:           string(m.Status()),
		},
	}
}

// TransformSlice converts a slice of Models to JSON:API Resources.
func TransformSlice(ms []Model) []server.Resource {
	out := make([]server.Resource, 0, len(ms))
	for _, m := range ms {
		out = append(out, Transform(m))
	}
	return out
}

// InternalMedia is the flat (deliberately NOT JSON:API) payload the
// network-restricted GET /internal/media returns, matching the shape
// fleet-service's other internal clients already consume.
type InternalMedia struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	ContentType string `json:"content_type"`
}

// InternalMediaResponse wraps the list so the payload can gain fields later
// without becoming a breaking change for the client.
type InternalMediaResponse struct {
	Media []InternalMedia `json:"media"`
}

// TransformInternalMedia converts Models to the internal payload.
func TransformInternalMedia(ms []Model) []InternalMedia {
	out := make([]InternalMedia, 0, len(ms))
	for _, m := range ms {
		out = append(out, InternalMedia{
			ID:          m.ID(),
			Status:      string(m.Status()),
			ContentType: m.ContentType(),
		})
	}
	return out
}
