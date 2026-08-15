package usecases

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestTheFrameworkClientVerifiesAPeerSignedByTheCA checks the path installed at
// package load: the HTTP client is fixed and the TLS configuration is published
// when enrollment completes, so an outbound HTTPS call has to verify a peer
// through the configuration it finds rather than through a client that was
// replaced under it.
//
// Written because a live cloud reported "certificate signed by unknown
// authority" on every system-to-system fetch, and the client path had just been
// rebuilt. This settles whether the framework or the deployment is at fault.
func TestTheFrameworkClientVerifiesAPeerSignedByTheCA(t *testing.T) {
	ca := newSigningCA(t)

	// A server holding a certificate this CA signed, asking for one in return —
	// the same shape startHTTPSServer builds.
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "reached")
	}))
	serverCert := ca.sign(t, "provider")
	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
		MinVersion:   tls.VersionTLS12,
	}
	server.StartTLS()
	defer server.Close()

	// What installTLSConfig publishes when enrollment completes.
	clientCert := ca.sign(t, "consumer")
	previous := clientTLS.Load()
	clientTLS.Store(&tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS12,
	})
	defer clientTLS.Store(previous)

	// Exactly what a system does: borrow the framework's transport.
	client := &http.Client{Transport: http.DefaultClient.Transport}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("the framework client could not reach a peer signed by its own CA: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "reached" {
		t.Errorf("got %q, want %q", body, "reached")
	}
}

// Before enrollment there is no configuration to dial with, and the failure has
// to say so rather than surface as a certificate error.
func TestTheFrameworkClientSaysWhenItHasNotEnrolled(t *testing.T) {
	previous := clientTLS.Load()
	clientTLS.Store(nil)
	defer clientTLS.Store(previous)

	client := &http.Client{Transport: http.DefaultClient.Transport}
	_, err := client.Get("https://192.0.2.1:30100/ca/kgraph")
	if err == nil {
		t.Fatal("an unenrolled system reached a TLS peer")
	}
	if !contains(err.Error(), "has not enrolled yet") {
		t.Errorf("the failure reads %q, which does not say the system is unenrolled", err)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestTheFrameworkClientDialsThroughTheTLSHook is the guard on the two mistakes
// that are easy to make here and invisible when made.
//
// There were two inits assigning http.DefaultClient — this package's files run
// in name order, so utilities.go quietly replaced the client authentication.go
// had installed. What was left had a nil transport, which means Go's default:
// no CA pool, no client certificate. Every call between systems then failed
// with "certificate signed by unknown authority", and nothing in the build, the
// vet or the rest of the suite noticed, because a client with no transport is
// perfectly valid — it just cannot speak the cloud's TLS.
//
// So this asserts the shape rather than the behavior: one transport, with the
// dial hook on it. Add a second assignment anywhere in the package and this
// fails, wherever in the file order it lands.
func TestTheFrameworkClientDialsThroughTheTLSHook(t *testing.T) {
	transport, ok := http.DefaultClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("the framework client's transport is %T, so outbound TLS uses Go's "+
			"defaults: no CA pool and no client certificate", http.DefaultClient.Transport)
	}
	if transport.DialTLSContext == nil {
		t.Fatal("the framework client has no TLS dial, so it cannot present this " +
			"system's certificate or verify a peer against the cloud's CA")
	}
}
