package twcSlave

import (
	"SystemController/twcMessage"
	"fmt"
	"github.com/goburrow/serial"
	"log"
	"time"
)

//type Slave interface {
//	SetCurrent(float32) float32
//	UpdateValues(*twcMessage.TwcMessage)
//	Address() uint
//	TimeSinceLastHeartBeat() time.Duration
//	GetRequested() float32
//	GetStatus() byte
//}

// status values :
// 0 - Ready
// 1 - charging
// 2 - no master
// 3 - do not charge
// 4 - ready to charge / charge scheduled
// 5 - busy
// 8 - starting to charge

const (
	StatusReady = iota
	StatusCharging
	StatusNoMaster
	StatusDoNotCharge
	StatusReadyToCharge
	StatusBusy
	StatusLoweringPower
	StatusRaisingPower
	StatusStartingToCharge
	StatusLimitingPower
	StatusAdjustmentPeriodComplete
)

type Slave struct {
	address        uint16
	current        uint16
	setPoint       uint16
	allowedValue   uint16
	status         byte
	lastHeartBeat  time.Time
	log            bool
	port           serial.Port
	spikeTime      time.Time
	spikeAmps      uint16
	timeSetTo0Amps time.Time
	stopped        bool
	disabled       bool
}

//goland:noinspection ALL
const (
	MasterStatusQuo = iota
	_
	_ //MasterError
	_
	_
	MasterChangeSetpoint
	_ //MasterTempIncrease2Amps
	_ //MasterTempDecrease2Amps
	_ //MasterAckCarStopped
	MasterLimitChargeCurrent
)

// New /*
// const (
//
//	SlaveReady = iota // May NOT be plugged in
//	SlaveCharging
//	SlaveLostComms
//	SlaveDoNotCharge
//	SlaveReadyToCharge
//	SlaveBusy
//	SlaveLoweringPower
//	SlaveRaisingPower
//	SlaveStartingToCharge
//	SlaveLimitingPower
//	SlaveAdjustmentPeriodComplete
//
// )
func New(address uint16, logEnabled bool, port serial.Port) Slave {
	s := Slave{address, 0.0, 0.0, 0.0, 0, time.Now(), logEnabled,
		port, time.Now(), 0, time.Unix(0, 0), true, false}
	if logEnabled {
		log.Println("New slave created.")
	}
	return s
}

// Enable enables or disable controlling its ability to supply power to the car.
func (slave *Slave) Enable(bEnable bool) {
	slave.disabled = !bEnable
}

func (slave *Slave) EnableLogging(bLoggingEnable bool) {
	slave.log = bLoggingEnable
}

func (slave *Slave) RequestCharge() bool {
	return (slave.status == StatusDoNotCharge) || (slave.status == StatusStartingToCharge) ||
		(slave.status == StatusReadyToCharge) || (slave.status == StatusCharging) ||
		(slave.status == StatusLoweringPower) || (slave.status == StatusRaisingPower) ||
		(slave.status == StatusLimitingPower) || (slave.status == StatusAdjustmentPeriodComplete)
}

func (slave *Slave) GetAddress() uint16 {
	return slave.address
}

func (slave *Slave) GetStatus() string {
	if slave.disabled {
		return "DISABLED"
	}
	switch slave.status {
	case StatusReady:
		return "Ready"
	case StatusCharging:
		return "Charging"
	case StatusNoMaster:
		return "No Master"
	case StatusDoNotCharge:
		return "Do Not Charge"
	case StatusReadyToCharge:
		return "Ready To Charge"
	case StatusBusy:
		return "Busy"
	case StatusLoweringPower:
		return "Lowering Power Temporarily"
	case StatusRaisingPower:
		return "Raising Power Temporarily"
	case StatusStartingToCharge:
		return "Starting To Charge"
	case StatusLimitingPower:
		return "Limiting Power"
	case StatusAdjustmentPeriodComplete:
		return "Adjustment Period Complete"
	}
	return fmt.Sprintf("Unknown Status [%d]", slave.status)
}

// SetCurrent /*
// Set the allowed current for this slave.
func (slave *Slave) SetCurrent(newValue uint16) {
	if (slave.allowedValue < newValue) && (slave.allowedValue < 1600) && (newValue < 1600) {
		slave.spikeAmps = 1600
		slave.spikeTime = time.Now().Add(time.Second * 6)
	}
	slave.allowedValue = newValue
	if newValue > 0 {
		slave.timeSetTo0Amps = time.Time{}
	} else {
		if slave.timeSetTo0Amps.IsZero() {
			slave.timeSetTo0Amps = time.Now()
		}
	}
}

func (slave *Slave) GetCurrent() uint16 {
	return slave.current
}

func (slave *Slave) GetRequested() uint16 {
	return slave.setPoint
}

func (slave *Slave) GetAllowed() uint16 {
	return slave.allowedValue
}

func (slave *Slave) GetStopped() bool {
	return slave.stopped
}

func (slave *Slave) UpdateValues(msg *twcMessage.TwcMessage) {
	slave.setPoint = msg.GetSetPoint()
	slave.current = msg.GetCurrent()
	slave.status = msg.GetStatus()
	slave.lastHeartBeat = time.Now()
}

func (slave *Slave) TimeSinceLastHeartbeat() time.Duration {
	return time.Since(slave.lastHeartBeat)
}

func (slave *Slave) SendMasterHeartbeat(masterAddress uint16) {
	msg := twcMessage.New(slave.port, slave.log)
	if masterAddress == 0 {
		log.Panicln("Attempt to send hearbeat from a master address of 0! This can't be correct.")
	}
	// If we are disabled then we need to stop sending hearbeats.
	if slave.disabled {
		if slave.log {
			log.Println("slave is disabled")
		}
		return
	} else {
		if slave.log {
			log.Println("slave allowed value = ", slave.allowedValue)
		}
	}
	if slave.setPoint != slave.allowedValue {
		// We are not charging at the allowed value so send an update
		//		log.Printf("slave.setPoint = %d : slave.allowedValue %d", slave.setPoint, slave.allowedValue)
		if slave.allowedValue >= 600 {
			// Tell the car to charge at the provided current
			if (slave.spikeTime.After(time.Now())) && (slave.spikeAmps > 0) {
				// We are in a spike cycle to get the car charging from stop.
				if slave.log {
					log.Println("Master Heartbeat - MasterChangeSetpoint/LimitChargeCurrent (spike) => ", slave.spikeAmps)
				}
				msg.SendMasterHeartbeat(masterAddress, slave.address, MasterChangeSetpoint, 0, slave.spikeAmps)
				msg.SendMasterHeartbeat(masterAddress, slave.address, MasterLimitChargeCurrent, 0, slave.spikeAmps)
			} else {
				// Spike time has expired
				slave.spikeAmps = 0
				if slave.log {
					log.Println("Master Heartbeat - MasterChangeSetpoint/LimitChargeCurrent => ", slave.allowedValue)
				}
				msg.SendMasterHeartbeat(masterAddress, slave.address, MasterChangeSetpoint, 0, slave.allowedValue)
				msg.SendMasterHeartbeat(masterAddress, slave.address, MasterLimitChargeCurrent, 0, slave.allowedValue)
			}
			slave.stopped = false
		} else {
			// 6A is the minimum, and we are below that, so we need to stop charging
			if slave.log {
				log.Println("Master Heartbeat - MasterChangeSetpoint/LimitChargeCurrent => 6A")
			}
			if slave.allowedValue == 0 {
				if !slave.timeSetTo0Amps.IsZero() && time.Since(slave.timeSetTo0Amps) > (time.Minute*5) {
					if slave.log {
						log.Println("disabling slave because allowedValue is set to 0A for more than 5 minutes")
					}
					msg.SendMasterHeartbeat(masterAddress, slave.GetAddress(), MasterChangeSetpoint, 0, 0)
					msg.SendMasterHeartbeat(masterAddress, slave.address, MasterLimitChargeCurrent, 0, 0)
					slave.disabled = true
				} else {
					if slave.log {
						log.Printf("the Tesla is at the minimum level. Wating for %d seconds before disabling it", ((time.Minute*5)-time.Since(slave.timeSetTo0Amps))/time.Second)
					}
					msg.SendMasterHeartbeat(masterAddress, slave.address, MasterChangeSetpoint, 0, 600)
					msg.SendMasterHeartbeat(masterAddress, slave.address, MasterLimitChargeCurrent, 0, 600)
				}
			}
		}
	} else {
		// Status Quo...
		if slave.log {
			log.Println("Status quo heartbeat")
		}
		msg.SendMasterHeartbeat(masterAddress, slave.GetAddress(), MasterStatusQuo, 0x0, 0x0)
	}
}
