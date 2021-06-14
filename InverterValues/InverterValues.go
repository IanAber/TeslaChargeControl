package InverterValues

import (
	"SystemController/quinticFunction"
	"github.com/golang/glog"
	"strconv"
	"sync"
)

type InverterValues struct {
	volts          float32
	amps           float32
	soc            float32
	vsetpoint      float32
	frequency      float64
	iMax           float32
	OnRelay1       bool
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

	vBattMin   float32
	vBattMax   float32
	vBattDelta float32
	qf         quinticFunction.QuinticFunction

	mu sync.Mutex

	Log bool
}

func (i *InverterValues) LoadFunctionConstants(filename string) error {
	return i.qf.LoadConstants(filename)
}

func (i *InverterValues) GetVolts() float32 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.volts
}

func (i *InverterValues) GetAmps() float32 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.amps
}
func (i *InverterValues) GetSOC() float32 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.soc
}
func (i *InverterValues) GetSetPoint() float32 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.vsetpoint
}
func (i *InverterValues) GetFrequency() float64 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.frequency
	//	return 57.0
}
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
}

func (i *InverterValues) SetSOC(soc float32) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.soc = soc
}

func (i *InverterValues) GetQuinticFunction() quinticFunction.QuinticFunction {
	return i.qf
}

// Return -1 if we need more power to charge
// Return 0 if we are inside the band of acceptable Delta V
// Return +1 if we can take more power for something else
func (i *InverterValues) GetChargeLevel() int {
	i.vBattMax, i.vBattMin = i.qf.Eval(i.soc)
	i.vBattDelta = i.vsetpoint - i.volts
	switch {
	case (i.frequency > 61) && (i.amps < 0):
		if i.Log {
			glog.Infof("(i.frequency(%f) > 61Hz) && (i.amps(%f) < 0) - Raise consumption", i.frequency, i.amps)
			glog.Flush()
		}
		return 1 // Inverters are throttled and battery is charging
	case (i.frequency < 59.5) && (i.amps > 0):
		if i.Log {
			glog.Infof("(i.frequency(%f) < 59.5Hz) && (i.amps(%f) > 0) - Lower consumption", i.frequency, i.amps)
			glog.Flush()
		}
		return -1 // Inverters are all running and battery is discharging
	case (i.vBattDelta > i.vBattMax) && (i.frequency < 61) && (i.amps > -60):
		if i.Log {
			glog.Infof("(i.vBattDelta(%f) > i.vBattMax(%f)) && (i.frequency(%f) < 60.25) - Lower consumption", i.vBattDelta, i.vBattMax, i.frequency)
			glog.Flush()
		}
		return -1 // Battery voltage is below the acceptable setpoint and the inverters
		// are not throttled and charge current is less than 60Amps
	case (i.vBattDelta < i.vBattMin) || (i.amps < -80):
		if i.Log {
			glog.Infof("(i.vBattDelta(%f) < i.vBattMin(%f)) - Raise consumption", i.vBattDelta, i.vBattMin)
			glog.Flush()
		}
		return 1 // Battery voltage is above the acceptable setpoint or we are charging at more than 80 amps
	default:
		return 0
	}
}

func (i *InverterValues) GetVBattDeltaMax() float32 {
	return i.vBattMax
}

func (i *InverterValues) GetVBattDeltaMin() float32 {
	return i.vBattMin
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
