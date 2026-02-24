// Package p12 encodes PKCS#12 PFX bundles (RFC 7292) using only the Go
// standard library.
//
// Format: PFX v3 with:
//   - unencrypted CertBag (cert safe bag)
//   - PBE-SHA1-3DES ShroudedKeyBag (key safe bag, RFC 7292 Appendix B KDF)
//   - HMAC-SHA1 MAC (RFC 7292 Appendix B KDF, ID=3)
//   - password encoded as BMPString (UTF-16BE + null terminator)
package p12

import (
	"crypto/cipher"
	"crypto/des"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
)

// OIDs used in PKCS#12 encoding.
var (
	oidP12Data     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidP12CertBag  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 3}
	oidP12KeyBag   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 2}
	oidP12X509Cert = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 22, 1}
	oidP12KeyID    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 21}
	oidPBE3DES     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 3}
	oidSHA1        = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
)

// ---- ASN.1 structures ---------------------------------------------------

type pfx struct {
	Version  int
	AuthSafe contentInfo
	MacData  macData
}

// contentInfo represents a PKCS#7 ContentInfo.
// Content is encoded as [0] EXPLICIT manually (see explicit0) because
// encoding/asn1 ignores struct tags when RawValue.FullBytes is set.
type contentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"optional"`
}

// safeBag represents a PKCS#12 SafeBag.
// Value is encoded as [0] EXPLICIT manually (see explicit0).
type safeBag struct {
	ID         asn1.ObjectIdentifier
	Value      asn1.RawValue `asn1:"optional"`
	Attributes []attribute   `asn1:"set,optional"`
}

type attribute struct {
	ID    asn1.ObjectIdentifier
	Value asn1.RawValue
}

// certBag represents a PKCS#12 CertBag.
// Value is encoded as [0] EXPLICIT manually (see explicit0).
type certBag struct {
	ID    asn1.ObjectIdentifier
	Value asn1.RawValue `asn1:"optional"`
}

type encryptedPKI struct {
	Algorithm pkix.AlgorithmIdentifier
	Data      []byte
}

type pbeParams struct {
	Salt       []byte
	Iterations int
}

type macData struct {
	Mac        digestInfo
	Salt       []byte
	Iterations int `asn1:"optional,default:1"`
}

type digestInfo struct {
	Algorithm pkix.AlgorithmIdentifier
	Digest    []byte
}

// ---- Public API ---------------------------------------------------------

// Encode encodes key and cert as a DER-encoded PKCS#12 PFX bundle protected
// by password.
func Encode(key *rsa.PrivateKey, cert *x509.Certificate, password string) ([]byte, error) {
	pw := passwordBytes(password)

	// LocalKeyID = SHA-1 of the certificate DER, used to link cert and key.
	keyID := sha1.Sum(cert.Raw)

	// 1. Cert SafeBag (unencrypted).
	certBagDER, err := marshalCertBag(cert, keyID[:])
	if err != nil {
		return nil, fmt.Errorf("p12: cert bag: %w", err)
	}
	certSafeContents, err := wrapInSequence(certBagDER)
	if err != nil {
		return nil, fmt.Errorf("p12: cert safe contents: %w", err)
	}
	certCI, err := makeDataCI(certSafeContents)
	if err != nil {
		return nil, fmt.Errorf("p12: cert content info: %w", err)
	}

	// 2. Key SafeBag (3DES encrypted).
	keyBagDER, err := marshalKeyBag(key, pw, keyID[:])
	if err != nil {
		return nil, fmt.Errorf("p12: key bag: %w", err)
	}
	keySafeContents, err := wrapInSequence(keyBagDER)
	if err != nil {
		return nil, fmt.Errorf("p12: key safe contents: %w", err)
	}
	keyCI, err := makeDataCI(keySafeContents)
	if err != nil {
		return nil, fmt.Errorf("p12: key content info: %w", err)
	}

	// 3. AuthenticatedSafe = SEQUENCE OF ContentInfo.
	authSafeDER, err := asn1.Marshal([]contentInfo{certCI, keyCI})
	if err != nil {
		return nil, fmt.Errorf("p12: authSafe: %w", err)
	}

	// 4. MAC over the DER encoding of AuthenticatedSafe.
	mac, err := computeMAC(authSafeDER, pw)
	if err != nil {
		return nil, fmt.Errorf("p12: mac: %w", err)
	}

	// 5. Outer ContentInfo wrapping AuthenticatedSafe in an OCTET STRING.
	outerCI, err := makeDataCI(authSafeDER)
	if err != nil {
		return nil, fmt.Errorf("p12: outer content info: %w", err)
	}

	return asn1.Marshal(pfx{
		Version:  3,
		AuthSafe: outerCI,
		MacData:  mac,
	})
}

// ---- Helpers ------------------------------------------------------------

// passwordBytes converts a password string to the BMPString (UTF-16BE + null
// terminator) representation required by the PKCS#12 KDF (RFC 7292).
func passwordBytes(s string) []byte {
	b := make([]byte, len(s)*2+2)
	for i, c := range s {
		b[i*2] = byte(c >> 8)
		b[i*2+1] = byte(c)
	}
	return b
}

// explicit0 wraps inner DER bytes in a [0] EXPLICIT context tag.
// Go's encoding/asn1 ignores struct tags when RawValue.FullBytes is set,
// so all [0] EXPLICIT wrappers must be constructed manually this way.
func explicit0(inner []byte) asn1.RawValue {
	return asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: inner}
}

// marshalCertBag encodes cert as a PKCS#12 CertBag SafeBag.
func marshalCertBag(cert *x509.Certificate, keyID []byte) ([]byte, error) {
	certOctetDER, err := asn1.Marshal(cert.Raw)
	if err != nil {
		return nil, err
	}
	// CertBag ::= SEQUENCE { certId OID, certValue [0] EXPLICIT OCTET STRING }
	certBagDER, err := asn1.Marshal(certBag{
		ID:    oidP12X509Cert,
		Value: explicit0(certOctetDER),
	})
	if err != nil {
		return nil, err
	}
	attr, err := localKeyIDAttr(keyID)
	if err != nil {
		return nil, err
	}
	// SafeBag ::= SEQUENCE { bagId OID, bagValue [0] EXPLICIT CertBag, bagAttributes SET }
	return asn1.Marshal(safeBag{
		ID:         oidP12CertBag,
		Value:      explicit0(certBagDER),
		Attributes: []attribute{attr},
	})
}

// marshalKeyBag encrypts key with PBE-SHA1-3DES and encodes it as a
// PKCS#12 ShroudedKeyBag SafeBag.
func marshalKeyBag(key *rsa.PrivateKey, password, keyID []byte) ([]byte, error) {
	pkcs8DER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}

	const iters = 2048
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	// Derive key and IV via PKCS#12 KDF (RFC 7292 Appendix B).
	encKey := kdf(1, password, salt, iters, 24) // 24-byte 3DES key
	encIV := kdf(2, password, salt, iters, 8)   // 8-byte IV

	block, err := des.NewTripleDESCipher(encKey)
	if err != nil {
		return nil, err
	}
	padded := pkcs7Pad(pkcs8DER, block.BlockSize())
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, encIV).CryptBlocks(ct, padded)

	paramsDER, err := asn1.Marshal(pbeParams{Salt: salt, Iterations: iters})
	if err != nil {
		return nil, err
	}
	alg := pkix.AlgorithmIdentifier{
		Algorithm:  oidPBE3DES,
		Parameters: asn1.RawValue{FullBytes: paramsDER},
	}
	epkiDER, err := asn1.Marshal(encryptedPKI{Algorithm: alg, Data: ct})
	if err != nil {
		return nil, err
	}

	attr, err := localKeyIDAttr(keyID)
	if err != nil {
		return nil, err
	}
	// SafeBag ::= SEQUENCE { bagId OID, bagValue [0] EXPLICIT EncryptedPrivateKeyInfo, ... }
	return asn1.Marshal(safeBag{
		ID:         oidP12KeyBag,
		Value:      explicit0(epkiDER),
		Attributes: []attribute{attr},
	})
}

// makeDataCI creates a pkcs-7-data ContentInfo wrapping data in an OCTET STRING.
func makeDataCI(data []byte) (contentInfo, error) {
	// Content ::= [0] EXPLICIT OCTET STRING { data }.
	octetDER, err := asn1.Marshal(data)
	if err != nil {
		return contentInfo{}, err
	}
	return contentInfo{
		ContentType: oidP12Data,
		Content:     explicit0(octetDER),
	}, nil
}

// localKeyIDAttr builds a PKCS#12 localKeyId attribute (SET { OCTET STRING }).
func localKeyIDAttr(keyID []byte) (attribute, error) {
	keyIDDER, err := asn1.Marshal(keyID)
	if err != nil {
		return attribute{}, err
	}
	return attribute{
		ID: oidP12KeyID,
		Value: asn1.RawValue{
			Tag:        17,      // SET
			Class:      0,       // UNIVERSAL
			IsCompound: true,
			Bytes:      keyIDDER,
		},
	}, nil
}

// wrapInSequence wraps the DER bytes of a single element into a SEQUENCE.
func wrapInSequence(elemDER []byte) ([]byte, error) {
	return asn1.Marshal(asn1.RawValue{
		Tag:        16, // SEQUENCE
		IsCompound: true,
		Bytes:      elemDER,
	})
}

// computeMAC computes HMAC-SHA1 over data using the PKCS#12 KDF (ID=3).
func computeMAC(data, password []byte) (macData, error) {
	const iters = 2048
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return macData{}, err
	}
	macKey := kdf(3, password, salt, iters, 20) // 20-byte HMAC-SHA1 key
	mac := hmac.New(sha1.New, macKey)
	mac.Write(data)
	return macData{
		Mac: digestInfo{
			Algorithm: pkix.AlgorithmIdentifier{
				Algorithm:  oidSHA1,
				Parameters: asn1.RawValue{Tag: 5}, // NULL
			},
			Digest: mac.Sum(nil),
		},
		Salt:       salt,
		Iterations: iters,
	}, nil
}

// ---- PKCS#12 Key Derivation Function (RFC 7292 Appendix B, SHA-1) -------

// kdf derives keyLen bytes using the PKCS#12 KDF with SHA-1.
//
//	id=1: encryption key
//	id=2: IV
//	id=3: MAC key
func kdf(id byte, password, salt []byte, iterations, keyLen int) []byte {
	const v = 64 // SHA-1 block size in bytes
	const u = 20 // SHA-1 output size in bytes

	D := make([]byte, v)
	for i := range D {
		D[i] = id
	}

	S := expand(salt, v)
	P := expand(password, v)
	I := append(S, P...)

	B := make([]byte, v)
	result := make([]byte, 0, keyLen)

	for len(result) < keyLen {
		// A = H^iterations(D || I)
		h := sha1.New()
		h.Write(D)
		h.Write(I)
		A := h.Sum(nil)
		for i := 1; i < iterations; i++ {
			h.Reset()
			h.Write(A)
			A = h.Sum(nil)
		}
		result = append(result, A...)

		// B = A repeated to fill v bytes.
		for j := range B {
			B[j] = A[j%u]
		}
		// I_j = (I_j + B + 1) mod 2^(v*8) for each v-byte chunk of I.
		for j := 0; j < len(I); j += v {
			addB(I[j:j+v], B)
		}
	}
	return result[:keyLen]
}

// expand repeats data until the output length is the smallest multiple of v
// that is >= len(data). Returns nil if data is empty.
func expand(data []byte, v int) []byte {
	if len(data) == 0 {
		return nil
	}
	n := v * ((len(data) + v - 1) / v)
	out := make([]byte, n)
	for i := range out {
		out[i] = data[i%len(data)]
	}
	return out
}

// addB computes a = (a + B + 1) mod 2^(len(a)*8) in-place (big-endian).
func addB(a, B []byte) {
	carry := 1
	for i := len(a) - 1; i >= 0; i-- {
		sum := int(a[i]) + int(B[i]) + carry
		a[i] = byte(sum)
		carry = sum >> 8
	}
}

// ---- PKCS#7 padding -----------------------------------------------------

// pkcs7Pad pads data to a multiple of blockSize bytes using PKCS#7.
func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}
