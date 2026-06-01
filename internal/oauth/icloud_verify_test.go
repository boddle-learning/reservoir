package oauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// fakeNonceStore is an in-memory nonceStore for tests (no Redis dependency).
type fakeNonceStore struct {
	mu      sync.Mutex
	issued  map[string]bool
	counter int
}

func newFakeNonceStore() *fakeNonceStore { return &fakeNonceStore{issued: map[string]bool{}} }

func (f *fakeNonceStore) Issue(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counter++
	n := "nonce-" + string(rune('a'+f.counter))
	f.issued[n] = true
	return n, nil
}

func (f *fakeNonceStore) Consume(ctx context.Context, nonce string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if nonce == "" || !f.issued[nonce] {
		return errNonce
	}
	delete(f.issued, nonce) // single use
	return nil
}

// preload seeds a nonce as already issued.
func (f *fakeNonceStore) preload(nonce string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issued[nonce] = true
}

var errNonce = jwtError("nonce is invalid, expired, or already used")

type jwtError string

func (e jwtError) Error() string { return string(e) }

// icloudTestHarness wires an ICloudService to a JWKS server backed by a known
// RSA key, so tests can mint ID tokens that verify (or deliberately don't).
type icloudTestHarness struct {
	svc     *ICloudService
	priv    *rsa.PrivateKey
	kid     string
	nonces  *fakeNonceStore
	jwksSrv *httptest.Server
}

func newICloudTestHarness(t *testing.T, audiences []string) *icloudTestHarness {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	kid := "test-key-1"

	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": kid,
				"use": "sig",
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(priv.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes()),
			}},
		})
	}))
	t.Cleanup(jwksSrv.Close)

	nonces := newFakeNonceStore()
	svc := &ICloudService{
		issuer:           appleIssuer,
		jwksURL:          jwksSrv.URL,
		allowedAudiences: audiences,
		httpClient:       jwksSrv.Client(),
		nonces:           nonces,
		keys:             map[string]*rsa.PublicKey{},
		keysTTL:          time.Hour,
	}
	return &icloudTestHarness{svc: svc, priv: priv, kid: kid, nonces: nonces, jwksSrv: jwksSrv}
}

// sign builds an RS256 ID token from the given claims using the harness key.
func (h *icloudTestHarness) sign(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = h.kid
	s, err := tok.SignedString(h.priv)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

func validClaims(nonce string) jwt.MapClaims {
	return jwt.MapClaims{
		"iss":   appleIssuer,
		"aud":   "com.boddle.app",
		"sub":   "001234.apple-sub.5678",
		"email": "kid@privaterelay.appleid.com",
		"exp":   time.Now().Add(10 * time.Minute).Unix(),
		"iat":   time.Now().Add(-1 * time.Minute).Unix(),
		"nonce": nonce,
	}
}

func TestVerifyIDToken_Valid(t *testing.T) {
	h := newICloudTestHarness(t, []string{"com.boddle.app"})
	h.nonces.preload("good-nonce")

	info, err := h.svc.VerifyIDToken(context.Background(), h.sign(t, validClaims("good-nonce")))
	if err != nil {
		t.Fatalf("expected valid token, got error: %v", err)
	}
	if info.ProviderUserID != "001234.apple-sub.5678" {
		t.Errorf("sub = %q, want verified value", info.ProviderUserID)
	}
}

func TestVerifyIDToken_Rejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(jwt.MapClaims)
		nonce  string // preloaded nonce; empty means none preloaded
	}{
		{"wrong issuer", func(c jwt.MapClaims) { c["iss"] = "https://evil.example.com" }, "n1"},
		{"wrong audience", func(c jwt.MapClaims) { c["aud"] = "com.attacker.app" }, "n1"},
		{"expired", func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-1 * time.Minute).Unix() }, "n1"},
		{"missing exp", func(c jwt.MapClaims) { delete(c, "exp") }, "n1"},
		{"missing sub", func(c jwt.MapClaims) { delete(c, "sub") }, "n1"},
		{"nonce not issued", func(c jwt.MapClaims) { c["nonce"] = "never-issued" }, ""},
		{"missing nonce", func(c jwt.MapClaims) { delete(c, "nonce") }, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newICloudTestHarness(t, []string{"com.boddle.app"})
			if tc.nonce != "" {
				h.nonces.preload(tc.nonce)
			}
			claims := validClaims("n1")
			tc.mutate(claims)
			if _, err := h.svc.VerifyIDToken(context.Background(), h.sign(t, claims)); err == nil {
				t.Errorf("%s: expected error, got nil", tc.name)
			}
		})
	}
}

func TestVerifyIDToken_RejectsBadSignature(t *testing.T) {
	h := newICloudTestHarness(t, []string{"com.boddle.app"})
	h.nonces.preload("n1")

	// Sign with a different key than the JWKS advertises.
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims("n1"))
	tok.Header["kid"] = h.kid
	forged, _ := tok.SignedString(other)

	if _, err := h.svc.VerifyIDToken(context.Background(), forged); err == nil {
		t.Error("expected error for token signed by an unknown key, got nil")
	}
}

func TestVerifyIDToken_RejectsAlgNone(t *testing.T) {
	h := newICloudTestHarness(t, []string{"com.boddle.app"})
	h.nonces.preload("n1")

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims("n1"))
	tok.Header["kid"] = h.kid
	unsigned, _ := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)

	if _, err := h.svc.VerifyIDToken(context.Background(), unsigned); err == nil {
		t.Error("expected error for alg:none token, got nil")
	}
}

func TestVerifyIDToken_NonceIsSingleUse(t *testing.T) {
	h := newICloudTestHarness(t, []string{"com.boddle.app"})
	h.nonces.preload("reuse-me")

	token := h.sign(t, validClaims("reuse-me"))
	if _, err := h.svc.VerifyIDToken(context.Background(), token); err != nil {
		t.Fatalf("first use should succeed: %v", err)
	}
	// Replaying the exact same token must fail — the nonce was consumed.
	if _, err := h.svc.VerifyIDToken(context.Background(), token); err == nil {
		t.Error("expected replay of consumed nonce to fail, got nil")
	}
}

func TestVerifyIDToken_FailsClosedWhenUnconfigured(t *testing.T) {
	h := newICloudTestHarness(t, nil) // no audiences configured
	h.nonces.preload("n1")
	if h.svc.Configured() {
		t.Fatal("service should report not configured")
	}
	if _, err := h.svc.VerifyIDToken(context.Background(), h.sign(t, validClaims("n1"))); err == nil {
		t.Error("expected error when APPLE_CLIENT_IDS is unset, got nil")
	}
}

func TestVerifyIDToken_MultipleAudiences(t *testing.T) {
	h := newICloudTestHarness(t, []string{"com.boddle.web", "com.boddle.app"})
	h.nonces.preload("n1")
	claims := validClaims("n1")
	claims["aud"] = "com.boddle.web" // second entry in the allowlist
	if _, err := h.svc.VerifyIDToken(context.Background(), h.sign(t, claims)); err != nil {
		t.Errorf("expected token for an allowlisted aud to verify, got %v", err)
	}
}

func TestParseAudienceList(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"   ", 0},
		{"a", 1},
		{"a,b", 2},
		{" a , , b ,", 2},
	}
	for _, tt := range tests {
		if got := len(parseAudienceList(tt.in)); got != tt.want {
			t.Errorf("parseAudienceList(%q) len = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseRSAPublicKey(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	n := base64.RawURLEncoding.EncodeToString(priv.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes())

	pk, err := parseRSAPublicKey(n, e)
	if err != nil {
		t.Fatalf("parseRSAPublicKey error: %v", err)
	}
	if pk.N.Cmp(priv.N) != 0 || pk.E != priv.E {
		t.Error("reconstructed RSA public key does not match original")
	}

	if _, err := parseRSAPublicKey("!!!not-base64!!!", e); err == nil {
		t.Error("expected error for invalid modulus encoding")
	}
}
