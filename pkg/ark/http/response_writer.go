package http

import (
	"bufio"
	"fmt"
	"net"
	nethttp "net/http"
)

type ResponseWriter struct {
	Status   int
	Hijacked bool

	w nethttp.ResponseWriter
}

func NewResponseWriter(w nethttp.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{
		w: w,
	}
}

func (w *ResponseWriter) Header() nethttp.Header {
	return w.w.Header()
}

func (w *ResponseWriter) Write(data []byte) (int, error) {
	if w.Status == 0 {
		w.Status = 200
	}

	return w.w.Write(data)
}

func (w *ResponseWriter) WriteHeader(status int) {
	w.Status = status

	w.w.WriteHeader(status)
}

func (w *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.w.(nethttp.Hijacker)
	if !ok {
		return nil, nil,
			fmt.Errorf("response writer does not support connection hijacking")
	}

	conn, rw, err := hijacker.Hijack()
	if err == nil {
		w.Hijacked = true
	}

	return conn, rw, err
}

func (w *ResponseWriter) Flush() {
	f := w.w.(nethttp.Flusher)
	f.Flush()
}
