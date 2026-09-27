package log

import "log/slog"

func DefaultLogger() *slog.Logger {
	return slog.New(DefaultHandler())
}

func DefaultHandler() slog.Handler {
	cfg := TerminalHandlerCfg{Level: slog.LevelInfo}
	return NewTerminalHandler(&cfg)
}
