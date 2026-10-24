package websocket

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	defaultConnectTimeout              = 3 * time.Second
	defaultWriteTimeout                = 5 * time.Second
	defaultTempSessionTimeout          = 30 * time.Second
	defaultTempQueueCapacity           = 64
	defaultMaxMessageBytes       int64 = 1 << 20
	defaultReconnectInitialDelay       = 500 * time.Millisecond
	defaultReconnectMaxDelay           = 5 * time.Second
	defaultHeartbeatInterval           = 20 * time.Second
	defaultSendQueueCapacity           = 128
)

type TempOptions struct {
	URL             string
	Headers         map[string]string
	ConnectTimeout  time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	SessionTimeout  time.Duration
	QueueCapacity   int
	MaxMessageBytes int64
	Logger          *zap.Logger
}

type ClientOptions struct {
	URL                   string
	Headers               map[string]string
	ConnectTimeout        time.Duration
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	ReconnectEnabled      bool
	ReconnectInitialDelay time.Duration
	ReconnectMaxDelay     time.Duration
	HeartbeatInterval     time.Duration
	MaxMessageBytes       int64
	CallbackWorkers       int
	SendQueueCapacity     int
	Logger                *zap.Logger
}

type tempSettings struct {
	URL             string
	Headers         map[string]string
	ConnectTimeout  time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	SessionTimeout  time.Duration
	QueueCapacity   int
	MaxMessageBytes int64
	Logger          *zap.Logger
}

type clientSettings struct {
	URL                   string
	Headers               map[string]string
	ConnectTimeout        time.Duration
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	ReconnectEnabled      bool
	ReconnectInitialDelay time.Duration
	ReconnectMaxDelay     time.Duration
	HeartbeatInterval     time.Duration
	MaxMessageBytes       int64
	CallbackWorkers       int
	SendQueueCapacity     int
	Logger                *zap.Logger
}

func normalizeTempOptions(opts TempOptions) (tempSettings, error) {
	settings := tempSettings{
		URL:             strings.TrimSpace(opts.URL),
		Headers:         cloneHeaders(opts.Headers),
		ConnectTimeout:  opts.ConnectTimeout,
		ReadTimeout:     opts.ReadTimeout,
		WriteTimeout:    opts.WriteTimeout,
		SessionTimeout:  opts.SessionTimeout,
		QueueCapacity:   opts.QueueCapacity,
		MaxMessageBytes: opts.MaxMessageBytes,
		Logger:          opts.Logger,
	}
	if settings.URL == "" {
		return tempSettings{}, fmt.Errorf("websocket url must not be empty")
	}
	if settings.ConnectTimeout <= 0 {
		settings.ConnectTimeout = defaultConnectTimeout
	}
	if settings.WriteTimeout <= 0 {
		settings.WriteTimeout = defaultWriteTimeout
	}
	if settings.SessionTimeout <= 0 {
		settings.SessionTimeout = defaultTempSessionTimeout
	}
	if settings.QueueCapacity <= 0 {
		settings.QueueCapacity = defaultTempQueueCapacity
	}
	if settings.MaxMessageBytes <= 0 {
		settings.MaxMessageBytes = defaultMaxMessageBytes
	}
	if settings.Logger == nil {
		settings.Logger = zap.NewNop()
	}
	return settings, nil
}

func normalizeClientOptions(opts ClientOptions) (clientSettings, error) {
	settings := clientSettings{
		URL:                   strings.TrimSpace(opts.URL),
		Headers:               cloneHeaders(opts.Headers),
		ConnectTimeout:        opts.ConnectTimeout,
		ReadTimeout:           opts.ReadTimeout,
		WriteTimeout:          opts.WriteTimeout,
		ReconnectEnabled:      opts.ReconnectEnabled,
		ReconnectInitialDelay: opts.ReconnectInitialDelay,
		ReconnectMaxDelay:     opts.ReconnectMaxDelay,
		HeartbeatInterval:     opts.HeartbeatInterval,
		MaxMessageBytes:       opts.MaxMessageBytes,
		CallbackWorkers:       opts.CallbackWorkers,
		SendQueueCapacity:     opts.SendQueueCapacity,
		Logger:                opts.Logger,
	}
	if settings.URL == "" {
		return clientSettings{}, fmt.Errorf("websocket url must not be empty")
	}
	if settings.ConnectTimeout <= 0 {
		settings.ConnectTimeout = defaultConnectTimeout
	}
	if settings.WriteTimeout <= 0 {
		settings.WriteTimeout = defaultWriteTimeout
	}
	if settings.ReconnectInitialDelay <= 0 {
		settings.ReconnectInitialDelay = defaultReconnectInitialDelay
	}
	if settings.ReconnectMaxDelay <= 0 {
		settings.ReconnectMaxDelay = defaultReconnectMaxDelay
	}
	if settings.ReconnectMaxDelay < settings.ReconnectInitialDelay {
		settings.ReconnectMaxDelay = settings.ReconnectInitialDelay
	}
	if settings.HeartbeatInterval <= 0 {
		settings.HeartbeatInterval = defaultHeartbeatInterval
	}
	if settings.MaxMessageBytes <= 0 {
		settings.MaxMessageBytes = defaultMaxMessageBytes
	}
	if settings.CallbackWorkers <= 0 {
		settings.CallbackWorkers = max(1, runtime.GOMAXPROCS(0))
	}
	if settings.SendQueueCapacity <= 0 {
		settings.SendQueueCapacity = defaultSendQueueCapacity
	}
	if settings.Logger == nil {
		settings.Logger = zap.NewNop()
	}
	return settings, nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	result := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		result[key] = strings.TrimSpace(value)
	}
	return result
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
