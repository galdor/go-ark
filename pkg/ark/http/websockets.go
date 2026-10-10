package http

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/coder/websocket"
	"go.n16f.net/ark/pkg/ark"
	"go.n16f.net/ark/pkg/ark/log"
)

type WebsocketHandlerBehavior interface {
	Start(*ark.Process, *Handler, *websocket.Conn) error
	Stop()
	ProcessMessage(websocket.MessageType, []byte) error
}

type WebsocketHandler struct {
	Log         *log.Logger
	process     *ark.Process
	httpHandler *Handler
	conn        *websocket.Conn
	behavior    WebsocketHandlerBehavior
}

func (h *WebsocketHandler) Start(p *ark.Process) error {
	h.Log = p.Log
	h.process = p

	return h.behavior.Start(p, h.httpHandler, h.conn)
}

func (h *WebsocketHandler) Stop() {
}

func (h *WebsocketHandler) Main() error {
	ctx := h.process.Context()

	for {
		msgType, msgData, err := h.conn.Read(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				h.Log.Info("connection closed")
				return nil
			}

			if errors.Is(err, context.Canceled) {
				return nil
			}

			return fmt.Errorf("cannot read connection: %w", err)
		}

		if err := h.behavior.ProcessMessage(msgType, msgData); err != nil {
			return err
		}
	}
}

func (h *Handler) HandleWebsocketUpgrade(behavior WebsocketHandlerBehavior) {
	wsOpts := websocket.AcceptOptions{}
	conn, err := websocket.Accept(h.ResponseWriter, h.Request, &wsOpts)
	if err != nil {
		h.Log.Error("cannot establish WebSocket connection: %v", err)
		return
	}
	defer conn.CloseNow()

	h.onRequestHandled()

	handler := &WebsocketHandler{
		httpHandler: h,
		conn:        conn,
		behavior:    behavior,
	}

	processOpts := ark.ProcessOptions{Inline: true, Unlinked: true}
	h.process.AddChildWithOptions("websocket_handler", handler, processOpts)
}
