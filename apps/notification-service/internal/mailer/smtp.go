package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"time"
)

type smtpSender struct{ cfg Config }

// NewSMTPSender returns a Sender that delivers over SMTP.
//
// TLS verification is ON in both TLS modes and there is no skip-verify escape
// hatch in config or code (FR-MAIL-2).
func NewSMTPSender(cfg Config) Sender { return &smtpSender{cfg: cfg} }

// Send composes and delivers one message. Errors are classified so the caller
// can tell a retryable blip from a rejected mailbox (FR-MAIL-5); the message
// body is never attached to an error (FR-OBS-2).
func (s *smtpSender) Send(ctx context.Context, msg Message) error {
	boundary, err := newBoundary()
	if err != nil {
		return err
	}
	messageID, err := newMessageID(s.cfg.FromAddress)
	if err != nil {
		// A From address with no domain is a configuration error that no retry
		// fixes.
		return &PermanentError{Err: err}
	}
	raw, err := compose(s.cfg.FromName, s.cfg.FromAddress, msg, time.Now(), messageID, boundary)
	if err != nil {
		return &PermanentError{Err: err}
	}

	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	c, err := s.dial(ctx)
	if err != nil {
		return classify(err)
	}
	defer func() { _ = c.Close() }()

	if err := s.authenticate(c); err != nil {
		return classify(err)
	}
	return classify(deliver(c, s.cfg.FromAddress, msg.To, raw))
}

// dial opens a connection appropriate to the configured TLS mode.
func (s *smtpSender) dial(ctx context.Context) (*smtp.Client, error) {
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port))
	// Timeout on the dialer as well as the context: tls.DialWithDialer takes no
	// context, so without this a black-holed relay hangs the implicit-TLS path.
	d := &net.Dialer{Timeout: s.cfg.Timeout}

	if s.cfg.TLSMode == TLSModeTLS {
		conn, err := tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return nil, err
		}
		// net.Conn Read/Write do not observe ctx cancellation — only the initial
		// dial does. Without an explicit deadline here, a relay that completes
		// the handshake and then stalls (e.g. a middlebox eating post-handshake
		// traffic) hangs smtp.NewClient's greeting read forever. One absolute
		// deadline covers the whole session, matching cfg.Timeout's role as the
		// per-attempt budget (design.md: "a black-holed relay cannot hang the
		// goroutine").
		if err := conn.SetDeadline(time.Now().Add(s.cfg.Timeout)); err != nil {
			_ = conn.Close()
			return nil, err
		}
		c, err := smtp.NewClient(conn, s.cfg.Host)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return c, nil
	}

	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	// Same reasoning as the implicit-TLS branch above: bound every subsequent
	// read/write (greeting, EHLO/STARTTLS, AUTH, MAIL/RCPT/DATA/QUIT) with an
	// absolute deadline, since ctx alone does not reach net.Conn I/O.
	if err := conn.SetDeadline(time.Now().Add(s.cfg.Timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if s.cfg.TLSMode == TLSModeStartTLS {
		// Erroring when the server does not advertise STARTTLS is the whole
		// point: silently continuing in plaintext is the classic downgrade.
		if ok, _ := c.Extension("STARTTLS"); !ok {
			_ = c.Close()
			return nil, errors.New("relay does not offer STARTTLS and SMTP_TLS_MODE is starttls")
		}
		if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			_ = c.Close()
			return nil, err
		}
	}
	return c, nil
}

// authenticate is a no-op when no credentials are configured, which is legal
// only for the plaintext local relay (ConfigFromEnv enforces that).
func (s *smtpSender) authenticate(c *smtp.Client) error {
	if s.cfg.Username == "" {
		return nil
	}
	return c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host))
}

func deliver(c *smtp.Client, from, to string, raw []byte) error {
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// classify turns a transport error into the taxonomy the consumer branches on
// (FR-MAIL-5):
//
//	5xx on MAIL FROM / RCPT TO / DATA → PermanentError (rejected recipient,
//	  malformed address). Retrying a rejected mailbox forever wedges a partition.
//	4xx (greylisting, mailbox busy), dial failures, TLS handshake failures and
//	  timeouts → returned bare, i.e. transient.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var te *textproto.Error
	if errors.As(err, &te) && te.Code >= 500 && te.Code < 600 {
		return &PermanentError{Err: err}
	}
	return err
}
