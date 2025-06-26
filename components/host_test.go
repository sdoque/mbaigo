package components

import (
	"os"
	"testing"
)

type hostnameTestStruct struct {
	expectedHostname string
	expectedErr      error
}

type ipAddressesTestStruct struct {
	expectedErr error
}

type macAddressesTestStruct struct {
	expectedErr error
}

type newDeviceTestStruct struct {
	expectedDevice *HostingDevice
}

var hostname, _ = os.Hostname()

var hostnameTestParams = []hostnameTestStruct{
	{hostname, nil},
}

var ipAddressesTestParams = []ipAddressesTestStruct{
	{nil},
}

var macAddressesTestParams = []macAddressesTestStruct{
	{nil},
}

var newDeviceTestParams = []newDeviceTestStruct{
	{createTestDevice()},
}

func createTestDevice() *HostingDevice {
	return NewDevice()
}

func TestHostname(t *testing.T) {
	for _, testCase := range hostnameTestParams {
		res, err := Hostname()

		if res != testCase.expectedHostname || err != testCase.expectedErr {
			t.Errorf("Expected %s and %v, got: %s and %v", testCase.expectedHostname, testCase.expectedErr, res, err)
		}
	}
}

func TestIpAddresses(t *testing.T) {
	for _, testCase := range ipAddressesTestParams {
		res, err := IpAddresses()

		if len(res) == 0 || err != testCase.expectedErr {
			t.Errorf("Expected %v, got: %s and %v", testCase.expectedErr, res, err)
		}
	}
}

func TestMacAddresses(t *testing.T) {
	for _, testCase := range macAddressesTestParams {
		ip, err := IpAddresses()
		res, err := MacAddresses(ip)

		if len(res) == 0 || err != testCase.expectedErr {
			t.Errorf("Expected error %v, got: %s and %v", testCase.expectedErr, res, err)
		}
	}
}

func TestNewDevice(t *testing.T) {
	for _, testCase := range newDeviceTestParams {
		res := NewDevice()

		if res.Name != testCase.expectedDevice.Name || len(res.IPAddresses) != len(testCase.expectedDevice.IPAddresses) || len(res.MACAddresses) != len(testCase.expectedDevice.MACAddresses) {
			t.Errorf("Expected %s, %v, %v, got: %s, %v, %v", testCase.expectedDevice.Name, testCase.expectedDevice.IPAddresses, testCase.expectedDevice.MACAddresses, res.Name, res.IPAddresses, res.MACAddresses)
		}
	}
}
