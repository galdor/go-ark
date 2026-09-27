package main

import (
	"log/slog"
	_ "net/http/pprof"
	"time"

	"go.n16f.net/ark/pkg/ark"
)

type Example struct {
	Log     *slog.Logger
	process *ark.Process
}

func NewExample() *Example {
	return &Example{}
}

func (e *Example) Start(p *ark.Process) error {
	e.Log = p.Log
	e.process = p

	e.Log.Info("start")
	return nil
}

func (e *Example) Stop() {
	e.Log.Info("stop")
}

func (e *Example) Main() error {
	e.Log.Info("main")

	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-e.process.Done():
			e.Log.Info("process done")
			return nil

		case <-timer.C:
			e.Log.Info("timer expired")
			e.process.Stop()
			return nil

		case <-time.After(time.Second):
			e.Log.Info("sleep")
		}
	}
}

func main() {
	logger := slog.Default().With("service", "example")
	ark.MustRun("example", NewExample(), logger)
}
