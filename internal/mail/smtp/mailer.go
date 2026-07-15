// Package smtp provides SMTP email sending functionality.
package smtp

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
)

// Config holds the SMTP server configuration.
type Config struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromEmail   string
	FromName    string
	StartTLS    bool
	AuthEnabled bool
}

// Mailer handles sending emails via SMTP.
type Mailer struct {
	config Config
}

// NewMailer creates a new SMTP mailer with the given configuration.
func NewMailer(config Config) *Mailer {
	return &Mailer{config: config}
}

// Message represents an email message to be sent.
type Message struct {
	ToEmails []string
	Subject  string
	Body     string
	IsHTML   bool
}

// SendEmail sends an email message.
func (m *Mailer) SendEmail(msg Message) error {
	// Build email message
	var contentType string
	if msg.IsHTML {
		contentType = "Content-Type: text/html; charset=UTF-8\r\n"
	} else {
		contentType = "Content-Type: text/plain; charset=UTF-8\r\n"
	}
	subject := fmt.Sprintf("Subject: %s\r\n", msg.Subject)
	headers := fmt.Sprintf("From: %s <%s>\r\nTo: %s\r\n", m.config.FromName, m.config.FromEmail, strings.Join(msg.ToEmails, ","))
	message := []byte(subject + headers + contentType + "\r\n" + msg.Body)

	addr := fmt.Sprintf("%s:%d", m.config.Host, m.config.Port)
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()

	if m.config.StartTLS {
		tlsConfig := &tls.Config{
			ServerName: m.config.Host,
			MinVersion: tls.VersionTLS12,
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}

	if m.config.AuthEnabled {
		auth := smtp.PlainAuth("", m.config.Username, m.config.Password, m.config.Host)
		if err = client.Auth(auth); err != nil {
			return err
		}
	}

	if err = client.Mail(m.config.FromEmail); err != nil {
		return err
	}
	for _, recipient := range msg.ToEmails {
		if err = client.Rcpt(recipient); err != nil {
			return err
		}
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(message)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	if err = client.Quit(); err != nil {
		return err
	}
	return nil
}
