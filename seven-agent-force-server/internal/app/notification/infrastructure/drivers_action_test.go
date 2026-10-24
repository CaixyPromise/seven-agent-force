package infrastructure

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
)

func TestResolveSMTPServerNameSupportsSecureLocalTunnel(t *testing.T) {
	if got := resolveSMTPServerName(smtpConfig{TLSServerName: " mail.example.test "}, "127.0.0.1"); got != "mail.example.test" {
		t.Fatalf("resolveSMTPServerName()=%q, want mail.example.test", got)
	}
	if got := resolveSMTPServerName(smtpConfig{}, " smtp.example.com "); got != "smtp.example.com" {
		t.Fatalf("resolveSMTPServerName() fallback=%q, want smtp.example.com", got)
	}
}

func TestSMTPDriverCaptureAcceptsActionMIME(t *testing.T) {
	host, port, captured := startCaptureSMTP(t, false)
	action := &domain.NotificationAction{Type: domain.NotificationActionTypeOpenInternal, Label: "查看任务", Link: "/workforce/tasks/42", Style: domain.NotificationActionStylePrimary}
	message := domain.DriverMessage{
		Channel: domain.Channel{ChannelType: domain.ChannelTypeEmail, ChannelCode: "capture-smtp", ConfigJSON: fmt.Sprintf(`{"host":%q,"port":%d,"from":"Seven <notice@example.com>","timeout":"2s"}`, host, port)},
		Target:  "user@example.com", Subject: "任务完成\r\nBcc: injected@example.com", Text: "结果已经准备好。", Presentation: domain.DeliveryPresentationHTML,
		PrimaryAction: action, ExternalActionURL: "https://workforce.example.com/console/workforce/tasks/42",
	}
	result, err := (SMTPDriver{}).SendResult(context.Background(), message)
	if err != nil || result.Status != domain.DriverResultProviderAccepted {
		t.Fatalf("SMTP capture result=%#v err=%v", result, err)
	}
	select {
	case capture := <-captured:
		if capture.err != nil {
			t.Fatalf("SMTP capture error: %v", capture.err)
		}
		if strings.Contains(capture.data, "\r\nBcc:") || !strings.Contains(capture.data, "查看任务：https://workforce.example.com/console/workforce/tasks/42") || strings.Count(capture.data, `href="https://workforce.example.com/console/workforce/tasks/42"`) != 1 {
			t.Fatalf("SMTP capture did not receive the safe actionable multipart body:\n%s", capture.data)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SMTP capture timed out")
	}
}

func TestSMTPDriverLostFinalDataResponseIsOutcomeUnknown(t *testing.T) {
	host, port, captured := startCaptureSMTP(t, true)
	result, err := (SMTPDriver{}).SendResult(context.Background(), domain.DriverMessage{
		Channel: domain.Channel{ChannelType: domain.ChannelTypeEmail, ChannelCode: "capture-smtp-loss", ConfigJSON: fmt.Sprintf(`{"host":%q,"port":%d,"from":"notice@example.com","timeout":"2s"}`, host, port)},
		Target:  "user@example.com", Subject: "任务完成", Text: "结果已经准备好。",
	})
	if err != nil || result.Status != domain.DriverResultUnknown || result.FailureClass != "SMTP_OUTCOME_UNKNOWN" {
		t.Fatalf("SMTP response-loss result=%#v err=%v", result, err)
	}
	select {
	case capture := <-captured:
		if capture.err != nil || !strings.Contains(capture.data, "结果已经准备好") {
			t.Fatalf("SMTP response-loss capture=%#v", capture)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SMTP response-loss capture timed out")
	}
}

func TestSMTPDiagnosticNeverPersistsRawTransportError(t *testing.T) {
	if got := smtpDiagnostic(smtpFailure("AUTH", errors.New("provider body with recipient@example.com"))); got != "SMTP_AUTH_FAILED" {
		t.Fatalf("smtpDiagnostic()=%q", got)
	}
	if got := smtpDiagnostic(smtpFailure("TLS", errors.New("certificate for mail.example"))); got != "SMTP_TLS_FAILED" {
		t.Fatalf("smtpDiagnostic()=%q", got)
	}
}

type smtpCapture struct {
	data string
	err  error
}

func startCaptureSMTP(t *testing.T, loseFinalDataResponse bool) (string, int, <-chan smtpCapture) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start capture SMTP: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	results := make(chan smtpCapture, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			results <- smtpCapture{err: acceptErr}
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReader(connection)
		writer := bufio.NewWriter(connection)
		writeReply := func(value string) error {
			if _, writeErr := writer.WriteString(value); writeErr != nil {
				return writeErr
			}
			return writer.Flush()
		}
		if writeErr := writeReply("220 capture.local ESMTP ready\r\n"); writeErr != nil {
			results <- smtpCapture{err: writeErr}
			return
		}
		var data strings.Builder
		inData := false
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				results <- smtpCapture{data: data.String(), err: readErr}
				return
			}
			command := strings.TrimSpace(line)
			if inData {
				if command == "." {
					if loseFinalDataResponse {
						results <- smtpCapture{data: data.String()}
						return
					}
					if writeErr := writeReply("250 2.0.0 accepted\r\n"); writeErr != nil {
						results <- smtpCapture{data: data.String(), err: writeErr}
						return
					}
					inData = false
					continue
				}
				data.WriteString(line)
				continue
			}
			switch {
			case strings.HasPrefix(command, "EHLO "), strings.HasPrefix(command, "HELO "):
				err = writeReply("250 capture.local\r\n")
			case strings.HasPrefix(command, "MAIL FROM:"), strings.HasPrefix(command, "RCPT TO:"):
				err = writeReply("250 2.1.0 ok\r\n")
			case command == "DATA":
				inData = true
				err = writeReply("354 end with <CRLF>.<CRLF>\r\n")
			case command == "QUIT":
				err = writeReply("221 2.0.0 bye\r\n")
				results <- smtpCapture{data: data.String(), err: err}
				return
			default:
				err = writeReply("500 5.5.1 unsupported\r\n")
			}
			if err != nil {
				results <- smtpCapture{data: data.String(), err: err}
				return
			}
		}
	}()
	address := listener.Addr().(*net.TCPAddr)
	return address.IP.String(), address.Port, results
}

func TestSMTPMIMEActionIsPlatformRenderedAndSafelyOmittedWithoutPublicURL(t *testing.T) {
	action := &domain.NotificationAction{Type: domain.NotificationActionTypeOpenInternal, Label: "查看任务", Link: "/workforce/tasks/42", Style: domain.NotificationActionStylePrimary}
	message := domain.DriverMessage{Subject: "任务完成\r\nBcc: injected@example.com", Text: "结果已经准备好。", HTML: "", PrimaryAction: action, ExternalActionURL: "https://workforce.example.com/console/workforce/tasks/42"}
	rendered := buildMIMEMessage("Seven <notice@example.com>", "user@example.com", message.Subject, externalTextWithAction(message), externalHTMLWithAction(message))
	if strings.Contains(rendered, "\r\nBcc:") || !strings.Contains(rendered, "查看任务：https://workforce.example.com/console/workforce/tasks/42") || strings.Count(rendered, `href="https://workforce.example.com/console/workforce/tasks/42"`) != 1 {
		t.Fatalf("unsafe or incomplete MIME message:\n%s", rendered)
	}

	message.ExternalActionURL = ""
	withoutURL := buildMIMEMessage("Seven <notice@example.com>", "user@example.com", "任务完成", externalTextWithAction(message), externalHTMLWithAction(message))
	if strings.Contains(withoutURL, "https://") || strings.Contains(withoutURL, "查看任务：") || strings.Contains(withoutURL, "<a ") {
		t.Fatalf("message without public base URL must contain no action URL: %s", withoutURL)
	}
}
