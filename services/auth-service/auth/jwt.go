package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleReadOnly = "read-only"
)

var (
	ErrInvalidToken = errors.New("invalid or tampered authentication token")
	ErrExpiredToken = errors.New("authentication token has expired")
)

type Header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type Claims struct {
	Sub  string `json:"sub"`
	Role string `json:"role"`
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
}

func getSecretKey() ([]byte, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil, errors.New("JWT_SECRET environment variable must be set")
	}
	return []byte(secret), nil
}

// IssueToken creates a signed HMAC-SHA256 JWT bearer token with specified subject, role, and expiration.
func IssueToken(userID string, role string, duration time.Duration) (string, error) {
	if role != RoleAdmin && role != RoleOperator && role != RoleReadOnly {
		return "", fmt.Errorf("invalid role %q", role)
	}

	header := Header{Alg: "HS256", Typ: "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := Claims{
		Sub:  userID,
		Role: role,
		Iat:  now.Unix(),
		Exp:  now.Add(duration).Unix(),
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	encHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)

	unsignedToken := fmt.Sprintf("%s.%s", encHeader, encClaims)
	secret, err := getSecretKey()
	if err != nil {
		return "", err
	}
	sig := calculateHMAC([]byte(unsignedToken), secret)
	encSig := base64.RawURLEncoding.EncodeToString(sig)

	return fmt.Sprintf("%s.%s", unsignedToken, encSig), nil
}

// VerifyToken validates the JWT signature, parsing claims and enforcing expiration.
func VerifyToken(tokenStr string) (*Claims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	unsignedToken := fmt.Sprintf("%s.%s", parts[0], parts[1])
	secret, err := getSecretKey()
	if err != nil {
		return nil, err
	}
	expectedSig := calculateHMAC([]byte(unsignedToken), secret)

	providedSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}

	if !hmac.Equal(expectedSig, providedSig) {
		return nil, ErrInvalidToken
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims Claims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	if time.Now().Unix() > claims.Exp {
		return nil, ErrExpiredToken
	}

	return &claims, nil
}

func calculateHMAC(data, secret []byte) []byte {
	h := hmac.New(sha256.New, secret)
	h.Write(data)
	return h.Sum(nil)
}
