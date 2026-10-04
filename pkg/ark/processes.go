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
	"go.n16f.net/ark/pkg/ark/utils"
)

type ProcessState string

const (
	ProcessStateStarting   ProcessState = "starting"
	ProcessStateRunning    ProcessState = "running"
	ProcessStateStopping   ProcessState = "stopping"
	ProcessStateRestarting ProcessState = "restarting"
	ProcessStateTerminated ProcessState = "terminated"
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
	Unlinked       bool
	RestartOnError bool
	RestartBackoff *Backoff
}

type Process struct {
	Name     string
	Behavior ProcessBehavior
	Log      *log.Logger
	Options  *ProcessOptions

	ctx    context.Context
	cancel context.CancelFunc

	childrenCtx    context.Context
	childrenCancel context.CancelFunc
	childrenWg     sync.WaitGroup

	parentErrChan chan<- error
	errChan       chan error

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

	errChan := make(chan error)

	opts := ProcessOptions{}

	p := Process{
		Name:     name,
		Behavior: behavior,
		Log:      logger,
		Options:  &opts,

		ctx:    ctx,
		cancel: cancel,

		childrenCtx:    childrenCtx,
		childrenCancel: childrenCancel,

		parentErrChan: errChan,
		errChan:       make(chan error),

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

		case <-errChan:
			p.Stop()

		case signo := <-sigChan:
			fmt.Fprintln(os.Stderr)
			p.Log.Info("received signal %d (%v)", signo, signo)
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

	switch p.state {
	case ProcessStateStopping:
		fallthrough
	case ProcessStateRestarting:
		fallthrough
	case ProcessStateTerminated:
		state := p.state
		p.mutex.Unlock()

		panic(fmt.Sprintf("cannot add child in state %q", state))
	}

	child := p.newChild(name, behavior, &opts)

	p.childrenWg.Add(1)

	p.mutex.Unlock()

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
		Options:  opts,

		parentErrChan: p.errChan,
		errChan:       make(chan error),

		terminated: make(chan struct{}),
	}

	// Note that the parent must hold p.mutex. Not a problem since newChild is
	// only called by AddChildWithOptions.
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

	go func() {
		select {
		case <-p.errChan:
			p.Stop()
		case <-p.Stopping():
			return
		}
	}()

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

			// We only propagate the error to the parent if we are not going to
			// restart and if we are linked to it.
			if !p.Options.RestartOnError && !p.Options.Unlinked {
				// The parent may have read an error for another child and is
				// canceling the children context (the one p.ctx derives from).
				// In that case there is nothing to do but return.
				select {
				case p.parentErrChan <- err:
				case <-p.ctx.Done():
				}
			}
		}

		select {
		case <-p.ctx.Done():
			terminated := newState == ProcessStateTerminated
			restarting := newState == ProcessStateRestarting
			if !(restarting || terminated) {
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

func (p *Process) onStarting() (state ProcessState, err error) {
	p.mutex.Lock()
	p.childrenCtx, p.childrenCancel = context.WithCancel(p.ctx)
	p.err = nil
	p.mutex.Unlock()

	defer func() {
		if v := recover(); v != nil {
			msg := utils.RecoverValueString(v)
			trace := utils.StackTrace(2, 20, true)

			state = ProcessStateStopping
			err = utils.NewPanicError(msg, trace)
			return
		}
	}()

	if err := p.Behavior.Start(p); err != nil {
		return ProcessStateStopping, err
	}

	return ProcessStateRunning, nil
}

func (p *Process) onRunning() (state ProcessState, err error) {
	defer func() {
		if v := recover(); v != nil {
			msg := utils.RecoverValueString(v)
			trace := utils.StackTrace(2, 20, true)

			state = ProcessStateStopping
			err = utils.NewPanicError(msg, trace)
			return
		}
	}()

	if err := p.Behavior.Main(); err != nil {
		return ProcessStateStopping, err
	}

	return ProcessStateStopping, nil
}

func (p *Process) onStopping() ProcessState {
	p.mutex.RLock()
	p.childrenCancel()
	p.mutex.RUnlock()
	p.childrenWg.Wait()

	func() {
		defer func() {
			if v := recover(); v != nil {
				msg := utils.RecoverValueString(v)
				trace := utils.StackTrace(2, 20, true)

				p.Log.Error("process error while stopping: %v",
					utils.NewPanicError(msg, trace))
			}
		}()

		p.Behavior.Stop()
	}()

	p.mutex.RLock()
	pErr := p.err
	p.mutex.RUnlock()

	// We check p.ctx.Err() because we do not restart if the context is
	// canceled.
	if pErr != nil && p.Options.RestartOnError && p.ctx.Err() == nil {
		return ProcessStateRestarting
	}

	return ProcessStateTerminated
}

func (p *Process) onRestarting() ProcessState {
	delay := p.Options.RestartBackoff.Delay()

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return ProcessStateStarting
	case <-p.Stopping():
		return ProcessStateTerminated
	}
}
