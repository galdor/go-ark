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
	Start(*WebsocketHandler) error
	Stop()
	ProcessMessage(websocket.MessageType, []byte) error
}

type WebsocketHandler struct {
	Log      *log.Logger
	Process  *ark.Process
	Behavior WebsocketHandlerBehavior
	Conn     *websocket.Conn
}

func (h *WebsocketHandler) Start(p *ark.Process) error {
	h.Log = p.Log
	h.Process = p

	return h.Behavior.Start(h)
}

func (h *WebsocketHandler) Stop() {
	h.Behavior.Stop()
}

func (h *WebsocketHandler) Main() error {
	ctx := h.Process.Context()

	for {
		msgType, msgData, err := h.Conn.Read(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			if errors.Is(err, context.Canceled) {
				return nil
			}

			return fmt.Errorf("cannot read connection: %w", err)
		}

		if err := h.Behavior.ProcessMessage(msgType, msgData); err != nil {
			return err
		}
	}
}

func (h *WebsocketHandler) Context() context.Context {
	return h.Process.Context()
}

func (h *WebsocketHandler) WriteBinaryMessage(data []byte) error {
	err := h.Conn.Write(h.Context(), websocket.MessageBinary, data)
	if err != nil {
		err = fmt.Errorf("cannot write message: %w", err)
		h.Log.Error("%v", err)
		return err
	}

	return nil
}

func (h *WebsocketHandler) WriteTextMessage(data []byte) error {
	err := h.Conn.Write(h.Context(), websocket.MessageText, data)
	if err != nil {
		err = fmt.Errorf("cannot write message: %w", err)
		h.Log.Error("%v", err)
		return err
	}

	return nil
}

func (h *WebsocketHandler) FormatTextMessage(format string, args ...any) error {
	data := fmt.Sprintf(format, args...)
	return h.WriteTextMessage([]byte(data))
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
		Behavior: behavior,
		Conn:     conn,
	}

	processOpts := ark.ProcessOptions{Inline: true, Unlinked: true}
	h.process.AddChildWithOptions("websocket_handler", handler, processOpts)
}
