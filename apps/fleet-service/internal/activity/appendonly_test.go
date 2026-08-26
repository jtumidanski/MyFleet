package activity_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/activity"
)

// The domain's own API stays mutation-free even though an admin transfer
// re-points fleet_id in raw SQL. Adding an Update or Delete here must be a
// deliberate act, not drift — so this fails the moment one appears.
func TestAdministrator_exposesNoMutationMethods(t *testing.T) {
	typ := reflect.TypeOf((*activity.Administrator)(nil)).Elem()
	for i := 0; i < typ.NumMethod(); i++ {
		name := typ.Method(i).Name
		if strings.HasPrefix(name, "Update") || strings.HasPrefix(name, "Delete") ||
			strings.HasPrefix(name, "Set") {
			t.Errorf("activity.Administrator gained %q; the feed is append-only "+
				"except for the admin transfer's fleet_id rewrite, which lives in internal/admin", name)
		}
	}
}
