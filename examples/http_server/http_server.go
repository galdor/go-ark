package main

import (
	"fmt"

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

func (e *Example) hWebsocket(h *http.Handler) {
	h.HandleWebsocketUpgrade(&WebsocketHandler{})
}

type WebsocketHandler struct {
	Log     *log.Logger
	process *ark.Process
	conn    *websocket.Conn
}

func (h *WebsocketHandler) Start(
	p *ark.Process, hh *http.Handler, conn *websocket.Conn,
) error {
	h.Log = p.Log
	h.process = p
	h.conn = conn
	return nil
}

func (h *WebsocketHandler) Stop() {
}

func (h *WebsocketHandler) ProcessMessage(
	msgType websocket.MessageType, msgData []byte,
) error {
	ctx := h.process.Context()

	response := fmt.Sprintf("received %d byte message", len(msgData))

	err := h.conn.Write(ctx, websocket.MessageText, []byte(response))
	if err != nil {
		return fmt.Errorf("cannot write connection: %w", err)
	}

	return nil
}

func main() {
	ark.MustRunProcess("example", &Example{}, log.DefaultLogger())
}
