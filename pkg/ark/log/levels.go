package log

import "log/slog"

func LevelString(level slog.Level) string {
	switch {
	case level < slog.LevelInfo:
		return "debug"
	case level < slog.LevelWarn:
		return "info"
	case level < slog.LevelError:
		return "warning"
	default:
		return "error"
	}
}
