package Params

import (
	"sync"
	"time"
)

const minAmps = 5.0

type Params struct {
	current    float32
	maxAmps    float32
	systemMax  float32
	lastChange time.Time
	mu         sync.Mutex
}

func (p *Params) GetMaxAmps() float32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.maxAmps
}

func (p *Params) GetCurrent() float32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}

func (p *Params) GetValues() (current float32, maxAmps float32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	current = p.current
	maxAmps = p.maxAmps
	return current, maxAmps
}

func (p *Params) SetCurrent(i float32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current = i
}

func (p *Params) SetMaxAmps(i float32) {
	if i > p.systemMax {
		i = 48
	} else if i < 0 {
		i = 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.maxAmps = i
	p.lastChange = time.Now()
}

func (p *Params) Reset() {
	p.mu.Lock()
	p.mu.Unlock()
	p.maxAmps = 10.0
	p.lastChange = time.Now()
	p.systemMax = 48.0
}

/// Change the charging current. Return true if it was changed or false if we
/// are already at maximum or minimum so no change was made
func (p *Params) ChangeCurrent(delta int16) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if delta > 0 {
		// Going up...
		if p.maxAmps == p.systemMax {
			// We are already at the system maximum so do nothing
			return false
		}
		// Give it at least 15 seconds between increases but pretend we did push it up.
		if p.lastChange.Add(time.Second * 15).Before(time.Now()) {
			// If it has been a minute since the last increase then push up the charge current and reset the time
			p.maxAmps += float32(delta)
			if p.maxAmps < minAmps {
				p.maxAmps = minAmps
			}
			if p.maxAmps > p.systemMax {
				// Don't go over the system maximum
				p.maxAmps = p.systemMax
			}
			p.lastChange = time.Now()
		}
	} else {
		if p.maxAmps == 0 {
			// Already at 0 Amps so do nothing
			return false
		}
		if p.maxAmps < 7 {

		}
		// Wait 15 seconds between each change going downward.
		// Hold the current for 45 seconds if it would shut the car down to lower it further.
		// Pretend we did it if less than 15 seconds since the last change
		if ((p.maxAmps < 7) && (p.lastChange.Add(time.Second * 45).Before(time.Now()))) || ((p.maxAmps >= 7) && (p.lastChange.Add(time.Second * 15).Before(time.Now()))) {
			// It has been at least 15 seconds since the last change so drop the current. Hold the current for 45 seconds if it would shut the car down to lower it further.
			p.maxAmps += float32(delta)
			if p.maxAmps < minAmps {
				// Don't let it go below 0 Amps
				p.maxAmps = 0.0
			}
			// Record the time
			p.lastChange = time.Now()
		}
	}
	return true
}
