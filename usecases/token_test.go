package usecases

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/sdoque/mbaigo/forms"
)

func signingKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	return key
}

// theRequest is the thermostat reading the kitchen sensor.
func theRequest() TokenRequest {
	return TokenRequest{
		Subject:  "thermostat",
		Provider: "ds18b20",
		Asset:    "sensor_Id",
		Service:  "temperature",
		Action:   "read",
	}
}

func claimsFor(req TokenRequest, now time.Time, life time.Duration) forms.AccessToken_v1 {
	return forms.AccessToken_v1{
		Subject:  req.Subject,
		Provider: req.Provider,
		Asset:    req.Asset,
		Service:  req.Service,
		Action:   req.Action,
		IssuedAt: now,
		Expires:  now.Add(life),
		Issuer:   "authorizer",
	}
}

func TestMintAndVerify(t *testing.T) {
	key := signingKey(t)
	now := time.Now()

	token, err := MintToken(key, claimsFor(theRequest(), now, 5*time.Minute))
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}

	claims, err := VerifyToken(token, &key.PublicKey, theRequest(), now)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if claims.Subject != "thermostat" || claims.Issuer != "authorizer" {
		t.Errorf("claims did not survive: %+v", claims)
	}
	if claims.FormVersion() != "AccessToken_v1" {
		t.Errorf("version = %q", claims.FormVersion())
	}
}

// A signature from any other key is not the authorizer's. This is the whole
// point of signing, so it is asserted directly rather than inferred.
func TestVerifyRejectsAnotherKeysSignature(t *testing.T) {
	mine, theirs := signingKey(t), signingKey(t)
	now := time.Now()

	token, err := MintToken(theirs, claimsFor(theRequest(), now, 5*time.Minute))
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}

	if _, err := VerifyToken(token, &mine.PublicKey, theRequest(), now); err == nil {
		t.Fatal("a token signed by another key verified")
	}
}

// Tampering with the claims must invalidate the signature — otherwise a
// consumer could promote its own read token to a write.
func TestVerifyRejectsTamperedClaims(t *testing.T) {
	key := signingKey(t)
	now := time.Now()

	token, err := MintToken(key, claimsFor(theRequest(), now, 5*time.Minute))
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}

	tampered := claimsFor(theRequest(), now, 5*time.Minute)
	tampered.Action = "write"
	forged, err := MintToken(signingKey(t), tampered) // re-signed with a key nobody trusts
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}

	// Splice the forged claims onto the genuine signature.
	spliced := strings.SplitN(forged, ".", 2)[0] + "." + strings.SplitN(token, ".", 2)[1]

	write := theRequest()
	write.Action = "write"
	if _, err := VerifyToken(spliced, &key.PublicKey, write, now); err == nil {
		t.Fatal("claims were swapped under a valid signature")
	}
}

// A signature only proves the authorizer issued *some* permission. Every claim
// has to describe this request, or a token for a reading opens a valve.
func TestVerifyRejectsAMismatchedRequest(t *testing.T) {
	key := signingKey(t)
	now := time.Now()
	token, err := MintToken(key, claimsFor(theRequest(), now, 5*time.Minute))
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*TokenRequest)
		wantErr string
	}{
		{"another system replays it", func(r *TokenRequest) { r.Subject = "collector" }, "subject"},
		{"presented to another provider", func(r *TokenRequest) { r.Provider = "parallax" }, "provider"},
		{"presented to another asset", func(r *TokenRequest) { r.Asset = "Servo_1" }, "asset"},
		{"used on another service", func(r *TokenRequest) { r.Service = "rotation" }, "service"},
		{"used for another action", func(r *TokenRequest) { r.Action = "write" }, "action"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := theRequest()
			tc.mutate(&req)
			_, err := VerifyToken(token, &key.PublicKey, req, now)
			if err == nil {
				t.Fatal("a token was accepted for a different request")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not name the mismatched claim (%q)", err, tc.wantErr)
			}
		})
	}
}

func TestVerifyEnforcesTheValidityWindow(t *testing.T) {
	key := signingKey(t)
	now := time.Now()
	token, err := MintToken(key, claimsFor(theRequest(), now, time.Minute))
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}

	if _, err := VerifyToken(token, &key.PublicKey, theRequest(), now.Add(30*time.Second)); err != nil {
		t.Errorf("a live token was refused: %v", err)
	}
	if _, err := VerifyToken(token, &key.PublicKey, theRequest(), now.Add(2*time.Minute)); err == nil {
		t.Error("an expired token was accepted; expiry is the only revocation there is")
	}
}

// An unbounded token is the one failure this design cannot recover from, so a
// missing expiry must fail the safe way.
func TestTokenWithoutAnExpiryIsExpired(t *testing.T) {
	key := signingKey(t)
	now := time.Now()

	claims := claimsFor(theRequest(), now, 0)
	claims.Expires = time.Time{}
	token, err := MintToken(key, claims)
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}

	if _, err := VerifyToken(token, &key.PublicKey, theRequest(), now); err == nil {
		t.Error("a token with no expiry was accepted as valid forever")
	}
}

func TestVerifyRejectsMalformedTokens(t *testing.T) {
	key := signingKey(t)
	for _, token := range []string{"", ".", "abc", "abc.", ".abc", "not-base64!.sig", "e30"} {
		if _, err := VerifyToken(token, &key.PublicKey, theRequest(), time.Now()); err == nil {
			t.Errorf("malformed token %q was accepted", token)
		}
	}
}

// Minting without a key, or verifying without one, must be an error rather than
// an unsigned token that something later trusts.
func TestMissingKeysAreErrors(t *testing.T) {
	if _, err := MintToken(nil, claimsFor(theRequest(), time.Now(), time.Minute)); err == nil {
		t.Error("a token was minted with no signing key")
	}
	key := signingKey(t)
	token, err := MintToken(key, claimsFor(theRequest(), time.Now(), time.Minute))
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}
	if _, err := VerifyToken(token, nil, theRequest(), time.Now()); err == nil {
		t.Error("a token verified with no public key")
	}
}

// ParseToken is for diagnostics: it must not be mistaken for verification, so
// it deliberately reads claims a verifier would reject.
func TestParseTokenDoesNotVerify(t *testing.T) {
	key := signingKey(t)
	now := time.Now()
	token, err := MintToken(key, claimsFor(theRequest(), now.Add(-time.Hour), time.Minute))
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}

	claims, err := ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.Subject != "thermostat" {
		t.Errorf("claims not readable: %+v", claims)
	}
	if _, err := VerifyToken(token, &key.PublicKey, theRequest(), now); err == nil {
		t.Error("the same token verified despite being long expired")
	}
}
