package telemetry

import (
	"github.com/sirupsen/logrus"
	"github.com/jtumidanski/myfleet/packages/shared-go/config"
)

// NewLogger returns a JSON structured logger; level from LOG_LEVEL.
func NewLogger() *logrus.Logger {
	l := logrus.New()
	l.SetFormatter(&logrus.JSONFormatter{})
	if lvl, err := logrus.ParseLevel(config.Get("LOG_LEVEL", "info")); err == nil {
		l.SetLevel(lvl)
	}
	return l
}
