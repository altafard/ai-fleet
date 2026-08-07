package provider

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"
)

// LoadRSAPrivateKey reads a GitHub App private key: PEM, PKCS#1 (GitHub's
// download format) or PKCS#8.
func LoadRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, fmt.Errorf("%s: not a PEM file", path)
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	any, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: not an RSA private key: %w", path, err)
	}
	k, ok := any.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New(path + ": not an RSA private key")
	}
	return k, nil
}

// appJWT signs the app-authentication JWT: RS256, issuer = app ID, iat
// backdated 60s for clock skew, expiry 9 minutes (GitHub caps at 10).
// Hand-rolled with stdlib crypto — a JWT library for one fixed header and
// three claims is not worth a dependency.
func appJWT(key *rsa.PrivateKey, appID string, now time.Time) (string, error) {
	b64 := base64.RawURLEncoding.EncodeToString
	header := b64([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{
		"iss": appID,
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
	})
	if err != nil {
		return "", err
	}
	signing := header + "." + b64(claims)
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signing + "." + b64(sig), nil
}
