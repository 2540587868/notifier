package channel

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/ysqss/notifier/internal/message"
	"github.com/ysqss/notifier/internal/retrier"
)

type EmailChannel struct {
	smtpHost string
	smtpPort string
	username string
	password string
	from     string
	to       []string
	retrier  *retrier.Retrier
}

func NewEmailChannel(smtpHost, smtpPort, username, password, from string, to []string) *EmailChannel {
	return &EmailChannel{
		smtpHost: smtpHost,
		smtpPort: smtpPort,
		username: username,
		password: password,
		from:     from,
		to:       to,
		retrier:  retrier.New(3, 2*time.Second, 30*time.Second),
	}
}

func newEmailFromConfig(cfg map[string]string) (Channel, error) {
	smtpHost := cfg["smtp_host"]
	smtpPort := cfg["smtp_port"]
	username := cfg["username"]
	password := cfg["password"]
	from := cfg["from"]
	toStr := cfg["to"]

	if smtpHost == "" || smtpPort == "" || username == "" || password == "" || from == "" || toStr == "" {
		return nil, fmt.Errorf("smtp_host, smtp_port, username, password, from, to are all required")
	}
	to := strings.Split(toStr, ",")
	for i := range to {
		to[i] = strings.TrimSpace(to[i])
	}
	return NewEmailChannel(smtpHost, smtpPort, username, password, from, to), nil
}

func (e *EmailChannel) Name() string { return "email" }

func (e *EmailChannel) Send(ctx context.Context, msg *message.RenderedMessage) error {
	payload, ok := msg.Payload.(string)
	if !ok {
		return fmt.Errorf("invalid payload type for email channel")
	}

	subject := fmt.Sprintf("[%s] %s", strings.ToUpper(string(msg.Original.Level)), msg.Original.Title)
	toStr := strings.Join(e.to, ", ")

	headers := make(map[string]string)
	headers["From"] = e.from
	headers["To"] = toStr
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	var mail bytes.Buffer
	for k, v := range headers {
		fmt.Fprintf(&mail, "%s: %s\r\n", k, v)
	}
	mail.WriteString("\r\n")
	mail.WriteString(payload)

	addr := e.smtpHost + ":" + e.smtpPort
	auth := smtp.PlainAuth("", e.username, e.password, e.smtpHost)

	return e.retrier.Do(ctx, func() error {
		conn, err := tls.Dial("tcp", addr, &tls.Config{
			ServerName: e.smtpHost,
		})
		if err != nil {
			return fmt.Errorf("tls dial: %w", err)
		}

		c, err := smtp.NewClient(conn, e.smtpHost)
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf("smtp client: %w", err)
		}

		err = c.Auth(auth)
		if err != nil {
			_ = c.Close()
			return fmt.Errorf("smtp auth: %w", err)
		}

		err = c.Mail(e.from)
		if err != nil {
			_ = c.Close()
			return fmt.Errorf("smtp mail: %w", err)
		}

		for _, to := range e.to {
			err = c.Rcpt(to)
			if err != nil {
				_ = c.Close()
				return fmt.Errorf("smtp rcpt: %w", err)
			}
		}

		w, err := c.Data()
		if err != nil {
			_ = c.Close()
			return fmt.Errorf("smtp data: %w", err)
		}

		if _, err := w.Write(mail.Bytes()); err != nil {
			_ = w.Close()
			_ = c.Close()
			return fmt.Errorf("smtp write: %w", err)
		}

		if err := w.Close(); err != nil {
			_ = c.Close()
			return fmt.Errorf("smtp close: %w", err)
		}

		_ = c.Quit()
		return nil
	})
}

func (e *EmailChannel) Validate(cfg map[string]string) error {
	required := []string{"smtp_host", "smtp_port", "username", "password", "from", "to"}
	for _, key := range required {
		if cfg[key] == "" {
			return fmt.Errorf("%s is required for email channel", key)
		}
	}
	return nil
}
