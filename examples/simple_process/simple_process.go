package main

import (
	nethttp "net/http"
	_ "net/http/pprof"
	"time"

	"go.n16f.net/ark/pkg/ark"
	"go.n16f.net/ark/pkg/ark/log"
)

type Example struct {
	Log     *log.Logger
	process *ark.Process
}

func (e *Example) Start(p *ark.Process) error {
	e.Log = p.Log
	e.process = p

	e.Log.Info("starting")

	return nil
}

func (e *Example) Stop() {
	e.Log.Info("stopping")
}

func (e *Example) Main() error {
	e.Log.Info("main")

	timer := time.NewTimer(3 * time.Second)
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
			e.Log.Info("sleep")
		}
	}

}

func main() {
	go func() {
		nethttp.ListenAndServe("localhost:6060", nil)
	}()

	ark.MustRun("example", &Example{}, log.DefaultLogger())
}
