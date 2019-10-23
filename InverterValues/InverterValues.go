package InverterValues

import (
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
	mu             sync.Mutex
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
