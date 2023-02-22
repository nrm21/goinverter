package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bearsh/hid"
)

var web2usbPipe chan string
var usbResponse string
var debug bool
var lastUsbUpdate int64

const httpIp = "" // leave blank for 0.0.0.0, else use a specific interface or localhost for more security
const httpPort = "8088"
const usbUpdateInterval int64 = 15

// Make it possible to kill program by typing ctrl-C
func SetupCloseHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nCtrl+C pressed in Terminal\n")
		os.Exit(0)
	}()
}

// Checks if the debug flag is set
func checkDebugFlag() bool {
	if len(os.Args) > 1 { // if we have arguments parse them
		if os.Args[1] == "-d" {
			fmt.Println("Debug flag set")
			return true
		}
	}
	return false
}

// Pretty print JSON data
func PrettyString(str string) string {
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, []byte(str), "", "    "); err != nil {
		return ""
	}
	return prettyJSON.String()
}

// Handle http to print status
func handleHttpStatus(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Client connected from %s with URI %s\n", strings.Split(r.RemoteAddr, ":")[0], r.URL.Path)
	fmt.Fprintf(w, "status: \"ok\"\n")
}

// Sends QPIGS then QPIRI to the USB and returns the result of both
func doStatusUpdate() string {
	qr := &QueryResponse{}

	cmd := "QPIGS"
	web2usbPipe <- cmd
	usbResponse = <-web2usbPipe
	responseParser(cmd, &usbResponse, qr)

	cmd = "QPIRI"
	web2usbPipe <- cmd
	usbResponse = <-web2usbPipe
	responseParser(cmd, &usbResponse, qr)

	// turn response into JSON and stringify it
	jsonStruct, _ := json.Marshal(qr)

	return string(jsonStruct)
}

// Handle http to run raw commands
func handleHttpRaw(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if strings.Contains(r.URL.Path, "query") {
		if lastUsbUpdate+usbUpdateInterval < time.Now().Unix() {
			usbResponse = doStatusUpdate()
			//fmt.Fprintf(w, "%s\n", PrettyString(response)) // pretty print for webpage display
			fmt.Fprintf(w, "%s\n", usbResponse)

			lastUsbUpdate = time.Now().Unix()
		} else { // we are querying again before the update interval expires, just return the previous data
			if debug {
				fmt.Println("Update interval not yet expired... resending previous data")
			}
			fmt.Fprintf(w, "%s\n", usbResponse)
		}
	} else if strings.Contains(r.URL.Path, "raw") {
		// run command and send response to web
		query := r.URL.Query()
		web2usbPipe <- query.Get("cmd")
		fmt.Fprintf(w, "%s\n", <-web2usbPipe) // and send response to console also
	}
}

// runs in a loop handling any misreads and doing them over again as needed
func handleUSBTraffic() {
	var devices []hid.DeviceInfo
	devices = hid.Enumerate(0, 0)
	inverterDev := devices[0] // the first device will usually be the one we want

	// loop forever until user breaks since we are web serving
	// and waiting for user response over and over again
	for {
		cmd := <-web2usbPipe

		var response string
		if cmd == "QPIRI" || cmd == "QPIGS" {
			var bytesRead int
			for bytesRead < 104 {
				response, bytesRead = writeToInverter(&inverterDev, &cmd)
				if debug && bytesRead < 104 {
					fmt.Println("Less than 104 byte response... resending command to inverter")
					time.Sleep(2500 * time.Millisecond)
				}
			}
		} else { // else this isn't a query that returns a long response
			response, _ = writeToInverter(&inverterDev, &cmd)
			//response = fmt.Sprintf("{\"msg\": \"%s\"}", response) // JSON-ify
		}

		web2usbPipe <- response
	}
}

func main() {
	SetupCloseHandler()
	debug = checkDebugFlag()

	// initiate our globally declared channels here
	web2usbPipe = make(chan string)

	http.HandleFunc("/status", handleHttpStatus) // health check (for k8s readiness probes)
	http.HandleFunc("/health", handleHttpStatus) // health check (for k8s readiness probes)
	http.HandleFunc("/query", handleHttpRaw)
	http.HandleFunc("/raw", handleHttpRaw)

	go handleUSBTraffic()

	fmt.Println("Server started at port " + httpPort)
	http.ListenAndServe(httpIp+":"+httpPort, nil)
}
