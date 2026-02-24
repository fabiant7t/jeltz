// Package ca manages the jeltz root CA and per-host leaf certificate issuance.
package ca

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	pkgca "github.com/fabiant7t/jeltz/pkg/ca"
	"github.com/fabiant7t/jeltz/pkg/p12"
)

const (
	caKeyFile  = "ca.key.pem"
	caCertFile = "ca.crt.pem"
	caP12File  = "ca.p12"
	certsDir   = "certs"

	// P12Password is the fixed password for all jeltz PKCS#12 bundles.
	P12Password = "jeltz"

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
		if err := generate(keyPath, certPath); err != nil {
			return nil, err
		}
	}

	loaded, err := loadFromDisk(dataDir, keyPath, certPath)
	if err != nil {
		return nil, err
	}

	// Write PKCS#12 bundle if missing (first run or upgrade).
	p12Path := filepath.Join(dataDir, caP12File)
	if _, err := os.Stat(p12Path); os.IsNotExist(err) {
		if der, err := p12.Encode(loaded.key, loaded.cert, P12Password); err == nil {
			_ = os.WriteFile(p12Path, der, 0o600)
		}
	}

	return loaded, nil
}

// CertPath returns the path to the CA certificate file.
func (ca *CA) CertPath() string {
	return filepath.Join(ca.dataDir, caCertFile)
}

// P12Path returns the path to the CA PKCS#12 bundle.
func (ca *CA) P12Path() string {
	return filepath.Join(ca.dataDir, caP12File)
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

// generate creates a new RSA 3072 CA key and self-signed certificate on disk.
func generate(keyPath, certPath string) error {
	key, cert, err := pkgca.GenerateCA("jeltz Root CA", 3072, validity)
	if err != nil {
		return err
	}
	if err := writePEM(keyPath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key), 0o600); err != nil {
		return err
	}
	return writePEM(certPath, "CERTIFICATE", cert.Raw, 0o644)
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
	return pkgca.IssueLeaf(ca.key, ca.cert, host, 2048, validity)
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
