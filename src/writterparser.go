package main

import (
	"fmt"
	"log"
	"time"

	"github.com/bearsh/hid"
)

type QueryResponse struct {
	// QPIGS
	Grid_voltage           float32
	Grid_freq              float32
	Out_voltage            float32
	Out_freq               float32
	Load_va                int32
	Load_watt              int32
	Load_percent           int32
	Bus_voltage            int32
	Batt_voltage           float32
	Batt_charge_current    int32
	Batt_capacity          int32
	Temp_heatsink          int32
	Pv_input_current       int32
	Pv_input_voltage       float32
	Scc_voltage            float32
	Batt_discharge_current int32
	Device_status          string

	// QPIRI
	Grid_voltage_rating      float32
	Grid_current_rating      float32
	Out_voltage_rating       float32
	Out_freq_rating          float32
	Out_current_rating       float32
	Out_va_rating            int32
	Out_watt_rating          int32
	Batt_rating              float32
	Batt_recharge_voltage    float32
	Batt_under_voltage       float32
	Batt_bulk_voltage        float32
	Batt_float_voltage       float32
	Batt_type                int32
	Max_grid_charge_current  int32
	Max_charge_current       int32
	In_voltage_range         int32
	Out_source_priority      int32
	Charger_source_priority  int32
	Machine_type             int32
	Topology                 int32
	Out_mode                 int32
	Batt_redischarge_voltage float32
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
		timeoutMillisecs := 2000

		// Exit the next for loop a few seconds into the future (if we havent
		// already) since it means something went wrong reading the value
		funcExpireTime := time.Now().Unix() + 10

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
				time.Sleep(75 * time.Millisecond)
			}
		}
		if debug {
			fmt.Printf("% 02x: %d bytes read\n", response, totalBytesRead)
		}

		// strip first open parens byte and last CRC + CR bytes
		startPos := locationAt(response, '(')
		endPos := locationAt(response, 0x0d)
		//fmt.Printf("Startpos: %d    Endpos: %d\n", startPos, endPos)

		if startPos == 0 && endPos > 0 {
			truncatedResponse = string(response[(startPos + 1):(endPos - 2)])
			if debug {
				fmt.Printf("%s\n", truncatedResponse)
			}
		}
	}

	return truncatedResponse, totalBytesRead
}

// Take the response string and seperated them into fields
func responseParser(cmd *string, response *string, qr *QueryResponse) {
	if *cmd == "QPIGS" {
		_, err := fmt.Sscanf(*response, "%f %f %f %f %d %d %d %d %f %d %d %d %d %f %f %d %s",
			&qr.Grid_voltage, &qr.Grid_freq, &qr.Out_voltage, &qr.Out_freq, &qr.Load_va,
			&qr.Load_watt, &qr.Load_percent, &qr.Bus_voltage, &qr.Batt_voltage,
			&qr.Batt_charge_current, &qr.Batt_capacity, &qr.Temp_heatsink, &qr.Pv_input_current,
			&qr.Pv_input_voltage, &qr.Scc_voltage, &qr.Batt_discharge_current, &qr.Device_status)
		if err != nil {
			fmt.Printf("Error parsing QPIGS into struct: %s\n", err)
		}
	} else if *cmd == "QPIRI" {
		_, err := fmt.Sscanf(*response, "%f %f %f %f %f %d %d %f %f %f %f %f %d %d %d %d %d %d - %d %d %d %f",
			&qr.Grid_voltage_rating, &qr.Grid_current_rating, &qr.Out_voltage_rating, &qr.Out_freq_rating,
			&qr.Out_current_rating, &qr.Out_va_rating, &qr.Out_watt_rating, &qr.Batt_rating, &qr.Batt_recharge_voltage,
			&qr.Batt_under_voltage, &qr.Batt_bulk_voltage, &qr.Batt_float_voltage, &qr.Batt_type,
			&qr.Max_grid_charge_current, &qr.Max_charge_current, &qr.In_voltage_range, &qr.Out_source_priority,
			&qr.Charger_source_priority, &qr.Machine_type, &qr.Topology, &qr.Out_mode, &qr.Batt_redischarge_voltage)
		if err != nil {
			fmt.Printf("Error parsing QPIRI into struct: %s\n", err)
		}
	}
}
