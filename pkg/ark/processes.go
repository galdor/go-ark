package ark

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.n16f.net/ark/pkg/ark/log"
)

type ProcessState string

const (
	ProcessStateStarting   = "starting"
	ProcessStateRunning    = "running"
	ProcessStateStopping   = "stopping"
	ProcessStateRestarting = "restarting"
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

type ProcessOptions struct {
	Inline         bool
	RestartOnError bool
	RestartBackoff *Backoff
}

type Process struct {
	Name     string
	Behavior ProcessBehavior
	Log      *log.Logger
	Opts     *ProcessOptions

	ctx    context.Context
	cancel context.CancelFunc

	childrenCtx    context.Context
	childrenCancel context.CancelFunc
	childrenWg     sync.WaitGroup

	mutex sync.RWMutex
	state ProcessState
	err   error

	terminated chan struct{}
}

type ProcessBehavior interface {
	Start(*Process) error
	Stop()
	Main() error
}

func MustRun(name string, behavior ProcessBehavior, logger *log.Logger) {
	if err := Run(name, behavior, logger); err != nil {
		logger.Error("%v", err)
		os.Exit(1)
	}
}

func Run(name string, behavior ProcessBehavior, logger *log.Logger) error {
	logger = logger.With("scope", name)

	ctx, cancel := context.WithCancel(context.Background())
	childrenCtx, childrenCancel := context.WithCancel(ctx)

	opts := ProcessOptions{}

	p := Process{
		Name:     name,
		Behavior: behavior,
		Log:      logger,
		Opts:     &opts,

		ctx:    ctx,
		cancel: cancel,

		childrenCtx:    childrenCtx,
		childrenCancel: childrenCancel,

		terminated: make(chan struct{}),
	}

	p.run(nil)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

wait:
	for {
		select {
		case <-p.terminated:
			break wait

		case <-sigChan:
			fmt.Fprintln(os.Stderr)
			p.Stop()
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

	child := p.newChild(name, behavior, &opts)

	p.childrenWg.Add(1)

	if opts.Inline {
		child.runInline(&p.childrenWg)
	} else {
		child.run(&p.childrenWg)
	}

	return child
}

func (p *Process) newChild(
	name string, behavior ProcessBehavior, opts *ProcessOptions,
) *Process {
	logger := p.Log.With("scope", name)

	if opts.RestartOnError {
		if opts.RestartBackoff == nil {
			opts.RestartBackoff = NewBackoff(1.0, 10.0, 1.5, 0.1)
		}
	}

	child := Process{
		Name:     name,
		Behavior: behavior,
		Log:      logger,
		Opts:     opts,

		terminated: make(chan struct{}),
	}

	child.ctx, child.cancel = context.WithCancel(p.childrenCtx)
	child.childrenCtx, child.childrenCancel = context.WithCancel(child.ctx)

	return &child
}

func (p *Process) Stop() {
	p.cancel()
}

func (p *Process) Context() context.Context {
	return p.ctx
}

func (p *Process) Stopping() <-chan struct{} {
	return p.ctx.Done()
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
	defer close(p.terminated)
	defer p.cancel()

	p.mutex.Lock()
	p.state = ProcessStateStarting
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

		case ProcessStateRestarting:
			newState = p.onRestarting()

		case ProcessStateTerminated:
			return
		}

		if err != nil {
			p.Log.Error("%v", err)
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
	p.childrenCtx, p.childrenCancel = context.WithCancel(p.ctx)
	p.err = nil

	// TODO panic recovery

	if err := p.Behavior.Start(p); err != nil {
		return ProcessStateStopping, err
	}

	return ProcessStateRunning, nil
}

func (p *Process) onRunning() (ProcessState, error) {
	// TODO panic recovery

	if err := p.Behavior.Main(); err != nil {
		return ProcessStateStopping, err
	}

	return ProcessStateStopping, nil
}

func (p *Process) onStopping() ProcessState {
	p.childrenCancel()
	p.childrenWg.Wait()

	p.Behavior.Stop()

	p.mutex.RLock()
	pErr := p.err
	p.mutex.RUnlock()

	if pErr != nil && p.Opts.RestartOnError {
		return ProcessStateRestarting
	}

	return ProcessStateTerminated
}

func (p *Process) onRestarting() ProcessState {
	delay := p.Opts.RestartBackoff.Delay()

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return ProcessStateStarting
	case <-p.Stopping():
		return ProcessStateTerminated
	}
}
