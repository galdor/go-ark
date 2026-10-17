package main

import (
	"fmt"
	_ "net/http/pprof"
	"time"

	"github.com/coder/websocket"
	"go.n16f.net/ark/pkg/ark"
	"go.n16f.net/ark/pkg/ark/http"
	"go.n16f.net/ark/pkg/ark/log"
)

type Example struct {
	Log     *log.Logger
	process *ark.Process
}

func (e *Example) Start(p *ark.Process) error {
	e.Log = p.Log
	e.process = p

	httpServerCfg := http.ServerCfg{
		Address:     "localhost:8080",
		EnablePprof: true,
	}

	httpServer, err := http.NewServer(&httpServerCfg)
	if err != nil {
		return fmt.Errorf("cannot create HTTP server: %v", err)
	}

	httpServer.Route("/ping", "GET", e.hPing)
	httpServer.Route("/panic", "GET", e.hPanic)
	httpServer.Route("/websocket", "GET", e.hWebsocket)
	httpServer.Route("/sleep", "GET", e.hSleep)

	e.process.AddChild("http_server", httpServer)

	return nil
}

func (e *Example) Main() error {
	<-e.process.Stopping()
	return nil
}

func (e *Example) Stop() {
}

func (e *Example) hPing(h *http.Handler) {
	h.ReplyText(200, "pong\n")
}

func (e *Example) hPanic(h *http.Handler) {
	panic("example error")
}

func (e *Example) hSleep(h *http.Handler) {
	select {
	case <-time.After(60 * time.Second):
	case <-h.Done():
	}

	h.ReplyEmpty(204)
}

func (e *Example) hWebsocket(h *http.Handler) {
	h.HandleWebsocketUpgrade(&WebsocketEcho{})
}

type WebsocketEcho struct {
	Log     *log.Logger
	Handler *http.WebsocketHandler
}

func (e *WebsocketEcho) Start(h *http.WebsocketHandler) error {
	e.Log = h.Log
	e.Handler = h
	return nil
}

func (e *WebsocketEcho) Stop() {
}

func (e *WebsocketEcho) ProcessMessage(
	msgType websocket.MessageType, data []byte,
) error {
	return e.Handler.WriteTextMessage(data)
}

func main() {
	ark.MustRunProcess("example", &Example{}, log.DefaultLogger())
}
