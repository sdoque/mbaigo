package components

import (
	"testing"
)

type sProtocolsTestStruct struct {
	input          map[string]int
	expectedOutput []string
}

var sProtocolsTestParams = []sProtocolsTestStruct{
	{makeEmptyProtoPortMap(), nil},
	{makeProtoPortMapWithPortZero(), nil},
	{makeFullProtoPortMap(), []string{"Port1", "Port2"}},
}

func makeEmptyProtoPortMap() map[string]int {
	return make(map[string]int)
}

func makeProtoPortMapWithPortZero() map[string]int {
	return map[string]int{"Port": 0}
}

func makeFullProtoPortMap() map[string]int {
	return map[string]int{"Port1": 123, "Port2": 404, "Port3": 0}
}

func TestSProtocols(t *testing.T) {
	for _, testCase := range sProtocolsTestParams {
		res := SProtocols(testCase.input)

		if len(res) != len(testCase.expectedOutput) {
			t.Errorf("Expected %v, got: %v", testCase.expectedOutput, res)
			continue
		}
		// Verify every expected protocol appears in the result (order is non-deterministic)
		resSet := make(map[string]bool, len(res))
		for _, p := range res {
			resSet[p] = true
		}
		for _, expected := range testCase.expectedOutput {
			if !resSet[expected] {
				t.Errorf("Expected protocol %q in result %v", expected, res)
			}
		}
	}
}
