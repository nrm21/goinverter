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
var debug bool

const httpIp = "" // leave blank for 0.0.0.0, else use a specific interface or localhost for more security
const httpPort = "8088"

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

// Handle http to run raw commands
func handleHttpRaw(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if strings.Contains(r.URL.Path, "query") {
		web2usbPipe <- "QPIGS"
		fmt.Fprintf(w, "%s\n", PrettyString(<-web2usbPipe))

		web2usbPipe <- "QPIRI"
		fmt.Fprintf(w, "%s\n", PrettyString(<-web2usbPipe))
	} else if strings.Contains(r.URL.Path, "raw") {
		query := r.URL.Query()
		web2usbPipe <- query.Get("cmd")

		// get parsed json struct back so we can send to web as response
		fmt.Fprintf(w, "%s\n", PrettyString(<-web2usbPipe))
	}
}

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
					fmt.Printf("Less than 104 byte response... resending command to inverter\n")
					time.Sleep(2500 * time.Millisecond)
				}
			}
			qr := &QueryResponse{}
			responseParser(&cmd, &response, qr)

			jsonStruct, err := json.Marshal(qr)
			if err != nil {
				fmt.Println("There was a problem creating the JSON")
			}

			response = string(jsonStruct)
		} else { // else this isn't a query that returns a long response
			response, _ = writeToInverter(&inverterDev, &cmd)
			response = fmt.Sprintf("{\"msg\": \"%s\"}", response) // JSON-ify
		}

		//fmt.Println(string(jsonStruct))  // troubleshooting
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
