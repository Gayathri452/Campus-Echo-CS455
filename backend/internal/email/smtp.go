package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"time"
)

const smtpTimeout = 10 * time.Second

type SMTP struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

func (s SMTP) SendOTP(ctx context.Context, recipient, code string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Host == "" || s.Port == "" || s.From == "" {
		return fmt.Errorf("SMTP email configuration is incomplete")
	}
	address := s.Host + ":" + s.Port
	dialer := net.Dialer{Timeout: smtpTimeout}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect to SMTP server: %w", err)
	}
	defer connection.Close()
	if s.Port == "465" {
		tlsConnection := tls.Client(connection, &tls.Config{
			ServerName: s.Host,
			MinVersion: tls.VersionTLS12,
		})
		if err := tlsConnection.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("start implicit SMTP TLS: %w", err)
		}
		connection = tlsConnection
	}
	_ = connection.SetDeadline(time.Now().Add(smtpTimeout))

	client, err := smtp.NewClient(connection, s.Host)
	if err != nil {
		return fmt.Errorf("create SMTP client: %w", err)
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("start SMTP TLS: %w", err)
		}
	}
	var auth smtp.Auth
	if s.Username != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
	}
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authenticate with SMTP server: %w", err)
		}
	}
	message := []byte("To: " + recipient + "\r\n" +
		"From: " + s.From + "\r\n" +
		"Subject: CampusEcho verification code\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		"Your CampusEcho verification code is: " + code + "\r\n")
	if err := client.Mail(s.From); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("start SMTP message: %w", err)
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("send SMTP message: %w", err)
	}
	// DATA completion means the SMTP server accepted the message. A later
	// QUIT failure must not make the caller delete the now-deliverable OTP.
	_ = client.Quit()
	return nil
}
