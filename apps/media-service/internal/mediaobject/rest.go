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
	UploadURL        string `json:"uploadUrl,omitempty"`
	DownloadURL      string `json:"downloadUrl,omitempty"`
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

// TransformWithUploadURL renders a freshly-created object plus its presigned
// PUT URL so the client can upload bytes directly to MinIO.
func TransformWithUploadURL(m Model, uploadURL string) server.Resource {
	res := Transform(m)
	attrs := res.Attributes.(Attributes)
	attrs.UploadURL = uploadURL
	res.Attributes = attrs
	return res
}

// TransformWithDownloadURL renders an object plus its presigned GET URL.
func TransformWithDownloadURL(m Model, downloadURL string) server.Resource {
	res := Transform(m)
	attrs := res.Attributes.(Attributes)
	attrs.DownloadURL = downloadURL
	res.Attributes = attrs
	return res
}

// TransformSlice converts a slice of Models to JSON:API Resources.
func TransformSlice(ms []Model) []server.Resource {
	out := make([]server.Resource, 0, len(ms))
	for _, m := range ms {
		out = append(out, Transform(m))
	}
	return out
}
