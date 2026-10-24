package infrastructure

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

type DriverRegistry struct {
	drivers map[string]domain.ChannelDriver
}

func NewDriverRegistry(cache cacheinfra.Manager) *DriverRegistry {
	return NewDriverRegistryWithHTTPClient(cache, nil)
}

// NewDriverRegistryWithOutboundGuard wires fixed-provider application traffic
// through the same SSRF guard used by URL-channel validation. A nil guard
// intentionally leaves G4 application adapters unavailable rather than
// creating an unguarded fallback HTTP client.
func NewDriverRegistryWithOutboundGuard(cache cacheinfra.Manager, guard *outboundurl.OutboundURLGuard, policies ...outboundurl.PolicyResolver) *DriverRegistry {
	if guard == nil {
		return NewDriverRegistryWithHTTPClient(cache, nil)
	}
	var resolver outboundurl.PolicyResolver
	if len(policies) > 0 {
		resolver = policies[0]
	}
	clientFor := func(policy outboundurl.Policy) enterpriseHTTPDoer {
		return guard.HTTPClient(policy)
	}
	return newDriverRegistry(cache, clientFor(outboundurl.Policy{Mode: outboundurl.ModePublic}), clientFor, resolver)
}

// NewDriverRegistryWithHTTPClient exists for isolated adapter tests. Runtime
// wiring must call NewDriverRegistryWithOutboundGuard instead.
func NewDriverRegistryWithHTTPClient(cache cacheinfra.Manager, client enterpriseHTTPDoer) *DriverRegistry {
	clientFor := func(outboundurl.Policy) enterpriseHTTPDoer { return client }
	return newDriverRegistry(cache, client, clientFor, nil)
}

func newDriverRegistry(cache cacheinfra.Manager, client enterpriseHTTPDoer, staticClients staticHTTPClientFactory, policies outboundurl.PolicyResolver) *DriverRegistry {
	drivers := map[string]domain.ChannelDriver{
		domain.ChannelTypeMock:  &MockDriver{cache: cache},
		domain.ChannelTypeEmail: SMTPDriver{},
	}
	if client != nil {
		tokens := newEnterpriseTokenCache()
		drivers[domain.ChannelTypeFeishuApp] = &enterpriseApplicationDriver{provider: domain.ChannelTypeFeishuApp, client: client, tokens: tokens}
		drivers[domain.ChannelTypeWeComApp] = &enterpriseApplicationDriver{provider: domain.ChannelTypeWeComApp, client: client, tokens: tokens}
		drivers[domain.ChannelTypeHTTPConnector] = &staticHTTPDriver{channelType: domain.ChannelTypeHTTPConnector, clients: staticClients, policies: policies}
		drivers[domain.ChannelTypeFeishuWebhook] = &staticHTTPDriver{channelType: domain.ChannelTypeFeishuWebhook, clients: staticClients, policies: policies}
		drivers[domain.ChannelTypeWeComWebhook] = &staticHTTPDriver{channelType: domain.ChannelTypeWeComWebhook, clients: staticClients, policies: policies}
	}
	return &DriverRegistry{drivers: drivers}
}

func (r *DriverRegistry) Driver(channelType string) domain.ChannelDriver {
	if r == nil {
		return nil
	}
	return r.drivers[strings.ToUpper(strings.TrimSpace(channelType))]
}

func (r *DriverRegistry) Capabilities(channelType string) domain.DriverCapabilities {
	switch strings.ToUpper(strings.TrimSpace(channelType)) {
	case domain.ChannelTypeEmail:
		return domain.DriverCapabilities{SupportedPresentations: []string{domain.DeliveryPresentationHTML, domain.DeliveryPresentationText}, SupportsPrimaryAction: true, SupportsExternalURL: true, OutcomeSemantics: "SMTP_ACCEPTED_OR_UNKNOWN"}
	case domain.ChannelTypeFeishuApp:
		return domain.DriverCapabilities{SupportedPresentations: []string{domain.DeliveryPresentationCard, domain.DeliveryPresentationText}, SupportsPrimaryAction: true, SupportsExternalURL: true, SupportsProviderIdempotency: true, OutcomeSemantics: "PROVIDER_IDEMPOTENT"}
	case domain.ChannelTypeWeComApp:
		return domain.DriverCapabilities{SupportedPresentations: []string{domain.DeliveryPresentationCard, domain.DeliveryPresentationText}, SupportsPrimaryAction: true, SupportsExternalURL: true, OutcomeSemantics: "ACCEPTED_OR_UNKNOWN"}
	case domain.ChannelTypeFeishuWebhook:
		return domain.DriverCapabilities{SupportedPresentations: []string{domain.DeliveryPresentationCard, domain.DeliveryPresentationText}, SupportsPrimaryAction: true, SupportsExternalURL: true, OutcomeSemantics: "ACCEPTED_OR_UNKNOWN"}
	case domain.ChannelTypeWeComWebhook:
		return domain.DriverCapabilities{SupportedPresentations: []string{domain.DeliveryPresentationCard, domain.DeliveryPresentationMarkdown, domain.DeliveryPresentationText}, SupportsPrimaryAction: true, SupportsExternalURL: true, OutcomeSemantics: "ACCEPTED_OR_UNKNOWN"}
	case domain.ChannelTypeHTTPConnector:
		return domain.DriverCapabilities{SupportedPresentations: []string{domain.DeliveryPresentationText}, SupportsPrimaryAction: true, SupportsExternalURL: true, OutcomeSemantics: "ACCEPTED_OR_UNKNOWN"}
	default:
		return domain.DriverCapabilities{SupportedPresentations: []string{domain.DeliveryPresentationText}}
	}
}

type MockDriver struct {
	cache cacheinfra.Manager
}

func (d *MockDriver) Send(ctx context.Context, message domain.DriverMessage) error {
	if strings.TrimSpace(message.Target) == "" {
		return fmt.Errorf("notification target must not be empty")
	}
	if d == nil || d.cache == nil {
		return nil
	}
	prefix := "notification:mock:capture"
	var cfg map[string]any
	if err := json.Unmarshal([]byte(message.Channel.ConfigJSON), &cfg); err == nil {
		if value, ok := cfg["capturePrefix"].(string); ok && strings.TrimSpace(value) != "" {
			prefix = strings.TrimSpace(value)
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"target":   message.Target,
		"subject":  message.Subject,
		"text":     message.Text,
		"html":     message.HTML,
		"markdown": message.Markdown,
		"sentAt":   time.Now().UTC().Format(time.RFC3339Nano),
	})
	return d.cache.SetBytes(ctx, prefix+":"+strings.ToLower(strings.TrimSpace(message.Target)), payload, 10*time.Minute)
}

type SMTPDriver struct{}

type smtpConfig struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	From          string `json:"from"`
	UseTLS        bool   `json:"useTls"`
	StartTLS      bool   `json:"startTls"`
	Timeout       string `json:"timeout"`
	SkipVerify    bool   `json:"skipVerify"`
	TLSServerName string `json:"tlsServerName"`
}

// smtpSendError retains only the delivery stage needed for a safe operator
// diagnostic.  The wrapped provider error stays inside the driver boundary and
// is intentionally never persisted to Delivery.LastError or surfaced to a
// notification recipient.
type smtpSendError struct {
	stage string
	err   error
}

func (e *smtpSendError) Error() string { return e.err.Error() }
func (e *smtpSendError) Unwrap() error { return e.err }

func smtpFailure(stage string, err error) error {
	if err == nil {
		return nil
	}
	return &smtpSendError{stage: stage, err: err}
}

func smtpDiagnostic(err error) string {
	var phase *smtpSendError
	if !errors.As(err, &phase) {
		return "SMTP_BEFORE_DATA_FAILED"
	}
	switch phase.stage {
	case "CONFIG":
		return "SMTP_CONFIG_INVALID"
	case "CONNECT":
		return "SMTP_CONNECT_FAILED"
	case "GREETING":
		return "SMTP_GREETING_FAILED"
	case "TLS":
		return "SMTP_TLS_FAILED"
	case "AUTH":
		return "SMTP_AUTH_FAILED"
	case "MAIL_FROM":
		return "SMTP_MAIL_FROM_REJECTED"
	case "RECIPIENT":
		return "SMTP_RECIPIENT_REJECTED"
	case "DATA":
		return "SMTP_DATA_START_FAILED"
	default:
		return "SMTP_BEFORE_DATA_FAILED"
	}
}

func resolveSMTPServerName(cfg smtpConfig, host string) string {
	if value := strings.TrimSpace(cfg.TLSServerName); value != "" {
		return value
	}
	return strings.TrimSpace(host)
}

func (SMTPDriver) Send(ctx context.Context, message domain.DriverMessage) error {
	_, err := sendSMTP(ctx, message)
	return err
}

func sendSMTP(ctx context.Context, message domain.DriverMessage) (bool, error) {
	var cfg smtpConfig
	if err := json.Unmarshal([]byte(message.Channel.ConfigJSON), &cfg); err != nil {
		return false, smtpFailure("CONFIG", fmt.Errorf("parse smtp channel config: %w", err))
	}
	if strings.TrimSpace(cfg.Host) == "" {
		return false, smtpFailure("CONFIG", fmt.Errorf("smtp host must not be empty"))
	}
	if cfg.Port <= 0 {
		cfg.Port = 25
	}
	timeout := 10 * time.Second
	if strings.TrimSpace(cfg.Timeout) != "" {
		if parsed, err := time.ParseDuration(cfg.Timeout); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	host := strings.TrimSpace(cfg.Host)
	serverName := resolveSMTPServerName(cfg, host)
	addr := fmt.Sprintf("%s:%d", host, cfg.Port)
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false, smtpFailure("CONNECT", err)
	}
	defer conn.Close()
	var client *smtp.Client
	if cfg.UseTLS {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: serverName, InsecureSkipVerify: cfg.SkipVerify}) //nolint:gosec
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return false, smtpFailure("TLS", err)
		}
		client, err = smtp.NewClient(tlsConn, serverName)
	} else {
		client, err = smtp.NewClient(conn, serverName)
	}
	if err != nil {
		return false, smtpFailure("GREETING", err)
	}
	defer client.Quit()
	if cfg.StartTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: serverName, InsecureSkipVerify: cfg.SkipVerify}); err != nil { //nolint:gosec
				return false, smtpFailure("TLS", err)
			}
		}
	}
	if strings.TrimSpace(cfg.Username) != "" {
		if err := client.Auth(smtp.PlainAuth("", cfg.Username, message.SecretPlain, serverName)); err != nil {
			return false, smtpFailure("AUTH", err)
		}
	}
	from := choose(cfg.From, message.Channel.ChannelName)
	if !strings.Contains(from, "@") && strings.Contains(cfg.Username, "@") {
		from = cfg.Username
	}
	fromAddress := extractAddress(from)
	if err := client.Mail(fromAddress); err != nil {
		return false, smtpFailure("MAIL_FROM", err)
	}
	if err := client.Rcpt(message.Target); err != nil {
		return false, smtpFailure("RECIPIENT", err)
	}
	writer, err := client.Data()
	if err != nil {
		return false, smtpFailure("DATA", err)
	}
	_, err = writer.Write([]byte(buildMIMEMessage(from, message.Target, message.Subject, externalTextWithAction(message), externalHTMLWithAction(message))))
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	return true, err
}

func (driver SMTPDriver) SendResult(ctx context.Context, message domain.DriverMessage) (domain.DriverResult, error) {
	afterData, err := sendSMTP(ctx, message)
	if err != nil && afterData {
		// net/smtp does not expose a reliable post-DATA acceptance boundary when
		// the final response is lost. Preserve uncertainty and never blindly send
		// a second email with another presentation.
		return domain.DriverResult{Status: domain.DriverResultUnknown, FailureClass: "SMTP_OUTCOME_UNKNOWN", Diagnostic: "SMTP_DATA_RESPONSE_UNCONFIRMED"}, nil
	}
	if err != nil {
		retryable := true
		failureClass := "SMTP_TRANSPORT"
		if smtpErr, ok := err.(*textproto.Error); ok {
			failureClass = "SMTP_REJECTED"
			retryable = smtpErr.Code >= 400 && smtpErr.Code < 500
		}
		return domain.DriverResult{Status: domain.DriverResultFailed, FailureClass: failureClass, Diagnostic: smtpDiagnostic(err), Retryable: retryable}, nil
	}
	return domain.DriverResult{Status: domain.DriverResultProviderAccepted, FailureClass: "", Diagnostic: "PROVIDER_ACCEPTED"}, nil
}

func buildMIMEMessage(from, to, subject, textBody, htmlBody string) string {
	boundary := "seven-notification-boundary"
	var builder strings.Builder
	builder.WriteString("From: " + safeMailHeader(from) + "\r\n")
	builder.WriteString("To: " + safeMailHeader(to) + "\r\n")
	builder.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", safeMailHeader(subject)) + "\r\n")
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
	builder.WriteString("--" + boundary + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + textBody + "\r\n")
	builder.WriteString("--" + boundary + "\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n" + htmlBody + "\r\n")
	builder.WriteString("--" + boundary + "--\r\n")
	return builder.String()
}

func safeMailHeader(value string) string {
	return strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(value))
}

func externalTextWithAction(message domain.DriverMessage) string {
	text := strings.TrimSpace(message.Text)
	if strings.TrimSpace(message.ExternalActionURL) == "" || message.PrimaryAction == nil {
		return text
	}
	return text + "\n\n" + message.PrimaryAction.Label + "：" + message.ExternalActionURL
}

func externalHTMLWithAction(message domain.DriverMessage) string {
	body := strings.TrimSpace(message.HTML)
	if body == "" {
		body = "<p>" + strings.ReplaceAll(html.EscapeString(message.Text), "\n", "<br>") + "</p>"
	}
	if strings.TrimSpace(message.ExternalActionURL) == "" || message.PrimaryAction == nil {
		return body
	}
	return body + `<p><a style="display:inline-block;padding:10px 18px;border-radius:8px;background:#1677ff;color:#fff;text-decoration:none" href="` + html.EscapeString(message.ExternalActionURL) + `">` + html.EscapeString(message.PrimaryAction.Label) + `</a></p>`
}

func choose(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func extractAddress(value string) string {
	if parsed, err := mail.ParseAddress(value); err == nil {
		return parsed.Address
	}
	return strings.TrimSpace(value)
}
