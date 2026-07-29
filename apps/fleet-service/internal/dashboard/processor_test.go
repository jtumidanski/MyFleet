package dashboard

import (
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func TestValidateLayout_rejectsUnknownWidget(t *testing.T) {
	t.Run("unknown widget type returns ErrValidation", func(t *testing.T) {
		widgets := []WidgetInput{
			{Type: "crypto-prices", PositionX: 0, PositionY: 0, Width: 1, Height: 1},
		}
		err := ValidateLayout(widgets)
		if !errors.Is(err, server.ErrValidation) {
			t.Fatalf("want ErrValidation for unknown widget type, got %v", err)
		}
	})

	t.Run("all catalog types pass", func(t *testing.T) {
		widgets := []WidgetInput{}
		for wt := range ValidCatalog {
			widgets = append(widgets, WidgetInput{
				Type:      wt,
				PositionX: 0,
				PositionY: 0,
				Width:     1,
				Height:    1,
			})
		}
		if err := ValidateLayout(widgets); err != nil {
			t.Fatalf("all catalog types should pass, got %v", err)
		}
	})

	t.Run("empty layout passes", func(t *testing.T) {
		if err := ValidateLayout(nil); err != nil {
			t.Fatalf("empty layout should pass, got %v", err)
		}
	})
}
