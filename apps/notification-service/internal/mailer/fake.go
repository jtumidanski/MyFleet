package mailer

import "context"

// FakeSender records what would have been sent and can be programmed to fail.
// Exported because mailconsumer's tests live in a different package.
//
// Errs is consumed one entry per call and takes precedence over Err, which lets
// a test express "fail twice, then succeed" — the shape the retry test needs.
type FakeSender struct {
	Sent []Message
	Err  error
	Errs []error
}

func (f *FakeSender) Send(_ context.Context, msg Message) error {
	f.Sent = append(f.Sent, msg)
	if len(f.Errs) > 0 {
		err := f.Errs[0]
		f.Errs = f.Errs[1:]
		return err
	}
	return f.Err
}

// Calls reports how many Send attempts were made, including failed ones.
func (f *FakeSender) Calls() int { return len(f.Sent) }
