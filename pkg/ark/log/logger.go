package log

import (
	"fmt"
	"log/slog"
)

func Data(args ...any) []any {
	return args
}

type Logger struct {
	*slog.Logger
}

func DefaultLogger() *Logger {
	return &Logger{Logger: slog.New(DefaultHandler())}
}

func DefaultHandler() slog.Handler {
	cfg := TerminalHandlerCfg{Level: slog.LevelInfo}
	return NewTerminalHandler(&cfg)
}

func (l *Logger) With(args ...any) *Logger {
	return &Logger{Logger: l.Logger.With(args...)}
}

func (l *Logger) WithGroup(name string) *Logger {
	return &Logger{Logger: l.Logger.WithGroup(name)}
}

func (l *Logger) Debug(format string, args ...any) {
	l.DebugData(nil, format, args...)
}

func (l *Logger) DebugData(attrs []any, format string, args ...any) {
	l.Logger.Debug(fmt.Sprintf(format, args...), attrs...)
}

func (l *Logger) Info(format string, args ...any) {
	l.InfoData(nil, format, args...)
}

func (l *Logger) InfoData(attrs []any, format string, args ...any) {
	l.Logger.Info(fmt.Sprintf(format, args...), attrs...)
}

func (l *Logger) Warn(format string, args ...any) {
	l.WarnData(nil, format, args...)
}

func (l *Logger) WarnData(attrs []any, format string, args ...any) {
	l.Logger.Warn(fmt.Sprintf(format, args...), attrs...)
}

func (l *Logger) Error(format string, args ...any) {
	l.ErrorData(nil, format, args...)
}

func (l *Logger) ErrorData(attrs []any, format string, args ...any) {
	l.Logger.Error(fmt.Sprintf(format, args...), attrs...)
}
