package usecases

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sdoque/mbaigo/components"
)

// signingCA answers an enrollment the way the real certificate authority does:
// it signs the certificate signing request it is given and hands back a
// certificate that prepareClientCertificate can parse. Without that the real
// path cannot be exercised at all — installTLSConfig calls log.Fatalf on a
// certificate it cannot load, which would take the test binary with it.
type signingCA struct {
	key  *ecdsa.PrivateKey
	cert *x509.Certificate
	pem  string
}

func newSigningCA(t *testing.T) *signingCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "synecdoque.com", Organization: []string{"Synecdoque"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CA cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}
	return &signingCA{
		key:  key,
		cert: cert,
		pem:  string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
	}
}

// sign issues a leaf certificate for one common name, usable by either end of a
// TLS connection.
func (ca *signingCA) sign(t *testing.T, cn string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost", "example.com"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatalf("signing %s: %v", cn, err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func (ca *signingCA) RoundTrip(req *http.Request) (*http.Response, error) {
	reply := func(body string) (*http.Response, error) {
		return &http.Response{
			Status: "200 OK", StatusCode: 200,
			Header:  http.Header{"Content-Type": []string{"application/x-pem-file"}},
			Body:    io.NopCloser(strings.NewReader(body)),
			Request: req,
		}, nil
	}

	if strings.HasSuffix(req.URL.Path, "/certify") {
		raw, _ := io.ReadAll(req.Body)
		block, _ := pem.Decode(raw)
		if block == nil {
			return nil, io.ErrUnexpectedEOF
		}
		csr, err := x509.ParseCertificateRequest(block.Bytes)
		if err != nil {
			return nil, err
		}
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano()),
			Subject:      csr.Subject,
			NotBefore:    time.Now().Add(-time.Minute),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
			DNSNames:     csr.DNSNames,
			IPAddresses:  csr.IPAddresses,
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, csr.PublicKey, ca.key)
		if err != nil {
			return nil, err
		}
		return reply(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))
	}

	// Anything else is getCACertificate asking for the root.
	return reply(ca.pem)
}

// TestEnrollmentAndPostureAreRaceFree is follow-up finding N3.
//
// The fix for the earlier CA_cert finding put sys.Mutex around the *reads* of
// Husk.Certificate and Husk.CA_cert and left the writers taking nothing, so the
// mutex synchronized with nothing at all — and the doc comment on Posture
// promised a guarantee the code did not provide.
//
// The window is not theoretical: SetoutServers binds the HTTP server before
// enrollment finishes, so any GET /<system>/kgraph in that window reaches
// modelSecurity, then Posture, and reads a string that acquireCertificate is
// concurrently writing.
func TestEnrollmentAndPostureAreRaceFree(t *testing.T) {
	ca := newSigningCA(t)
	useTransport(t, ca)

	sys := components.NewSystem("thermostat", context.Background())
	sys.Husk = &components.Husk{
		ProtoPort: map[string]int{"http": 20101, "https": 30101},
		Host:      components.NewDevice(),
		DName:     pkix.Name{CommonName: "thermostat", Organization: []string{"Synecdoque"}},
		CoreS: []*components.CoreSystem{{
			Name: "ca", Url: "http://localhost:20100/ca/certification",
		}},
	}

	certReady := EnsureCertReady(&sys)

	// The enrollment goroutine, exactly as RequestCertificate starts it.
	go acquireCertificate(&sys, certReady)

	// The registration loop is also running by now — RegisterServices starts
	// before SetoutServers — and every one of its calls reads http.DefaultClient,
	// which installTLSConfig replaces.
	var outbound sync.WaitGroup
	outbound.Add(1)
	go func() {
		defer outbound.Done()
		for j := 0; j < 200; j++ {
			req, err := http.NewRequest(http.MethodGet, "http://localhost:20100/ca/cert", nil)
			if err != nil {
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = resp.Body.Close()
			}
		}
	}()

	// What a caller can do in the meantime: the HTTP server is already bound, so
	// /kgraph is answerable while the certificate is still arriving.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				w := httptest.NewRecorder()
				KGraphing(w, httptest.NewRequest(http.MethodGet, "/thermostat/kgraph", nil), &sys)
				if w.Body.Len() == 0 {
					t.Error("the knowledge graph came back empty")
					return
				}
			}
		}()
	}
	wg.Wait()
	outbound.Wait()

	select {
	case <-certReady:
	case <-time.After(10 * time.Second):
		t.Fatal("enrollment never completed against the mocked authority")
	}

	if p := Posture(&sys); !p.Identified {
		t.Errorf("the system enrolled but does not report itself identified: %+v", p)
	}
	_ = bytes.MinRead // keep the import honest if the body check above changes
}
