package twcSlave

import (
	"SystemController/TeslaAPI"
	"SystemController/twcMessage"
	"fmt"
	"github.com/goburrow/serial"
	"log"
	"net/smtp"
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
	verbose        bool
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
//)
func New(address uint16, verbose bool, port serial.Port) Slave {
	s := Slave{address, 0.0, 0.0, 0.0, 0, time.Now(), verbose,
		port, time.Now(), 0, time.Unix(0, 0), true, false}
	if verbose {
		fmt.Println("New slave created.")
	}
	return s
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

func (slave *Slave) SendMasterHeartbeat(masterAddress uint16, api *TeslaAPI.TeslaAPI) {
	msg := twcMessage.New(slave.port, slave.verbose)
	if masterAddress == 0 {
		log.Panicln("Attempt to send hearbeat from a master address of 0! This can't be correct.")
	}
	if slave.allowedValue == 0 {
		if slave.current > 20 {
			if slave.stopped {
				if time.Since(slave.timeSetTo0Amps) > (time.Minute * 4) {
					if !api.APIDisabled {
						log.Println("Stopping Tesla charging. current is shown as ", float32(slave.current)/100)
						go func() {
							err := api.StopCharging()
							if err != nil {
								log.Println(err)
							}
						}()
					} else {
						slave.disabled = true
					}
				}
			}
		}
	}
	// If we are disabled then we need to stop sending hearbeats.
	//This should only happen when we failed to stop the car charging through the API.
	if slave.disabled {
		return
	}
	if slave.setPoint != slave.allowedValue {
		if slave.allowedValue >= 600 {
			if slave.stopped {
				slave.disabled = false
				if time.Since(slave.timeSetTo0Amps) > (time.Minute * 4) {
					if !api.APIDisabled {
						go func() {
							err := api.StartCharging()
							if err != nil {
								log.Println("Failed to call StartCharging in the Tesla API.", err)
							}
						}()
					} else {
						slave.disabled = true
					}
				}
			}
			// Tell the car to charge at the provided current
			if (slave.spikeTime.After(time.Now())) && (slave.spikeAmps > 0) {
				if slave.verbose {
					fmt.Println("Master Heartbeat - MasterChangeSetpoint/LimitChargeCurrent (spike) => ", slave.spikeAmps)
				}
				msg.SendMasterHeartbeat(masterAddress, slave.address, MasterChangeSetpoint, 0, slave.spikeAmps)
				msg.SendMasterHeartbeat(masterAddress, slave.address, MasterLimitChargeCurrent, 0, slave.spikeAmps)
			} else {
				slave.spikeAmps = 0
				if slave.verbose {
					fmt.Println("Master Heartbeat - MasterChangeSetpoint/LimitChargeCurrent => ", slave.allowedValue)
				}
				msg.SendMasterHeartbeat(masterAddress, slave.address, MasterChangeSetpoint, 0, slave.allowedValue)
				msg.SendMasterHeartbeat(masterAddress, slave.address, MasterLimitChargeCurrent, 0, slave.allowedValue)
			}
			slave.stopped = false
			slave.timeSetTo0Amps = time.Unix(0, 0)
		} else {
			// With Protocol-2 we cannot stop the car charging
			// Tell the car to stop charging as we cannot supply at least 6 amps
			if slave.verbose {
				fmt.Println("Master Heartbeat - MasterChangeSetpoint/LimitChargeCurrent => 0")
			}
			msg.SendMasterHeartbeat(masterAddress, slave.GetAddress(), MasterChangeSetpoint, 0, 0)
			msg.SendMasterHeartbeat(masterAddress, slave.address, MasterLimitChargeCurrent, 0, 0)
			if slave.timeSetTo0Amps == time.Unix(0, 0) {
				slave.timeSetTo0Amps = time.Now()
			} else if slave.current > 20 {
				// If we increasing teslaset the amps to 0 more than 1 minute ago and we are still charging then use the API to stop the car charging.
				if (time.Since(slave.timeSetTo0Amps) > (4 * time.Minute)) && (slave.current > 50) && !api.IsHoldoff() {
					log.Println("Turn off the car. this.current = ", slave.current)
					if slave.current > 50 {
						log.Println("Sending STOP via the Tesla API.")
						if time.Since(slave.timeSetTo0Amps) > (time.Minute * 4) {
							if !api.APIDisabled {
								log.Println("Stopping Tesla charging. current is shown as ", float64(slave.current))
								go func() {
									err := api.StopCharging()
									if err != nil {
										log.Println(err)
									}
								}()
								slave.stopped = true
							} else {
								slave.disabled = true
							}
						}
					}
				}
				// If we set amps to 0 more than 5 minutes ago and we STILL have not stopped charging then stop the Tesla communications and send an email
				if (time.Since(slave.timeSetTo0Amps) > (time.Minute * 5)) && !slave.stopped {
					log.Println("Tried to stop the car from charging 5 minutes ago but it is still going.")
					smtperr := smtp.SendMail("mail.cedartechnology.com:587",
						smtp.PlainAuth("", "pi@cedartechnology.com", "7444561", "mail.cedartechnology.com"),
						"pi@cedartechnology.com", []string{"ian.abercrombie@cedartechnology.com"}, []byte("From: Aberhome1\r\nTo: Ian.Abercrombie@CedarTechnology.com\r\nSubject: Tesla Stop Charging Failed\r\n\r\nTried t stop the car from charging 5 minutes ago but it is still going."))
					if smtperr != nil {
						log.Println("Failed to send email about the error above. ", smtperr)
					}
				}
			}
		}
	} else {
		// Status Quo...
		if slave.verbose {
			fmt.Println("Status quo heartbeat")
		}
		msg.SendMasterHeartbeat(masterAddress, slave.GetAddress(), MasterStatusQuo, 0x0, 0x0)
	}
}

func (slave *Slave) StopFlag() bool {
	return slave.stopped
}
