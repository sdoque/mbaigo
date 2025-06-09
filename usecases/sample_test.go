package usecases

import (
	"testing"
)

/*
// mockTransport is used for replacing the default network Transport (used by
// http.DefaultClient) and it will intercept network requests.

	type mockTransport struct {
		resp *http.Response
	}

	func newMockTransport(resp *http.Response) mockTransport {
		t := mockTransport{
			resp: resp,
		}
		// Hijack the default http client so no actual http requests are sent over the network
		http.DefaultClient.Transport = t
		return t
	}

// RoundTrip method is required to fulfil the RoundTripper interface (as required by the DefaultClient).
// It prevents the request from being sent over the network and count how many times
// a domain was requested.

	func (t mockTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
		t.resp.Request = req
		return t.resp, nil
	}
*/
func TestSample(t *testing.T) {
	return
}
