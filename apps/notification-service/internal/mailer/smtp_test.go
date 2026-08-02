package mailer

import (
	"errors"
	"net/textproto"
	"testing"
)

// FR-MAIL-5. Classification decides whether the consumer burns four attempts or
// gives up after one, so it is asserted directly rather than inferred from
// consumer behaviour.
func TestClassify(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		permanent bool
	}{
		{"5xx recipient rejected", &textproto.Error{Code: 550, Msg: "no such user"}, true},
		{"5xx syntax", &textproto.Error{Code: 501, Msg: "bad address"}, true},
		{"4xx greylisting", &textproto.Error{Code: 451, Msg: "try again later"}, false},
		{"4xx mailbox busy", &textproto.Error{Code: 450, Msg: "busy"}, false},
		{"dial failure", errors.New("dial tcp: connection refused"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classify(c.err)
			if c.err == nil {
				if got != nil {
					t.Fatalf("classify(nil)=%v want nil", got)
				}
				return
			}
			var perm *PermanentError
			if isPerm := errors.As(got, &perm); isPerm != c.permanent {
				t.Fatalf("classify(%v) permanent=%v want %v", c.err, isPerm, c.permanent)
			}
			// The original error must remain inspectable either way.
			if !errors.Is(got, c.err) {
				t.Fatalf("classify lost the underlying error: %v", got)
			}
		})
	}
}
