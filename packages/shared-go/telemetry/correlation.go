package telemetry

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type ctxKey int

const correlationKey ctxKey = iota

const headerCorrelationID = "X-Correlation-ID"

// CorrelationID middleware ensures every request carries a correlation id on
// its context and echoes it on the response (design §15).
func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerCorrelationID)
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(headerCorrelationID, id)
		ctx := context.WithValue(r.Context(), correlationKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func CorrelationIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(correlationKey).(string); ok {
		return v
	}
	return ""
}
