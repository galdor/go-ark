package log

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

const DefaultScopeWidth = 32

type TerminalHandlerCfg struct {
	Level      slog.Leveler `json:"level,omitempty"`
	ScopeWidth int          `json:"scope_width,omitempty"`
}

type TerminalHandler struct {
	Cfg        *TerminalHandlerCfg
	group      string
	attributes Attributes
	scope      string
	mutex      *sync.Mutex // used to write to os.Stderr
}

func NewTerminalHandler(cfg *TerminalHandlerCfg) *TerminalHandler {
	if cfg.ScopeWidth == 0 {
		cfg.ScopeWidth = DefaultScopeWidth
	}

	return &TerminalHandler{Cfg: cfg, mutex: &sync.Mutex{}}
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

	buf = fmt.Appendf(buf, "%-7s", LevelString(r.Level))
	buf = append(buf, "  "...)

	buf = fmt.Appendf(buf, "%-*s", h.Cfg.ScopeWidth, h.scope)
	buf = append(buf, "  "...)

	buf = append(buf, r.Message...)
	buf = append(buf, '\n')

	if len(h.attributes)+r.NumAttrs() > 0 {
		buf = append(buf, "        "...)

		appendAttribute := func(buf []byte, a Attribute) []byte {
			if a.Key == "scope" {
				return buf
			}

			buf = append(buf, ' ')
			buf = append(buf, a.Key...)
			buf = append(buf, '=')
			buf = append(buf, a.Value...)

			return buf
		}

		for _, a := range h.attributes {
			buf = appendAttribute(buf, a)
		}

		var recordAttributes Attributes
		r.Attrs(func(attr slog.Attr) bool {
			recordAttributes.AppendAttr(attr, h.group)
			return true
		})
		for _, a := range recordAttributes {
			buf = appendAttribute(buf, a)
		}

		buf = append(buf, '\n')
	}

	h.mutex.Lock()
	_, err := os.Stderr.Write(buf)
	h.mutex.Unlock()

	return err
}
