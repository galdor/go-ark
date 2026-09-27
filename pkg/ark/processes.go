package ark

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
)

type ProcessState string

const (
	ProcessStateStarting   = "starting"
	ProcessStateRunning    = "running"
	ProcessStateStopping   = "stopping"
	ProcessStateTerminated = "terminated"
)

type ProcessStartError struct {
	Err error
}

func (err *ProcessStartError) Error() string {
	return fmt.Sprintf("cannot start process: %v", err.Err)
}

func (err *ProcessStartError) Unwrap() error {
	return err.Err
}

type ProcessMainError struct {
	Err error
}

func (err *ProcessMainError) Error() string {
	return fmt.Sprintf("process error: %v", err.Err)
}

func (err *ProcessMainError) Unwrap() error {
	return err.Err
}

var (
	ErrProcessStopping = errors.New("process stopping")
)

type ProcessEvent string

const (
	ProcessEventStarted    = "started"
	ProcessEventTerminated = "terminated"
)

type ProcessOptions struct {
	Inline bool
}

type Process struct {
	Name     string
	Behavior ProcessBehavior
	Log      *slog.Logger

	ctx    context.Context
	cancel context.CancelCauseFunc
	wg     sync.WaitGroup

	mutex sync.RWMutex
	state ProcessState
	err   error

	eventConsumerMutex sync.RWMutex
	eventConsumers     []chan ProcessEvent
}

type ProcessBehavior interface {
	Start(*Process) error
	Stop()
	Main() error
}

func MustRun(name string, behavior ProcessBehavior, logger *slog.Logger) {
	if err := Run(name, behavior, logger); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func Run(name string, behavior ProcessBehavior, logger *slog.Logger) error {
	logger = logger.With(slog.Group("process", "name", name))

	ctx, cancel := context.WithCancelCause(context.Background())

	p := Process{
		Name:     name,
		Behavior: behavior,
		Log:      logger,

		ctx:    ctx,
		cancel: cancel,
	}

	p.run(nil)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	eventChan := p.SubscribeEvents()

	if eventChan != nil {
	wait:
		for {
			select {
			case event := <-eventChan:
				if event == ProcessEventTerminated {
					break wait
				}

			case <-sigChan:
				fmt.Fprintln(os.Stderr)
				p.Stop()
			}
		}
	}

	return p.Error()
}

func (p *Process) State() (state ProcessState) {
	p.mutex.RLock()
	state = p.state
	p.mutex.RUnlock()
	return
}

func (p *Process) Error() error {
	var state ProcessState
	var err error

	p.mutex.RLock()
	state = p.state
	err = p.err
	p.mutex.RUnlock()

	if state != ProcessStateTerminated {
		return nil
	}

	return err
}

func (p *Process) AddChild(name string, behavior ProcessBehavior) *Process {
	return p.AddChildWithOptions(name, behavior, ProcessOptions{})
}

func (p *Process) AddChildWithOptions(
	name string, behavior ProcessBehavior, opts ProcessOptions,
) *Process {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.state == ProcessStateStopping || p.state == ProcessStateTerminated {
		panic(fmt.Sprintf("cannot add child in state %q", p.state))
	}

	child := p.newChild(name, behavior)

	p.wg.Add(1)

	if opts.Inline {
		child.runInline(&p.wg)
	} else {
		child.run(&p.wg)
	}

	return child
}

func (p *Process) newChild(name string, behavior ProcessBehavior) *Process {
	logger := p.Log.With(slog.Group("process", "name", name))

	ctx, cancel := context.WithCancelCause(p.ctx)

	child := Process{
		Name:     name,
		Behavior: behavior,
		Log:      logger,

		ctx:    ctx,
		cancel: cancel,
	}

	return &child
}

func (p *Process) Stop() {
	p.cancel(ErrProcessStopping)
}

func (p *Process) Context() context.Context {
	return p.ctx
}

func (p *Process) Done() <-chan struct{} {
	return p.ctx.Done()
}

func (p *Process) SubscribeEvents() <-chan ProcessEvent {
	p.mutex.RLock()
	p.eventConsumerMutex.Lock()

	var eventChan chan ProcessEvent
	if p.state != ProcessStateTerminated {
		eventChan = make(chan ProcessEvent)
		p.eventConsumers = append(p.eventConsumers, eventChan)
	}

	p.eventConsumerMutex.Unlock()
	p.mutex.RUnlock()

	return eventChan
}

func (p *Process) UnsubscribeEvents(ch <-chan ProcessEvent) {
	p.eventConsumerMutex.Lock()
	p.eventConsumers = slices.DeleteFunc(p.eventConsumers,
		func(ch2 chan ProcessEvent) bool { return ch2 == ch })
	p.eventConsumerMutex.Unlock()
}

func (p *Process) WaitForTermination(p2 *Process) bool {
	eventChan := p2.SubscribeEvents()
	if eventChan == nil {
		return true
	}

	for {
		select {
		case event := <-eventChan:
			if event == ProcessEventTerminated {
				return true
			}

		case <-p.Done():
			p2.UnsubscribeEvents(eventChan)
			return false
		}
	}
}

func (p *Process) run(wg *sync.WaitGroup) {
	go func() {
		defer func() {
			if wg != nil {
				wg.Done()
			}
		}()

		p.main()
	}()
}

func (p *Process) runInline(wg *sync.WaitGroup) {
	defer func() {
		if wg != nil {
			wg.Done()
		}
	}()

	p.main()
}

func (p *Process) main() {
	defer func() {
		err := p.Error()
		if err == nil {
			err = ErrProcessStopping
		}

		p.cancel(err)
	}()

	p.mutex.Lock()
	p.state = ProcessStateStarting
	p.err = nil
	p.mutex.Unlock()

	for {
		state := p.State()

		var newState ProcessState
		var err error

		switch state {
		case ProcessStateStarting:
			newState, err = p.onStarting()
			if err != nil {
				err = &ProcessStartError{Err: err}
			}

		case ProcessStateRunning:
			newState, err = p.onRunning()
			if err != nil {
				err = &ProcessMainError{Err: err}
			}

		case ProcessStateStopping:
			newState = p.onStopping()

		case ProcessStateTerminated:
			p.onTerminated()
			return
		}

		select {
		case <-p.ctx.Done():
			if newState != ProcessStateTerminated {
				newState = ProcessStateStopping
			}

		default:
		}

		p.mutex.Lock()
		p.state = newState
		if err != nil {
			p.err = err
		}
		p.mutex.Unlock()
	}
}

func (p *Process) onStarting() (ProcessState, error) {
	if err := p.Behavior.Start(p); err != nil {
		return ProcessStateStopping, err
	}

	p.publishEvent(ProcessEventStarted)

	return ProcessStateRunning, nil
}

func (p *Process) onRunning() (ProcessState, error) {
	if err := p.Behavior.Main(); err != nil {
		return ProcessStateStopping, err
	}

	return ProcessStateStopping, nil
}

func (p *Process) onStopping() ProcessState {
	p.cancel(ErrProcessStopping)
	p.wg.Wait() // wait for children

	p.Behavior.Stop()
	return ProcessStateTerminated
}

func (p *Process) onTerminated() {
	p.eventConsumerMutex.Lock()
	consumers := p.eventConsumers
	p.eventConsumers = nil
	p.eventConsumerMutex.Unlock()

	for _, eventChan := range consumers {
		eventChan <- ProcessEventTerminated
		close(eventChan)
	}
}

func (p *Process) publishEvent(event ProcessEvent) {
	p.eventConsumerMutex.RLock()
	eventConsumers := slices.Clone(p.eventConsumers)
	p.eventConsumerMutex.RUnlock()

	for _, eventChan := range eventConsumers {
		eventChan <- event
	}
}
