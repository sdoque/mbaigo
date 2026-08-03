package usecases

// Token verification runs on the provider's request path, so its cost is the one
// piece of the authorization design that every served request pays. These
// benchmarks exist to keep that number measured rather than assumed, and to make
// the design alternatives comparable on whatever hardware a deployment runs on.
//
//	go test -run '^$' -bench Token ./usecases/
//
// Reference figures, Apple M4 Pro (darwin/arm64), Go 1.26.5:
//
//	VerifyToken            36357 ns/op    the full provider-side path
//	VerifyTokenParseOnly    1487 ns/op    base64 + JSON, no signature
//	VerifyTokenClaimsOnly   16.4 ns/op    the five claim comparisons alone
//	MintToken              16561 ns/op    the authorizer's side
//	ECDSAP256Verify        33990 ns/op    the primitive in use
//	Ed25519Verify          27405 ns/op    1.24x faster — not worth a migration
//	HMACSHA256Verify       255.9 ns/op    133x faster, at the cost of key distribution
//
// A Raspberry Pi 4 should be read as roughly 5-10x slower throughout.
//
// What the spread says: the signature is 96% of the cost, and everything else a
// provider must do per request — decoding the token and checking that its claims
// describe this call — is 1.5 us, or 16 ns if the parsed claims are kept.
//
// Since a token is reused until it expires — up to 300 times at a five-minute
// TTL and 1 Hz — caching successful verifications by token string would buy more
// than switching to symmetric keys does, without any key to distribute or
// rotate. Neither is worth building until a deployment shows the cost mattering:
// at mbaigo's control-loop periods a provider serves on the order of one request
// per second, where 36 us is four thousandths of a percent of a core.

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"testing"
	"time"
)

// benchKey mirrors the in-memory key a system generates when it enrols.
func benchKey(b *testing.B) *ecdsa.PrivateKey {
	b.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		b.Fatalf("generating a key: %v", err)
	}
	return key
}

// benchToken is a token for the thermostat reading the kitchen sensor.
func benchToken(b *testing.B, key *ecdsa.PrivateKey, now time.Time) string {
	b.Helper()
	token, err := MintToken(key, claimsFor(theRequest(), now, 5*time.Minute))
	if err != nil {
		b.Fatalf("MintToken: %v", err)
	}
	return token
}

// BenchmarkVerifyToken measures what a provider pays per request today.
func BenchmarkVerifyToken(b *testing.B) {
	key := benchKey(b)
	now := time.Now()
	token := benchToken(b, key, now)
	req := theRequest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := VerifyToken(token, &key.PublicKey, req, now); err != nil {
			b.Fatalf("VerifyToken: %v", err)
		}
	}
}

// BenchmarkVerifyTokenParseOnly measures the path without the signature: what a
// provider would pay per request if it cached successful verifications by token
// string and re-read the claims each time.
func BenchmarkVerifyTokenParseOnly(b *testing.B) {
	key := benchKey(b)
	token := benchToken(b, key, time.Now())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ParseToken(token); err != nil {
			b.Fatalf("ParseToken: %v", err)
		}
	}
}

// BenchmarkVerifyTokenClaimsOnly measures the floor: the comparisons that must
// happen on every request however the signature is handled, because a cached
// token is still valid only for its own subject, provider, asset, service and
// action.
func BenchmarkVerifyTokenClaimsOnly(b *testing.B) {
	key := benchKey(b)
	now := time.Now()
	token := benchToken(b, key, now)
	claims, err := ParseToken(token)
	if err != nil {
		b.Fatalf("ParseToken: %v", err)
	}
	req := theRequest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if claims.Expired(now) {
			b.Fatal("token expired")
		}
		if mismatch("subject", claims.Subject, req.Subject) != nil ||
			mismatch("provider", claims.Provider, req.Provider) != nil ||
			mismatch("asset", claims.Asset, req.Asset) != nil ||
			mismatch("service", claims.Service, req.Service) != nil ||
			mismatch("action", claims.Action, req.Action) != nil {
			b.Fatal("claims did not match")
		}
	}
}

// BenchmarkMintToken measures the authorizer's side. It runs once per grant per
// orchestration, not per request, so it is far less sensitive than verification.
func BenchmarkMintToken(b *testing.B) {
	key := benchKey(b)
	claims := claimsFor(theRequest(), time.Now(), 5*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := MintToken(key, claims); err != nil {
			b.Fatalf("MintToken: %v", err)
		}
	}
}

// The three primitives, on identical input, so the choice of signature scheme
// can be argued from measurement rather than reputation.
var benchPayload = []byte(`{"sub":"thermostat","provider":"ds18b20","asset":"sensor_Id","service":"temperature","action":"read"}`)

func BenchmarkTokenPrimitiveECDSAP256Verify(b *testing.B) {
	key := benchKey(b)
	digest := sha256.Sum256(benchPayload)
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		b.Fatalf("signing: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !ecdsa.VerifyASN1(&key.PublicKey, digest[:], sig) {
			b.Fatal("verification failed")
		}
	}
}

func BenchmarkTokenPrimitiveEd25519Verify(b *testing.B) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatalf("generating a key: %v", err)
	}
	sig := ed25519.Sign(priv, benchPayload)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !ed25519.Verify(pub, benchPayload, sig) {
			b.Fatal("verification failed")
		}
	}
}

// HMAC is the symmetric alternative: far cheaper per request, but it needs a key
// shared between the authorizer and each provider, which turns distribution and
// rotation into the design's hardest problem rather than its simplest.
func BenchmarkTokenPrimitiveHMACSHA256Verify(b *testing.B) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		b.Fatalf("generating a key: %v", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(benchPayload)
	want := mac.Sum(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := hmac.New(sha256.New, key)
		m.Write(benchPayload)
		if !hmac.Equal(m.Sum(nil), want) {
			b.Fatal("verification failed")
		}
	}
}
