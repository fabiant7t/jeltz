package ca_test

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"

	"github.com/fabiant7t/jeltz/internal/ca"
)

func TestLoad_CreatesCA(t *testing.T) {
	dir := t.TempDir()
	c, err := ca.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil CA")
	}

	// Check files were created.
	if _, err := os.Stat(filepath.Join(dir, "ca.key.pem")); err != nil {
		t.Error("ca.key.pem not created")
	}
	if _, err := os.Stat(filepath.Join(dir, "ca.crt.pem")); err != nil {
		t.Error("ca.crt.pem not created")
	}
	if _, err := os.Stat(filepath.Join(dir, "ca.p12")); err != nil {
		t.Error("ca.p12 not created")
	}
}

func TestLoad_Idempotent(t *testing.T) {
	dir := t.TempDir()
	c1, err := ca.Load(dir)
	if err != nil {
		t.Fatalf("first Load: %v", err)
	}
	c2, err := ca.Load(dir)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	cert1, _ := c1.LeafCert("reload.test")
	// Clear memory cache between loads — c2 is a fresh load from disk.
	cert2, _ := c2.LeafCert("reload.test")
	// Both should produce valid certs for the same host.
	if cert1 == nil || cert2 == nil {
		t.Error("expected certs from both loads")
	}
}

func TestLeafCert_ValidForHost(t *testing.T) {
	dir := t.TempDir()
	c, err := ca.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	host := "example.com"
	cert, err := c.LeafCert(host)
	if err != nil {
		t.Fatalf("LeafCert: %v", err)
	}
	if cert == nil {
		t.Fatal("nil cert")
	}

	// Verify DNSNames.
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	found := false
	for _, n := range leaf.DNSNames {
		if n == host {
			found = true
		}
	}
	if !found {
		t.Errorf("DNSNames %v does not contain %q", leaf.DNSNames, host)
	}
}

func TestLeafCert_Cached(t *testing.T) {
	dir := t.TempDir()
	c, err := ca.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	c1, err := c.LeafCert("cached.test")
	if err != nil {
		t.Fatalf("first LeafCert: %v", err)
	}
	c2, err := c.LeafCert("cached.test")
	if err != nil {
		t.Fatalf("second LeafCert: %v", err)
	}
	// Same pointer — served from memory cache.
	if c1 != c2 {
		t.Error("expected same pointer from cache")
	}
}

func TestLeafCert_DiskCache(t *testing.T) {
	dir := t.TempDir()
	c, err := ca.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	host := "disktest.example"
	_, err = c.LeafCert(host)
	if err != nil {
		t.Fatalf("LeafCert: %v", err)
	}

	// Check disk file was created.
	diskPath := filepath.Join(dir, "certs", "disktest_example.pem")
	if _, err := os.Stat(diskPath); err != nil {
		t.Errorf("disk cache not written: %v", err)
	}

	// Load fresh CA (clears memory cache), should load from disk.
	c2, err := ca.Load(dir)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	cert2, err := c2.LeafCert(host)
	if err != nil {
		t.Fatalf("LeafCert from disk cache: %v", err)
	}
	if cert2 == nil {
		t.Fatal("nil cert from disk cache")
	}
}

func TestLeafCert_VerifiableWithCA(t *testing.T) {
	dir := t.TempDir()
	c, err := ca.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Build CA cert pool.
	caCertPEM, err := os.ReadFile(c.CertPath())
	if err != nil {
		t.Fatalf("read CA cert: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCertPEM) {
		t.Fatal("failed to add CA cert to pool")
	}

	tlsCert, err := c.LeafCert("verify.test")
	if err != nil {
		t.Fatalf("LeafCert: %v", err)
	}

	leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}

	_, err = leaf.Verify(x509.VerifyOptions{
		DNSName: "verify.test",
		Roots:   pool,
	})
	if err != nil {
		t.Errorf("leaf cert verification failed: %v", err)
	}
}

