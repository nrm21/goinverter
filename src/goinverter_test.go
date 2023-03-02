package main

import "testing"

func TestCheckArgs(t *testing.T) {
	CheckArgs([]string{"-i", "5", "-d", "-p", "8080"})
	// CheckArgs([]string{"-i", "-d", "5"})

	if debug != true {
		t.Fatalf("Got: %t, Expected true", debug)
	}
	if usbUpdateInterval != 5 {
		t.Fatalf("Got: %d, expected 5", usbUpdateInterval)
	}
	if httpPort != "8080" {
		t.Fatalf("Got: %s, expected 8080", httpPort)
	}
}
