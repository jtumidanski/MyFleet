package events

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type capture struct{ published []Envelope }

func (c *capture) Publish(_ context.Context, e Envelope) error {
	c.published = append(c.published, e)
	return nil
}

func TestRelayOnce_publishesAndMarksSent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateOutbox(db); err != nil {
		t.Fatal(err)
	}
	if err := Enqueue(db, Envelope{EventID: "e1", Type: "vehicle.created", FleetID: "f1"}); err != nil {
		t.Fatal(err)
	}
	cap := &capture{}
	if err := RelayOnce(context.Background(), logrus.New(), db, cap); err != nil {
		t.Fatal(err)
	}
	if len(cap.published) != 1 || cap.published[0].EventID != "e1" {
		t.Fatalf("want 1 published e1, got %+v", cap.published)
	}
	var unsent int64
	db.Model(&OutboxRow{}).Where("sent_at IS NULL").Count(&unsent)
	if unsent != 0 {
		t.Fatalf("want 0 unsent, got %d", unsent)
	}
}
