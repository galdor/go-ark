package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.n16f.net/ark/pkg/ark"
	"go.n16f.net/ark/pkg/ark/json"
	"go.n16f.net/ark/pkg/ark/log"
	"go.n16f.net/ark/pkg/ark/utils"
)

var (
	contextKeyHandler contextKey = struct{}{}
)

type Handler struct {
	Log            *log.Logger
	Server         *Server
	Options        *RouteOptions
	Request        *http.Request
	ResponseWriter http.ResponseWriter

	process      *ark.Process
	startTime    time.Time
	responseTime *time.Duration
	errorCode    string
}

func (h *Handler) Start(p *ark.Process) error {
	h.Log = p.Log

	ctx := h.Request.Context()
	ctx = context.WithValue(ctx, contextKeyHandler, h)
	h.Request = h.Request.WithContext(ctx)

	h.startTime = time.Now()

	return nil
}

func (h *Handler) Main() error {
	h.Server.mux.ServeHTTP(h.ResponseWriter, h.Request)
	return nil
}

func (h *Handler) Stop() {
	h.responseTime = new(time.Since(h.startTime))

	h.logRequest()
}

func (h *Handler) logRequest() {
	if h.Options.DisableAccessLog {
		return
	}

	req := h.Request
	w := h.ResponseWriter.(*ResponseWriter)

	attrs := []any{
		"event", "http_server.request",
		"response_time", h.responseTime.Microseconds(),
	}

	if h.errorCode != "" {
		attrs = append(attrs, "error")
		attrs = append(attrs, h.errorCode)
	}

	statusString := "-"
	if w.Status != 0 {
		statusString = strconv.Itoa(w.Status)

		attrs = append(attrs, "status")
		attrs = append(attrs, w.Status)
	}

	h.Log.InfoData(attrs, "%s %s %s %s",
		req.Method, req.URL.Path, statusString,
		utils.FormatSeconds(h.responseTime.Seconds(), 1))
}

func (h *Handler) Reply(status int, r io.Reader) {
	h.ResponseWriter.WriteHeader(status)

	if r != nil {
		if _, err := io.Copy(h.ResponseWriter, r); err != nil {
			h.Log.Error("cannot write response: %v", err)
			return
		}
	}
}

func (h *Handler) ReplyEmpty(status int) {
	h.Reply(status, nil)
}

func (h *Handler) ReplyRedirect(status int, uri string) {
	header := h.ResponseWriter.Header()
	header.Set("Location", uri)

	h.Reply(status, nil)
}

func (h *Handler) ReplyText(status int, body string) {
	header := h.ResponseWriter.Header()
	header.Set("Content-Type", "text/plain; charset=UTF-8")

	h.Reply(status, strings.NewReader(body))
}

func (h *Handler) ReplyJSON(status int, value any) {
	header := h.ResponseWriter.Header()
	header.Set("Content-Type", "application/json")

	var buf bytes.Buffer

	opts := json.EncodingOptions{Indent: true}
	if err := json.EncodeToWithOptions(value, &buf, opts); err != nil {
		h.Log.Error("cannot encode JSON data: %v", err)
		h.ResponseWriter.WriteHeader(500)
		return
	}

	buf.WriteByte('\n')

	h.Reply(status, &buf)
}

func (h *Handler) ReplyJSONFast(status int, value any) {
	header := h.ResponseWriter.Header()
	header.Set("Content-Type", "application/json")

	if err := json.EncodeTo(value, h.ResponseWriter); err != nil {
		h.Log.Error("cannot write JSON response: %v", err)
		h.ResponseWriter.WriteHeader(500)
		return
	}
}

func (h *Handler) ReplyError(status int, code, format string, args ...any) {
	h.ReplyErrorData(status, code, nil, format, args...)
}

func (h *Handler) ReplyErrorData(
	status int, code string, data ErrorData, format string, args ...any,
) {
	h.errorCode = code

	msg := fmt.Sprintf(format, args...)
	h.Server.Cfg.ErrorHandler(h, status, code, msg, data)
}

func (h *Handler) ReplyInternalError(status int, format string, args ...any) {
	msg := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	h.Log.Error("internal error: %s", msg)

	if h.Server.Cfg.HideInternalErrors {
		msg = "internal error"
	}

	h.ReplyError(status, "internal_error", "%s", msg)
}
