// Package ca manages the jeltz root CA and per-host leaf certificate issuance.
package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	caKeyFile  = "ca.key.pem"
	caCertFile = "ca.crt.pem"
	certsDir   = "certs"

	// 100-year validity as required by spec.
	validity = 100 * 365 * 24 * time.Hour
)

// CA holds the loaded root CA key and certificate plus a thread-safe
// per-host leaf certificate cache.
type CA struct {
	dataDir string
	key     *rsa.PrivateKey
	cert    *x509.Certificate
	raw     []byte // DER-encoded CA cert (for tls.Certificate)

	mu    sync.Mutex
	cache map[string]*tls.Certificate // host → leaf cert
}

// Load loads or creates the CA from dataDir.
func Load(dataDir string) (*CA, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("ca: create data dir: %w", err)
	}

	keyPath := filepath.Join(dataDir, caKeyFile)
	certPath := filepath.Join(dataDir, caCertFile)

	// Generate CA if key or cert are missing.
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		if err := generate(dataDir, keyPath, certPath); err != nil {
			return nil, err
		}
	}

	return loadFromDisk(dataDir, keyPath, certPath)
}

// CertPath returns the path to the CA certificate file.
func (ca *CA) CertPath() string {
	return filepath.Join(ca.dataDir, caCertFile)
}

// LeafCert returns a *tls.Certificate for host, issuing and caching a new one
// if necessary. Thread-safe.
func (ca *CA) LeafCert(host string) (*tls.Certificate, error) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	if cert, ok := ca.cache[host]; ok {
		return cert, nil
	}

	// Try loading from disk cache.
	diskPath := filepath.Join(ca.dataDir, certsDir, sanitizeHost(host)+".pem")
	if cert, err := loadLeafFromDisk(diskPath); err == nil {
		ca.cache[host] = cert
		return cert, nil
	}

	// Issue a new leaf cert.
	cert, err := ca.issue(host)
	if err != nil {
		return nil, fmt.Errorf("issuing cert for %q: %w", host, err)
	}

	// Persist to disk.
	if err := saveLeafToDisk(diskPath, cert); err != nil {
		// Non-fatal: memory cache still works.
		_ = err
	}

	ca.cache[host] = cert
	return cert, nil
}

// generate creates a new RSA 3072 CA key and self-signed certificate.
func generate(dataDir, keyPath, certPath string) error {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return fmt.Errorf("ca: generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("ca: serial: %w", err)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "jeltz Root CA"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(validity),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("ca: create cert: %w", err)
	}

	if err := writePEM(keyPath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key), 0o600); err != nil {
		return err
	}
	if err := writePEM(certPath, "CERTIFICATE", certDER, 0o644); err != nil {
		return err
	}
	return nil
}

func loadFromDisk(dataDir, keyPath, certPath string) (*CA, error) {
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("ca: read key: %w", err)
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("ca: read cert: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("ca: decode key PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ca: parse key: %w", err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, fmt.Errorf("ca: decode cert PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ca: parse cert: %w", err)
	}

	return &CA{
		dataDir: dataDir,
		key:     key,
		cert:    cert,
		raw:     certBlock.Bytes,
		cache:   make(map[string]*tls.Certificate),
	}, nil
}

// issue creates and returns a new leaf TLS certificate signed by ca.
func (ca *CA) issue(host string) (*tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate leaf key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("serial: %w", err)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(validity),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	// Use IPAddresses SAN for IP addresses, DNSNames for hostnames.
	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, fmt.Errorf("create cert: %w", err)
	}

	tlsCert := &tls.Certificate{
		Certificate: [][]byte{certDER, ca.raw},
		PrivateKey:  key,
	}
	tlsCert.Leaf, err = x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse leaf cert: %w", err)
	}
	return tlsCert, nil
}

// loadLeafFromDisk tries to load a PEM-encoded key+cert pair from path.
func loadLeafFromDisk(path string) (*tls.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cert, err := tls.X509KeyPair(data, data)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

// saveLeafToDisk writes key+cert PEM to path (creates dirs as needed).
func saveLeafToDisk(path string, cert *tls.Certificate) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write private key.
	if pk, ok := cert.PrivateKey.(*rsa.PrivateKey); ok {
		if err := pem.Encode(f, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(pk)}); err != nil {
			return err
		}
	}
	// Write all certs in chain.
	for _, c := range cert.Certificate {
		if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: c}); err != nil {
			return err
		}
	}
	return nil
}

func writePEM(path, pemType string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("ca: open %s: %w", path, err)
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: pemType, Bytes: data})
}

// sanitizeHost makes a host string safe as a filename component.
func sanitizeHost(host string) string {
	out := make([]byte, len(host))
	for i, c := range []byte(host) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			out[i] = c
		} else {
			out[i] = '_'
		}
	}
	return string(out)
}
