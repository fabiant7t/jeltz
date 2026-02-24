package p12_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/fabiant7t/jeltz/pkg/p12"
)

// testKey generates a 2048-bit RSA key for use in tests.
func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

// testCert issues a self-signed certificate for key.
func testCert(t *testing.T, key *rsa.PrivateKey) *x509.Certificate {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "p12 test CA"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return cert
}

func TestEncode(t *testing.T) {
	key := testKey(t)
	cert := testCert(t, key)

	der, err := p12.Encode(key, cert, "testpass")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(der) == 0 {
		t.Fatal("Encode returned empty bytes")
	}
}

func TestEncode_EmptyPassword(t *testing.T) {
	key := testKey(t)
	cert := testCert(t, key)

	der, err := p12.Encode(key, cert, "")
	if err != nil {
		t.Fatalf("Encode with empty password: %v", err)
	}
	if len(der) == 0 {
		t.Fatal("Encode returned empty bytes")
	}
}

// TestEncode_Openssl verifies the bundle using openssl pkcs12. The test is
// skipped when openssl is not in PATH.
func TestEncode_Openssl(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl not in PATH")
	}

	key := testKey(t)
	cert := testCert(t, key)
	const pass = "testpass"

	der, err := p12.Encode(key, cert, pass)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "test.p12")
	if err := os.WriteFile(bundlePath, der, 0o600); err != nil {
		t.Fatalf("write p12: %v", err)
	}

	// Verify MAC and structure (exits non-zero on wrong password or corrupt MAC).
	out, err := exec.Command("openssl", "pkcs12",
		"-info", "-in", bundlePath,
		"-passin", "pass:"+pass,
		"-nokeys", "-nocerts",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("openssl rejected the bundle: %v\noutput:\n%s", err, out)
	}

	// Extract the certificate from the bundle.
	certPath := filepath.Join(dir, "extracted.crt")
	out, err = exec.Command("openssl", "pkcs12",
		"-in", bundlePath,
		"-passin", "pass:"+pass,
		"-nokeys", "-clcerts",
		"-out", certPath,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("openssl cert extraction failed: %v\noutput:\n%s", err, out)
	}

	// Parse the extracted cert and compare to the original.
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read extracted cert: %v", err)
	}
	// openssl prepends Bag Attributes lines; skip them to find the PEM block.
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatalf("no PEM block in extracted cert:\n%s", certPEM)
	}
	extracted, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse extracted cert: %v", err)
	}

	if !bytes.Equal(extracted.Raw, cert.Raw) {
		t.Error("extracted certificate does not match original")
	}
}
