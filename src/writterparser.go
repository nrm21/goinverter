package main

import (
	"fmt"
	"log"
	"time"

	"github.com/bearsh/hid"
)

type QueryResponse struct {
	// QMOD
	Inverter_mode_str string

	// QPIGS
	AC_grid_voltage           float64
	AC_out_voltage            float64
	PV_in_voltage             float64
	PV_in_current             float64
	PV_in_watts               float64
	PV_in_watthour            float64
	SCC_voltage               float64
	Load_pct                  int64
	Load_watts                float64
	Load_watthour             float64
	Load_va                   int64
	Bus_voltage               int64
	Heatsink_temperature      int64
	Battery_capacity          int64
	Battery_voltage           float64
	Battery_charge_current    int64
	Battery_discharge_current int64
	Device_status             string

	// QPIRI
	Battery_recharge_voltage    float64
	Battery_under_voltage       float64
	Battery_bulk_voltage        float64
	Battery_float_voltage       float64
	Max_grid_charge_current     int64
	Max_charge_current          int64
	Out_source_priority         int64
	Charger_source_priority     int64
	Battery_redischarge_voltage float64

	Measurement string
	LastUpdated int64
}

// Find if a byte exists in an array of bytes and return it's location, if it can't be found then return -1
func locationAt(arr []byte, b byte) int {
	for i, v := range arr {
		if v == b {
			return i
		}
	}
	return -1
}

// Calc's the CRC needed to send the commands
func calcCrcData(data []byte) []byte {
	var crc uint16 = 0
	var da uint8 = 0
	crc_ta := [16]uint16{0x0000, 0x1021, 0x2042, 0x3063,
		0x4084, 0x50a5, 0x60c6, 0x70e7, 0x8108, 0x9129,
		0xa14a, 0xb16b, 0xc18c, 0xd1ad, 0xe1ce, 0xf1ef}

	for i := 0; len(data) > i; i++ {
		da = (uint8(crc >> 8)) >> 4
		crc <<= 4
		crc ^= crc_ta[da^(data[i]>>4)]
		da = (uint8(crc >> 8)) >> 4
		crc <<= 4
		crc ^= crc_ta[da^(data[i]&0x0f)]
	}
	bCRCLow := uint8(crc)
	bCRCHign := uint8(crc >> 8)
	if bCRCLow == 0x28 || bCRCLow == 0x0d || bCRCLow == 0x0a {
		bCRCLow++
	}
	if bCRCHign == 0x28 || bCRCHign == 0x0d || bCRCHign == 0x0a {
		bCRCHign++
	}
	crc = uint16(bCRCHign) << 8
	crc += uint16(bCRCLow)

	return []byte{byte(crc >> 8), byte(crc & 0xff), 0x0d} // convert uint16 to byte array and add CR to the end as also
}

// Sends a command to the inverter via USB and returns the truncated
// response with any superfluous info removed from the ends
func writeToInverter(dev *hid.DeviceInfo, cmdToWrite *string) (string, int) {
	var truncatedResponse string
	var totalBytesRead int

	conn, err := dev.Open()
	defer conn.Close()
	if err != nil {
		fmt.Println("Cannot open HID connection, try running as root")
	} else {
		var response []byte

		crc := calcCrcData([]byte(*cmdToWrite))
		cmdSegments1 := []byte(*cmdToWrite)
		cmdSegments2 := []byte(*cmdToWrite)
		if len(*cmdToWrite) <= 5 {
			cmdSegments1 = append(cmdSegments1, crc...) // append CRC + CR to the end
			totalBytesWritten, err := conn.Write(cmdSegments1)
			if err != nil {
				log.Fatal("There was a write problem")
			}
			if debug {
				fmt.Printf("% 02x: %d bytes written, cmd sent: %s\n", cmdSegments1, totalBytesWritten, *cmdToWrite)
			}
		} else { // > 5
			cmdSegments1 = append(cmdSegments1[:5], crc...) // append first 5 bytes
			cmdSegments2 = append(cmdSegments2[5:], crc...) // append remaining bytes
			totalBytesWritten, err := conn.Write(cmdSegments1)
			if debug {
				fmt.Printf("% 02x: %d bytes written, cmd sent: %s\n", cmdSegments1, totalBytesWritten, *cmdToWrite)
			}
			time.Sleep(350 * time.Millisecond)
			totalBytesWritten, err = conn.Write(cmdSegments2)
			if debug {
				fmt.Printf("% 02x: %d bytes written, cmd sent: %s\n", cmdSegments2, totalBytesWritten, *cmdToWrite)
			}

			if err != nil {
				log.Fatal("There was a write problem")
			}
		}

		readBuffer := make([]byte, 8)
		response = []byte{}
		timeoutMillisecs := 500

		// Exit the next for loop (if we havent already) a few seconds into
		// the future since it means something went wrong reading the value
		funcExpireTime := time.Now().Unix() + 7

		for time.Now().Unix() < funcExpireTime {
			bytesRead, err := conn.ReadTimeout(readBuffer, timeoutMillisecs)
			if err != nil {
				fmt.Println("There was a read problem, might be at eof")
			}
			// if debug {
			// 	fmt.Printf("Searching for end of buffer...\n")
			// }

			response = append(response, readBuffer...) // append this 8 bytes to main buffer
			totalBytesRead += bytesRead
			if locationAt(readBuffer, 0x0d) != -1 {
				break // find the 0d byte at the end and break out of loop
			} else {
				// Instantly reading the next buffer seems to sometimes cause issues, perhaps the
				// USB bus cannot keep up, also taking too long between reads seems to confuse as
				// well.  After some trial and error a slight delay between reads seems to work best
				time.Sleep(50 * time.Millisecond)
			}
		}
		if debug {
			fmt.Printf("% 02x: %d bytes read\n", response, totalBytesRead)
		}

		// strip first open parens byte and last CRC + CR bytes
		startPos := locationAt(response, '(')
		endPos := locationAt(response, 0x0d)
		//fmt.Printf("Startpos: %d    Endpos: %d\n", startPos, endPos)

		if startPos < 10 && endPos > 0 {
			truncatedResponse = string(response[(startPos + 1):(endPos - 2)])
			if debug {
				fmt.Printf("%s\n", truncatedResponse)
			}
		}
	}

	return truncatedResponse, totalBytesRead
}

// Take the response string and seperated them into fields
func responseParser(cmd string, response string, qr *QueryResponse) {
	var _ignoreVarI int64   // placeholder for vars we want to throw away
	var _ignoreVarF float64 // placeholder for vars we want to throw away

	if cmd == "QMOD" {
		_, err := fmt.Sscanf(response, "%s", &qr.Inverter_mode_str)
		if err != nil {
			fmt.Printf("Error parsing QMOD into struct: %s\n", err)
		}
	} else if cmd == "QPIGS" {
		_, err := fmt.Sscanf(response, "%f %f %f %f %d %f %d %d %f %d %d %d %f %f %f %d %s",
			&qr.AC_grid_voltage, &_ignoreVarF, &qr.AC_out_voltage, &_ignoreVarF, &qr.Load_va,
			&qr.Load_watts, &qr.Load_pct, &qr.Bus_voltage, &qr.Battery_voltage, &qr.Battery_charge_current,
			&qr.Battery_capacity, &qr.Heatsink_temperature, &qr.PV_in_current, &qr.PV_in_voltage,
			&qr.SCC_voltage, &qr.Battery_discharge_current, &qr.Device_status)
		if err != nil {
			fmt.Printf("Error parsing QPIGS into struct: %s\n", err)
		}
	} else if cmd == "QPIRI" {
		_, err := fmt.Sscanf(response, "%f %f %f %f %f %d %d %f %f %f %f %f %d %d %d %d %d %d - %d %d %d %f",
			&_ignoreVarF, &_ignoreVarF, &_ignoreVarF, &_ignoreVarF, &_ignoreVarF, &_ignoreVarI, &_ignoreVarI,
			&_ignoreVarF, &qr.Battery_recharge_voltage, &qr.Battery_under_voltage, &qr.Battery_bulk_voltage,
			&qr.Battery_float_voltage, &_ignoreVarI, &qr.Max_grid_charge_current, &qr.Max_charge_current,
			&_ignoreVarI, &qr.Out_source_priority, &qr.Charger_source_priority, &_ignoreVarI, &_ignoreVarI,
			&_ignoreVarI, &qr.Battery_redischarge_voltage)
		if err != nil {
			fmt.Printf("Error parsing QPIRI into struct: %s\n", err)
		}
	}
}
