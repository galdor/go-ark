package ark

import (
	"errors"
	"testing"

	"go.n16f.net/ark/pkg/ark/log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	return Run("test", p, logger)
}

func TestQuickRun(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	tp := TestProcess{}
	require.NoError(RunTestProcess(&tp))

	assert.Equal(1, tp.NbStartCalls)
	assert.Equal(1, tp.NbMainCalls)
	assert.Equal(1, tp.NbStopCalls)
}

func TestStartError(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	tp := TestProcess{StartFailure: true}
	err := RunTestProcess(&tp)

	var startErr *ProcessStartError
	require.Error(err)
	require.ErrorAs(err, &startErr)
	require.ErrorIs(err, ErrStartFailure)

	assert.Equal(1, tp.NbStartCalls)
	assert.Equal(0, tp.NbMainCalls)
	assert.Equal(1, tp.NbStopCalls)
}

func TestMainError(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	tp := TestProcess{MainFailure: true}
	err := RunTestProcess(&tp)

	var mainErr *ProcessMainError
	require.Error(err)
	require.ErrorAs(err, &mainErr)
	require.ErrorIs(err, ErrMainFailure)

	assert.Equal(1, tp.NbStartCalls)
	assert.Equal(1, tp.NbMainCalls)
	assert.Equal(1, tp.NbStopCalls)
}
