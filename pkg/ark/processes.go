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
	ProcessName string
	Err         error
}

func (err *ProcessStartError) Error() string {
	return fmt.Sprintf("cannot start process %q: %v", err.ProcessName, err.Err)
}

func (err *ProcessStartError) Unwrap() error {
	return err.Err
}

type ProcessMainError struct {
	ProcessName string
	Err         error
}

func (err *ProcessMainError) Error() string {
	return fmt.Sprintf("error running process %q: %v", err.ProcessName, err.Err)
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

	// Controls the full lifecycle of the process. The process stops with all
	// its children when it is canceled and cannot be restarted.
	ctx    context.Context
	cancel context.CancelFunc

	// Controls the lifecycle of one run (start/main/stop) for the process. The
	// process stops with all its children when it is canceled and can be
	// restarted afterwards.
	runCtx    context.Context
	runCancel context.CancelFunc

	// Controls the lifecycle of the children.
	childrenCtx    context.Context
	childrenCancel context.CancelFunc
	childrenWg     sync.WaitGroup

	parentErrChan chan<- error
	errChan       chan error

	// Note that mutex also protects runCtx, runCancel, childrenCtx and
	// childrenCancel.
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

func MustRunProcess(name string, behavior ProcessBehavior, logger *log.Logger) {
	if err := RunProcess(name, behavior, logger); err != nil {
		logger.Error("%v", err)
		os.Exit(1)
	}
}

func RunProcess(
	name string, behavior ProcessBehavior, logger *log.Logger,
) error {
	logger = logger.With("scope", name)

	ctx, cancel := context.WithCancel(context.Background())
	runCtx, runCancel := context.WithCancel(ctx)
	childrenCtx, childrenCancel := context.WithCancel(runCtx)

	opts := ProcessOptions{Unlinked: true}

	p := Process{
		Name:     name,
		Behavior: behavior,
		Log:      logger,
		Options:  &opts,

		ctx:    ctx,
		cancel: cancel,

		runCtx:    runCtx,
		runCancel: runCancel,

		childrenCtx:    childrenCtx,
		childrenCancel: childrenCancel,

		errChan: make(chan error),

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
	child.runCtx, child.runCancel = context.WithCancel(child.ctx)
	child.childrenCtx, child.childrenCancel = context.WithCancel(child.runCtx)

	return &child
}

func (p *Process) Stop() {
	p.cancel()
}

func (p *Process) Context() context.Context {
	p.mutex.RLock()
	runCtx := p.runCtx
	p.mutex.RUnlock()
	return runCtx
}

func (p *Process) Stopping() <-chan struct{} {
	p.mutex.RLock()
	runCtx := p.runCtx
	p.mutex.RUnlock()
	return runCtx.Done()
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
				err = p.startError(err)
			}

		case ProcessStateRunning:
			newState, err = p.onRunning()
			if err != nil {
				err = p.mainError(err)
			}

		case ProcessStateStopping:
			newState = p.onStopping()

		case ProcessStateRestarting:
			newState = p.onRestarting()

		case ProcessStateTerminated:
			p.maybePropagateError(p.Error())
			return
		}

		if err != nil {
			p.Log.Error("%v", err)
		}

		if state == ProcessStateStarting || state == ProcessStateRunning {
			p.mutex.RLock()
			runCtx := p.runCtx
			p.mutex.RUnlock()

			select {
			case <-p.ctx.Done():
				newState = ProcessStateStopping
			case <-runCtx.Done():
				newState = ProcessStateStopping
			default:
			}
		}

		p.mutex.Lock()
		p.state = newState
		if err != nil && p.err == nil {
			p.err = err
		}
		p.mutex.Unlock()
	}
}

func (p *Process) onStarting() (state ProcessState, err error) {
	p.mutex.Lock()
	runCtx, runCancel := context.WithCancel(p.ctx)
	p.runCtx, p.runCancel = runCtx, runCancel
	p.childrenCtx, p.childrenCancel = context.WithCancel(p.runCtx)
	p.err = nil
	p.mutex.Unlock()

	go func() {
		select {
		case err := <-p.errChan:
			p.mutex.Lock()
			if p.err == nil {
				p.err = err
			}
			p.mutex.Unlock()
			runCancel()

		case <-runCtx.Done():
		}
	}()

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
	runCancel := p.runCancel
	p.mutex.RUnlock()
	runCancel()

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
	case <-p.ctx.Done():
		return ProcessStateTerminated
	}
}

func (p *Process) maybePropagateError(err error) {
	if err == nil {
		return
	}

	// We do not propagate the error if we are going to restart or if we are
	// unlinked.
	if p.Options.RestartOnError || p.Options.Unlinked {
		return
	}

	// The parent may have read an error for another child and is
	// canceling the children context (the one p.ctx derives from).
	// In that case there is nothing to do but return.
	select {
	case p.parentErrChan <- err:
	case <-p.ctx.Done():
	}
}

func (p *Process) startError(err error) *ProcessStartError {
	return &ProcessStartError{
		ProcessName: p.Name,
		Err:         err,
	}
}

func (p *Process) mainError(err error) *ProcessMainError {
	return &ProcessMainError{
		ProcessName: p.Name,
		Err:         err,
	}
}
