package components

import (
	"testing"
)

func TestHostname(t *testing.T) {
	res, err := Hostname()

	if res == "" || err != nil {
		t.Errorf("Expected a host name and no error, got: %s and %v", res, err)
	}
}

func TestIpAddresses(t *testing.T) {
	res, err := IpAddresses()

	if len(res) == 0 || err != nil {
		t.Errorf("Expected IP addresses and no error, got: %s and %v", res, err)
	}
}

func TestMacAddresses(t *testing.T) {
	ip, err := IpAddresses()
	if err != nil {
		t.Fatalf("An error occurred in getting IP Addresses for the Mac Address test")
	}
	res, err := MacAddresses(ip)

	if len(res) == 0 || err != nil {
		t.Errorf("Expected no error, got: %s and %v", res, err)
	}
}

func TestNewDevice(t *testing.T) {
	res := NewDevice()

	if res == nil {
		t.Errorf("Expected a new device, got: %v", res)
	}
}
