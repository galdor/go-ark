package ark

import (
	"errors"
	"testing"

	"go.n16f.net/ark/pkg/ark/log"

	"go.n16f.net/check"
)

var (
	ErrStartFailure = errors.New("start failure")
	ErrMainFailure  = errors.New("main failure")
)

type TestProcess struct {
	StartFailure bool
	MainFailure  bool

	NbStartCalls int
	NbMainCalls  int
	NbStopCalls  int
}

func (tp *TestProcess) Start(ap *Process) error {
	tp.NbStartCalls++

	if tp.StartFailure {
		return ErrStartFailure
	}

	return nil
}

func (tp *TestProcess) Stop() {
	tp.NbStopCalls++
}

func (tp *TestProcess) Main() error {
	tp.NbMainCalls++

	if tp.MainFailure {
		return ErrMainFailure
	}

	return nil
}

func RunTestProcess(p *TestProcess) error {
	logger := log.DefaultLogger().With("scope", "test")
	return RunProcess("test", p, logger)
}

func TestQuickRun(t *testing.T) {
	check := check.New(t)

	tp := TestProcess{}

	if check.NotError(RunTestProcess(&tp)) {
		check.Equal(tp.NbStartCalls, 1)
		check.Equal(tp.NbMainCalls, 1)
		check.Equal(tp.NbStopCalls, 1)
	}
}

func TestStartError(t *testing.T) {
	check := check.New(t)

	tp := TestProcess{StartFailure: true}
	err := RunTestProcess(&tp)

	var startErr *ProcessStartError
	if check.ErrorAs(err, &startErr) {
		check.ErrorIs(err, ErrStartFailure)
	}

	check.Equal(tp.NbStartCalls, 1)
	check.Equal(tp.NbMainCalls, 0)
	check.Equal(tp.NbStopCalls, 1)
}

func TestMainError(t *testing.T) {
	check := check.New(t)

	tp := TestProcess{MainFailure: true}
	err := RunTestProcess(&tp)

	var mainErr *ProcessMainError
	if check.ErrorAs(err, &mainErr) {
		check.ErrorIs(err, ErrMainFailure)
	}

	check.Equal(tp.NbStartCalls, 1)
	check.Equal(tp.NbMainCalls, 1)
	check.Equal(tp.NbStopCalls, 1)
}
