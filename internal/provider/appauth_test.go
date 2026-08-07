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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTestKey(t *testing.T, pkcs8 bool) (string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var block *pem.Block
	if pkcs8 {
		der, _ := x509.MarshalPKCS8PrivateKey(key)
		block = &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	} else {
		block = &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	}
	p := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(p, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	return p, key
}

func TestLoadRSAPrivateKeyBothEncodings(t *testing.T) {
	for _, pkcs8 := range []bool{false, true} {
		p, _ := writeTestKey(t, pkcs8)
		if _, err := LoadRSAPrivateKey(p); err != nil {
			t.Errorf("pkcs8=%v: %v", pkcs8, err)
		}
	}
	if _, err := LoadRSAPrivateKey(filepath.Join(t.TempDir(), "nope.pem")); err == nil {
		t.Error("missing file must error")
	}
	bad := filepath.Join(t.TempDir(), "bad.pem")
	os.WriteFile(bad, []byte("not a key"), 0o600)
	if _, err := LoadRSAPrivateKey(bad); err == nil {
		t.Error("garbage must error")
	}
}

func TestAppJWTClaimsAndSignature(t *testing.T) {
	_, key := writeTestKey(t, false)
	now := time.Unix(1_800_000_000, 0)
	tok, err := appJWT(key, "12345", now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", tok)
	}
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, h[:], sig); err != nil {
		t.Fatalf("signature does not verify: %v", err)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Iss string `json:"iss"`
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Iss != "12345" || claims.Iat != now.Add(-60*time.Second).Unix() || claims.Exp != now.Add(9*time.Minute).Unix() {
		t.Fatalf("claims = %+v", claims)
	}
}
