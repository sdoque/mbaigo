package usecases

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestAnUnenrolledSystemStillReachesThePublicInternet is the case that broke a
// live cloud twice over.
//
// Systems talk to more than each other. A weather station's API, an electricity
// spot price, a message broker — all public hosts, all over TLS, and often at
// startup, before the CA has issued anything. The dial used to refuse those
// outright with "this system has not enrolled yet", which is both a failure they
// should not have had and an explanation that does not fit the call being made.
//
// Verified against a host the test's own CA did not sign, so reaching it at all
// means the host's trusted authorities were consulted rather than the cloud's.
func TestAnUnenrolledSystemStillReachesThePublicInternet(t *testing.T) {
	previous := clientTLS.Load()
	clientTLS.Store(nil)
	defer clientTLS.Store(previous)

	stranger := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("reached")) }))
	defer stranger.Close()

	client := &http.Client{Transport: http.DefaultClient.Transport}
	_, err := client.Get(stranger.URL)
	if err == nil {
		t.Fatal("a server signed by nobody this host trusts was accepted")
	}
	// The point is which failure. Ordinary verification means the dial went
	// through to a handshake; an enrollment message means it never tried.
	if !strings.Contains(err.Error(), "certificate") {
		t.Errorf("the failure reads %q, which is not a verification failure — the "+
			"dial refused the call rather than attempting it", err)
	}
	if strings.Contains(err.Error(), "not enrolled") {
		t.Errorf("a call to a public host was refused for want of enrollment: %v", err)
	}
}

// TestTheCloudsCAIsAddedToWhatTheHostTrusts covers the other half of the same
// failure, and one that predates the dial.
//
// The pool was built from the cloud's CA alone, so from enrollment onward every
// public host failed with "certificate signed by unknown authority" — a system
// that worked for the first few minutes and then stopped. Peers inside the cloud
// are verified by the same CA whether or not the host's own authorities are in
// the pool beside it.
func TestTheCloudsCAIsAddedToWhatTheHostTrusts(t *testing.T) {
	ca := newSigningCA(t)

	pool, err := trustPool(ca.pem)
	if err != nil {
		t.Fatalf("the cloud's CA could not be trusted: %v", err)
	}

	host, err := x509.SystemCertPool()
	if err != nil {
		t.Skip("this host exposes no trusted certificates, so there is nothing to add to")
	}
	if pool.Equal(host) {
		t.Error("the cloud's CA is not in the pool, so no peer in the cloud can be verified")
	}
	if !host.AppendCertsFromPEM([]byte(ca.pem)) {
		t.Fatal("the test CA is not readable as PEM")
	}
	if !pool.Equal(host) {
		t.Error("the pool is not the host's authorities plus the cloud's CA, so a " +
			"system reaching a public API verifies against the wrong set")
	}
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
