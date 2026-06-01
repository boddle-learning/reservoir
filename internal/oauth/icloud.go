package oauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/boddle/reservoir/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

const (
	// appleIssuer is the only issuer a genuine Apple ID token may carry.
	appleIssuer = "https://appleid.apple.com"
	// appleJWKSURL serves Apple's rotating set of RSA public signing keys.
	appleJWKSURL = "https://appleid.apple.com/auth/keys"
)

// ICloudService verifies Apple "Sign in with Apple" ID tokens.
//
// The client performs Sign in with Apple directly and sends the resulting ID
// token (a JWT) to Reservoir. We verify its RS256 signature against Apple's
// JWKS and validate iss/aud/exp plus a server-issued single-use nonce before
// trusting the `sub` claim. This is what closes LMS-6512: previously the
// endpoint trusted a bare, unauthenticated `uid`.
type ICloudService struct {
	issuer           string
	jwksURL          string
	allowedAudiences []string
	httpClient       *http.Client
	nonces           nonceStore

	// JWKS cache. Apple rotates keys, so entries are refreshed past keysTTL and
	// whenever a token references a kid we don't have.
	mu          sync.RWMutex
	keys        map[string]*rsa.PublicKey
	keysFetched time.Time
	keysTTL     time.Duration
}

// NewICloudService builds an ICloudService. APPLE_CLIENT_IDS supplies the aud
// allowlist; when empty the service fails closed (every verification errors)
// rather than trusting unaudienced tokens.
func NewICloudService(cfg config.ICloudConfig, redisClient *redis.Client) *ICloudService {
	return &ICloudService{
		issuer:           appleIssuer,
		jwksURL:          appleJWKSURL,
		allowedAudiences: parseAudienceList(cfg.ClientIDs),
		httpClient:       &http.Client{Timeout: 10 * time.Second},
		nonces:           &redisNonceStore{client: redisClient, ttl: 10 * time.Minute},
		keys:             map[string]*rsa.PublicKey{},
		keysTTL:          1 * time.Hour,
	}
}

// Configured reports whether an audience allowlist is present. When false the
// endpoint cannot verify tokens and rejects all requests.
func (is *ICloudService) Configured() bool {
	return len(is.allowedAudiences) > 0
}

// IssueNonce mints a single-use nonce, stores it, and returns it. The client
// must feed this value into Sign in with Apple so it reappears as the `nonce`
// claim of the resulting ID token.
func (is *ICloudService) IssueNonce(ctx context.Context) (string, error) {
	return is.nonces.Issue(ctx)
}

// VerifyIDToken verifies an Apple ID token end to end and returns the identity
// Apple asserts. Any failure (bad signature, wrong issuer/audience, expired,
// missing/replayed nonce) returns an error and no identity.
func (is *ICloudService) VerifyIDToken(ctx context.Context, idToken string) (*OAuthUserInfo, error) {
	if !is.Configured() {
		return nil, fmt.Errorf("iCloud verification is not configured (APPLE_CLIENT_IDS unset)")
	}

	// WithValidMethods pins RS256, rejecting alg:none and HS/RS confusion.
	// Expiration is required and validated by the parser.
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(is.issuer),
		jwt.WithExpirationRequired(),
	)

	claims := jwt.MapClaims{}
	if _, err := parser.ParseWithClaims(idToken, claims, is.keyFunc(ctx)); err != nil {
		return nil, fmt.Errorf("invalid Apple ID token: %w", err)
	}

	// Audience must include one of our client IDs. We check manually because
	// the parser's WithAudience accepts only a single expected value.
	aud, err := claims.GetAudience()
	if err != nil || !audienceAllowed(aud, is.allowedAudiences) {
		return nil, fmt.Errorf("invalid Apple ID token: audience not allowed")
	}

	// Replay protection: the nonce must match a server-issued, unconsumed one.
	nonce, _ := claims["nonce"].(string)
	if err := is.nonces.Consume(ctx, nonce); err != nil {
		return nil, fmt.Errorf("invalid Apple ID token: %w", err)
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, fmt.Errorf("invalid Apple ID token: missing sub")
	}

	email, _ := claims["email"].(string)
	emailVerified := claims["email_verified"]
	return &OAuthUserInfo{
		ProviderUserID: sub,
		Email:          email,
		EmailVerified:  emailVerified == "true" || emailVerified == true,
	}, nil
}

// keyFunc resolves the RSA public key for a token's kid, refreshing the JWKS
// cache on a miss to tolerate Apple key rotation.
func (is *ICloudService) keyFunc(ctx context.Context) jwt.Keyfunc {
	return func(t *jwt.Token) (interface{}, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("missing kid header")
		}
		return is.publicKey(ctx, kid)
	}
}

func (is *ICloudService) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	is.mu.RLock()
	key, ok := is.keys[kid]
	fresh := time.Since(is.keysFetched) < is.keysTTL
	is.mu.RUnlock()
	if ok && fresh {
		return key, nil
	}

	if err := is.refreshKeys(ctx); err != nil {
		// Fall back to a stale-but-present key rather than failing on a
		// transient JWKS fetch error.
		if ok {
			return key, nil
		}
		return nil, err
	}

	is.mu.RLock()
	key, ok = is.keys[kid]
	is.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no Apple signing key for kid %q", kid)
	}
	return key, nil
}

func (is *ICloudService) refreshKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", is.jwksURL, nil)
	if err != nil {
		return err
	}

	resp, err := is.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch Apple JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Apple JWKS returned status %d", resp.StatusCode)
	}

	var jwks struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed to decode Apple JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pk, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pk
	}
	if len(keys) == 0 {
		return fmt.Errorf("Apple JWKS contained no usable RSA keys")
	}

	is.mu.Lock()
	is.keys = keys
	is.keysFetched = time.Now()
	is.mu.Unlock()
	return nil
}

// parseRSAPublicKey builds an RSA public key from the base64url modulus (n) and
// exponent (e) of a JWK.
func parseRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("invalid modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("invalid exponent: %w", err)
	}

	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() || e.Int64() < 2 {
		return nil, fmt.Errorf("invalid exponent value")
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(e.Int64()),
	}, nil
}

// parseAudienceList splits a comma-separated client-ID allowlist into trimmed,
// non-empty entries.
func parseAudienceList(raw string) []string {
	var out []string
	for _, a := range strings.Split(raw, ",") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// audienceAllowed reports whether any audience in the token is on the allowlist.
func audienceAllowed(aud jwt.ClaimStrings, allowed []string) bool {
	for _, a := range aud {
		for _, want := range allowed {
			if a == want {
				return true
			}
		}
	}
	return false
}

// nonceStore issues and consumes single-use nonces for Apple Sign In replay
// protection.
type nonceStore interface {
	Issue(ctx context.Context) (string, error)
	Consume(ctx context.Context, nonce string) error
}

// redisNonceStore is the production nonceStore. Nonces live in Redis with a
// short TTL and are deleted on first use (GetDel), so each is valid once.
type redisNonceStore struct {
	client *redis.Client
	ttl    time.Duration
}

func nonceKey(nonce string) string { return "icloud:nonce:" + nonce }

func (s *redisNonceStore) Issue(ctx context.Context) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	nonce := hex.EncodeToString(b)
	if err := s.client.Set(ctx, nonceKey(nonce), "1", s.ttl).Err(); err != nil {
		return "", fmt.Errorf("failed to store nonce: %w", err)
	}
	return nonce, nil
}

func (s *redisNonceStore) Consume(ctx context.Context, nonce string) error {
	if nonce == "" {
		return fmt.Errorf("missing nonce")
	}
	err := s.client.GetDel(ctx, nonceKey(nonce)).Err()
	if err == redis.Nil {
		return fmt.Errorf("nonce is invalid, expired, or already used")
	}
	if err != nil {
		return fmt.Errorf("failed to validate nonce: %w", err)
	}
	return nil
}
