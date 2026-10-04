package main

import (
	"errors"
	nethttp "net/http"
	_ "net/http/pprof"
	"time"

	"go.n16f.net/ark/pkg/ark"
	"go.n16f.net/ark/pkg/ark/log"
)

type Example struct {
	Level   int
	Log     *log.Logger
	process *ark.Process
}

func NewExample(level int) *Example {
	return &Example{Level: level}
}

func (e *Example) Start(p *ark.Process) error {
	p.Log = p.Log.With("level", e.Level)
	e.Log = p.Log
	e.process = p

	e.Log.Info("starting")

	if e.Level < 3 {
		backoff := ark.NewBackoff(0.25, 1.0, 1.5, 0.1)
		e.process.AddChildWithOptions("example", NewExample(e.Level+1),
			ark.ProcessOptions{RestartOnError: true, RestartBackoff: backoff})
	}

	return nil
}

func (e *Example) Stop() {
	e.Log.Info("stopping")
}

func (e *Example) Main() error {
	e.Log.Info("main")

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-e.process.Stopping():
			e.Log.Info("process stopping")
			return nil

		case <-timer.C:
			e.Log.Info("timer expired")
			e.process.Stop()
			return nil

		case <-time.After(time.Second):
			if e.Level == 2 {
				return errors.New("test error")
			}
		}
	}

}

func main() {
	go func() {
		nethttp.ListenAndServe("localhost:6060", nil)
	}()

	ark.MustRun("example", NewExample(0), log.DefaultLogger())
}
