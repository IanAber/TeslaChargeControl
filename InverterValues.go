package main

import (
	"strconv"
	"sync"
)

type InverterValues struct {
	volts          float32 // battery voltage
	amps           float32 // battery current
	soc            float32 // battery state of charge
	vsetpoint      float32 // setpoint for charging the battery
	frequency      float64 // mains frequency
	iMax           float32 // maximum battery charging current
	OnRelay1       bool    // Generator relay
	OnRelay2       bool
	OnRelay1Slave1 bool
	OnRelay2Slave1 bool
	OnRelay1Slave2 bool
	OnRelay2Slave2 bool
	GnRun          bool
	GnRunSlave1    bool
	GnRunSlave2    bool
	AutoGn         bool
	AutoLodExt     bool
	AutoLodSoc     bool
	Tm1            bool
	Tm2            bool
	ExtPwrDer      bool
	ExtVfOk        bool
	GdOn           bool
	Errror         bool
	Run            bool
	BatFan         bool
	AcdCir         bool
	MccBatFan      bool
	MccAutoLod     bool
	Chp            bool
	ChpAdd         bool
	SiComRemote    bool
	OverLoad       bool
	ExtSrcConn     bool
	Silent         bool
	Current        bool
	FeedSelfC      bool
	Esave          bool

	iBattValues       [60]float32
	iBattValuePointer int

	bmsVBat               float64
	bmsIBat               float64
	bmsTBat               float64
	bmsChargeVolts        float64
	bmsDischargeVolts     float64
	bmsChargeCurrentMax   float64
	bmsDichargeCurrentMax float64
	bmsSOC                uint16
	bmsSOH                uint16

	inverterPower float32
	mu            sync.Mutex

	Log bool
}

// GetPower returns the total inverter power
func (i *InverterValues) GetPower() float32 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.inverterPower
}

func (i *InverterValues) SetPower(power float32) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.inverterPower = power
}

// GetVolts returns the battery voltage
func (i *InverterValues) GetVolts() float32 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.volts
}

// GetAmps returns the battery current in Amps. Negative = Charging
func (i *InverterValues) GetAmps() float32 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.amps
}

// GetBatAmps returns the current as reported by the battery
func (i *InverterValues) GetBatAmps() float64 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.bmsIBat
}

// GetAvgAmps returns the average battery current over the past 300 readings
func (i *InverterValues) GetAvgAmps() float32 {
	i.mu.Lock()
	defer i.mu.Unlock()

	avg := float32(0)
	for _, val := range i.iBattValues {
		avg += val
	}
	return avg / float32(len(i.iBattValues))
}

// GetSOC returns the battery state of charge in percent
func (i *InverterValues) GetSOC() float32 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.soc
}

// GetSetPoint returns the battery charging voltage setpoint in Volts
func (i *InverterValues) GetSetPoint() float32 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.vsetpoint
}

// GetFrequency returns the mains frequency in Hz
func (i *InverterValues) GetFrequency() float64 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.frequency
	//	return 57.0
}

// GetIMax returns the maximum allowed battery charging current
func (i *InverterValues) GetIMax() float32 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.iMax
}

func (i *InverterValues) GetFlags() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	sFlags := `                "onRelay1":"` + strconv.FormatBool(i.OnRelay1) + `",
`
	sFlags += `                "onRelay2":"` + strconv.FormatBool(i.OnRelay2) + `",
`
	sFlags += `                "onRelay1Slave1":"` + strconv.FormatBool(i.OnRelay1Slave1) + `",
`
	sFlags += `                "onRelay2Slave1":"` + strconv.FormatBool(i.OnRelay1Slave2) + `",
`
	sFlags += `                "onRelay1Slave2":"` + strconv.FormatBool(i.OnRelay2Slave1) + `",
`
	sFlags += `                "onRelay2Slave2":"` + strconv.FormatBool(i.OnRelay2Slave2) + `",
`
	sFlags += `                "gnRun":"` + strconv.FormatBool(i.GnRun) + `",
`
	sFlags += `                "gnRunSlave1":"` + strconv.FormatBool(i.GnRunSlave1) + `",
`
	sFlags += `                "gnRunSlave2":"` + strconv.FormatBool(i.GnRunSlave2) + `",
`
	sFlags += `                "autoGn":"` + strconv.FormatBool(i.AutoGn) + `",
`
	sFlags += `                "autoLodExt":"` + strconv.FormatBool(i.AutoLodExt) + `",
`
	sFlags += `                "autoLodSoc":"` + strconv.FormatBool(i.AutoLodSoc) + `",
`
	sFlags += `                "tm1":"` + strconv.FormatBool(i.Tm1) + `",
`
	sFlags += `                "tm2":"` + strconv.FormatBool(i.Tm2) + `",
`
	sFlags += `                "extPwrDer":"` + strconv.FormatBool(i.ExtPwrDer) + `",
`
	sFlags += `                "extVfOk":"` + strconv.FormatBool(i.ExtVfOk) + `",
`
	sFlags += `                "gdOn":"` + strconv.FormatBool(i.GdOn) + `",
`
	sFlags += `                "errror":"` + strconv.FormatBool(i.Errror) + `",
`
	sFlags += `                "run":"` + strconv.FormatBool(i.Run) + `",
`
	sFlags += `                "batFan":"` + strconv.FormatBool(i.BatFan) + `",
`
	sFlags += `                "acdCir":"` + strconv.FormatBool(i.AcdCir) + `",
`
	sFlags += `                "mccBatFan":"` + strconv.FormatBool(i.MccBatFan) + `",
`
	sFlags += `                "mccAutoLod":"` + strconv.FormatBool(i.MccAutoLod) + `",
`
	sFlags += `                "chp":"` + strconv.FormatBool(i.Chp) + `",
`
	sFlags += `                "chpAdd":"` + strconv.FormatBool(i.ChpAdd) + `",
`
	sFlags += `                "siComRemote":"` + strconv.FormatBool(i.SiComRemote) + `",
`
	sFlags += `                "overLoad":"` + strconv.FormatBool(i.OverLoad) + `",
`
	sFlags += `                "extSrcConn":"` + strconv.FormatBool(i.ExtSrcConn) + `",
`
	sFlags += `                "silent":"` + strconv.FormatBool(i.Silent) + `",
`
	sFlags += `                "current":"` + strconv.FormatBool(i.Current) + `",
`
	sFlags += `                "feedSelfC":"` + strconv.FormatBool(i.FeedSelfC) + `",
`
	sFlags += `                "esave":"` + strconv.FormatBool(i.Esave) + `"`
	return sFlags
}

func (i *InverterValues) SetVolts(volts float32) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.volts = volts
}

func (i *InverterValues) SetAmps(amps float32) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.amps = amps
	//i.iBattValues[i.iBattValuePointer] = amps
	//i.iBattValuePointer++
	//if i.iBattValuePointer >= len(i.iBattValues) {
	//	i.iBattValuePointer = 0
	//}
}

func (i *InverterValues) SetBmsVolts(volts float64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.bmsVBat = volts
}

func (i *InverterValues) SetBmsAmps(amps float64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.bmsIBat = amps
	i.iBattValues[i.iBattValuePointer] = float32(amps)
	i.iBattValuePointer++
	if i.iBattValuePointer >= len(i.iBattValues) {
		i.iBattValuePointer = 0
	}
}

func (i *InverterValues) SetSOC(soc float32) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.soc = soc
}

// GetChargeLevel says whether the Tesla chargers may take more power (+1),
// should take less (-1) or should stay as they are (0), from the battery SOC
// and the BMS battery current.
func (i *InverterValues) GetChargeLevel() int {
	i.mu.Lock()
	defer i.mu.Unlock()

	//	log.Printf("SOC = %f | Amps = %f", i.soc, i.bmsIBat)
	if i.soc > 95 {
		return 1
	}
	if i.soc < 30 { // Never increase if SOC < 30%
		if i.bmsIBat < 50 { // Decrease if charging < 50A
			return -1
		} else {
			return 0
		}
	} else {
		if i.bmsIBat > 30 { // OK to start charging if charging is above 30A
			return 1
		} else {
			if i.bmsIBat < 0 { // Reduce charging if current drops to discharge otherwise leave it alone
				return -1
			} else {
				return 0
			}
		}
	}
}

func (i *InverterValues) SetSetPoint(setPoint float32) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.vsetpoint = setPoint
}

func (i *InverterValues) SetFrequency(f float64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.frequency = f
}

func (i *InverterValues) SetIMax(iMax float32) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.iMax = iMax
}

func (i *InverterValues) IsSame(v *InverterValues) bool {
	return (i.vsetpoint == v.vsetpoint) && (i.frequency == v.frequency) && (i.amps == v.amps) && (i.soc == v.soc) && (i.volts == v.volts) && (i.iMax == v.iMax)
}
