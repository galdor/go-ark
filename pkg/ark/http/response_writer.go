package http

import (
	"bufio"
	"fmt"
	"net"
	nethttp "net/http"
)

type ResponseWriter struct {
	Status int

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

	return hijacker.Hijack()
}

func (w *ResponseWriter) Flush() {
	f := w.w.(nethttp.Flusher)
	f.Flush()
}
