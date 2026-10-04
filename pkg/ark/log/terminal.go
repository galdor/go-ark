package log

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

const DefaultScopeWidth = 32

type Color int

var (
	ColorBlack   = Color(0)
	ColorRed     = Color(1)
	ColorGreen   = Color(2)
	ColorYellow  = Color(3)
	ColorBlue    = Color(4)
	ColorMagenta = Color(5)
	ColorCyan    = Color(6)
	ColorWhite   = Color(7)
	ColorDefault = Color(9)
)

type TerminalHandlerCfg struct {
	Level        slog.Leveler `json:"level,omitempty"`
	DisableColor bool         `json:"disable_color,omitempty"`
	ForceColor   bool         `json:"force_color,omitempty"`
	ScopeWidth   int          `json:"scope_width,omitempty"`
}

type TerminalHandler struct {
	Cfg        *TerminalHandlerCfg
	group      string
	attributes Attributes
	scope      string
	mutex      *sync.Mutex // used to write to os.Stderr
	color      bool
}

func NewTerminalHandler(cfg *TerminalHandlerCfg) *TerminalHandler {
	if cfg.ScopeWidth == 0 {
		cfg.ScopeWidth = DefaultScopeWidth
	}

	isCharDev, err := isCharDevice(os.Stderr)
	if err != nil {
		// If we cannot check for some reason, assume it is a character
		// device for color purposes. Better to be conservative.
		isCharDev = true
	}
	color := (!cfg.DisableColor && isCharDev) || cfg.ForceColor

	return &TerminalHandler{
		Cfg:   cfg,
		mutex: &sync.Mutex{},
		color: color,
	}
}

func (h *TerminalHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.Cfg.Level.Level()
}

func (h *TerminalHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := *h

	for _, attr := range attrs {
		if attr.Key == "scope" {
			if value := FormatAttributeValue(attr.Value); value != "" {
				if h2.scope != "" {
					h2.scope += "."
				}
				h2.scope += value
			}
		} else {
			h2.attributes.AppendAttr(attr, h2.group)
		}
	}

	return &h2
}

func (h *TerminalHandler) WithGroup(name string) slog.Handler {
	h2 := *h

	if name != "" {
		if h2.group != "" {
			h2.group += "."
		}
		h2.group = name
	}

	return &h2
}

func (h *TerminalHandler) Handle(ctx context.Context, r slog.Record) error {
	buf := make([]byte, 0, 1024)

	baseColor := ColorDefault
	if r.Level >= slog.LevelError {
		baseColor = ColorRed
	} else if r.Level >= slog.LevelWarn {
		baseColor = ColorYellow
	} else {
		baseColor = ColorDefault
	}

	buf = append(buf, h.colorize(baseColor, LevelString(r.Level), 7)...)
	buf = append(buf, "  "...)

	buf = append(buf, h.colorize(ColorGreen, h.scope, h.Cfg.ScopeWidth)...)
	buf = append(buf, "  "...)

	buf = append(buf, h.colorize(baseColor, r.Message, 0)...)
	buf = append(buf, '\n')

	attributes := h.recordAttributes(r)
	if len(attributes) > 0 {
		buf = append(buf, "        "...)

		appendAttribute := func(buf []byte, a Attribute) []byte {
			if a.Key == "scope" {
				return buf
			}

			buf = append(buf, ' ')
			buf = append(buf, h.colorize(ColorBlue, a.Key, 0)...)
			buf = append(buf, '=')
			buf = append(buf, a.Value...)

			return buf
		}

		for _, a := range attributes {
			buf = appendAttribute(buf, a)
		}

		buf = append(buf, '\n')
	}

	h.mutex.Lock()
	_, err := os.Stderr.Write(buf)
	h.mutex.Unlock()

	return err
}

func (h *TerminalHandler) recordAttributes(r slog.Record) Attributes {
	var attributes Attributes

	for _, a := range h.attributes {
		attributes = append(attributes, a)
	}

	r.Attrs(func(attr slog.Attr) bool {
		attributes.AppendAttr(attr, h.group)
		return true
	})

	attributes.SortAndDeduplicate()

	return attributes
}

func (h *TerminalHandler) colorize(color Color, s string, pad int) string {
	if !h.color {
		return s
	}

	if pad != 0 {
		s = fmt.Sprintf("%-*s", pad, s)
	}

	// Yes we only support ANSI terminals, no we do not care.
	return fmt.Sprintf("\033[%dm%s\033[0m", 30+int(color), s)
}

func isCharDevice(file *os.File) (bool, error) {
	info, err := file.Stat()
	if err != nil {
		return false, err
	}

	return info.Mode()&os.ModeCharDevice != 0, nil
}
