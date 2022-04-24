package Params

import (
	"log"
	"sync"
	"time"
)

const minAmps = 5

// Params /**
type Params struct {
	current    float32   // Current being drawn
	maxAmps    float32   // Maximum amps the car is allowed to draw
	systemMax  float32   // Maximum amps the system can deliver
	lastChange time.Time // Time the last change was made.
	mu         sync.Mutex
}

// GetMaxAmps Return the maximum charging current allowed /**
func (p *Params) GetMaxAmps() float32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.maxAmps
}

// GetCurrent /**
func (p *Params) GetCurrent() float32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}

// GetValues /**
// Return the actual and the maximum allowed charging currents.
func (p *Params) GetValues() (current float32, maxAmps float32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	current = p.current
	maxAmps = p.maxAmps
	return current, maxAmps
}

// SetCurrent /**
// Set the actual charging current as read fromt he Tesla charger
func (p *Params) SetCurrent(i float32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current = i
}

// SetMaxAmps /**
// Set the maximum allowed current that the car can have.
func (p *Params) SetMaxAmps(i float32) {
	// Limit to no more than the system maximum and no less than zero
	if i > p.systemMax {
		i = p.systemMax
	} else if i < 0 {
		i = 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.maxAmps = i
	p.lastChange = time.Now()
}

// Reset /**
func (p *Params) Reset() {
	p.mu.Lock()
	p.mu.Unlock()
	p.maxAmps = 25.0 // This will be the starting current when the car is frst plugged in nd charging starts.
	p.lastChange = time.Now()
	p.systemMax = 47.9 // This is the highest current we can supply to the car.
}

// ChangeCurrent /**
// Change the charging current. Return true if it was changed or false
// if we are already at maximum or minimum so no change was made
func (p *Params) ChangeCurrent(delta int16) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	maxAmps := int16(p.maxAmps)
	systemMax := int16(p.systemMax)

	if delta > 0 {
		// Going up...
		if maxAmps >= systemMax {
			// We are already at the system maximum so do nothing
			return false
		}
		// Give it at least 5 seconds between increases but pretend we did push it up.
		if p.lastChange.Add(time.Second * 5).Before(time.Now()) {
			// If it has been 5 seconds since the last increase then push up the charge current and reset the time
			maxAmps += delta

			if maxAmps < minAmps {
				maxAmps = minAmps
			}
			if maxAmps > systemMax {
				// Don't go over the system maximum
				maxAmps = systemMax
			}
			log.Println("Increasing Tesla current to", maxAmps, "Amps")
			p.lastChange = time.Now()
			p.maxAmps = float32(maxAmps)
		}
	} else {
		if maxAmps == 0 {
			// Already at 0 Amps so do nothing
			return false
		}
		// Wait 15 seconds between each change going downward.
		// Hold the current for 45 seconds if it would shut the car down to lower it further.
		// Pretend we did it if less than 15 seconds since the last change
		if ((maxAmps < 7) && (p.lastChange.Add(time.Second * 45).Before(time.Now()))) || ((maxAmps >= 7) && (p.lastChange.Add(time.Second * 15).Before(time.Now()))) {
			// It has been at least 15 seconds since the last change so drop the current. Hold the current for 45 seconds if it would shut the car down to lower it further.
			maxAmps += delta
			if maxAmps < minAmps {
				// Don't let it go below 0 Amps
				maxAmps = 0
			}
			p.maxAmps = float32(maxAmps)
			log.Println("Decreasing Tesla current to", maxAmps, "Amps")
			// Record the time
			p.lastChange = time.Now()
		}
	}
	return true
}
