package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

func New(cfg config.LoggingConfig, profile string) (*zap.Logger, error) {
	return newWithConsoleWriter(cfg, profile, os.Stdout)
}

func newWithConsoleWriter(cfg config.LoggingConfig, profile string, consoleWriter io.Writer) (*zap.Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	cores := make([]zapcore.Core, 0, 2)
	cores = append(cores, zapcore.NewCore(
		newConsoleEncoder(cfg, profile),
		zapcore.AddSync(consoleWriter),
		level,
	))

	if shouldEnableFile(profile, cfg.File) {
		fileSyncer, err := newFileWriteSyncer(cfg.File)
		if err != nil {
			return nil, err
		}
		cores = append(cores, zapcore.NewCore(
			newFileEncoder(),
			fileSyncer,
			level,
		))
	}

	return zap.New(zapcore.NewTee(cores...), zap.AddCaller()), nil
}

func newConsoleEncoder(cfg config.LoggingConfig, profile string) zapcore.Encoder {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.MessageKey = "message"
	encoderCfg.LevelKey = "level"
	encoderCfg.CallerKey = "caller"

	if cfg.Format == "console" || strings.EqualFold(profile, "dev") {
		encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		return zapcore.NewConsoleEncoder(encoderCfg)
	}

	encoderCfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	return zapcore.NewJSONEncoder(encoderCfg)
}

func newFileEncoder() zapcore.Encoder {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.MessageKey = "message"
	encoderCfg.LevelKey = "level"
	encoderCfg.CallerKey = "caller"
	encoderCfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	return zapcore.NewJSONEncoder(encoderCfg)
}

func shouldEnableFile(profile string, cfg config.FileLoggingConfig) bool {
	_ = profile
	return cfg.Enabled
}

func newFileWriteSyncer(cfg config.FileLoggingConfig) (zapcore.WriteSyncer, error) {
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		return nil, fmt.Errorf("logging.file.path must not be empty")
	}

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create log directory %s: %w", dir, err)
		}
	}

	return zapcore.AddSync(&lumberjack.Logger{
		Filename:   path,
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAgeDays,
		Compress:   cfg.Compress,
		LocalTime:  cfg.LocalTime,
	}), nil
}

func parseLevel(level string) (zapcore.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "info":
		return zapcore.InfoLevel, nil
	case "debug":
		return zapcore.DebugLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("unsupported log level: %s", level)
	}
}

func Sync(log *zap.Logger) {
	if log == nil {
		return
	}
	_ = log.Sync()
}
