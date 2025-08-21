package main

import "encoding/binary"

type BMS351 struct {
	data [8]byte
}

func NewBMS351(buffer []byte) *BMS351 {
	bms := new(BMS351)
	for i := 0; i < 8; i++ {
		bms.data[i] = buffer[i]
	}
	return bms
}

func (bms *BMS351) ChargeVolts() float64 {
	volts := binary.LittleEndian.Uint16(bms.data[0:2])
	return float64(volts) / 10
}

func (bms *BMS351) ChargeCurrentLimit() float64 {
	amps := binary.LittleEndian.Uint16(bms.data[2:4])
	return float64(amps) / 10
}
func (bms *BMS351) DischargeCurrentLimit() float64 {
	amps := binary.LittleEndian.Uint16(bms.data[4:6])
	return float64(amps) / 10
}

func (bms *BMS351) DischargeVoltage() float64 {
	amps := binary.LittleEndian.Uint16(bms.data[6:8])
	return float64(amps) / 10
}

type BMS355 struct {
	data [8]byte
}

func NewBMS355(buffer []byte) *BMS355 {
	bms := new(BMS355)
	for i := 0; i < 8; i++ {
		bms.data[i] = buffer[i]
	}
	return bms
}

func (bms *BMS355) SOC() uint16 {
	return binary.LittleEndian.Uint16(bms.data[0:2])
}

func (bms *BMS355) SOH() uint16 {
	return binary.LittleEndian.Uint16(bms.data[2:4])
}

type BMS356 struct {
	data [8]byte
}

func NewBMS356(buffer []byte) *BMS356 {
	bms := new(BMS356)
	for i := 0; i < 8; i++ {
		bms.data[i] = buffer[i]
	}
	return bms
}

func (bms *BMS356) VBatt() float64 {
	volts := binary.LittleEndian.Uint16(bms.data[0:2])
	return float64(volts) / 100
}

func (bms *BMS356) IBatt() float64 {
	amps := binary.LittleEndian.Uint16(bms.data[2:4])
	return float64(int16(amps)) / 10
}

func (bms *BMS356) TBatt() float64 {
	temp := binary.LittleEndian.Uint16(bms.data[4:6])
	return float64(temp) / 10
}
