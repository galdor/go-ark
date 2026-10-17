package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.n16f.net/ark/pkg/ark"
	"go.n16f.net/ark/pkg/ark/json"
	"go.n16f.net/ark/pkg/ark/log"
	"go.n16f.net/ark/pkg/ark/utils"
)

// Assumes that the hash is SHA1 (40 characters).
var filePathCacheBustingRE = regexp.MustCompile(
	`^(.+)\.(?:[a-z0-9]{40})\.([A-Za-z0-9]{1,5})$`)

var (
	contextKeyHandler contextKey = struct{}{}
)

type Handler struct {
	Log            *log.Logger
	Server         *Server
	Options        *RouteOptions
	Request        *http.Request
	ResponseWriter http.ResponseWriter

	Method        string
	PathPattern   string
	RouteId       string
	ClientAddress string // optional

	process      *ark.Process
	startTime    time.Time
	responseTime *time.Duration
	errorCode    string
}

func (h *Handler) Start(p *ark.Process) error {
	h.Log = p.Log
	h.process = p
	h.startTime = time.Now()

	ctx := h.Request.Context()
	ctx = context.WithValue(ctx, contextKeyHandler, h)
	h.Request = h.Request.WithContext(ctx)

	return nil
}

func (h *Handler) Main() error {
	h.Server.mux.ServeHTTP(h.ResponseWriter, h.Request)
	return nil
}

func (h *Handler) Stop() {
	if !h.ConnectionHijacked() {
		h.onRequestHandled()
	}
}

func (h *Handler) Context() context.Context {
	return h.Request.Context()
}

func (h *Handler) Done() <-chan struct{} {
	return h.Context().Done()
}

func (h *Handler) onRequestHandled() {
	h.responseTime = new(time.Since(h.startTime))

	h.logRequest()
}

func (h *Handler) ConnectionHijacked() bool {
	w := h.ResponseWriter.(*ResponseWriter)
	return w.Hijacked
}

func (h *Handler) logRequest() {
	if h.Options.DisableAccessLog {
		return
	}

	req := h.Request
	w := h.ResponseWriter.(*ResponseWriter)

	attrs := []any{
		"event", "http_server.request",
		"http.response_time", h.responseTime.Microseconds(),
	}

	if h.errorCode != "" {
		attrs = append(attrs, "error")
		attrs = append(attrs, h.errorCode)
	}

	statusString := "-"
	if w.Status != 0 {
		statusString = strconv.Itoa(w.Status)

		attrs = append(attrs, "http.status")
		attrs = append(attrs, w.Status)
	}

	h.Log.InfoData(attrs, "%s %s %s %s",
		req.Method, utils.EscapeNonPrintable(req.URL.Path), statusString,
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
	header.Set("Content-Type", "text/plain;charset=UTF-8")

	h.Reply(status, strings.NewReader(body))
}

type Template interface {
	ExecuteTemplate(io.Writer, string, any) error
}

func (h *Handler) ReplyTemplate(
	status int, tpl Template, name string, data any,
) {
	header := h.ResponseWriter.Header()
	header.Set("Content-Type", "text/html;charset=UTF-8")

	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, name, data); err != nil {
		h.ReplyInternalError(500, "cannot render template: %v", err)
		return
	}

	h.Reply(status, &buf)
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

	h.ResponseWriter.WriteHeader(status)

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
	msg := fmt.Sprintf(format, args...)

	h.Log.Error("internal error: %s", msg)

	if h.Server.Cfg.HideInternalErrors {
		msg = "internal error"
	}

	h.ReplyError(status, "internal_error", "%s", msg)
}

func (h *Handler) ReplyNotImplemented(feature string) {
	h.ReplyError(501, "not_implemented", "%s not implemented", feature)
}

func (h *Handler) ReplyFile(filePath string) {
	matches := filePathCacheBustingRE.FindAllStringSubmatch(filePath, -1)
	if len(matches) == 1 {
		groups := matches[0][1:]
		filePath = groups[0] + "." + groups[1]
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			h.ReplyError(404, "file_not_found", "file not found")
			return
		}

		h.ReplyInternalError(500, "cannot stat %q: %v", filePath, err)
		return
	}

	if !info.Mode().IsRegular() {
		h.ReplyError(400, "non_regular_file", "file is not a regular file")
		return
	}

	modTime := info.ModTime()

	body, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			h.ReplyError(404, "file_not_found", "file not found")
			return
		}

		h.ReplyInternalError(500, "cannot open %q: %v", filePath, err)
		return
	}
	defer body.Close()

	http.ServeContent(h.ResponseWriter, h.Request, filePath, modTime, body)
}
