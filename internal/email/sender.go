// SPDX-License-Identifier: AGPL-3.0-only
package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"io"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"chastedesk/internal/id"
)

type SenderConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLS      string
}

type Sender struct {
	cfg SenderConfig
}

func NewSender(cfg SenderConfig) *Sender {
	return &Sender{cfg: cfg}
}

func (s *Sender) Enabled() bool {
	return s != nil && s.cfg.Host != "" && s.cfg.From != ""
}

func (s *Sender) From() string {
	if s == nil {
		return ""
	}
	return s.cfg.From
}

func (s *Sender) MessageID() string {
	domain := "chastedesk.local"
	if parsed, err := mail.ParseAddress(s.cfg.From); err == nil {
		if at := strings.LastIndexByte(parsed.Address, '@'); at >= 0 && at < len(parsed.Address)-1 {
			domain = parsed.Address[at+1:]
		}
	}
	return "<" + id.UUID() + "@" + domain + ">"
}

func (s *Sender) Send(ctx context.Context, to, subject, textBody, htmlBody, messageID, inReplyTo, references string) error {
	if !s.Enabled() {
		return fmt.Errorf("email: SMTP is not configured")
	}
	if _, err := mail.ParseAddress(to); err != nil {
		return fmt.Errorf("email: invalid recipient: %w", err)
	}
	body, err := buildMessage(s.cfg.From, to, subject, textBody, htmlBody, messageID, inReplyTo, references)
	if err != nil {
		return err
	}
	hostPort := net.JoinHostPort(s.cfg.Host, fmt.Sprintf("%d", s.cfg.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	if s.cfg.TLS == "tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", hostPort, &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", hostPort)
	}
	if err != nil {
		return fmt.Errorf("email: dial SMTP: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
	}
	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("email: SMTP handshake: %w", err)
	}
	defer client.Close()
	if s.cfg.TLS == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("email: SMTP server does not advertise STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("email: STARTTLS: %w", err)
		}
	}
	if s.cfg.Username != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("email: SMTP server does not advertise AUTH")
		}
		if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("email: SMTP auth: %w", err)
		}
	}
	if err := client.Mail(s.cfg.From); err != nil {
		return fmt.Errorf("email: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("email: RCPT TO: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: DATA: %w", err)
	}
	if _, err := io.WriteString(writer, body); err != nil {
		_ = writer.Close()
		return fmt.Errorf("email: write body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("email: finish body: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("email: QUIT: %w", err)
	}
	return nil
}

func buildMessage(from, to, subject, textBody, htmlBody, messageID, inReplyTo, references string) (string, error) {
	if strings.TrimSpace(messageID) == "" {
		return "", fmt.Errorf("email: message id is required")
	}
	textBody = ensureNewline(textBody)
	if strings.TrimSpace(htmlBody) == "" {
		htmlBody = "<!doctype html><html><body style=\"font-family: system-ui, sans-serif; line-height: 1.6; color: #211b16;\"><p>" + strings.ReplaceAll(html.EscapeString(strings.TrimSpace(textBody)), "\n", "<br>") + "</p></body></html>"
	}
	boundary := "chastedesk-" + strings.TrimPrefix(strings.TrimSuffix(messageID, ">"), "<")
	var builder strings.Builder
	writeHeader(&builder, "From", from)
	writeHeader(&builder, "To", to)
	writeHeader(&builder, "Subject", mime.QEncoding.Encode("utf-8", subject))
	writeHeader(&builder, "Date", time.Now().UTC().Format(time.RFC1123Z))
	writeHeader(&builder, "Message-ID", messageID)
	if strings.TrimSpace(inReplyTo) != "" {
		writeHeader(&builder, "In-Reply-To", inReplyTo)
	}
	if strings.TrimSpace(references) != "" {
		writeHeader(&builder, "References", references)
	}
	writeHeader(&builder, "MIME-Version", "1.0")
	writeHeader(&builder, "Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	builder.WriteString("\r\n")
	builder.WriteString("--" + boundary + "\r\n")
	builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	builder.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	builder.WriteString(textBody)
	builder.WriteString("\r\n--" + boundary + "\r\n")
	builder.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	builder.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	builder.WriteString(htmlBody)
	builder.WriteString("\r\n--" + boundary + "--\r\n")
	return builder.String(), nil
}

func writeHeader(builder *strings.Builder, name, value string) {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\n", "")
	builder.WriteString(name)
	builder.WriteString(": ")
	builder.WriteString(value)
	builder.WriteString("\r\n")
}

func ensureNewline(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	if !strings.HasSuffix(value, "\n") {
		value += "\n"
	}
	return value
}
