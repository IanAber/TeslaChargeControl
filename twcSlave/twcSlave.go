package twcSlave

import (
	"TeslaChargeControl/twcMessage"
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
	Status_Ready = iota
	Status_Charging
	Status_No_Master
	Status_DoNotCharge
	Status_ReadyToCharge
	Status_Busy
	_
	_
	Status_StartingToCharge
)

type Slave struct {
	address       uint
	current       int
	setPoint      int
	allowedValue  int
	status        byte
	lastHeartBeat time.Time
	listenMode    bool
	port          serial.Port
	spikeTime     time.Time
	spikeAmps     int
}

const (
	MasterStatusQuo = iota
	_
	MasterError
	_
	_
	MasterChangeSetpoint
	MasterTempIncrease2Amps
	MasterTempDecrease2Amps
	MasterAckCarStopped
	MasterLimitChargeCurrent
)

const (
	SlaveReady = iota // May NOT be plugged in
	SlaveCharging
	SlaveLostComms
	SlaveDoNotCharge
	SlaveReadyToCharge
	SlaveBusy
	_
	_
	SlaveStartingToCharge
)

func New(address uint, listenMode bool, port serial.Port) Slave {
	s := Slave{address, 0.0, 0.0, 0.0, 0, time.Now(), listenMode, port, time.Now(), 0}
	_, _, _, _, _ = MasterError, MasterTempIncrease2Amps, MasterTempDecrease2Amps, MasterAckCarStopped, MasterLimitChargeCurrent
	_, _, _, _, _, _, _ = SlaveReady, SlaveCharging, SlaveLostComms, SlaveDoNotCharge, SlaveReadyToCharge, SlaveBusy, SlaveStartingToCharge
	return s
}

func (s *Slave) RequestCharge() bool {
	return ((s.status == Status_DoNotCharge) || (s.status == Status_StartingToCharge) ||
		(s.status == Status_ReadyToCharge) || (s.status == Status_Charging))
}

func (s *Slave) GetAddress() uint {
	return s.address
}

func (s *Slave) GetStatus() string {
	switch s.status {
	case Status_Ready:
		return "Ready"
	case Status_Charging:
		return "Charging"
	case Status_No_Master:
		return "No Master"
	case Status_DoNotCharge:
		return "Do Not Charge"
	case Status_ReadyToCharge:
		return "Ready To Charge"
	case Status_Busy:
		return "Busy"
	case Status_StartingToCharge:
		return "Starting To Charge"
	}
	return "Unknown Status"
}

// Set the allowed current for this slave.
func (s *Slave) SetCurrent(newValue int) {
	if (s.allowedValue < newValue) && (s.allowedValue < 1600) && (newValue < 1600) {
		s.spikeAmps = 1600
		s.spikeTime = time.Now().Add(time.Second * 6)
	}
	s.allowedValue = newValue
}

func (s *Slave) GetCurrent() int {
	return s.current
}

func (s *Slave) GetRequested() int {
	return s.setPoint
}

func (s *Slave) GetAllowed() int {
	return s.allowedValue
}

func (s *Slave) UpdateValues(msg *twcMessage.TwcMessage) {
	s.setPoint = msg.GetSetPoint()
	s.current = msg.GetCurrent()
	s.status = msg.GetStatus()
	s.lastHeartBeat = time.Now()
}

func (s *Slave) TimeSinceLastHeartbeat() time.Duration {
	return time.Since(s.lastHeartBeat)
}

func (s *Slave) SendMasterHeartbeat(masterAddress uint, listenMode bool) {
	msg := twcMessage.New(s.port, s.listenMode)
	if masterAddress == 0 {
		log.Panicln("Attempt to send hearbeat from a master address of 0! This can't be correct.")
	}
	if s.setPoint != s.allowedValue {
		if s.allowedValue >= 500 {
			// Tell the car to charge at the provided current
			if (s.spikeTime.After(time.Now())) && (s.spikeAmps > 0) {
				msg.SendMasterHeartbeat(masterAddress, s.address, MasterChangeSetpoint, 0, s.spikeAmps)
			} else {
				s.spikeAmps = 0
				msg.SendMasterHeartbeat(masterAddress, s.address, MasterChangeSetpoint, 0, s.allowedValue)
			}
			//			if listenMode {
			//				fmt.Printf("Sending to %04x from %04x status = %02x Setpoint = %0.2f Current = %0.2f\n", s.address, masterAddress, MasterChangeSetpoint, float32(s.allowedValue) / 100, float32(s.allowedValue) / 100)
			//			}
		} else {
			// Tell the car to stop charging as we cannot supply at least 5 amps
			msg.SendMasterHeartbeat(masterAddress, s.GetAddress(), MasterChangeSetpoint, 0, 0)
			//			if listenMode {
			//				fmt.Printf("Sending to %04x from %04x status = %02x Setpoint = 0.0 Current = 0.0\n", s.GetAddress(), masterAddress, MasterChangeSetpoint)
			//			}
		}
	} else {
		// Status Quo...
		//		if listenMode {
		//			fmt.Printf("Sending status quo message to slave at %02x from %02x\n", s.GetAddress(), masterAddress)
		//		}
		msg.SendMasterHeartbeat(masterAddress, s.GetAddress(), MasterStatusQuo, 0x0, 0x0)
	}
}
