package main

import "encoding/binary"

type Can308 struct {
	Power uint16
}

func (c *Can308) LoadPwr() float32 {
	return float32(int16(c.Power)) / 10.0
}

func NewCan308(d []byte) Can308 {
	c := Can308{binary.LittleEndian.Uint16(d[0:])}
	return c
}
