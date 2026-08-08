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
	"net/http"
	"net/http/httptest"
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

func TestInstallationTokenWithDiscovery(t *testing.T) {
	pemPath, _ := writeTestKey(t, false)
	var discovered, minted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ey") {
			t.Errorf("no JWT bearer on %s: %q", r.URL.Path, auth)
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/repos/o/r/installation":
			discovered = true
			json.NewEncoder(w).Encode(map[string]int{"id": 42})
		case r.Method == "POST" && r.URL.Path == "/app/installations/42/access_tokens":
			minted = true
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["permissions"] == nil {
				t.Error("permissions not requested")
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]string{"token": "ghs_test"})
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	gh := &GitHub{APIBase: srv.URL}
	tok, err := gh.InstallationToken(srv.Client(), "o/r", "12345", pemPath, "")
	if err != nil || tok != "ghs_test" {
		t.Fatalf("token = %q, err = %v", tok, err)
	}
	if !discovered || !minted {
		t.Fatal("discovery or mint skipped")
	}
}

func TestInstallationTokenExplicitIDSkipsDiscovery(t *testing.T) {
	pemPath, _ := writeTestKey(t, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/installations/7/access_tokens" {
			t.Errorf("unexpected call %s", r.URL.Path)
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]string{"token": "ghs_x"})
	}))
	defer srv.Close()
	gh := &GitHub{APIBase: srv.URL}
	if tok, err := gh.InstallationToken(srv.Client(), "o/r", "1", pemPath, "7"); err != nil || tok != "ghs_x" {
		t.Fatalf("token = %q, err = %v", tok, err)
	}
}

func TestInstallationTokenErrors(t *testing.T) {
	pemPath, _ := writeTestKey(t, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(403)
		w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer srv.Close()
	gh := &GitHub{APIBase: srv.URL}

	_, err := gh.InstallationToken(srv.Client(), "o/r", "1", pemPath, "")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("404 discovery: %v", err)
	}
	_, err = gh.InstallationToken(srv.Client(), "o/r", "1", pemPath, "7")
	if err == nil || !strings.Contains(err.Error(), "contents") {
		t.Fatalf("403 exchange must name the permissions: %v", err)
	}
}

// TestInstallationTokenRejectsNonNumericID guards against a hand-edited
// git.app.installation-id reaching the URL uninterpolated-validated: schema
// validation only runs at `config set` time, not on Load/Apply. The server
// must never see a request — a non-numeric ID is rejected before any call.
func TestInstallationTokenRejectsNonNumericID(t *testing.T) {
	pemPath, _ := writeTestKey(t, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP call for non-numeric installation id: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	gh := &GitHub{APIBase: srv.URL}

	_, err := gh.InstallationToken(srv.Client(), "o/r", "1", pemPath, "42; rm -rf /")
	if err == nil || !strings.Contains(err.Error(), `invalid installation id "42; rm -rf /": must be a number`) {
		t.Fatalf("err = %v, want invalid installation id error", err)
	}
}
