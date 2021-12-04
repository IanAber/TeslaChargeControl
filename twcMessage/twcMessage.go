package twcMessage

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/goburrow/serial"
	"log"
	"time"
)

// Message structure
//
// Byte	Function
//  00	C0
//  01	C?
//  02	E?
//  03	Source Address
//  04	Source Address
//  05	Destination Address
//  06	Destination Address
//  07	Status
//  08	Allowed/Requested Current
//  09	Allowed/Requested Current
//  10	Charging Current
//  11	Charging Current
//  12	00
//  13	00
//  14	Checksum
//  15	C0
//  16	Fe

type TwcMessage struct {
	bytes       []byte
	currentByte int
	isEscaped   bool
	port        serial.Port
	verbose     bool
	isC0        bool
	inProgress  bool
}

// New /*
//Set up a new message buffer
func New(p serial.Port, verbose bool) TwcMessage {
	// 17 byte buffer
	m := TwcMessage{make([]byte, 20), 0, false, p, verbose, false, false}
	return m
}

// AddByte /*
//Add a byte to the buffer
func (this *TwcMessage) AddByte(b byte) (bool, error) {
	if b == 0xC0 {
		if !this.inProgress {
			// 0xC0 is the delimiter. If we are not currently buffering a message we should expect the next character to be FD for a slave message
			// Set the pointer to the start of the buffer in anticipation
			this.currentByte = 0
			this.inProgress = true
			return false, nil
		} else {
			if this.currentByte < 10 {
				// We already saw the 0xC0 code byte we have not got enough characters for a real message yet message yet so this can't be the end.
				// Treat it as another start
				this.currentByte = 0
				this.isEscaped = false
				return false, nil
			} else {
				// If we are actually receiving a message 0xC0 signifies the end of the message but we must have at least 14 bytes
				this.inProgress = false
				if this.IsValid() {
					return true, nil
				} else {
					this.currentByte = 0
					this.isEscaped = false
					this.inProgress = false
					return false, fmt.Errorf("invalid message")
				}
			}
		}
	}
	// 0xDB is the escape code so just set the flag bu do not record the byte
	if b == 0xDB {
		this.isEscaped = true
		return false, nil
	}
	// If the last byte was an escape character process this byte accordingly
	// Escape sequences start with 0xdb then 0xdc => 0xc0 or 0xdd => 0xdb
	if this.isEscaped {
		switch b {
		case 0xDC:
			this.bytes[this.currentByte] = 0xC0
		case 0xDD:
			this.bytes[this.currentByte] = 0xDB
		default:
			return false, fmt.Errorf("received 0x%x in an escape sequence. Only 0xDC or 0xDD expected", b)
		}
	} else {
		// Not in an escape sequence so record the actual byte sent
		this.bytes[this.currentByte] = b
	}
	// Move the pointer to the next byte
	this.currentByte++
	// Make sure we do not overrun the buffer
	if this.currentByte >= len(this.bytes) {
		this.currentByte = 0
		this.inProgress = false
		this.isEscaped = false
		return false, fmt.Errorf("buffer overrun!\n%s", hex.Dump(this.bytes))
	}
	// Not at the end of the message yet but no errors
	return false, nil
}

/**
Calculate the chaecksum for the data in the current message buffer.
*/
func (twcMessage *TwcMessage) calculateChecksum(bufferLength int) byte {
	var chksum byte = 0
	for _, b := range twcMessage.bytes[1 : bufferLength-1] {
		chksum += b
	}
	return chksum
}

// IsValid /*
// Validate the message by calculating the checksum and comparing it to what was sent.
// The checksum include all the bytes except the first and last. The fist is the meessage
// type and the last is the checksum sent.
func (twcMessage *TwcMessage) IsValid() (valid bool) {
	if twcMessage.currentByte < 10 {
		fmt.Println("Buffer is too short - returning fail.")
		return false
	}
	chksum := twcMessage.calculateChecksum(twcMessage.currentByte)
	if chksum != twcMessage.bytes[twcMessage.currentByte-1] {
		log.Printf("Checksum error. Expected 0x%02x got 0x%02x at position %d\n", chksum&0xff, twcMessage.bytes[twcMessage.currentByte], twcMessage.currentByte)
	}
	return chksum == twcMessage.bytes[twcMessage.currentByte-1]
}

// Reset /*
// Reset the received message buffer
func (twcMessage *TwcMessage) Reset() {
	twcMessage.isEscaped = false
	twcMessage.isC0 = false
	twcMessage.currentByte = 0
	for i := 0; i < cap(twcMessage.bytes); i++ {
		twcMessage.bytes[i] = 0
	}
}

func (twcMessage *TwcMessage) Print() {
	fmt.Printf("Code %02x\nFrom %04x\nT0 %04x\n%s", twcMessage.GetCode(), twcMessage.GetFromAddress(), twcMessage.GetToAddress(), hex.Dump(twcMessage.bytes))
}

// GetCode /*
// Return the code from the buffer
func (twcMessage *TwcMessage) GetCode() uint16 {
	return binary.BigEndian.Uint16(twcMessage.bytes[0:])
}

// PutCode /*
// Set the code in the buffer
func (twcMessage *TwcMessage) PutCode(c uint16) {
	binary.BigEndian.PutUint16(twcMessage.bytes[0:], c)
}

// GetFromAddress /*
// Return the from address located in bytes 2 and 3 of the buffer
func (twcMessage *TwcMessage) GetFromAddress() uint16 {
	return binary.BigEndian.Uint16(twcMessage.bytes[2:])
}

// PutFromAddress /*
// Set the from address in the buffer
func (twcMessage *TwcMessage) PutFromAddress(a uint16) {
	binary.BigEndian.PutUint16(twcMessage.bytes[2:], a)
}

// GetToAddress /*
// Get the to address from the buffer
func (twcMessage *TwcMessage) GetToAddress() uint16 {
	return binary.BigEndian.Uint16(twcMessage.bytes[4:])
}

// PutToAddress /*
// Set the to address in the buffer
func (twcMessage *TwcMessage) PutToAddress(a uint16) {
	binary.BigEndian.PutUint16(twcMessage.bytes[4:], a)
}

// GetStatus /*
// Get the status from the buffer
func (twcMessage *TwcMessage) GetStatus() byte {
	return twcMessage.bytes[6]
}

// PutCommand /*
// Set the command in the buffer
func (twcMessage *TwcMessage) PutCommand(c byte) {
	twcMessage.bytes[6] = c
}

// GetSetPoint /*
// Get the setpoint from the buffer
func (twcMessage *TwcMessage) GetSetPoint() uint16 {
	return binary.BigEndian.Uint16(twcMessage.bytes[7:])
}

// PutSetPoint /*
// Set the set point in the buffer
func (twcMessage *TwcMessage) PutSetPoint(i uint16) {
	binary.BigEndian.PutUint16(twcMessage.bytes[7:], i)
}

// GetCurrent /*
// Get the current from the buffer
func (twcMessage *TwcMessage) GetCurrent() uint16 {
	return binary.BigEndian.Uint16(twcMessage.bytes[9:])
}

// PutCurrent /*
// Set the current in the buffer
func (twcMessage *TwcMessage) PutCurrent(i uint16) {
	binary.BigEndian.PutUint16(twcMessage.bytes[9:], i)
}

// writeByte /*
// Write a single character to the serial port
func (twcMessage *TwcMessage) writeByte(b byte) {
	bytes := make([]byte, 0)
	bytes = append(bytes, b)
	_, _ = (twcMessage.port).Write(bytes)
}

// SendMessage /*
// Send a message to the Tesla wall charger
func (twcMessage *TwcMessage) SendMessage(messageLength int) {
	//Calculate a new checksum
	chksum := twcMessage.calculateChecksum(messageLength)
	twcMessage.bytes[messageLength-1] = chksum
	twcMessage.writeByte(0xC0)
	for _, b := range twcMessage.bytes[0:messageLength] {
		//Escape 0xc0 and 0xdb codes if in the data area of the message (bytes 1..14)
		switch b {
		case 0xc0:
			twcMessage.writeByte(0xDB)
			twcMessage.writeByte(0xDC)
		case 0xdb:
			twcMessage.writeByte(0xDB)
			twcMessage.writeByte(0xDD)
		default:
			twcMessage.writeByte(b)
		}
	}
	twcMessage.writeByte(0xC0)
	time.Sleep(100 * time.Millisecond)
}

// SendMasterLinkReady1 /*
// Send the Master Link Ready I message
func (twcMessage *TwcMessage) SendMasterLinkReady1(fromAddress uint16) {
	copy(twcMessage.bytes, []byte{0xfc, 0xe1, 0x00, 0x00, 0x77, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	twcMessage.PutFromAddress(fromAddress)
	if twcMessage.verbose {
		fmt.Print("LinkReady-1 ")
	}
	twcMessage.SendMessage(14)
}

// SendMasterLinkReady2 /*
// Send the Master Link Ready II message
func (twcMessage *TwcMessage) SendMasterLinkReady2(fromAddress uint16) {
	copy(twcMessage.bytes, []byte{0xfb, 0xe2, 0x00, 0x00, 0x77, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	twcMessage.PutFromAddress(fromAddress)
	twcMessage.PutToAddress(fromAddress)
	if twcMessage.verbose {
		fmt.Print("LinkReady-1 ")
	}
	twcMessage.SendMessage(14)
}

// SendMasterHeartbeat /*
//
//      # Meaning of data:
//      #
//      # Byte 1 is a command:
//      #   00 Make no changes
//      #   02 Error
//      #     Byte 2 appears to act as a bitmap where each set bit causes the
//      #     slave TWC to enter a different error state. First 8 digits below
//      #     show which bits are set and these values were tested on a Protocol
//      #     2 TWC:
//      #       0000 0001 = Middle LED blinks 3 times red, top LED solid green.
//      #                   Manual says this code means 'Incorrect rotary switch
//      #                   setting.'
//      #       0000 0010 = Middle LED blinks 5 times red, top LED solid green.
//      #                   Manual says this code means 'More than three Wall
//      #                   Connectors are set to Slave.'
//      #       0000 0100 = Middle LED blinks 6 times red, top LED solid green.
//      #                   Manual says this code means 'The networked Wall
//      #                   Connectors have different maximum current
//      #                   capabilities.'
//      #   	0000 1000 = No effect
//      #   	0001 0000 = No effect
//      #   	0010 0000 = No effect
//      #   	0100 0000 = No effect
//  	  #     1000 0000 = No effect
//      #     When two bits are set, the lowest bit (rightmost bit) seems to
//      #     take precedence (ie 111 results in 3 blinks, 110 results in 5
//      #     blinks).
//      #
//      #     If you send 02 to a slave TWC with an error code that triggers
//      #     the middle LED to blink red, slave responds with 02 in its
//      #     heartbeat, then stops sending heartbeat and refuses further
//      #     communication. Slave's error state can be cleared by holding red
//      #     reset button on its left side for about 4 seconds.
//      #     If you send an error code with bitmap 11110xxx (where x is any bit),
//      #     the error can not be cleared with a 4-second reset.  Instead, you
//      #     must power cycle the TWC or 'reboot' reset which means holding
//      #     reset for about 6 seconds till all the LEDs turn green.
//      #   05 Tell slave charger to limit power to number of amps in bytes 2-3.
//      #
//      # Protocol 2 adds a few more command codes:
//      #   06 Increase charge current by 2 amps.  Slave changes its heartbeat
//      #      state to 06 in response. After 44 seconds, slave state changes to
//      #      0A but amp value doesn't change.  This state seems to be used to
//      #      safely creep up the amp value of a slave when the Master has extra
//      #      power to distribute.  If a slave is attached to a car that doesn't
//      #      want that many amps, Master will see the car isn't accepting the
//      #      amps and stop offering more.  It's possible the 0A state change
//      #      is not time based but rather indicates something like the car is
//      #      now using as many amps as it's going to use.
//      #   07 Lower charge current by 2 amps. Slave changes its heartbeat state
//      #      to 07 in response. After 10 seconds, slave raises its amp setting
//      #      back up by 2A and changes state to 0A.
//      #      I could be wrong, but when a real car doesn't want the higher amp
//      #      value, I think the TWC doesn't raise by 2A after 10 seconds. Real
//      #      Master TWCs seem to send 07 state to all children periodically as
//      #      if to check if they're willing to accept lower amp values. If
//      #      they do, Master assigns those amps to a different slave using the
//      #      06 state.
//      #   08 Master acknowledges that slave stopped charging (I think), but
//      #      the next two bytes contain an amp value the slave could be using.
//      #   09 Tell slave charger to limit power to number of amps in bytes 2-3.
//      #      This command replaces the 05 command in Protocol 1. However, 05
//      #      continues to be used, but only to set an amp value to be used
//      #      before a car starts charging. If 05 is sent after a car is
//      #      already charging, it is ignored.
//      #
//      # Byte 2-3 is the max current a slave TWC can charge at in command codes
//      # 05, 08, and 09. In command code 02, byte 2 is a bitmap. With other
//      # command codes, bytes 2-3 are ignored.
//      # If bytes 2-3 are an amp value of 0F A0, combine them as 0x0fa0 hex
//      # which is 4000 in base 10. Move the decimal point two places left and
//      # you get 40.00Amps max.
//      #
//      # Byte 4: 01 when a Master TWC is physically plugged in to a car.
//      # Otherwise 00.
//      #
//      # Remaining bytes are always 00.
//      #
//      # Example 7-byte data that real masters have sent in Protocol 1:
//      #   00 00 00 00 00 00 00  (Idle)
//      #   02 04 00 00 00 00 00  (Error bitmap 04.  This happened when I
//      #                         advertised a fake Master using an invalid max
//      #                         amp value)
//      #   05 0f a0 00 00 00 00  (Master telling slave to limit power to 0f a0
//      #                         (40.00A))
//      #   05 07 d0 01 00 00 00  (Master plugged in to a car and presumably
//      #                          telling slaves to limit power to 07 d0
//      #                          (20.00A). 01 byte indicates Master is plugged
//      #                          in to a car.)
func (twcMessage *TwcMessage) SendMasterHeartbeat(fromAddress uint16, toAddress uint16, command byte, current uint16, setPoint uint16) {
	copy(twcMessage.bytes, []byte{0xfb, 0xe0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	twcMessage.PutFromAddress(fromAddress)
	twcMessage.PutToAddress(toAddress)
	twcMessage.PutCommand(command)
	twcMessage.PutCurrent(current)
	twcMessage.PutSetPoint(setPoint)
	twcMessage.SendMessage(16)
}
