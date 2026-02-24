package ca_test

import (
	"crypto/x509"
	"net"
	"testing"
	"time"

	"github.com/fabiant7t/jeltz/pkg/ca"
)

const (
	testBitsCA   = 2048 // smaller than production for test speed
	testBitsLeaf = 2048
	testValidity = time.Hour
)

func TestGenerateCA_ReturnsKeyAndCert(t *testing.T) {
	key, cert, err := ca.GenerateCA("Test CA", testBitsCA, testValidity)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if key == nil {
		t.Fatal("key is nil")
	}
	if cert == nil {
		t.Fatal("cert is nil")
	}
}

func TestGenerateCA_KeySize(t *testing.T) {
	key, _, err := ca.GenerateCA("Test CA", testBitsCA, testValidity)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if key.N.BitLen() != testBitsCA {
		t.Errorf("key bit length = %d, want %d", key.N.BitLen(), testBitsCA)
	}
}

func TestGenerateCA_CommonName(t *testing.T) {
	const cn = "My Root CA"
	_, cert, err := ca.GenerateCA(cn, testBitsCA, testValidity)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if cert.Subject.CommonName != cn {
		t.Errorf("CommonName = %q, want %q", cert.Subject.CommonName, cn)
	}
}

func TestGenerateCA_IsCA(t *testing.T) {
	_, cert, err := ca.GenerateCA("Test CA", testBitsCA, testValidity)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if !cert.IsCA {
		t.Error("IsCA = false, want true")
	}
	if !cert.BasicConstraintsValid {
		t.Error("BasicConstraintsValid = false, want true")
	}
}

func TestGenerateCA_KeyUsage(t *testing.T) {
	_, cert, err := ca.GenerateCA("Test CA", testBitsCA, testValidity)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	want := x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	if cert.KeyUsage&want != want {
		t.Errorf("KeyUsage = %v, want CertSign|CRLSign", cert.KeyUsage)
	}
}

func TestGenerateCA_Validity(t *testing.T) {
	validity := 48 * time.Hour
	// x509 encodes times at second precision, so truncate before comparing.
	before := time.Now().Truncate(time.Second)
	_, cert, err := ca.GenerateCA("Test CA", testBitsCA, validity)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	after := time.Now().Add(time.Second) // +1s margin for second truncation

	if cert.NotAfter.Before(before.Add(validity)) {
		t.Errorf("NotAfter %v is before expected minimum %v", cert.NotAfter, before.Add(validity))
	}
	if cert.NotAfter.After(after.Add(validity)) {
		t.Errorf("NotAfter %v is after expected maximum %v", cert.NotAfter, after.Add(validity))
	}
}

func TestGenerateCA_SelfSigned(t *testing.T) {
	_, cert, err := ca.GenerateCA("Test CA", testBitsCA, testValidity)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		t.Errorf("self-signed cert does not verify: %v", err)
	}
}

func TestIssueLeaf_ReturnsChain(t *testing.T) {
	caKey, caCert, err := ca.GenerateCA("Test CA", testBitsCA, testValidity)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	tlsCert, err := ca.IssueLeaf(caKey, caCert, "example.com", testBitsLeaf, testValidity)
	if err != nil {
		t.Fatalf("IssueLeaf: %v", err)
	}
	if tlsCert == nil {
		t.Fatal("tlsCert is nil")
	}
	// Chain must contain leaf + CA cert.
	if len(tlsCert.Certificate) != 2 {
		t.Errorf("chain length = %d, want 2", len(tlsCert.Certificate))
	}
	if tlsCert.Leaf == nil {
		t.Error("Leaf field is nil")
	}
}

func TestIssueLeaf_DNSSan(t *testing.T) {
	caKey, caCert, err := ca.GenerateCA("Test CA", testBitsCA, testValidity)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	const host = "example.com"
	tlsCert, err := ca.IssueLeaf(caKey, caCert, host, testBitsLeaf, testValidity)
	if err != nil {
		t.Fatalf("IssueLeaf: %v", err)
	}
	leaf := tlsCert.Leaf
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != host {
		t.Errorf("DNSNames = %v, want [%q]", leaf.DNSNames, host)
	}
	if len(leaf.IPAddresses) != 0 {
		t.Errorf("unexpected IPAddresses: %v", leaf.IPAddresses)
	}
}

func TestIssueLeaf_IPSan(t *testing.T) {
	caKey, caCert, err := ca.GenerateCA("Test CA", testBitsCA, testValidity)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	const host = "192.168.1.1"
	tlsCert, err := ca.IssueLeaf(caKey, caCert, host, testBitsLeaf, testValidity)
	if err != nil {
		t.Fatalf("IssueLeaf: %v", err)
	}
	leaf := tlsCert.Leaf
	if len(leaf.IPAddresses) != 1 || !leaf.IPAddresses[0].Equal(net.ParseIP(host)) {
		t.Errorf("IPAddresses = %v, want [%s]", leaf.IPAddresses, host)
	}
	if len(leaf.DNSNames) != 0 {
		t.Errorf("unexpected DNSNames: %v", leaf.DNSNames)
	}
}

func TestIssueLeaf_ExtKeyUsage(t *testing.T) {
	caKey, caCert, err := ca.GenerateCA("Test CA", testBitsCA, testValidity)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	tlsCert, err := ca.IssueLeaf(caKey, caCert, "example.com", testBitsLeaf, testValidity)
	if err != nil {
		t.Fatalf("IssueLeaf: %v", err)
	}
	for _, eku := range tlsCert.Leaf.ExtKeyUsage {
		if eku == x509.ExtKeyUsageServerAuth {
			return
		}
	}
	t.Errorf("ExtKeyUsage %v does not contain ServerAuth", tlsCert.Leaf.ExtKeyUsage)
}

func TestIssueLeaf_VerifiableWithCA(t *testing.T) {
	caKey, caCert, err := ca.GenerateCA("Test CA", testBitsCA, testValidity)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	const host = "verify.example.com"
	tlsCert, err := ca.IssueLeaf(caKey, caCert, host, testBitsLeaf, testValidity)
	if err != nil {
		t.Fatalf("IssueLeaf: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := tlsCert.Leaf.Verify(x509.VerifyOptions{
		DNSName: host,
		Roots:   pool,
	}); err != nil {
		t.Errorf("leaf cert verification failed: %v", err)
	}
}
