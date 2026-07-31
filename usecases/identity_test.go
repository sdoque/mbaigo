package usecases

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"log"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

// tlsStateWithCN builds a connection state carrying a single peer certificate
// with the given common name, as the HTTPS server would present after a
// successful mutually authenticated handshake.
func tlsStateWithCN(cn string) *tls.ConnectionState {
	return &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{
			{Subject: pkix.Name{CommonName: cn}},
		},
	}
}

func TestPeerCN(t *testing.T) {
	tests := []struct {
		name   string
		state  *tls.ConnectionState
		wantCN string
		wantOk bool
	}{
		{"plain HTTP has no identity", nil, "", false},
		{"TLS without a client certificate", &tls.ConnectionState{}, "", false},
		{"mTLS yields the common name", tlsStateWithCN("thermostat"), "thermostat", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/thermostat/controller_1/setpoint", nil)
			r.TLS = tc.state

			cn, ok := PeerCN(r)
			if ok != tc.wantOk {
				t.Errorf("PeerCN ok = %v; want %v", ok, tc.wantOk)
			}
			if cn != tc.wantCN {
				t.Errorf("PeerCN cn = %q; want %q", cn, tc.wantCN)
			}
		})
	}
}

func TestLogPeerReportsEachIdentifiedPeerOnce(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	sys := &components.System{Name: "ds18b20"}
	r := httptest.NewRequest("GET", "/ds18b20/sensor_Id/temperature", nil)
	r.TLS = tlsStateWithCN("collector")

	logPeer(sys, r)
	logPeer(sys, r)
	logPeer(sys, r)

	if got := strings.Count(buf.String(), "collector"); got != 1 {
		t.Errorf("peer reported %d times; want 1\n%s", got, buf.String())
	}

	// A different peer on the same system is a separate sighting.
	r.TLS = tlsStateWithCN("thermostat")
	logPeer(sys, r)

	if got := strings.Count(buf.String(), "thermostat"); got != 1 {
		t.Errorf("second peer reported %d times; want 1\n%s", got, buf.String())
	}
}

func TestLogPeerReportsUnidentifiedPeers(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	sys := &components.System{Name: "orchestrator"}
	r := httptest.NewRequest("POST", "/orchestrator/orchestration/squest", nil)

	logPeer(sys, r)

	if !strings.Contains(buf.String(), "unidentified") {
		t.Errorf("plain-HTTP caller not reported as unidentified\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "plain HTTP") {
		t.Errorf("transport not reported\n%s", buf.String())
	}
}

// Unidentified traffic must keep being reported: one line at startup could not be
// told apart from a deployment that had since moved to mTLS.
func TestLogPeerRepeatsUnidentifiedAfterInterval(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	sys := &components.System{Name: "esr"}
	r := httptest.NewRequest("POST", "/esr/registry/register", nil)

	// Within the interval, only the first call reports.
	logPeer(sys, r)
	logPeer(sys, r)
	if got := strings.Count(buf.String(), "unidentified"); got != 1 {
		t.Fatalf("reported %d times within the interval; want 1\n%s", got, buf.String())
	}

	// Once it has elapsed, the system says so again.
	saved := unidentifiedReportInterval
	unidentifiedReportInterval = 0
	defer func() { unidentifiedReportInterval = saved }()

	logPeer(sys, r)
	if got := strings.Count(buf.String(), "unidentified"); got != 2 {
		t.Errorf("reported %d times after the interval elapsed; want 2\n%s", got, buf.String())
	}
}

// An identified peer must not be silenced by unidentified traffic to the same
// system, nor the reverse: the two paths keep separate state.
func TestLogPeerKeepsIdentifiedAndUnidentifiedApart(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	sys := &components.System{Name: "parallax"}

	plain := httptest.NewRequest("PUT", "/parallax/Servo_1/position", nil)
	logPeer(sys, plain)

	secure := httptest.NewRequest("PUT", "/parallax/Servo_1/position", nil)
	secure.TLS = tlsStateWithCN("thermostat-kitchen")
	logPeer(sys, secure)

	out := buf.String()
	if !strings.Contains(out, "unidentified") {
		t.Errorf("unidentified caller not reported\n%s", out)
	}
	if !strings.Contains(out, "thermostat-kitchen") {
		t.Errorf("identified caller not reported\n%s", out)
	}
}
