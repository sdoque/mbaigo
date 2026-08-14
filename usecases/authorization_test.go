package usecases

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sdoque/mbaigo/components"
	"github.com/sdoque/mbaigo/forms"
)

// certificateAuthority issues certificates the way the cloud's CA does, so the
// validation path under test is the real one rather than a stub.
type certificateAuthority struct {
	key  *ecdsa.PrivateKey
	cert *x509.Certificate
	pem  string
}

func newCA(t *testing.T) certificateAuthority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("CA key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CA certificate: %v", err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing the CA certificate: %v", err)
	}
	return certificateAuthority{
		key:  key,
		cert: parsed,
		pem:  string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
	}
}

// issue signs a system certificate, as the CA does after the maitreD attests it.
func (ca certificateAuthority) issue(t *testing.T, commonName string) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("system key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatalf("issuing to %s: %v", commonName, err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), key
}

func TestAuthorizerPublicKeyAcceptsTheAuthorizersCertificate(t *testing.T) {
	ca := newCA(t)
	certPEM, key := ca.issue(t, AuthorizerName)

	got, err := authorizerPublicKey([]byte(certPEM), ca.pem)
	if err != nil {
		t.Fatalf("authorizerPublicKey: %v", err)
	}
	if !got.Equal(&key.PublicKey) {
		t.Error("the extracted key is not the authorizer's")
	}
}

// The trap this pinning exists to close: every enrolled system holds a valid
// CA-signed certificate, so without a Common Name check any of them could stand
// in for the authorizer and mint its own tokens.
func TestAuthorizerPublicKeyRejectsAnotherSystemsCertificate(t *testing.T) {
	ca := newCA(t)
	certPEM, _ := ca.issue(t, "thermostat")

	_, err := authorizerPublicKey([]byte(certPEM), ca.pem)
	if err == nil {
		t.Fatal("another system's certificate was accepted as the authorizer's")
	}
	if !strings.Contains(err.Error(), "thermostat") {
		t.Errorf("error %q does not name the impostor", err)
	}
}

// Chain validation is what makes fetching the certificate over plain HTTP safe.
// If it did not hold, the whole transport argument collapses.
func TestAuthorizerPublicKeyRejectsAnotherCloudsCA(t *testing.T) {
	ours, theirs := newCA(t), newCA(t)
	certPEM, _ := theirs.issue(t, AuthorizerName)

	if _, err := authorizerPublicKey([]byte(certPEM), ours.pem); err == nil {
		t.Fatal("a certificate from another cloud's CA was accepted")
	}
}

func TestAuthorizerPublicKeyRejectsUnusableInput(t *testing.T) {
	ca := newCA(t)
	certPEM, _ := ca.issue(t, AuthorizerName)

	tests := []struct {
		name    string
		cert    string
		caPEM   string
		wantErr string
	}{
		{"not PEM at all", "hello", ca.pem, "not PEM"},
		{"no CA to check against", certPEM, "", "no CA certificate"},
		{"an unparsable CA", certPEM, "-----BEGIN CERTIFICATE-----\nnope\n-----END CERTIFICATE-----", "could not be parsed"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := authorizerPublicKey([]byte(tc.cert), tc.caPEM)
			if err == nil {
				t.Fatal("unusable input was accepted")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not describe the problem (%q)", err, tc.wantErr)
			}
		})
	}
}

// The configured core-system URL addresses a unit asset; the certificate is
// served by the system. Getting this wrong would make every provider fetch a
// 404 and never verify anything.
func TestSystemRootURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"the configured form", "http://localhost:20104/authorizer/authorization", "http://localhost:20104/authorizer/cert", false},
		{"over https", "https://10.0.0.5:20104/authorizer/authorization", "https://10.0.0.5:20104/authorizer/cert", false},
		{"already at the system root", "http://localhost:20104/authorizer", "http://localhost:20104/authorizer/cert", false},
		{"a URL for another system", "http://localhost:20103/orchestrator/orchestration", "", true},
		{"no host", "/authorizer/authorization", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := systemRootURL(tc.in, AuthorizerName)
			if tc.wantErr != (err != nil) {
				t.Fatalf("systemRootURL(%q) error = %v; wantErr %v", tc.in, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("systemRootURL(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A cloud that has not adopted authorization keeps serving: a system that
// predates the authorizer must not stop working because the framework grew one.
func TestAuthorizeRequestIsInertWithoutAnAuthorizer(t *testing.T) {
	sys := systemUnderTest(t, nil)
	r := httptest.NewRequest("GET", "/ds18b20/sensor_Id/temperature", nil)

	status, err := AuthorizeRequest(sys, r, "sensor_Id", &components.Service{Definition: "temperature"})
	if status != 0 || err != nil {
		t.Errorf("status = %d, err = %v; want the request to proceed", status, err)
	}
}

// Once an authorizer is declared, a provider that cannot yet verify refuses.
// "Not ready" is not "allowed".
func TestAuthorizeRequestRefusesBeforeTheKeyArrives(t *testing.T) {
	sys := systemUnderTest(t, &components.CoreSystem{
		Name: AuthorizerName, Url: "http://localhost:20104/authorizer/authorization",
	})
	r := httptest.NewRequest("GET", "/ds18b20/sensor_Id/temperature", nil)

	status, err := AuthorizeRequest(sys, r, "sensor_Id", &components.Service{Definition: "temperature"})
	if status != http.StatusServiceUnavailable {
		t.Errorf("status = %d; want %d while the key is missing", status, http.StatusServiceUnavailable)
	}
	if err == nil || !strings.Contains(err.Error(), "verify") {
		t.Errorf("error %v does not explain that verification is not possible yet", err)
	}
}

// With a key in place, an unidentified caller and a caller with no token are
// both refused — and distinguishably so.
func TestAuthorizeRequestRequiresIdentityAndToken(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	sys := systemUnderTest(t, &components.CoreSystem{
		Name: AuthorizerName, Url: "http://localhost:20104/authorizer/authorization",
	})
	sys.Husk.AuthorizerKey.Store(&key.PublicKey)

	serv := &components.Service{Definition: "temperature"}

	anonymous := httptest.NewRequest("GET", "/ds18b20/sensor_Id/temperature", nil)
	if status, _ := AuthorizeRequest(sys, anonymous, "sensor_Id", serv); status != http.StatusUnauthorized {
		t.Errorf("an unidentified caller got %d; want %d", status, http.StatusUnauthorized)
	}

	identified := httptest.NewRequest("GET", "/ds18b20/sensor_Id/temperature", nil)
	identified.TLS = tlsStateWithCN("thermostat")
	status, err := AuthorizeRequest(sys, identified, "sensor_Id", serv)
	if status != http.StatusUnauthorized {
		t.Errorf("a caller with no token got %d; want %d", status, http.StatusUnauthorized)
	}
	if err == nil || !strings.Contains(err.Error(), "no access token") {
		t.Errorf("error %v does not say the token is missing", err)
	}
}

// The whole point: a token for this exact call is served, and one for anything
// else is refused.
func TestAuthorizeRequestAcceptsOnlyTheMatchingToken(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	sys := systemUnderTest(t, &components.CoreSystem{
		Name: AuthorizerName, Url: "http://localhost:20104/authorizer/authorization",
	})
	sys.Husk.AuthorizerKey.Store(&key.PublicKey)
	serv := &components.Service{Definition: "temperature"}

	now := time.Now()
	good, err := MintToken(key, forms.AccessToken_v1{
		Subject: "thermostat", Provider: "ds18b20", Asset: "sensor_Id",
		Service: "temperature", Action: "read",
		IssuedAt: now, Expires: now.Add(5 * time.Minute), Issuer: "authorizer",
	})
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}

	request := func(method, token string) *http.Request {
		r := httptest.NewRequest(method, "/ds18b20/sensor_Id/temperature", nil)
		r.TLS = tlsStateWithCN("thermostat")
		r.Header.Set(TokenHeader, token)
		return r
	}

	if status, err := AuthorizeRequest(sys, request("GET", good), "sensor_Id", serv); status != 0 {
		t.Errorf("the matching token was refused with %d: %v", status, err)
	}

	// The same token used to write rather than read: a read permission must not
	// become a write one by changing the verb.
	if status, _ := AuthorizeRequest(sys, request("PUT", good), "sensor_Id", serv); status != http.StatusForbidden {
		t.Errorf("a read token authorized a write (status %d)", status)
	}

	// The same token presented for another asset on the same provider.
	if status, _ := AuthorizeRequest(sys, request("GET", good), "sensor_2", serv); status != http.StatusForbidden {
		t.Errorf("a token for one asset authorized another (status %d)", status)
	}
}

func TestActionForMethod(t *testing.T) {
	cases := map[string]string{
		http.MethodGet: "read", http.MethodHead: "read",
		http.MethodPut: "write", http.MethodPatch: "write",
		http.MethodPost: "invoke",
		// A method nobody classified is one nobody authorized: the empty action
		// matches no claim.
		http.MethodDelete: "", http.MethodOptions: "",
	}
	for method, want := range cases {
		if got := ActionForMethod(method); got != want {
			t.Errorf("ActionForMethod(%s) = %q; want %q", method, got, want)
		}
	}
}

// systemUnderTest builds a provider, optionally in a cloud that declares an
// authorizer.
func systemUnderTest(t *testing.T, authorizer *components.CoreSystem) *components.System {
	t.Helper()
	sys := components.NewSystem("ds18b20", context.Background())
	sys.Husk = &components.Husk{}
	if authorizer != nil {
		sys.Husk.CoreS = []*components.CoreSystem{authorizer}
	}
	return &sys
}

// TestAFileRequestCanBeAuthorized is the regression the previous fix
// introduced, and it goes through handleFiveParts rather than calling
// AuthorizeRequest directly — the defect was never in the check, it was in what
// the file path handed to it.
//
// No unit asset registers a service whose subpath is "files": photographer,
// recognizer and kgrapher handle the word inside their own dispatch. So
// findServiceByPath returned nil, an empty Service was substituted, and
// mismatch requires claimed == actual && claimed != "". Nothing can satisfy
// that, so every file request in an authorized cloud was refused for good — a
// working feature disabled by the check meant to protect it.
func TestAFileRequestCanBeAuthorized(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	sys := createTestSystem(false)
	sys.Husk.CoreS = append(sys.Husk.CoreS, &components.CoreSystem{
		Name: AuthorizerName, Url: "http://localhost:20104/authorizer/authorization",
	})
	sys.Husk.AuthorizerKey.Store(&key.PublicKey)

	now := time.Now()
	token, err := MintToken(key, forms.AccessToken_v1{
		Subject: "clerk", Provider: sys.Name, Asset: "testUnitAsset",
		Service: FileService, Action: "read",
		IssuedAt: now, Expires: now.Add(5 * time.Minute), Issuer: "authorizer",
	})
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}

	fileRequest := func(tok string) *http.Request {
		r := httptest.NewRequest("GET",
			"/"+sys.Name+"/testUnitAsset/files/image_20240325-211555.jpg", nil)
		r.TLS = tlsStateWithCN("clerk")
		r.Header.Set(TokenHeader, tok)
		return r
	}

	// With a token naming the file service, the guard lets it through. The
	// transfer then fails on a file that is not there, which is a 404 or a 500
	// from TransferFile — anything but the 403 the guard used to return
	// unconditionally.
	w := httptest.NewRecorder()
	ResourceHandler(&sys, w, fileRequest(token))
	if w.Code == http.StatusForbidden {
		t.Errorf("a file request carrying a token for it was refused: %s",
			strings.TrimSpace(w.Body.String()))
	}

	// The guard still guards: a token for a different service does not open the
	// files.
	other, err := MintToken(key, forms.AccessToken_v1{
		Subject: "clerk", Provider: sys.Name, Asset: "testUnitAsset",
		Service: "testServ", Action: "read",
		IssuedAt: now, Expires: now.Add(5 * time.Minute), Issuer: "authorizer",
	})
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}
	w = httptest.NewRecorder()
	ResourceHandler(&sys, w, fileRequest(other))
	if w.Code != http.StatusForbidden {
		t.Errorf("a token for another service opened the files (status %d)", w.Code)
	}

	// And no token at all is still refused.
	r := httptest.NewRequest("GET", "/"+sys.Name+"/testUnitAsset/files/image.jpg", nil)
	r.TLS = tlsStateWithCN("clerk")
	w = httptest.NewRecorder()
	ResourceHandler(&sys, w, r)
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Errorf("an untokened file request was served (status %d)", w.Code)
	}
}

// An empty service definition can satisfy no token, so passing one to
// AuthorizeRequest is always a refusal — which is why permitted must not
// manufacture one when it cannot resolve the path.
func TestAnUnnamedServiceCanNeverBeAuthorized(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	sys := systemUnderTest(t, &components.CoreSystem{
		Name: AuthorizerName, Url: "http://localhost:20104/authorizer/authorization",
	})
	sys.Husk.AuthorizerKey.Store(&key.PublicKey)

	now := time.Now()
	token, err := MintToken(key, forms.AccessToken_v1{
		Subject: "clerk", Provider: "ds18b20", Asset: "picam",
		Service: "", Action: "read",
		IssuedAt: now, Expires: now.Add(5 * time.Minute), Issuer: "authorizer",
	})
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}

	r := httptest.NewRequest("GET", "/photographer/picam/anything", nil)
	r.TLS = tlsStateWithCN("clerk")
	r.Header.Set(TokenHeader, token)

	// Even a token minted with an empty service — which the orchestrator would
	// never produce — cannot match one.
	if status, _ := AuthorizeRequest(sys, r, "picam", &components.Service{}); status == 0 {
		t.Error("a service with no definition was authorized; nothing should satisfy it")
	}
}
