package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"go.uber.org/zap"
)

type Sender interface {
	SendChallengeOTP(ctx context.Context, request ChallengeOTPRequest) error
}

type ChallengeOTPRequest struct {
	ToEmail   string
	Code      string
	Scene     string
	SceneName string
	TTL       time.Duration
	Metadata  map[string]any
}

type Message struct {
	From    string
	To      string
	Subject string
	Text    string
	HTML    string
}

type Provider interface {
	Send(ctx context.Context, message Message) error
}

type DeliveryStore interface {
	Capture(ctx context.Context, scene, toEmail, code string, message Message, ttl time.Duration) error
}

type Service struct {
	cfg      config.EmailConfig
	provider Provider
	store    DeliveryStore
	log      *zap.Logger
}

func New(cfg config.EmailConfig, cache cacheinfra.Manager, log *zap.Logger) (*Service, error) {
	if log == nil {
		log = zap.NewNop()
	}
	service := &Service{cfg: cfg, log: log.Named("email")}
	if !cfg.Enabled {
		return service, nil
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "mock":
		service.provider = NewMockProvider(cache, cfg.Mock)
		service.store = service.provider.(DeliveryStore)
	case "smtp":
		provider, err := NewSMTPProvider(cfg)
		if err != nil {
			return nil, err
		}
		service.provider = provider
		service.store = NewMockCaptureStore(cache, cfg.Mock)
	default:
		return nil, fmt.Errorf("unsupported email provider %q", cfg.Provider)
	}
	return service, nil
}

func Disabled() *Service {
	return &Service{}
}

func (s *Service) SendChallengeOTP(ctx context.Context, request ChallengeOTPRequest) error {
	if s == nil || !s.cfg.Enabled {
		return fmt.Errorf("email sender is disabled")
	}
	if strings.TrimSpace(request.ToEmail) == "" || strings.TrimSpace(request.Code) == "" {
		return fmt.Errorf("email otp target and code must not be empty")
	}
	if _, err := mail.ParseAddress(request.ToEmail); err != nil {
		return fmt.Errorf("invalid recipient email: %w", err)
	}
	if request.TTL <= 0 {
		request.TTL = 5 * time.Minute
	}
	sceneName := strings.TrimSpace(request.SceneName)
	if sceneName == "" {
		sceneName = sceneDisplayName(request.Scene)
	}
	vars := map[string]any{
		"AppName":    choose(s.cfg.AppName, "SevenFramework"),
		"Code":       request.Code,
		"Scene":      request.Scene,
		"SceneName":  sceneName,
		"TTLMinutes": int(request.TTL.Minutes()),
		"ToEmail":    request.ToEmail,
	}
	for key, value := range request.Metadata {
		vars[key] = value
	}
	spec := s.cfg.Templates.ChallengeOTP
	message := Message{
		From:    choose(s.cfg.SMTP.From, s.cfg.DefaultFrom),
		To:      request.ToEmail,
		Subject: renderOrDefault(spec.Subject, "【{{.AppName}}】-{{.SceneName}}", vars),
		Text:    renderOrDefault(spec.Text, "您的验证码是 {{.Code}}，{{.TTLMinutes}} 分钟内有效。", vars),
		HTML:    renderOrDefault(spec.HTML, "<p>您的验证码是 <strong>{{.Code}}</strong>，{{.TTLMinutes}} 分钟内有效。</p>", vars),
	}
	started := time.Now()
	err := s.provider.Send(ctx, message)
	if err != nil {
		s.log.Warn("email_delivery_failed", zap.String("provider", s.cfg.Provider), zap.String("scene", request.Scene), zap.Duration("duration", time.Since(started)), zap.Error(err))
		return err
	}
	if s.store != nil {
		_ = s.store.Capture(ctx, request.Scene, request.ToEmail, request.Code, message, request.TTL)
	}
	s.log.Info("email_delivery_succeeded", zap.String("provider", s.cfg.Provider), zap.String("scene", request.Scene), zap.Duration("duration", time.Since(started)))
	return nil
}

type MockProvider struct {
	store *MockCaptureStore
}

func NewMockProvider(cache cacheinfra.Manager, cfg config.EmailMockConfig) *MockProvider {
	return &MockProvider{store: NewMockCaptureStore(cache, cfg)}
}

func (p *MockProvider) Send(ctx context.Context, message Message) error {
	_ = ctx
	if strings.TrimSpace(message.To) == "" {
		return fmt.Errorf("recipient email must not be empty")
	}
	return nil
}

func (p *MockProvider) Capture(ctx context.Context, scene, toEmail, code string, message Message, ttl time.Duration) error {
	if p == nil || p.store == nil {
		return nil
	}
	return p.store.Capture(ctx, scene, toEmail, code, message, ttl)
}

type MockCaptureStore struct {
	cache cacheinfra.Manager
	cfg   config.EmailMockConfig
}

func NewMockCaptureStore(cache cacheinfra.Manager, cfg config.EmailMockConfig) *MockCaptureStore {
	return &MockCaptureStore{cache: cache, cfg: cfg}
}

func (s *MockCaptureStore) Capture(ctx context.Context, scene, toEmail, code string, message Message, ttl time.Duration) error {
	if s == nil || s.cache == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = s.cfg.TTL
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	prefix := strings.TrimSpace(s.cfg.CapturePrefix)
	if prefix == "" {
		prefix = "email:mock:capture"
	}
	payload, _ := json.Marshal(map[string]any{
		"scene":   scene,
		"toEmail": toEmail,
		"code":    code,
		"subject": message.Subject,
		"text":    message.Text,
		"html":    message.HTML,
		"sentAt":  time.Now().UTC().Format(time.RFC3339Nano),
	})
	normalizedEmail := strings.ToLower(strings.TrimSpace(toEmail))
	keys := []string{
		prefix + ":" + scene + ":" + normalizedEmail,
		prefix + ":latest:" + normalizedEmail,
	}
	for _, key := range keys {
		if err := s.cache.SetBytes(ctx, key, payload, ttl); err != nil {
			return err
		}
	}
	return nil
}

type SMTPProvider struct {
	cfg config.EmailConfig
}

func NewSMTPProvider(cfg config.EmailConfig) (*SMTPProvider, error) {
	if strings.TrimSpace(cfg.SMTP.Host) == "" {
		return nil, fmt.Errorf("smtp host must not be empty")
	}
	return &SMTPProvider{cfg: cfg}, nil
}

func (p *SMTPProvider) Send(ctx context.Context, message Message) error {
	if p == nil {
		return fmt.Errorf("smtp provider is not configured")
	}
	host := strings.TrimSpace(p.cfg.SMTP.Host)
	addr := fmt.Sprintf("%s:%d", host, p.cfg.SMTP.Port)
	timeout := p.cfg.SMTP.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	var client *smtp.Client
	if p.cfg.SMTP.UseTLS {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, InsecureSkipVerify: p.cfg.SMTP.SkipVerify}) //nolint:gosec
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return err
		}
		client, err = smtp.NewClient(tlsConn, host)
	} else {
		client, err = smtp.NewClient(conn, host)
	}
	if err != nil {
		return err
	}
	defer client.Quit()
	if p.cfg.SMTP.StartTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: host, InsecureSkipVerify: p.cfg.SMTP.SkipVerify}); err != nil { //nolint:gosec
				return err
			}
		}
	}
	if p.cfg.SMTP.Username != "" {
		auth := smtp.PlainAuth("", p.cfg.SMTP.Username, p.cfg.SMTP.Password, host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	fromAddress := extractAddress(choose(message.From, p.cfg.DefaultFrom))
	if err := client.Mail(fromAddress); err != nil {
		return err
	}
	if err := client.Rcpt(message.To); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	_, err = writer.Write([]byte(buildMIMEMessage(message)))
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	return err
}

func buildMIMEMessage(message Message) string {
	boundary := "seven-email-boundary"
	var builder strings.Builder
	builder.WriteString("From: " + message.From + "\r\n")
	builder.WriteString("To: " + message.To + "\r\n")
	builder.WriteString("Subject: " + message.Subject + "\r\n")
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
	builder.WriteString("--" + boundary + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + message.Text + "\r\n")
	builder.WriteString("--" + boundary + "\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n" + message.HTML + "\r\n")
	builder.WriteString("--" + boundary + "--\r\n")
	return builder.String()
}

func renderOrDefault(pattern, fallback string, vars map[string]any) string {
	if strings.TrimSpace(pattern) == "" {
		pattern = fallback
	}
	tmpl, err := template.New("email").Parse(pattern)
	if err != nil {
		return fallback
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return fallback
	}
	return buf.String()
}

func sceneDisplayName(scene string) string {
	switch strings.TrimSpace(scene) {
	case "RESET_EMAIL":
		return "修改邮箱"
	case "RESET_PASSWORD":
		return "重置密码"
	case "LOGIN_UNLOCK":
		return "登录解锁"
	case "ACTIVE_USER":
		return "激活账户"
	default:
		return "安全验证"
	}
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
