package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	nethttp "net/http"
	"net/http/pprof"
	"strings"
	"time"

	"go.n16f.net/ark/pkg/ark"
	"go.n16f.net/ark/pkg/ark/json"
	"go.n16f.net/ark/pkg/ark/log"
	"go.n16f.net/ark/pkg/ark/utils"
)

const (
	DefaultServerShutdownTimeout = 5 * time.Second

	ErrorCodeHeaderField = "X-Ark-Error-Code"
)

type contextKey struct{}

type RouteFunc func(*Handler)

type RouteOptions struct {
	RouteId           string
	MethodlessRouteId bool
	DisableAccessLog  bool
}

type ErrorData any
type ErrorHandler func(*Handler, int, string, string, ErrorData)

type ServerCfg struct {
	Address                  string `json:"address"`
	HideInternalErrors       bool   `json:"hide_internal_errors"`
	EnablePprof              bool   `json:"enable_pprof"`
	TrustAddressHeaderFields bool   `json:"trust_address_header_fields"`
	ShutdownTimeout          int    `json:"shutdown_timeout"`

	ErrorHandler ErrorHandler `json:"-"`
}

func (cfg *ServerCfg) ValidateJSON(v *json.Validator) {
	v.CheckNetworkAddress("address", cfg.Address)

	if cfg.ShutdownTimeout != 0 {
		v.CheckIntMin("shutdown_timeout", cfg.ShutdownTimeout, 0)
	}
}

type Server struct {
	Cfg *ServerCfg
	Log *log.Logger

	process  *ark.Process
	listener net.Listener
	server   *nethttp.Server
	mux      *nethttp.ServeMux
}

func NewServer(cfg *ServerCfg) (*Server, error) {
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = AdaptativeErrorHandler
	}

	s := Server{
		Cfg: cfg,

		mux: http.NewServeMux(),
	}

	s.mux.HandleFunc("/", s.hNotFound)

	if cfg.EnablePprof {
		s.initPprofRoutes()
	}

	return &s, nil
}

func (s *Server) Start(p *ark.Process) error {
	s.Log = p.Log
	s.process = p

	s.server = &nethttp.Server{
		Addr:     s.Cfg.Address,
		Handler:  s,
		ErrorLog: slog.NewLogLogger(s.Log.Handler(), slog.LevelWarn),

		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       10 * time.Second,
	}

	listener, err := net.Listen("tcp", s.Cfg.Address)
	if err != nil {
		err = utils.UnwrapNetOpError(err, "listen")
		return fmt.Errorf("cannot listen on %q: %w", s.Cfg.Address, err)
	}
	s.listener = listener

	s.Log.Info("listening on %s", s.Cfg.Address)

	return nil
}

func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}

	s.server = nil
}

func (s *Server) Main() error {
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.server.Serve(s.listener)
	}()

	select {
	case err := <-errChan:
		return err

	case <-s.process.Stopping():
	}

	timeout := DefaultServerShutdownTimeout
	if s.Cfg.ShutdownTimeout >= 0 {
		timeout = time.Duration(s.Cfg.ShutdownTimeout) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	shutdownErr := s.server.Shutdown(ctx)
	if shutdownErr != nil {
		s.server.Close()
	}

	if err := <-errChan; err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server initialization failed: %w", err)
	}

	if shutdownErr != nil {
		if errors.Is(shutdownErr, context.DeadlineExceeded) {
			return nil
		}

		return fmt.Errorf("HTTP server shutdown error: %w", shutdownErr)
	}

	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h := Handler{
		Server:         s,
		Options:        &RouteOptions{},
		Request:        req,
		ResponseWriter: NewResponseWriter(w),
	}

	processOpts := ark.ProcessOptions{Inline: true, Unlinked: true}
	s.process.AddChildWithOptions("handler", &h, processOpts)
}

func (s *Server) Route(pathPattern, method string, routeFunc RouteFunc) {
	s.RouteWithOptions(pathPattern, method, routeFunc, RouteOptions{})
}

func (s *Server) RouteWithOptions(
	pathPattern, method string, routeFunc RouteFunc, options RouteOptions,
) {
	handlerFunc := func(w http.ResponseWriter, req *http.Request) {
		h := requestHandler(req)
		s.finalizeHandler(h, req, pathPattern, method, routeFunc, &options)

		defer func() {
			if v := recover(); v != nil {
				valueString := utils.RecoverValueString(v)
				trace := utils.StackTrace(2, 20, true)
				msg := fmt.Sprintf("panic: %s\n%s", valueString, trace)
				msg = strings.TrimRight(msg, "\n")

				if h.ConnectionHijacked() {
					h.Log.Error("%s", msg)
					return
				}

				h.ReplyInternalError(500, "%s", msg)
			}
		}()

		routeFunc(h)
	}

	pattern := pathPattern
	if method != "" {
		pattern = method + " " + pattern
	}

	s.mux.HandleFunc(pattern, handlerFunc)

	// We usually want /foo and /foo/ to be handled the same way, so we have to
	// register both variants.

	hasSuffix := func(s string) bool {
		return strings.HasSuffix(pattern, s)
	}

	if !hasSuffix("/") && !hasSuffix("{$}") && !hasSuffix("...}") {
		s.mux.HandleFunc(pattern+"/{$}", handlerFunc)
	}
}

func (s *Server) finalizeHandler(
	h *Handler, req *http.Request, pathPattern, method string,
	routeFunc RouteFunc, options *RouteOptions,
) {
	if options != nil {
		h.Options = options
	}

	h.Request = req // the request may have been modified by the muxer

	h.Method = method
	h.PathPattern = pathPattern
	h.RouteId = s.RouteId(method, pathPattern, h.Options)
	h.ClientAddress = s.requestClientAddress(req)

	var attrs []any

	if h.RouteId != "" {
		attrs = append(attrs, "http.route")
		attrs = append(attrs, h.RouteId)
	}

	if h.ClientAddress != "" {
		attrs = append(attrs, "http.client_address")
		attrs = append(attrs, h.ClientAddress)
	}

	h.Log = h.Log.With(attrs...)
}

func (s *Server) RouteId(method, pathPattern string, options *RouteOptions) string {
	if options.RouteId != "" {
		return options.RouteId
	}

	if pathPattern == "" {
		return ""
	}

	routeId := pathPattern

	// We want "/foo/{$}" to have the route id "/foo" and "/{$}" to have the
	// route id "/" and not "".
	routeId = strings.TrimSuffix(pathPattern, "/{$}")
	if routeId == "" {
		routeId = "/"
	}

	if method == "" || options.MethodlessRouteId {
		return routeId
	}

	return routeId + " " + strings.ToUpper(method)
}

func TextErrorHandler(
	h *Handler, status int, code string, msg string, data ErrorData,
) {
	header := h.ResponseWriter.Header()
	header.Set(ErrorCodeHeaderField, code)

	h.ReplyText(status, msg+"\n")
}

func JSONErrorHandler(
	h *Handler, status int, code string, msg string, data ErrorData,
) {
	header := h.ResponseWriter.Header()
	header.Set(ErrorCodeHeaderField, code)

	responseData := JSONError{
		Code:    code,
		Message: msg,
		Data:    data,
	}

	h.ReplyJSON(status, &responseData)
}

func AdaptativeErrorHandler(
	h *Handler, status int, code string, msg string, data ErrorData,
) {
	var handler ErrorHandler

	if RequestAcceptsText(h.Request) {
		handler = TextErrorHandler
	} else {
		handler = JSONErrorHandler
	}

	handler(h, status, code, msg, data)
}

func (s *Server) hNotFound(w http.ResponseWriter, req *http.Request) {
	h := requestHandler(req)
	s.finalizeHandler(h, req, "", req.Method, nil, nil)

	h.ReplyError(404, "http_route_not_found", "HTTP route not found")
}

func requestHandler(req *http.Request) *Handler {
	value := req.Context().Value(contextKeyHandler)
	if value == nil {
		return nil
	}

	return value.(*Handler)
}

func (s *Server) requestClientAddress(req *http.Request) string {
	if s.Cfg.TrustAddressHeaderFields {
		if v := req.Header.Get("X-Real-IP"); v != "" {
			return v
		}

		if v := req.Header.Get("X-Forwarded-For"); v != "" {
			i := strings.Index(v, ",")
			if i == -1 {
				return v
			}

			return strings.TrimSpace(v[:i])
		}
	}

	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return ""
	}

	return host
}

func RequestAcceptsText(req *http.Request) bool {
	accept := req.Header.Get("Accept")
	if accept == "" {
		return false
	}

	mediaTypes := strings.Split(accept, ",")

	for _, mediaType := range mediaTypes {
		mediaType = strings.TrimSpace(mediaType)

		if strings.HasPrefix(mediaType, "text/") {
			return true
		}
	}

	return false
}

func (s *Server) initPprofRoutes() {
	handlerFunc := func(handler http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			handler.ServeHTTP(w, req)
		}
	}

	wrap := func(fn http.HandlerFunc) RouteFunc {
		return func(h *Handler) {
			fn(h.ResponseWriter, h.Request)
		}
	}

	routes := map[string]http.HandlerFunc{
		"/cmdline": pprof.Cmdline,
		"/profile": pprof.Profile,
		"/symbol":  pprof.Symbol,
		"/trace":   pprof.Trace,

		"/allocs":       handlerFunc(pprof.Handler("allocs")),
		"/block":        handlerFunc(pprof.Handler("block")),
		"/goroutine":    handlerFunc(pprof.Handler("goroutine")),
		"/heap":         handlerFunc(pprof.Handler("heap")),
		"/mutex":        handlerFunc(pprof.Handler("mutex")),
		"/threadcreate": handlerFunc(pprof.Handler("threadcreate")),
	}

	// It would be convenient to serve pprof routes at /pprof but pprof assumes
	// that the URI starts with /debug/pprof/ (not the final "/").

	s.Route("/debug/pprof/", "GET", wrap(pprof.Index))

	for subpath, handler := range routes {
		s.Route("/debug/pprof"+subpath, "GET", wrap(handler))
	}
}
