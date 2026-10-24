package logger

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"go.uber.org/zap"
)

type HertzAdapter struct {
	base  *zap.Logger
	level hlog.Level
}

func NewHertzAdapter(base *zap.Logger) *HertzAdapter {
	if base == nil {
		base = zap.NewNop()
	}
	return &HertzAdapter{
		base:  base.WithOptions(zap.AddCallerSkip(2)),
		level: hlog.LevelInfo,
	}
}

func (l *HertzAdapter) SetLevel(level hlog.Level) {
	l.level = level
}

func (l *HertzAdapter) SetOutput(_ io.Writer) {}

func (l *HertzAdapter) Trace(v ...interface{}) {
	l.log(context.Background(), hlog.LevelTrace, fmt.Sprint(v...))
}
func (l *HertzAdapter) Debug(v ...interface{}) {
	l.log(context.Background(), hlog.LevelDebug, fmt.Sprint(v...))
}
func (l *HertzAdapter) Info(v ...interface{}) {
	l.log(context.Background(), hlog.LevelInfo, fmt.Sprint(v...))
}
func (l *HertzAdapter) Notice(v ...interface{}) {
	l.log(context.Background(), hlog.LevelNotice, fmt.Sprint(v...))
}
func (l *HertzAdapter) Warn(v ...interface{}) {
	l.log(context.Background(), hlog.LevelWarn, fmt.Sprint(v...))
}
func (l *HertzAdapter) Error(v ...interface{}) {
	l.log(context.Background(), hlog.LevelError, fmt.Sprint(v...))
}
func (l *HertzAdapter) Fatal(v ...interface{}) {
	l.log(context.Background(), hlog.LevelFatal, fmt.Sprint(v...))
}

func (l *HertzAdapter) Tracef(format string, v ...interface{}) {
	l.log(context.Background(), hlog.LevelTrace, fmt.Sprintf(format, v...))
}
func (l *HertzAdapter) Debugf(format string, v ...interface{}) {
	l.log(context.Background(), hlog.LevelDebug, fmt.Sprintf(format, v...))
}
func (l *HertzAdapter) Infof(format string, v ...interface{}) {
	l.log(context.Background(), hlog.LevelInfo, fmt.Sprintf(format, v...))
}
func (l *HertzAdapter) Noticef(format string, v ...interface{}) {
	l.log(context.Background(), hlog.LevelNotice, fmt.Sprintf(format, v...))
}
func (l *HertzAdapter) Warnf(format string, v ...interface{}) {
	l.log(context.Background(), hlog.LevelWarn, fmt.Sprintf(format, v...))
}
func (l *HertzAdapter) Errorf(format string, v ...interface{}) {
	l.log(context.Background(), hlog.LevelError, fmt.Sprintf(format, v...))
}
func (l *HertzAdapter) Fatalf(format string, v ...interface{}) {
	l.log(context.Background(), hlog.LevelFatal, fmt.Sprintf(format, v...))
}

func (l *HertzAdapter) CtxTracef(ctx context.Context, format string, v ...interface{}) {
	l.log(ctx, hlog.LevelTrace, fmt.Sprintf(format, v...))
}

func (l *HertzAdapter) CtxDebugf(ctx context.Context, format string, v ...interface{}) {
	l.log(ctx, hlog.LevelDebug, fmt.Sprintf(format, v...))
}

func (l *HertzAdapter) CtxInfof(ctx context.Context, format string, v ...interface{}) {
	l.log(ctx, hlog.LevelInfo, fmt.Sprintf(format, v...))
}

func (l *HertzAdapter) CtxNoticef(ctx context.Context, format string, v ...interface{}) {
	l.log(ctx, hlog.LevelNotice, fmt.Sprintf(format, v...))
}

func (l *HertzAdapter) CtxWarnf(ctx context.Context, format string, v ...interface{}) {
	l.log(ctx, hlog.LevelWarn, fmt.Sprintf(format, v...))
}

func (l *HertzAdapter) CtxErrorf(ctx context.Context, format string, v ...interface{}) {
	l.log(ctx, hlog.LevelError, fmt.Sprintf(format, v...))
}

func (l *HertzAdapter) CtxFatalf(ctx context.Context, format string, v ...interface{}) {
	l.log(ctx, hlog.LevelFatal, fmt.Sprintf(format, v...))
}

func (l *HertzAdapter) log(ctx context.Context, level hlog.Level, message string) {
	if level < l.level {
		return
	}

	log := WithContext(ctx, l.base)
	switch level {
	case hlog.LevelTrace, hlog.LevelDebug:
		log.Debug(message)
	case hlog.LevelInfo, hlog.LevelNotice:
		log.Info(message)
	case hlog.LevelWarn:
		log.Warn(message)
	case hlog.LevelError:
		log.Error(message)
	case hlog.LevelFatal:
		log.Error(message)
		os.Exit(1)
	default:
		log.Info(message)
	}
}
