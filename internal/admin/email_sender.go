package admin

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLSMode  string
}

type smtpEmailSender struct {
	config SMTPConfig
	from   string
	slots  chan struct{}
}

func newSMTPEmailSender(config SMTPConfig) (*smtpEmailSender, error) {
	if config.Host == "" && config.Username == "" && config.Password == "" && config.From == "" {
		return nil, nil
	}
	from, err := normalizeRecoveryEmail(config.From)
	if err != nil || strings.TrimSpace(config.Host) != config.Host || strings.ContainsAny(config.Host, "\r\n/: ") || config.Host == "" || config.Port < 1 || config.Port > 65535 || config.Username == "" || config.Password == "" || (config.TLSMode != "tls" && config.TLSMode != "starttls") {
		return nil, errors.New("后台邮件配置不完整或无效，需有效 SMTP 地址、端口、账号、专用密码、发件地址与加密模式")
	}
	return &smtpEmailSender{config: config, from: from, slots: make(chan struct{}, 2)}, nil
}

func (s *smtpEmailSender) send(ctx context.Context, recipient, subject, body string) error {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	conn, err := (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(s.config.Host, strconv.Itoa(s.config.Port)))
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline := time.Now().Add(15 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	tlsConfig := &tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12}
	var client *smtp.Client
	if s.config.TLSMode == "tls" {
		secure := tls.Client(conn, tlsConfig)
		if err := secure.HandshakeContext(ctx); err != nil {
			return err
		}
		client, err = smtp.NewClient(secure, s.config.Host)
	} else {
		client, err = smtp.NewClient(conn, s.config.Host)
		if err == nil {
			err = client.StartTLS(tlsConfig)
		}
	}
	if client != nil {
		defer client.Close()
	}
	if err != nil {
		return err
	}
	if err := client.Auth(smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)); err != nil {
		return err
	}
	if err := client.Mail(s.from); err != nil {
		return err
	}
	if err := client.Rcpt(recipient); err != nil {
		return err
	}
	messageID, err := randomToken(16)
	if err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	domain := strings.SplitN(s.from, "@", 2)[1]
	var message strings.Builder
	fmt.Fprintf(&message, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s@%s>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n", (&mail.Address{Name: "Track 管理后台", Address: s.from}).String(), (&mail.Address{Address: recipient}).String(), mime.QEncoding.Encode("UTF-8", subject), time.Now().Format(time.RFC1123Z), messageID, domain)
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	for len(encoded) > 0 {
		n := 76
		if len(encoded) < n {
			n = len(encoded)
		}
		message.WriteString(encoded[:n] + "\r\n")
		encoded = encoded[n:]
	}
	if _, err := writer.Write([]byte(message.String())); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	// The DATA acknowledgement confirms acceptance; a lost QUIT reply must not
	// turn a delivered code into an invalid code or encourage duplicate sends.
	_ = client.Quit()
	return nil
}
