package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"time"

	"go.n16f.net/ark/pkg/ark"
)

type Example struct {
	process *ark.Process
}

func NewExample() *Example {
	return &Example{}
}

func (e *Example) Start(p *ark.Process) error {
	e.process = p

	fmt.Printf("XXX example start\n")
	return nil
}

func (e *Example) Stop() {
	fmt.Printf("XXX example stop\n")
}

func (e *Example) Main() error {
	fmt.Printf("XXX example main\n")

	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-e.process.Done():
			fmt.Printf("XXX example process done\n")
			return nil

		case <-timer.C:
			fmt.Printf("XXX example timer done\n")
			e.process.Stop()
			return nil

		case <-time.After(time.Second):
			fmt.Printf("XXX example sleep\n")
		}
	}
}

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	ark.MustRun("example", NewExample())
}
