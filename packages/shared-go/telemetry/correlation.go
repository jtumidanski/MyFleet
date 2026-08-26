package telemetry

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type ctxKey int

const correlationKey ctxKey = iota

// HeaderCorrelationID is the header the middleware reads, echoes, and that
// outbound service-to-service clients set so one user action can be followed
// across services. It is exported because fleet-service's adminclient sets it
// on every internal call, and a duplicated literal there would drift.
const HeaderCorrelationID = "X-Correlation-ID"

// CorrelationID middleware ensures every request carries a correlation id on
// its context and echoes it on the response (design §15).
func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderCorrelationID)
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(HeaderCorrelationID, id)
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

// ContextWithCorrelationID seeds a context with an id. Production code gets one
// from the CorrelationID middleware; this exists so a client test can assert
// the header is propagated without standing up a server to mint one.
func ContextWithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey, id)
}
