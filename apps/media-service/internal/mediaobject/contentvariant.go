package mediaobject

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// ContentVariant names which rendition of a media object's bytes to serve.
// The values match mediavariant.Variant's for the derived kinds; "original"
// has no variant row because it is the uploaded bytes themselves.
type ContentVariant string

const (
	ContentOriginal  ContentVariant = "original"
	ContentThumbnail ContentVariant = "thumbnail"
	ContentCard      ContentVariant = "card"
	ContentDisplay   ContentVariant = "display"
)

// ParseContentVariant maps the raw ?variant= query value to a ContentVariant.
//
// An absent or empty parameter means the original, which preserves the
// pre-existing contract of GET /media/{id}/content exactly. Anything else that
// is not an exact lowercase match is server.ErrBadRequest (400) — never a
// silent fallback to the original, which would ship multi-megabyte responses
// for a typo.
func ParseContentVariant(raw string) (ContentVariant, error) {
	switch raw {
	case "":
		return ContentOriginal, nil
	case string(ContentOriginal):
		return ContentOriginal, nil
	case string(ContentThumbnail):
		return ContentThumbnail, nil
	case string(ContentCard):
		return ContentCard, nil
	case string(ContentDisplay):
		return ContentDisplay, nil
	default:
		return "", server.ErrBadRequest
	}
}
