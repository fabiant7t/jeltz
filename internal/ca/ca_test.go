package ca

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLoad_CreatesCA(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
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
	c1, err := Load(dir)
	if err != nil {
		t.Fatalf("first Load: %v", err)
	}
	c2, err := Load(dir)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	cert1, err := c1.LeafCert("reload.test")
	if err != nil {
		t.Fatalf("first LeafCert: %v", err)
	}
	cert2, err := c2.LeafCert("reload.test")
	if err != nil {
		t.Fatalf("second LeafCert: %v", err)
	}
	// Both should produce valid certs for the same host.
	if cert1 == nil || cert2 == nil {
		t.Error("expected certs from both loads")
	}
}

func TestLeafCert_ValidForHost(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
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

func TestLeafCert_OneYearValidity(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	before := time.Now()
	cert, err := c.LeafCert("validity.example")
	if err != nil {
		t.Fatalf("LeafCert: %v", err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}

	validity := leaf.NotAfter.Sub(leaf.NotBefore)
	min := 364 * 24 * time.Hour
	max := 366 * 24 * time.Hour
	if validity < min || validity > max {
		t.Fatalf("validity %v out of expected 1-year range [%v, %v]", validity, min, max)
	}
	if leaf.NotAfter.Before(before.Add(min)) {
		t.Fatalf("NotAfter too soon: %v", leaf.NotAfter)
	}
}

func TestLeafCert_KeySize3072(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cert, err := c.LeafCert("bits.example")
	if err != nil {
		t.Fatalf("LeafCert: %v", err)
	}

	pk, ok := cert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		t.Fatalf("private key type: got %T, want *rsa.PrivateKey", cert.PrivateKey)
	}
	if pk.N.BitLen() != 3072 {
		t.Fatalf("leaf key bits: got %d, want 3072", pk.N.BitLen())
	}
}

func TestLeafCert_Cached(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
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

func TestLeafCert_LRUCapacityEvictsOldest(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	firstHost := "h0.example"
	firstCert, err := c.LeafCert(firstHost)
	if err != nil {
		t.Fatalf("first host leaf cert: %v", err)
	}

	c.cacheMax = 8

	// Fill cache past capacity.
	for i := 1; i <= c.cacheMax; i++ {
		host := fmt.Sprintf("h%d.example", i)
		if _, err := c.LeafCert(host); err != nil {
			t.Fatalf("LeafCert(%s): %v", host, err)
		}
	}

	// First host should have been evicted; next request reissues.
	firstCertAgain, err := c.LeafCert(firstHost)
	if err != nil {
		t.Fatalf("LeafCert again: %v", err)
	}
	if firstCertAgain == firstCert {
		t.Fatal("expected first host cert to be reissued after LRU eviction")
	}
}

func TestLeafCert_ConcurrentDifferentHosts(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	const n = 8
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			host := fmt.Sprintf("concurrent-%d.example", i)
			if _, err := c.LeafCert(host); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("LeafCert error: %v", err)
		}
	}
}

func TestLeafCert_VerifiableWithCA(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
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
