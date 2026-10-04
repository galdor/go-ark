package ark

import (
	"math/rand/v2"
	"time"

	"go.n16f.net/ark/pkg/ark/utils"
)

type Backoff struct {
	BaseDelay float64
	MaxDelay  float64
	Factor    float64
	Jitter    float64

	delay      float64
	lastUpdate time.Time
}

func NewBackoff(baseDelay, maxDelay, factor, jitter float64) *Backoff {
	if baseDelay <= 0.0 {
		utils.Panic("invalid negative or zero base delay")
	}

	if maxDelay <= 0.0 {
		utils.Panic("invalid negative or zero max delay")
	}

	if factor <= 0.0 {
		utils.Panic("invalid negative or zero factor")
	}

	if jitter < 0.0 || jitter > 1.0 {
		utils.Panic("invalid jitter: value must be between 0.0 and 1.0")
	}

	return &Backoff{
		BaseDelay: baseDelay,
		MaxDelay:  maxDelay,
		Factor:    factor,
		Jitter:    jitter,

		delay:      baseDelay,
		lastUpdate: time.Now(),
	}
}

func (b *Backoff) Reset() {
	b.delay = b.BaseDelay
}

func (b *Backoff) Delay() time.Duration {
	now := time.Now()

	resetDelay := 5.0 * b.MaxDelay
	if now.Sub(b.lastUpdate) >= time.Duration(resetDelay*float64(time.Second)) {
		b.delay = b.BaseDelay
	} else {
		newDelay := b.delay * b.Factor
		minDelay := newDelay * (1.0 - b.Jitter)
		maxDelay := newDelay * (1.0 + b.Jitter)
		newDelay = minDelay + rand.Float64()*(maxDelay-minDelay)
		b.delay = min(newDelay, b.MaxDelay)
	}

	b.lastUpdate = now

	return time.Duration(b.delay * float64(time.Second))
}
